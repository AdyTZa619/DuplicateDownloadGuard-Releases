from __future__ import annotations

from datetime import date
from pathlib import Path
import tempfile
import time

from cinecalendar.db import Database
from cinecalendar.profile import build_profile
from cinecalendar.recommender_v3 import FastRecommendationEngine
from cinecalendar.semantic import extract_semantic
from cinecalendar.models import Movie
from cinecalendar.util import identity_key, json_dumps, normalize_text, utcnow_iso


RATED = 2434
CANDIDATES = 260_000


def add_ratings(db: Database) -> None:
    genres = ["History", "Crime", "Thriller", "Sci-Fi", "Documentary", "Biography", "War", "Adventure"]
    now = utcnow_iso()
    with db.tx() as con:
        for i in range(1, RATED + 1):
            title = f"Rated Benchmark {i}"
            year = 1980 + (i % 46)
            genre = genres[i % len(genres)]
            director = f"Director {i % 80}"
            imdb_id = f"tt9{i:06d}"
            m = Movie(title=title, original_title=title, year=year, title_type="Movie", runtime_min=85 + i % 80,
                      genres=[genre], directors=[director], imdb_rating=6.0 + (i % 30) / 10.0,
                      num_votes=500 + i * 10, source="benchmark")
            m.semantic = extract_semantic(m)
            ident = identity_key(title, title, year, "Movie")
            cur = con.execute(
                """INSERT INTO movies(imdb_id,identity_key,title,original_title,title_norm,original_title_norm,
                   year,title_type,runtime_min,genres_json,directors_json,semantic_json,imdb_rating,num_votes,
                   source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
                (imdb_id, ident, title, title, normalize_text(title), normalize_text(title), year, "Movie",
                 m.runtime_min, json_dumps(m.genres), json_dumps(m.directors), json_dumps(m.semantic),
                 m.imdb_rating, m.num_votes, "benchmark", now, now),
            )
            rating = 3 + (i * 7) % 8
            con.execute(
                "INSERT INTO ratings(movie_id,rating,date_rated,source,imported_at,updated_at) VALUES(?,?,?,?,?,?)",
                (cur.lastrowid, rating, f"202{(i % 7)}-09-01", "benchmark", now, now),
            )
    build_profile(db)


def add_candidates(db: Database) -> None:
    genres = ["History", "Crime", "Thriller", "Sci-Fi", "Documentary", "Biography", "War", "Adventure", "Mystery", "Horror"]
    now = utcnow_iso()
    sql = """INSERT INTO movies(imdb_id,identity_key,title,original_title,title_norm,original_title_norm,
             year,title_type,runtime_min,genres_json,directors_json,semantic_json,imdb_rating,num_votes,
             source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"""
    with db.tx() as con:
        batch = []
        for i in range(1, CANDIDATES + 1):
            title = f"Candidate Benchmark {i}"
            year = 1940 + (i % 87)
            genre = genres[i % len(genres)]
            director = f"Director {i % 160}"
            imdb_id = f"tt8{i:06d}"
            semantic = {normalize_text(genre).replace(" ", "_"): 0.85}
            votes = 50 + ((i * 7919) % 1_500_000)
            rating = 5.5 + ((i * 37) % 35) / 10.0
            batch.append((
                imdb_id, identity_key(title, title, year, "Movie"), title, title,
                normalize_text(title), normalize_text(title), year, "Movie", 80 + i % 120,
                json_dumps([genre]), json_dumps([director]), json_dumps(semantic), rating, votes,
                "benchmark", now, now,
            ))
            if len(batch) >= 5000:
                con.executemany(sql, batch)
                batch.clear()
        if batch:
            con.executemany(sql, batch)


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="cinecalendar-v3-bench-") as td:
        db = Database(Path(td) / "benchmark.db")
        t0 = time.perf_counter()
        add_ratings(db)
        add_candidates(db)
        seed_seconds = time.perf_counter() - t0

        t1 = time.perf_counter()
        engine = FastRecommendationEngine(db)
        index_seconds = time.perf_counter() - t1

        t2 = time.perf_counter()
        primary, backups = engine.decision_pick(date(2026, 9, 14))
        first_seconds = time.perf_counter() - t2
        if primary is None or len(backups) < 2:
            raise SystemExit("benchmark: no recommendation result")
        if engine.last_candidate_count > FastRecommendationEngine.NORMAL_POOL:
            raise SystemExit(f"benchmark: scored too many candidates: {engine.last_candidate_count}")

        excluded = {int(primary.movie.id)}
        t3 = time.perf_counter()
        next_primary, _ = engine.decision_pick(date(2026, 9, 14), excluded)
        cached_seconds = time.perf_counter() - t3
        if next_primary is None or next_primary.movie.id == primary.movie.id:
            raise SystemExit("benchmark: cached Alt film failed")

        print(f"seed_seconds={seed_seconds:.3f}")
        print(f"index_seconds={index_seconds:.3f}")
        print(f"first_decision_seconds={first_seconds:.3f}")
        print(f"cached_alt_seconds={cached_seconds:.4f}")
        print(f"fully_scored_candidates={engine.last_candidate_count}")

        # Generous CI gates: they catch regressions back to 45k/100k full scoring while
        # leaving headroom for variable GitHub-hosted Windows runners.
        if first_seconds > 10.0:
            raise SystemExit(f"benchmark: first decision too slow ({first_seconds:.2f}s > 10s)")
        if cached_seconds > 0.40:
            raise SystemExit(f"benchmark: cached Alt film too slow ({cached_seconds:.3f}s > 0.40s)")
        if index_seconds > 12.0:
            raise SystemExit(f"benchmark: one-time index setup too slow ({index_seconds:.2f}s > 12s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
