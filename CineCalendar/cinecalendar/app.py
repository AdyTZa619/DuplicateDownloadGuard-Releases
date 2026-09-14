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

    # Keep one authoritative package version in inherited/base widgets.
    from . import qt_ui as base_ui
    base_ui.APP_VERSION = __version__

    # The 2.3.x graceful Qt quit could leave the frozen parent PID alive after the
    # window disappeared. Patch the inherited update action before loading Premium UI
    # so the external updater always receives a clean process handoff.
    from . import qt_ui_v2 as decision_ui
    from .update_exit_guard import install_update_exit_guard
    install_update_exit_guard(decision_ui.DecisionWindow)

    from .premium_calendar_ui import run_premium_calendar

    on_ready = None
    if post_update:
        health_path, expected_version = post_update

        def on_ready():
            write_health_marker(health_path, expected_version)
            service.log.info("Post-update health marker written for %s", expected_version)

    return run_premium_calendar(service, on_ready=on_ready)


if __name__ == "__main__":
    raise SystemExit(main())
