from __future__ import annotations
from dataclasses import asdict
from datetime import date, datetime, timezone
from math import log10
from typing import Iterable
from .calendar_engine import CalendarEngine
from .db import Database
from .models import Movie, Recommendation, ScoreBreakdown
from .profile import get_profile
from .semantic import extract_semantic, feature_vector
from .util import clamp, cosine_sparse, json_loads, normalize_text, utcnow_iso

WEIGHTS={
    "taste":.36,
    "semantic":.22,
    "calendar":.14,
    "season":.08,
    "director_cinema":.07,
    "novelty":.05,
    "diversity":.03,
    "quality":.05,
}
ENGINE_VERSION="0.1.0"


def row_to_movie(row) -> Movie:
    return Movie(id=row["id"],imdb_id=row["imdb_id"],title=row["title"],original_title=row["original_title"] or "",
        year=row["year"],title_type=row["title_type"] or "Movie",runtime_min=row["runtime_min"],genres=json_loads(row["genres_json"],[]),
        directors=json_loads(row["directors_json"],[]),countries=json_loads(row["countries_json"],[]),overview=row["overview"] or "",
        keywords=json_loads(row["keywords_json"],[]),imdb_rating=row["imdb_rating"],num_votes=row["num_votes"],release_date=row["release_date"],
        poster_url=row["poster_url"],source=row["source"],semantic=json_loads(row["semantic_json"],{}) or {})


def romance_policy(movie: Movie, exclude_romance: bool=True) -> tuple[bool,float,str]:
    if not exclude_romance: return True,0.0,""
    genres={g.lower() for g in movie.genres}; sem=movie.semantic or extract_semantic(movie)
    has_romance="romance" in genres or sem.get("romance",0)>=.45
    if not has_romance: return True,0.0,""
    strong_non_romance=max([sem.get(x,0) for x in ("history","war","christianity","holocaust","crime","horror","scifi","documentary","biography","nature","survival","politics","antiquity","medieval")]+[0])
    hard = "romance" in genres and strong_non_romance < .55 and len(genres - {"romance","drama","comedy"}) == 0
    if hard: return False,1.0,"Romance este dominant."
    # Secondary romance is allowed only with a strong main reason, but gets a visible penalty.
    penalty=.08 if strong_non_romance>=.7 else .16
    return True,penalty,"Romance pare secundar; s-a aplicat penalizare."


