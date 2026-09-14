from pathlib import Path

from cinecalendar.updater_v3 import cleanup_update_residue, _powershell_helper


def test_cleanup_update_residue_removes_only_transient_payloads(tmp_path):
    updates = tmp_path / "updates"
    updates.mkdir()
    (updates / "staged-2.3.2").mkdir()
    (updates / "staged-2.3.2" / "x.bin").write_bytes(b"x")
    (updates / "CineCalendar-2.3.2.pending.zip").write_bytes(b"zip")
    (updates / "CineCalendar-2.3.2.pending.zip.download").write_bytes(b"part")
    (updates / "apply_update.ps1").write_text("x", encoding="utf-8")
    (updates / "apply_update.json").write_text("{}", encoding="utf-8")
    (updates / "health.ok").write_text("2.3.2", encoding="utf-8")
    (updates / "backup" / "previous").mkdir(parents=True)
    (updates / "backup" / "previous" / "old.exe").write_bytes(b"old")
    keep = updates / "keep-me.txt"
    keep.write_text("user-like sentinel", encoding="utf-8")

    cleanup_update_residue(updates)

    assert keep.is_file()
    assert not (updates / "staged-2.3.2").exists()
    assert not (updates / "CineCalendar-2.3.2.pending.zip").exists()
    assert not (updates / "apply_update.ps1").exists()
    assert not (updates / "apply_update.json").exists()
    assert not (updates / "health.ok").exists()
    assert not (updates / "backup").exists()


def test_clean_helper_forces_stuck_parent_and_cleans_success_residue():
    script = _powershell_helper()
    assert "Stop-Process -Id ([int]$req.parent_pid) -Force" in script
    assert "Procesul părinte este închis; încep înlocuirea bundle-ului." in script
    assert "Remove-Item -LiteralPath $backup -Recurse -Force" in script
    assert "Remove-Item -LiteralPath $req.request_path -Force" in script
    assert "Remove-Item -LiteralPath $req.helper_script -Force" in script
