from __future__ import annotations

from datetime import date

from PySide6.QtCore import Qt, QTimer
from PySide6.QtWidgets import QCheckBox, QComboBox, QFrame, QHBoxLayout, QLabel, QPushButton, QVBoxLayout


GENRES = (
    "Orice gen",
    "Action", "Adventure", "Animation", "Biography", "Comedy", "Crime", "Documentary",
    "Drama", "Family", "Fantasy", "History", "Horror", "Mystery", "Romance", "Sci-Fi",
    "Sport", "Thriller", "War", "Western",
)


def install_daily_genre_ui_patch(window_cls) -> None:
    """Make genre a temporary daily intent, not a permanent taste exclusion."""
    original_settings = window_cls.page_settings

    def _today_genre(self) -> str:
        payload = self.db.get_setting("daily_genre_filter", {})
        if not isinstance(payload, dict) or str(payload.get("date") or "") != date.today().isoformat():
            return ""
        return str(payload.get("genre") or "").strip()

    def _set_today_genre(self, label: str) -> None:
        genre = "" if label == "Orice gen" else str(label).strip()
        self.db.set_setting("daily_genre_filter", {"date": date.today().isoformat(), "genre": genre})
        # Recommendation caches include this setting through the engine state token.
        self.show_page("today")

    def page_today(self):
        page, content = self.page_shell(
            "Ce văd acum?",
            "O singură alegere bine argumentată din ratingurile tale. Dacă ai chef de un gen anume, îl alegi doar pentru azi.",
        )
        self.today_content = content
        _total, rated, cand = self.catalog_count()
        if cand <= 0:
            box = QFrame(); box.setObjectName("HeroCard")
            l = QVBoxLayout(box); l.setContentsMargins(26,26,26,26); l.setSpacing(12)
            h = QLabel("Catalogul personal nu este încă pregătit"); h.setObjectName("SectionTitle"); l.addWidget(h)
            d = QLabel(f"Am {rated:,} ratinguri de învățat. Catalogul IMDb și regizorii se descarcă și se leagă automat — nu introduci filme manual.")
            d.setObjectName("Muted"); d.setWordWrap(True); l.addWidget(d)
            b = QPushButton("Pregătește automat catalogul"); b.setProperty("accent",True); b.clicked.connect(lambda:self.bootstrap_catalog(False)); l.addWidget(b, alignment=Qt.AlignLeft)
            content.addWidget(box); content.addStretch(1); return page

        chooser = QFrame(); chooser.setObjectName("PremiumCard")
        row = QHBoxLayout(chooser); row.setContentsMargins(16,12,16,12); row.setSpacing(12)
        label = QLabel("Ce gen ai chef să vezi azi?"); label.setObjectName("BodyStrong"); row.addWidget(label)
        combo = QComboBox(); combo.addItems(list(GENRES)); combo.setMinimumWidth(190)
        current = self._today_genre()
        combo.setCurrentText(current if current in GENRES else "Orice gen")
        combo.currentTextChanged.connect(lambda value: self._set_today_genre(value))
        row.addWidget(combo)
        note = QLabel("Valabil numai azi; mâine revine automat la Orice gen.")
        note.setObjectName("Muted"); row.addWidget(note, 1)
        content.addWidget(chooser)

        active = self._today_genre()
        text = "Analizez profilul colaborativ MovieLens + ratingurile tale și contextul zilei."
        if active:
            text += f" Filtrez întâi doar filmele din genul {active}."
        content.addWidget(self.loading_panel("Îți aleg filmul…", text))
        content.addStretch(1)
        QTimer.singleShot(0, self._load_today_async)
        return page

    def page_settings(self):
        page = original_settings(self)
        # Keep compatibility with old settings pages but remove the obsolete global Romance switch.
        for checkbox in page.findChildren(QCheckBox):
            if "romance" in checkbox.text().lower():
                checkbox.setChecked(False)
                checkbox.hide()
        return page

    window_cls._today_genre = _today_genre
    window_cls._set_today_genre = _set_today_genre
    window_cls.page_today = page_today
    window_cls.page_settings = page_settings
