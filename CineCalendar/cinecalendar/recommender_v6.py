from __future__ import annotations

from datetime import date

from .models import Recommendation
from .profile import get_profile
from .recommendation import romance_policy, row_to_movie
from .recommender_v5 import FastRecommendationEngineV5
from .semantic import extract_semantic
from .util import clamp


ENGINE_VERSION = "6.0.0"


class FastRecommendationEngineV6(FastRecommendationEngineV5):
    """Adds one-pass, cached calendar-day programs on top of the fast v5 engine.

    The old month page called the complete recommender repeatedly for several date ranges.
    This class scans one balanced pool once, keeps representation for each calendar relation,
    fully scores only a bounded union, and then reuses those scores for all visible sections.
    """

    CALENDAR_POOL = 2500
    CALENDAR_FINALISTS = 380

    def __init__(self, db, calendar=None):
        super().__init__(db, calendar)
        self._calendar_program_cache: dict[tuple, dict] = {}
        self.last_calendar_pre_rank_count = 0
        self.last_calendar_full_score_count = 0

    @staticmethod
    def _calendar_rank(rec: Recommendation) -> float:
        s = rec.score
        return (
            .60 * float(s.calendar) +
            .18 * float(s.final) +
            .10 * clamp(float(s.predicted_rating) / 10.0) +
            .07 * float(s.confidence) +
            .05 * float(s.season)
        )

    def calendar_day_program(self, when: date | None = None, count_per_section: int = 6) -> dict:
        when = when or date.today()
        count_per_section = max(3, min(8, int(count_per_section or 6)))
        cache_key = (when.isoformat(), count_per_section, self._state_token())
        cached = self._calendar_program_cache.get(cache_key)
        if cached is not None:
            return cached

        profile = get_profile(self.db)
        if int(profile.get("rated_count", 0) or 0) < self.MIN_PERSONAL_RATINGS:
            raise RuntimeError(
                "Nu am suficiente ratinguri personale încărcate pentru recomandări. "
                "CineCalendar nu va inventa un scor «pentru tine»."
            )

        context = self._run_context()
        exclude_romance = bool(self.db.get_setting("exclude_romance", True))
        rows = self._candidate_rows(when, self.CALENDAR_POOL)

        rough = []
        for row in rows:
            movie = row_to_movie(row)
            if not movie.semantic:
                movie.semantic = extract_semantic(movie)
            allowed, _penalty, _reason = romance_policy(movie, exclude_romance)
            if not allowed:
                continue
            calendar_score, kind, reason = self._calendar_score_cached(movie, when)
            season_score, _season_label = self._season_score_cached(movie, when)
            affinity, evidence = self._cheap_personal_affinity(movie, profile)
            quality = self._quality(movie)
            novelty = self._novelty(movie, context)
            # Calendar dominates this pre-rank, but personal taste and quality still stop
            # weak contextual matches from flooding the finalist set.
            value = (
                .46 * calendar_score + .25 * affinity + .11 * quality +
                .07 * season_score + .06 * evidence + .05 * novelty
            )
            rough.append((value, movie, calendar_score, kind, season_score, reason))

        rough.sort(key=lambda item: item[0], reverse=True)
        self.last_calendar_pre_rank_count = len(rough)

        # Build one bounded finalist union.  Preserve the strongest films for every relation
        # type so a general high-quality title cannot wipe out direct/spiritual/history lanes.
        chosen: list = []
        chosen_ids: set[int] = set()

        def add_many(items, limit: int):
            for item in items[:limit]:
                movie = item[1]
                mid = int(movie.id)
                if mid in chosen_ids:
                    continue
                chosen.append(movie)
                chosen_ids.add(mid)
                if len(chosen) >= self.CALENDAR_FINALISTS:
                    return

        add_many(rough, 220)
        for relation in ("directă", "spirituală", "istorică", "atmosferică"):
            lane = sorted(
                (item for item in rough if item[3] == relation and item[2] >= .10),
                key=lambda item: (item[2], item[0]), reverse=True,
            )
            add_many(lane, 34)
        seasonal = sorted(
            (item for item in rough if item[4] > .34),
            key=lambda item: (item[4], item[0]), reverse=True,
        )
        add_many(seasonal, 40)

        scored: list[Recommendation] = []
        for movie in chosen:
            score = self._score_one(movie, when, profile, context, exclude_romance, mode="calendar")
            if score is None:
                continue
            # Do not promote weak personal predictions merely because a title matches a date.
            if score.confidence >= .55 and score.predicted_rating < 5.8:
                continue
            scored.append(Recommendation(movie, score))

        self.last_calendar_full_score_count = len(chosen)
        self._assert_no_blocked_leak(scored)
        scored.sort(key=self._calendar_rank, reverse=True)

        def lane(kind: str | None = None, min_calendar: float = .10, season_only: bool = False):
            out = []
            for rec in scored:
                s = rec.score
                if season_only:
                    if s.season <= .34:
                        continue
                else:
                    if s.calendar < min_calendar:
                        continue
                    if kind is not None and s.calendar_kind != kind:
                        continue
                out.append(rec)
            if season_only:
                out.sort(key=lambda r: (r.score.season, self._calendar_rank(r)), reverse=True)
            else:
                out.sort(key=lambda r: (r.score.calendar, self._calendar_rank(r)), reverse=True)
            return out

        # First section is explicitly the answer to "what fits this exact day?".
        exact = [r for r in scored if r.score.calendar >= .10]
        exact.sort(key=self._calendar_rank, reverse=True)
        if not exact:
            # Some ordinary days have no strong named observance.  In that case keep the
            # section useful via seasonal/contextual matches rather than fabricating a link.
            exact = [r for r in scored if r.score.season > .34]
            exact.sort(key=lambda r: (r.score.season, r.score.final), reverse=True)

        specs = [
            ("exact", "Pentru ziua asta", "Legătura cu ziua/perioada este criteriul principal; gustul tău decide ordinea.", exact),
            ("direct", "Legătură directă", "Filme cu o legătură tematică directă cu reperul calendaristic activ.", lane("directă", .10)),
            ("spiritual", "Legătură spirituală", "Credință, creștinism, viață spirituală sau teme apropiate reperului zilei.", lane("spirituală", .10)),
            ("historical", "Legătură istorică", "Filme conectate istoric cu evenimentul, epoca sau memoria zilei.", lane("istorică", .10)),
            ("atmosphere", "Atmosfera zilei", "Ton, anotimp și atmosferă potrivite perioadei, fără a pretinde o legătură directă.", lane("atmosferică", .09)),
            ("season", "Sezon și perioadă", "Potrivire cu momentul anului și micro-perioada calendaristică.", lane(season_only=True)),
        ]

        sections = []
        used: set[int] = set()
        for key, title, subtitle, recs in specs:
            picked = []
            for rec in recs:
                mid = int(rec.movie.id)
                # The first section is authoritative. Later lanes avoid repeating it so the
                # page gives the user more genuinely different options.
                if key != "exact" and mid in used:
                    continue
                picked.append(rec)
                used.add(mid)
                if len(picked) >= count_per_section:
                    break
            if picked:
                sections.append({"key": key, "title": title, "subtitle": subtitle, "recommendations": picked})

        events = self.calendar.relevant_events(when)
        phase, season_tags = self.calendar.season_phase(when)
        result = {
            "date": when,
            "phase": phase,
            "season_tags": dict(season_tags),
            "events": events,
            "sections": sections,
            "pre_rank_count": self.last_calendar_pre_rank_count,
            "full_score_count": self.last_calendar_full_score_count,
        }

        if len(self._calendar_program_cache) >= 12:
            self._calendar_program_cache.clear()
        self._calendar_program_cache[cache_key] = result
        return result
