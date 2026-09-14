from __future__ import annotations
import sys

from . import __version__
from .service import CineCalendarService
from .updater import parse_special_startup, write_health_marker


def main():
    exit_code, post_update = parse_special_startup(sys.argv)
    if exit_code is not None:
        return exit_code

    service = CineCalendarService()
    service.log.info("CineCalendar Premium start")

    # Keep one authoritative package version in all inherited/base widgets.
    from . import qt_ui as base_ui
    base_ui.APP_VERSION = __version__
    from . import qt_ui_v2 as decision_ui
    from . import premium_ui

    # Premium now uses the verified bundle-aware updater from qt_ui_v2/updater.py.
    # Reuse that page instead of the obsolete placeholder page in premium_ui.py.
    premium_ui.PremiumDecisionWindow.page_updates = decision_ui.DecisionWindow.page_updates

    on_ready = None
    if post_update:
        health_path, expected_version = post_update

        def on_ready():
            write_health_marker(health_path, expected_version)
            service.log.info("Post-update health marker written for %s", expected_version)

    return premium_ui.run_premium(service, on_ready=on_ready)


if __name__ == "__main__":
    raise SystemExit(main())
