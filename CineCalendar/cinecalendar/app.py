from __future__ import annotations
import sys
import threading

from . import __version__
from .service import CineCalendarService
from .updater import parse_special_startup, write_health_marker
from .updater_v3 import cleanup_update_residue


def main():
    exit_code, post_update = parse_special_startup(sys.argv)
    if exit_code is not None:
        return exit_code

    service = CineCalendarService()
    service.log.info("CineCalendar Premium start")

    # Keep one authoritative package version in inherited/base widgets.
    from . import qt_ui as base_ui
    base_ui.APP_VERSION = __version__

    # Patch the inherited update action before loading Premium UI. The v3 updater launches
    # an independent helper and can force-close a stuck parent process safely.
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

            # Give either the legacy or v3 helper enough time to observe health.ok, then
            # remove any staged ZIP/backup/helper/request residue left by older updaters.
            def delayed_cleanup():
                try:
                    cleanup_update_residue(service.paths.root / "updates")
                    service.log.info("Updater residue cleanup completed after %s", expected_version)
                except Exception as exc:
                    service.log.exception("Updater residue cleanup failed: %s", exc)

            timer = threading.Timer(5.0, delayed_cleanup)
            timer.daemon = True
            timer.start()

    return run_premium_calendar(service, on_ready=on_ready)


if __name__ == "__main__":
    raise SystemExit(main())
