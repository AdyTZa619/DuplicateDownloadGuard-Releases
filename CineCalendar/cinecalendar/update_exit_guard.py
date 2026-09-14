from __future__ import annotations

import os

from PySide6.QtCore import QTimer
from PySide6.QtWidgets import QMessageBox

from .qt_ui import WorkerThread
from .updater import stage_and_start_update, update_supported


def install_update_exit_guard(decision_cls) -> None:
    """Guarantee that the old frozen process really exits after updater handoff.

    QApplication.quit() only stops the Qt event loop. A lingering QThread can keep the
    frozen Python process alive, while the external updater waits for that PID to vanish.
    The bundle is already downloaded, SHA-verified, extracted and the helper process is
    already running when the success callback fires, so a short delayed hard exit is the
    safest handoff point.
    """

    def start_update(self, info, confirm: bool = True):
        if not update_supported():
            QMessageBox.information(
                self,
                "Actualizări",
                "Updaterul automat funcționează numai din CineCalendar.exe pe Windows.",
            )
            return
        if confirm:
            text = (
                f"Instalez CineCalendar {info.version}?\n\n"
                "Versiunea curentă este păstrată ca backup până când noua versiune "
                "pornește și confirmă health-check-ul."
            )
            if QMessageBox.question(
                self,
                "Confirmă actualizarea",
                text,
                QMessageBox.Yes | QMessageBox.No,
                QMessageBox.Yes,
            ) != QMessageBox.Yes:
                return
        if self.update_worker and self.update_worker.isRunning():
            return

        self.set_status(f"Descarc CineCalendar {info.version}…", True)
        worker = WorkerThread(
            lambda progress: stage_and_start_update(info, self.s.paths.root, progress),
            self,
        )
        self.update_worker = worker
        worker.message.connect(lambda m: self.set_status(m, True))

        def success(_req):
            self.set_status("Update verificat. Predau instalarea updaterului și închid procesul…", True)
            # Do not call QApplication.quit() here. It can leave the frozen process alive
            # while worker/QThread objects are still winding down. The external helper is
            # already launched; force the parent PID to disappear so replacement can start.
            QTimer.singleShot(350, lambda: os._exit(0))

        def failure(message):
            self.update_worker = None
            self.set_status("Actualizarea a eșuat; versiunea curentă nu a fost înlocuită.", False)
            QMessageBox.critical(self, "Actualizare eșuată", message)

        worker.success.connect(success)
        worker.failure.connect(failure)
        worker.start()

    start_update._cinecalendar_force_exit_guard = True
    decision_cls.start_update = start_update
