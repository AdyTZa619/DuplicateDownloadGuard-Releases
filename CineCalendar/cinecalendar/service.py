from __future__ import annotations
from pathlib import Path

from .autoseed import ensure_initial_ratings
from .calendar_engine_v2 import RichCalendarEngine
from .db import Database
from .logging_setup import setup_logging
from .recommender_v11 import FastRecommendationEngineV11
from .util import AppPaths


class CineCalendarService:
    def __init__(self, paths: AppPaths | None = None):
        self.paths = paths or AppPaths.portable()
        self.log = setup_logging(self.paths.logs)
        self.db = Database(self.paths.data / "cinecalendar.db")
        self._defaults()
        self.initial_ratings_state = ensure_initial_ratings(self.db, self.paths.root.parent, self.log)
        self.calendar = RichCalendarEngine()
        self.recommender = FastRecommendationEngineV11(self.db, self.calendar)
        # Modelul colaborativ se încarcă/descarcă în fundal. Pornirea aplicației și UI-ul nu
        # așteaptă rețeaua sau încărcarea factorilor de pe disc; până e gata rămâne fallback v10.
        self.recommender.collaborative.start_background()

    def _defaults(self):
        if self.db.get_setting("exclude_romance", None) is None:
            self.db.set_setting("exclude_romance", True)
        if self.db.get_setting("auto_watch_enabled", None) is None:
            self.db.set_setting("auto_watch_enabled", True)
        if self.db.get_setting("ratings_folder", None) is None:
            self.db.set_setting("ratings_folder", str(Path.home() / "Downloads"))
        if self.db.get_setting("theme", None) is None:
            self.db.set_setting("theme", "dark")
        self.db.set_setting("catalog_bootstrap_running", False)
