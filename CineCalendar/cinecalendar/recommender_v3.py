from __future__ import annotations

from datetime import date
import math

from .profile import get_profile
from .recommendation import (
    FEATURE_KIND_WEIGHT,
    RecommendationEngine,
)
from .semantic import feature_vector
from .util import clamp


ENGINE_VERSION = "3.0.0"


class FastRecommendationEngine(RecommendationEngine):
    """Fast, defensive recommendation engine for the large local IMDb catalog.

    The v2 engine could fully score up to 100k titles for one click and relied on movie_id
    alone to exclude rated titles. This version uses a bounded mixed shortlist, independent
    seen/rated identity barriers and a cached decision pool for instant "Alt film" actions.
    """

    NORMAL_POOL = 6000
    EXPLORE_POOL = 9000
    DECISION_CACHE_SIZE = 30
    MIN_PERSONAL_RATINGS = 5

    def __init__(self, db, calendar=None):
        super().__init__(db, calendar)
        self._decision_cache: dict[tuple, list] = {}
        self._rated_cache_token = None
        self._rated_cache = (set(), set(), set())
        self.last_candidate_count = 0
        self._ensure_performance_indexes()

    def _ensure_performance_indexes(self) -> None:
        # CREATE INDEX IF NOT EXISTS is cheap after the first run and makes shortlist queries
        # stable on a 250k+ title catalog.
        try:
            with self.db.connect() as con:
                con.execute("CREATE INDEX IF NOT EXISTS ix_movies_type_votes_v3 ON movies(title_type,num_votes DESC)")
                con.execute("CREATE INDEX IF NOT EXISTS ix_movies_rating_votes_v3 ON movies(imdb_rating DESC,num_votes DESC)")
                con.execute("CREATE INDEX IF NOT EXISTS ix_movies_year_votes_v3 ON movies(year DESC,num_votes DESC)")
        except Exception:
            # The engine remains correct without the optional indexes.
            pass

    def _state_token(self) -> tuple:
        with self.db.connect() as con:
            r = con.execute("SELECT COUNT(*),COALESCE(MAX(updated_at),'') FROM ratings").fetchone()
            f = con.execute("SELECT COUNT(*),COALESCE(MAX(created_at),'') FROM feedback").fetchone()
            w = con.execute("SELECT COUNT(*),COALESCE(MAX(updated_at),'') FROM watchlist").fetchone()
            p = con.execute("SELECT COALESCE(MAX(updated_at),'') FROM user_profile").fetchone()
        return (int(r[0]), str(r[1]), int(f[0]), str(f[1]), int(w[0]), str(w[1]), str(p[0]))

    def _rated_identities(self) -> tuple[set[int], set[str], set[str]]:
        token = self._state_token()[:2]
        if token == self._rated_cache_token:
            return self._rated_cache
        with self.db.connect() as con:
            rows = con.execute(
                """SELECT m.id,m.imdb_id,m.identity_key
                   FROM ratings r JOIN movies m ON m.id=r.movie_id"""
            ).fetchall()
        ids = {int(r["id"]) for r in rows}
        imdb = {str(r["imdb_id"]) for r in rows if r["imdb_id"]}
        ident = {str(r["identity_key"]) for r in rows if r["identity_key"]}
        self._rated_cache_token = token
        self._rated_cache = (ids, imdb, ident)
        return self._rated_cache

    def _eligible_sql(self) -> str:
        # Three independent rated barriers: exact row, IMDb Const and normalized identity.
        # This prevents a catalog duplicate from resurfacing a movie already rated by the user.
        return """ FROM movies m
            WHERE lower(COALESCE(m.title_type,'movie')) IN ('movie','short','tvmovie','video','tv movie')
              AND NOT EXISTS (
                    SELECT 1 FROM ratings rr JOIN movies rm ON rm.id=rr.movie_id
                    WHERE rm.id=m.id
                       OR (m.imdb_id IS NOT NULL AND rm.imdb_id=m.imdb_id)
                       OR (m.identity_key IS NOT NULL AND rm.identity_key=m.identity_key)
              )
              AND NOT EXISTS (
                    SELECT 1 FROM feedback f
                    WHERE f.movie_id=m.id AND f.kind IN ('not_interested','seen','never_similar')
              ) """

    @staticmethod
    def _round_robin(groups: list[list], limit: int) -> list:
        out = []
        seen: set[int] = set()
        pos = [0] * len(groups)
        while len(out) < limit:
            progressed = False
            for i, group in enumerate(groups):
                while pos[i] < len(group):
                    row = group[pos[i]]
                    pos[i] += 1
                    mid = int(row["id"])
                    if mid in seen:
                        continue
                    seen.add(mid)
                    out.append(row)
                    progressed = True
                    break
                if len(out) >= limit:
                    break
            if not progressed:
                break
        return out

    def _effective_limit(self, requested: int, mode: str = "decide") -> int:
        ceiling = self.EXPLORE_POOL if mode in {"surprise", "calendar"} else self.NORMAL_POOL
        return max(500, min(int(requested or ceiling), ceiling))

    def _candidate_rows(self, when: date, limit: int = 100000):
        limit = self._effective_limit(limit)
        base = self._eligible_sql()
        # Fetch more than each quota because the groups overlap; round-robin keeps the final
        # shortlist balanced between mainstream, hidden gems, recent titles and high-rated films.
        popular_n = max(900, int(limit * .44))
        hidden_n = max(700, int(limit * .34))
        recent_n = max(600, int(limit * .30))
        quality_n = max(600, int(limit * .26))
        cutoff = when.year - 10
        with self.db.connect() as con:
            watch = con.execute(
                "SELECT m.*" + base +
                " AND m.id IN (SELECT movie_id FROM watchlist) ORDER BY COALESCE(m.num_votes,0) DESC LIMIT 400"
            ).fetchall()
            popular = con.execute(
                "SELECT m.*" + base +
                " ORDER BY COALESCE(m.num_votes,0) DESC LIMIT ?", (popular_n * 2,)
            ).fetchall()
            hidden = con.execute(
                "SELECT m.*" + base +
                " AND COALESCE(m.num_votes,0) BETWEEN 50 AND 25000 "
                "ORDER BY COALESCE(m.imdb_rating,0) DESC,COALESCE(m.num_votes,0) DESC LIMIT ?",
                (hidden_n * 2,),
            ).fetchall()
            recent = con.execute(
                "SELECT m.*" + base +
                " AND COALESCE(m.year,0)>=? ORDER BY COALESCE(m.num_votes,0) DESC LIMIT ?",
                (cutoff, recent_n * 2),
            ).fetchall()
            quality = con.execute(
                "SELECT m.*" + base +
                " AND COALESCE(m.num_votes,0)>=250 ORDER BY COALESCE(m.imdb_rating,0) DESC,COALESCE(m.num_votes,0) DESC LIMIT ?",
                (quality_n * 2,),
            ).fetchall()
        rows = self._round_robin(
            [list(watch), list(popular), list(hidden), list(recent), list(quality)], limit
        )
        self.last_candidate_count = len(rows)
        return rows

    def _predict_user_rating(self, movie, profile: dict):
        feats = profile.get("features", {})
        global_mean = float(profile.get("global_mean_rating", 6.5) or 6.5)
        vec = feature_vector(movie)
        num = den = evidence = 0.0
        details = []
        for feature, fw in vec.items():
            st = feats.get(feature)
            if not st or st.get("mean_rating") is None:
                continue
            count = max(0, int(st.get("count", 0) or 0))
            mean = float(st.get("mean_rating", global_mean))
            std = float(st.get("std_rating", 1.6) or 1.6)
            reliability = count / (count + 6.0)
            precision = 1.0 / (.85 + max(.40, std))
            kind_w = FEATURE_KIND_WEIGHT.get(feature.split(":", 1)[0], .5)
            w = max(.05, float(fw)) * kind_w * reliability * precision
            residual = mean - global_mean
            num += residual * w
            den += abs(w)
            ev = max(.05, float(fw)) * kind_w * min(1.0, count / 14.0)
            evidence += ev
            details.append({
                "feature": feature,
                "mean": mean,
                "count": count,
                "std": std,
                "weight": w,
                "support": residual * w,
            })

        raw_delta = (num / den) if den else 0.0
        # Sparse evidence is shrunk hard toward the user's real mean instead of pretending
        # that one weak feature is a precise personal prediction.
        shrink = evidence / (evidence + .85) if evidence > 0 else 0.0
        predicted = max(1.0, min(10.0, global_mean + raw_delta * shrink))

        metadata = 0.0
        if movie.genres:
            metadata += .30
        if movie.directors:
            metadata += .20
        if movie.runtime_min:
            metadata += .10
        if movie.semantic or movie.overview or movie.keywords:
            metadata += .25
        if movie.year:
            metadata += .15

        # Confidence means PERSONAL evidence. Metadata can add only a tiny amount; with
        # zero personal evidence it stays below 10%, never the old misleading 38%.
        confidence = .04 + .88 * (1.0 - math.exp(-evidence / 2.45)) + .05 * metadata
        confidence = clamp(confidence)
        details.sort(key=lambda x: abs(x["support"]), reverse=True)
        return predicted, confidence, evidence, details

    def _score_one(self, movie, when, profile, context, exclude_romance=True, mode="decide"):
        score = super()._score_one(movie, when, profile, context, exclude_romance, mode)
        if score is None:
            return None
        if score.evidence < .08:
            penalty = .18
            score.final = clamp(score.final - penalty)
            score.personal_reason = (
                f"Date personale insuficiente pentru o predicție fermă. {score.predicted_rating:.1f}/10 "
                "este aproape de media ta generală, nu o potrivire confirmată."
            )
            score.contributions.append((
                "Dovezi personale insuficiente", -penalty * 100,
                "Titlul nu are încă suficiente caracteristici care să poată fi comparate cu ratingurile tale."
            ))
        elif score.evidence < .35:
            penalty = .08
            score.final = clamp(score.final - penalty)
            score.contributions.append((
                "Dovezi personale puține", -penalty * 100,
                "Predicția este păstrată conservatoare până există mai multe semnale personale."
            ))
        return score

    def _assert_no_rated_leak(self, recs) -> None:
        ids, imdb, ident = self._rated_identities()
        leaks = []
        for rec in recs:
            m = rec.movie
            if int(m.id) in ids or (m.imdb_id and m.imdb_id in imdb) or (m.identity_key if hasattr(m, "identity_key") else None) in ident:
                leaks.append(m.imdb_id or m.title)
        if leaks:
            raise RuntimeError("Protecția anti-văzut a detectat un titlu evaluat: " + ", ".join(leaks[:3]))

    def recommend(self, when: date | None = None, count: int = 3, exclude_ids: set[int] | None = None,
                  record: bool = False, slot: str = "today", candidate_limit: int = 100000,
                  mode: str = "decide", runtime_max: int | None = None, runtime_min: int | None = None):
        profile = get_profile(self.db)
        rated_count = int(profile.get("rated_count", 0) or 0)
        if rated_count < self.MIN_PERSONAL_RATINGS:
            raise RuntimeError(
                "Nu am suficiente ratinguri personale încărcate pentru recomandări. "
                "CineCalendar nu va inventa un scor «pentru tine»."
            )
        effective = self._effective_limit(candidate_limit, mode)
        recs = super().recommend(
            when=when,
            count=count,
            exclude_ids=exclude_ids,
            record=record,
            slot=slot,
            candidate_limit=effective,
            mode=mode,
            runtime_max=runtime_max,
            runtime_min=runtime_min,
        )
        self._assert_no_rated_leak(recs)
        return recs

    def decision_pick(self, when: date | None = None, exclude_ids: set[int] | None = None,
                      mode: str = "decide"):
        when = when or date.today()
        exclude_ids = set(exclude_ids or set())
        key = (when.isoformat(), mode, self._state_token())
        pool = self._decision_cache.get(key)
        if pool is None:
            pool = self.recommend(
                when, self.DECISION_CACHE_SIZE, record=False, slot="decision",
                candidate_limit=self.NORMAL_POOL if mode != "surprise" else self.EXPLORE_POOL,
                mode=mode,
            )
            self._decision_cache = {key: list(pool)}
        available = [r for r in pool if int(r.movie.id) not in exclude_ids]
        if len(available) < 3:
            # Extremely long skip sessions are rare; refill once with the exploration ceiling.
            refill = self.recommend(
                when, self.DECISION_CACHE_SIZE, exclude_ids=exclude_ids, record=False,
                slot="decision-refill", candidate_limit=self.EXPLORE_POOL, mode=mode,
            )
            known = {int(r.movie.id) for r in available}
            available.extend(r for r in refill if int(r.movie.id) not in known)
        self._assert_no_rated_leak(available[:3])
        return (available[0] if available else None, available[1:3])
