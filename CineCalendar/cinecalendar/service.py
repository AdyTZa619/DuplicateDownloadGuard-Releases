from __future__ import annotations
from pathlib import Path

from .autoseed import ensure_initial_ratings
from .calendar_engine import CalendarEngine
from .db import Database
from .logging_setup import setup_logging
from .recommender_v3 import FastRecommendationEngine
from .util import AppPaths


class CineCalendarService:
    def __init__(self, paths: AppPaths | None = None):
        self.paths = paths or AppPaths.portable()
        self.log = setup_logging(self.paths.logs)
        self.db = Database(self.paths.data / "cinecalendar.db")
        self._defaults()
        self.initial_ratings_state = ensure_initial_ratings(self.db, self.paths.root.parent, self.log)
        self.calendar = CalendarEngine()
        self.recommender = FastRecommendationEngine(self.db, self.calendar)

    def _defaults(self):
        if self.db.get_setting("exclude_romance", None) is None:
            self.db.set_setting("exclude_romance", True)
        if self.db.get_setting("auto_watch_enabled", None) is None:
            # New installs should follow new IMDb exports automatically. The user can still
            # turn this off explicitly later.
            self.db.set_setting("auto_watch_enabled", True)
        if self.db.get_setting("ratings_folder", None) is None:
            downloads = Path.home() / "Downloads"
            self.db.set_setting("ratings_folder", str(downloads))
        if self.db.get_setting("theme", None) is None:
            self.db.set_setting("theme", "dark")
        # Runtime-only lock: always clear it after a previous crash/interrupted catalog bootstrap.
        self.db.set_setting("catalog_bootstrap_running", False)
