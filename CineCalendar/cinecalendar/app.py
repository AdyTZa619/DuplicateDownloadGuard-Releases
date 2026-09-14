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
    service.log.info("CineCalendar start")

    # The v2 window inherits shell widgets from qt_ui.py; keep the single package
    # version authoritative there as well, instead of the old legacy constant.
    from . import qt_ui as base_ui
    base_ui.APP_VERSION = __version__
    from .qt_ui_v2 import run_qt

    on_ready = None
    if post_update:
        health_path, expected_version = post_update
        def on_ready():
            write_health_marker(health_path, expected_version)
            service.log.info("Post-update health marker written for %s", expected_version)

    return run_qt(service, on_ready=on_ready)


if __name__ == "__main__":
    raise SystemExit(main())
