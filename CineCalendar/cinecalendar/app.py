from __future__ import annotations
from .service import CineCalendarService


def main():
    service = CineCalendarService()
    service.log.info("CineCalendar start")
    from .qt_ui_v2 import run_qt
    return run_qt(service)


if __name__ == "__main__":
    raise SystemExit(main())