class RecommendationEngine:
    def __init__(self, db:Database, calendar:CalendarEngine|None=None):
        self.db=db; self.calendar=calendar or CalendarEngine()

    def _candidate_rows(self, limit:int=100000):
        # Rated titles and explicit 'not interested' titles are excluded at SQL level.
        with self.db.connect() as con:
            return con.execute("""SELECT m.* FROM movies m
                LEFT JOIN ratings r ON r.movie_id=m.id
                WHERE r.movie_id IS NULL
                  AND m.id NOT IN (SELECT movie_id FROM feedback WHERE kind IN ('not_interested','seen','never_similar'))
                  AND lower(COALESCE(m.title_type,'movie')) IN ('movie','short','tvmovie','video','tv movie')
                ORDER BY COALESCE(m.num_votes,0) DESC LIMIT ?""",(limit,)).fetchall()

    def _repeat_penalty(self,movie_id:int,when:date)->tuple[float,str]:
        with self.db.connect() as con:
            rows=con.execute("SELECT recommended_at,action,ignored FROM recommendation_history WHERE movie_id=? ORDER BY recommended_at DESC",(movie_id,)).fetchall()
        if not rows: return 0.0,""
        latest=rows[0]
        try: d=datetime.fromisoformat(latest["recommended_at"].replace("Z","+00:00")).date()
        except Exception: d=when
        days=max(0,(when-d).days); count=len(rows)
        if days<=14: p=.26
        elif days<=60: p=.18
        elif days<=120: p=.11
        elif days<=365: p=.05
        else: p=.0
        ignored_count=sum(int(r["ignored"] or 0) for r in rows)
        p+=min(.08,max(0,count-2)*.02)+min(.04,ignored_count*.01)
        return min(.34,p),f"Recomandat de {count} ori; ignorat {ignored_count} ori; ultima dată acum {days} zile."

    def _feature_taste(self,movie:Movie,profile:dict)->tuple[float,list[tuple[str,float]]]:
        vec=feature_vector(movie); feats=profile.get("features",{}); pairs=[]; num=den=0.0
        for k,w in vec.items():
            if k in feats:
                pref=float(feats[k].get("preference",0)); num+=pref*w; den+=abs(w); pairs.append((k,pref*w))
        raw=num/den if den else 0.0
        pairs.sort(key=lambda x:abs(x[1]),reverse=True)
        return clamp(.5+.5*raw),pairs[:5]

    def _semantic_score(self,movie:Movie,profile:dict)->float:
        return clamp(.5+.5*cosine_sparse(feature_vector(movie),profile.get("semantic_vector",{})))

    def _season_score(self,movie:Movie,when:date)->tuple[float,str]:
        label,tags=self.calendar.season_phase(when); sem=movie.semantic or extract_semantic(movie)
        overlap=sum(min(1.0,sem.get(k,0))*v for k,v in tags.items())/(sum(tags.values()) or 1)
        return clamp(.3+.7*overlap),label

    def _director_cinema(self,movie:Movie,profile:dict)->float:
        feats=profile.get("features",{}); vals=[]
        for d in movie.directors:
            s=feats.get("director:"+normalize_text(d))
            if s: vals.append(float(s["preference"]))
        for c in movie.countries:
            s=feats.get("country:"+normalize_text(c))
            if s: vals.append(float(s["preference"]))
        return clamp(.5+.5*(sum(vals)/len(vals) if vals else 0.0))

    def _quality(self,movie:Movie)->float:
        if movie.imdb_rating is None: return .45
        votes=max(0,movie.num_votes or 0); m=2500; prior=6.5
        bayes=(votes/(votes+m))*movie.imdb_rating+(m/(votes+m))*prior
        return clamp((bayes-4.0)/5.0)

    def _novelty(self,movie:Movie)->float:
        with self.db.connect() as con:
            h=con.execute("SELECT COUNT(*) c FROM recommendation_history WHERE movie_id=?",(movie.id,)).fetchone()[0]
            w=con.execute("SELECT 1 FROM watchlist WHERE movie_id=?",(movie.id,)).fetchone()
        base=1.0 if h==0 else max(.2,1.0-.18*h)
        if w: base=min(1.0,base+.08)
        return base

    def _score_one(self,movie:Movie,when:date,profile:dict,exclude_romance:bool=True)->ScoreBreakdown|None:
        if not movie.semantic: movie.semantic=extract_semantic(movie)
        allowed,rom_penalty,rom_reason=romance_policy(movie,exclude_romance)
        if not allowed: return None
        taste,taste_features=self._feature_taste(movie,profile)
        semantic=self._semantic_score(movie,profile)
        cal,kind,cal_reason=self.calendar.calendar_relevance(movie,when)
        season,season_label=self._season_score(movie,when)
        dc=self._director_cinema(movie,profile); novelty=self._novelty(movie); quality=self._quality(movie)
        repeat_pen,repeat_reason=self._repeat_penalty(movie.id,when)
        # Diversity is finalized by reranking; .5 is neutral at candidate-score stage.
        diversity=.5
        subtotal=taste*WEIGHTS["taste"]+semantic*WEIGHTS["semantic"]+cal*WEIGHTS["calendar"]+season*WEIGHTS["season"]+dc*WEIGHTS["director_cinema"]+novelty*WEIGHTS["novelty"]+diversity*WEIGHTS["diversity"]+quality*WEIGHTS["quality"]
        final=clamp(subtotal-repeat_pen-rom_penalty)
        personal_bits=[]
        for k,v in taste_features[:3]:
            if v>0: personal_bits.append(k.replace("genre:","gen ").replace("theme:","temă ").replace("director:","regizor ").replace("decade:","deceniu "))
        personal_reason="Potrivire bazată pe " + ", ".join(personal_bits) if personal_bits else "Profilul tău nu are încă suficiente semnale specifice pentru acest titlu."
        contrib=[
            ("Gust personal",taste*WEIGHTS["taste"]*100,personal_reason),
            ("Similaritate semantică",semantic*WEIGHTS["semantic"]*100,"Similaritate cu tiparele din ratingurile tale."),
            ("Calendar",cal*WEIGHTS["calendar"]*100,cal_reason),
            ("Anotimp",season*WEIGHTS["season"]*100,season_label),
            ("Regizor/cinematografie",dc*WEIGHTS["director_cinema"]*100,"Semnale de regizor/țară."),
            ("Noutate",novelty*WEIGHTS["novelty"]*100,"Penalizează expunerile repetate."),
            ("Calitate",quality*WEIGHTS["quality"]*100,"IMDb rating cu regularizare după numărul de voturi."),
        ]
        if repeat_pen: contrib.append(("Repetare",-repeat_pen*100,repeat_reason))
        if rom_penalty: contrib.append(("Romance",-rom_penalty*100,rom_reason))
        return ScoreBreakdown(taste,semantic,cal,season,dc,novelty,diversity,quality,repeat_pen,rom_penalty,final,kind,cal_reason,personal_reason,contrib)

    def recommend(self,when:date|None=None,count:int=3,exclude_ids:set[int]|None=None,record:bool=False,slot:str="today",candidate_limit:int=100000)->list[Recommendation]:
        when=when or date.today(); exclude_ids=exclude_ids or set(); profile=get_profile(self.db)
        # A recommendation shown on a prior context date with no action is treated as ignored.
        with self.db.tx() as con:
            con.execute("UPDATE recommendation_history SET ignored=1 WHERE ignored=0 AND action IS NULL AND context_date < ?", (when.isoformat(),))
        exclude_romance=bool(self.db.get_setting("exclude_romance",True))
        candidates=[]
        for row in self._candidate_rows(candidate_limit):
            if row["id"] in exclude_ids: continue
            m=row_to_movie(row); s=self._score_one(m,when,profile,exclude_romance)
            if s is None: continue
            candidates.append(Recommendation(m,s))
        candidates.sort(key=lambda r:r.score.final,reverse=True)
        # MMR-style diversity reranking. It cannot override personal fit by much because diversity is only 3%.
        selected=[]
        pool=candidates[:max(150,count*20)]
        while pool and len(selected)<count:
            best=None; best_value=-1
            for rec in pool:
                if not selected: div=1.0
                else: div=1.0-max(cosine_sparse(feature_vector(rec.movie),feature_vector(x.movie)) for x in selected)
                adjusted=rec.score.final + WEIGHTS["diversity"]*(div-.5)
                if adjusted>best_value: best_value=adjusted; best=(rec,div)
            rec,div=best; rec.score.diversity=clamp(div); rec.score.final=clamp(best_value)
            rec.score.contributions.append(("Diversitate",rec.score.diversity*WEIGHTS["diversity"]*100,"Evită trei recomandări aproape identice."))
            selected.append(rec); pool.remove(rec)
        if record and selected:
            now=utcnow_iso()
            with self.db.tx() as con:
                for rec in selected:
                    con.execute("INSERT INTO recommendation_history(movie_id,recommended_at,context_date,slot,final_score) VALUES(?,?,?,?,?)",
                                (rec.movie.id,now,when.isoformat(),slot,rec.score.final))
                con.execute("INSERT INTO recommendation_runs(context_date,slot,generated_at,candidate_count,result_count,engine_version) VALUES(?,?,?,?,?,?)",
                            (when.isoformat(),slot,now,len(candidates),len(selected),ENGINE_VERSION))
        return selected

    def month_program(self,start:date|None=None,count_per_group:int=3,record:bool=False):
        start=start or date.today(); groups=self.calendar.month_groups(start); used=set(); result=[]
        for gstart,gend,label in groups:
            midpoint=gstart+(gend-gstart)//2
            recs=self.recommend(midpoint,count_per_group,exclude_ids=used,record=record,slot=f"month:{gstart}:{gend}")
            used.update(r.movie.id for r in recs)
            result.append((gstart,gend,label,recs))
        return result
