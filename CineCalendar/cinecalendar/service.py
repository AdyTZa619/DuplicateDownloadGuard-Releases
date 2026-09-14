from __future__ import annotations
from pathlib import Path
from .db import Database
from .util import AppPaths
from .logging_setup import setup_logging
from .profile import build_profile,get_profile
from .calendar_engine import CalendarEngine
from .recommendation import RecommendationEngine

class CineCalendarService:
    def __init__(self,paths:AppPaths|None=None):
        self.paths=paths or AppPaths.portable()
        self.log=setup_logging(self.paths.logs)
        self.db=Database(self.paths.data/"cinecalendar.db")
        self.calendar=CalendarEngine()
        self.recommender=RecommendationEngine(self.db,self.calendar)
        self._defaults()

    def _defaults(self):
        if self.db.get_setting("exclude_romance",None) is None: self.db.set_setting("exclude_romance",True)
        if self.db.get_setting("auto_watch_enabled",None) is None: self.db.set_setting("auto_watch_enabled",False)
        if self.db.get_setting("ratings_folder",None) is None:
            downloads=Path.home()/"Downloads"; self.db.set_setting("ratings_folder",str(downloads))
        if self.db.get_setting("theme",None) is None: self.db.set_setting("theme","dark")
        # Runtime-only lock: always clear it after a previous crash/interrupted catalog bootstrap.
        self.db.set_setting("catalog_bootstrap_running",False)
