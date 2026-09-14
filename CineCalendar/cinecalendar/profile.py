from __future__ import annotations
from collections import defaultdict
from datetime import date, datetime
import math
from .db import Database
from .models import Movie
from .semantic import extract_semantic, feature_vector, popularity_bucket, runtime_bucket
from .util import json_dumps, json_loads, normalize_text, utcnow_iso

RATING_SIGNAL = {1:-1.0,2:-.82,3:-.62,4:-.38,5:-.14,6:.08,7:.34,8:.62,9:.84,10:1.0}


def _recency_weight(date_rated: str | None, today: date | None = None) -> float:
    # Old ratings never disappear; recent ratings can be at most ~15% stronger.
    today = today or date.today()
    if not date_rated:
        return 1.0
    try:
        d = datetime.strptime(date_rated[:10], "%Y-%m-%d").date()
    except ValueError:
        return 1.0
    days = max(0, (today - d).days)
    return 1.0 + 0.15 * math.exp(-days / 730.0)


def _shrink(sum_signal: float, weight: float, prior_strength: float = 4.0) -> float:
    # Bayesian shrinkage prevents a single extreme rating from dominating a feature.
    return sum_signal / (weight + prior_strength) if weight > 0 else 0.0


def _movie_from_row(row) -> Movie:
    return Movie(
        id=row["id"], imdb_id=row["imdb_id"], title=row["title"], original_title=row["original_title"] or "",
        year=row["year"], title_type=row["title_type"] or "Movie", runtime_min=row["runtime_min"],
        genres=json_loads(row["genres_json"], []), directors=json_loads(row["directors_json"], []),
        countries=json_loads(row["countries_json"], []), overview=row["overview"] or "", keywords=json_loads(row["keywords_json"], []),
        imdb_rating=row["imdb_rating"], num_votes=row["num_votes"], release_date=row["release_date"], poster_url=row["poster_url"],
        source=row["source"], semantic=json_loads(row["semantic_json"], {}) or {},
    )


def build_profile(db: Database) -> dict:
    aggregates: dict[str, list[float]] = defaultdict(lambda: [0.0,0.0,0.0,0.0])  # signal*w, w, rating*w, count
    semantic_profile: dict[str,float] = defaultdict(float)
    rated_count = 0
    imdb_delta_sum = imdb_delta_w = 0.0
    with db.connect() as con:
        rows = con.execute("""SELECT m.*, r.rating AS user_rating, r.date_rated FROM ratings r JOIN movies m ON m.id=r.movie_id""").fetchall()
        feedback = con.execute("SELECT f.kind,f.weight,m.* FROM feedback f JOIN movies m ON m.id=f.movie_id").fetchall()
    for row in rows:
        rated_count += 1
        movie = _movie_from_row(row)
        if not movie.semantic:
            movie.semantic = extract_semantic(movie)
        rating = int(row["user_rating"])
        signal = RATING_SIGNAL[rating]
        rw = _recency_weight(row["date_rated"])
        # Cap per-title feature effect: one title contributes at most rw per feature.
        features = feature_vector(movie)
        for feature, fweight in features.items():
            w = rw * min(1.0, max(.15, fweight))
            a = aggregates[feature]
            a[0] += signal * w; a[1] += w; a[2] += rating * w; a[3] += 1
            semantic_profile[feature] += signal * w
        if movie.imdb_rating is not None:
            imdb_delta_sum += (rating - movie.imdb_rating) * rw
            imdb_delta_w += rw
    # Feedback is intentionally weaker than explicit 1-10 ratings and capped.
    feedback_adjust: dict[str,float] = defaultdict(float)
    for row in feedback:
        movie = _movie_from_row(row)
        if not movie.semantic: movie.semantic = extract_semantic(movie)
        for f, fw in feature_vector(movie).items():
            feedback_adjust[f] += float(row["weight"]) * min(1.0, fw)
    features_out = {}
    for feature, (ss, w, sr, count) in aggregates.items():
        pref = _shrink(ss, w)
        pref += max(-.22, min(.22, feedback_adjust.get(feature, 0.0) * .12))
        features_out[feature] = {
            "preference": round(max(-1.0,min(1.0,pref)),4),
            "mean_rating": round(sr/w,3) if w else None,
            "count": int(count),
            "weight": round(w,3),
        }
    # Normalize semantic profile for cosine use.
    norm = math.sqrt(sum(v*v for v in semantic_profile.values())) or 1.0
    semantic_norm = {k: round(v/norm,6) for k,v in semantic_profile.items() if abs(v/norm) >= .002}
    profile = {
        "version": 1,
        "rated_count": rated_count,
        "features": features_out,
        "semantic_vector": semantic_norm,
        "mean_user_minus_imdb": round(imdb_delta_sum/imdb_delta_w,3) if imdb_delta_w else None,
        "generated_at": utcnow_iso(),
    }
    with db.tx() as con:
        con.execute("""INSERT INTO user_profile(profile_key,value_json,updated_at) VALUES('main',?,?)
                       ON CONFLICT(profile_key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at""",
                    (json_dumps(profile), utcnow_iso()))
    return profile


def get_profile(db: Database) -> dict:
    with db.connect() as con:
        row = con.execute("SELECT value_json FROM user_profile WHERE profile_key='main'").fetchone()
    return json_loads(row[0], {}) if row else build_profile(db)


def top_profile_features(profile: dict, prefix: str | None = None, positive: bool = True, limit: int = 12):
    items = []
    for name, stats in profile.get("features", {}).items():
        if prefix and not name.startswith(prefix):
            continue
        p = float(stats.get("preference",0))
        if positive and p <= 0: continue
        if not positive and p >= 0: continue
        items.append((name, stats))
    items.sort(key=lambda x: (abs(float(x[1].get("preference",0))), int(x[1].get("count",0))), reverse=True)
    return items[:limit]
