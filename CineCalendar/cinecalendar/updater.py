from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
from typing import Callable

import requests


MANIFEST_URL = (
    "https://raw.githubusercontent.com/AdyTZa619/"
    "DuplicateDownloadGuard-Releases/cinecalendar-direct-exe/CineCalendar/update.json"
)
UPDATER_MODE = "--cinecalendar-native-updater"
CLEANUP_MODE = "--cinecalendar-updater-cleanup"
POST_UPDATE_MODE = "--cinecalendar-post-update"


@dataclass(frozen=True)
class UpdateInfo:
    version: str
    url: str
    sha256: str
    notes: str = ""
    published_at: str = ""
    channel: str = "stable"


@dataclass
class NativeUpdateRequest:
    parent_pid: int
    current: str
    pending: str
    backup: str
    health: str
    log: str
    expected_version: str
    expected_sha256: str
    updates_dir: str
    helper: str
    request_path: str


def _version_key(value: str) -> tuple[int, ...]:
    nums = [int(x) for x in re.findall(r"\d+", str(value or ""))]
    return tuple(nums or [0])


def is_newer_version(remote: str, current: str) -> bool:
    a = _version_key(remote)
    b = _version_key(current)
    n = max(len(a), len(b))
    return a + (0,) * (n - len(a)) > b + (0,) * (n - len(b))


def parse_manifest(payload: dict) -> UpdateInfo:
    version = str(payload.get("version") or "").strip()
    url = str(payload.get("url") or "").strip()
    sha = str(payload.get("sha256") or "").strip().lower()
    channel = str(payload.get("channel") or "stable").strip().lower()
    if not version:
        raise ValueError("Manifestul de update nu conține versiunea.")
    if not url.startswith("https://"):
        raise ValueError("URL-ul update-ului trebuie să fie HTTPS.")
    if not re.fullmatch(r"[0-9a-f]{64}", sha):
        raise ValueError("SHA-256 invalid în manifestul de update.")
    if channel != "stable":
        raise ValueError("Canalul manifestului nu este stable.")
    return UpdateInfo(
        version=version,
        url=url,
        sha256=sha,
        notes=str(payload.get("notes") or "").strip(),
        published_at=str(payload.get("publishedAt") or payload.get("published_at") or "").strip(),
        channel=channel,
    )


def check_for_update(current_version: str, timeout: int = 12) -> UpdateInfo | None:
    response = requests.get(
        MANIFEST_URL,
        timeout=(5, timeout),
        headers={"User-Agent": f"CineCalendar/{current_version}", "Cache-Control": "no-cache"},
    )
    response.raise_for_status()
    info = parse_manifest(response.json())
    return info if is_newer_version(info.version, current_version) else None


def sha256_path(path: str | Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        while True:
            block = fh.read(1024 * 1024)
            if not block:
                break
            h.update(block)
    return h.hexdigest()


def update_supported() -> bool:
    exe = Path(sys.executable)
    return bool(getattr(sys, "frozen", False) and os.name == "nt" and exe.suffix.lower() == ".exe")


def _log(path: str | Path, message: str) -> None:
    try:
        p = Path(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        stamp = datetime.now(timezone.utc).isoformat(timespec="seconds")
        with p.open("a", encoding="utf-8") as fh:
            fh.write(f"{stamp} {message}\n")
    except Exception:
        pass


def _download_to(url: str, destination: Path, progress: Callable[[str], None] | None = None) -> None:
    progress = progress or (lambda _m: None)
    destination.parent.mkdir(parents=True, exist_ok=True)
    tmp = destination.with_suffix(destination.suffix + ".download")
    tmp.unlink(missing_ok=True)
    with requests.get(url, stream=True, timeout=(10, 180), headers={"User-Agent": "CineCalendar-Updater/2"}) as r:
        r.raise_for_status()
        total = int(r.headers.get("Content-Length") or 0)
        done = 0
        last = 0.0
        with tmp.open("wb") as fh:
            for chunk in r.iter_content(chunk_size=1024 * 1024):
                if not chunk:
                    continue
                fh.write(chunk)
                done += len(chunk)
                now = time.monotonic()
                if now - last > .35:
                    if total:
                        progress(f"Descarc update: {done/1024/1024:.1f}/{total/1024/1024:.1f} MB ({done*100/total:.0f}%)")
                    else:
                        progress(f"Descarc update: {done/1024/1024:.1f} MB")
                    last = now
            fh.flush()
            os.fsync(fh.fileno())
    os.replace(tmp, destination)


def _durable_copy(src: str | Path, dst: str | Path) -> None:
    src = Path(src); dst = Path(dst)
    dst.parent.mkdir(parents=True, exist_ok=True)
    tmp = dst.with_suffix(dst.suffix + ".copying")
    tmp.unlink(missing_ok=True)
    with src.open("rb") as inp, tmp.open("wb") as out:
        shutil.copyfileobj(inp, out, 1024 * 1024)
        out.flush(); os.fsync(out.fileno())
    os.replace(tmp, dst)


def _popen(args: list[str]) -> subprocess.Popen:
    kwargs: dict = {"close_fds": True}
    if os.name == "nt":
        kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0)
    return subprocess.Popen(args, **kwargs)


def stage_and_start_update(info: UpdateInfo, data_root: str | Path,
                           progress: Callable[[str], None] | None = None) -> NativeUpdateRequest:
    if not update_supported():
        raise RuntimeError("Updaterul automat funcționează numai în CineCalendar.exe pentru Windows.")
    progress = progress or (lambda _m: None)
    current = Path(sys.executable).resolve()
    updates = Path(data_root).resolve() / "updates"
    backup_dir = updates / "backup"
    updates.mkdir(parents=True, exist_ok=True); backup_dir.mkdir(parents=True, exist_ok=True)

    pending = updates / "CineCalendar.pending.exe"
    progress("Pregătesc actualizarea…")
    _download_to(info.url, pending, progress)
    got = sha256_path(pending)
    if got.lower() != info.sha256.lower():
        pending.unlink(missing_ok=True)
        raise RuntimeError(f"SHA-256 diferit. Așteptat {info.sha256}, primit {got}.")
    progress("SHA-256 verificat. Pregătesc updaterul sigur…")

    # The helper is a copy of the currently running, trusted EXE. It performs the
    # replacement only after the parent process has exited, just like DDG.
    helper = updates / f"CineCalendar.updater_{os.getpid()}.exe"
    _durable_copy(current, helper)
    backup = backup_dir / f"CineCalendar_{info.version}_previous.exe"
    health = updates / "health.ok"
    log = updates / "updater.log"
    request_path = updates / "apply_update.json"

    req = NativeUpdateRequest(
        parent_pid=os.getpid(), current=str(current), pending=str(pending), backup=str(backup),
        health=str(health), log=str(log), expected_version=info.version,
        expected_sha256=info.sha256, updates_dir=str(updates), helper=str(helper),
        request_path=str(request_path),
    )
    request_path.write_text(json.dumps(asdict(req), ensure_ascii=False, indent=2), encoding="utf-8")
    _log(log, f"Update {info.version} staged; handoff către helper.")
    _popen([str(helper), UPDATER_MODE, str(request_path)])
    progress("Update pregătit. CineCalendar se va închide și va reporni automat.")
    return req


def _process_alive_windows(pid: int) -> bool:
    if pid <= 0:
        return False
    if os.name != "nt":
        try:
            os.kill(pid, 0)
            return True
        except OSError:
            return False
    import ctypes
    SYNCHRONIZE = 0x00100000
    WAIT_TIMEOUT = 0x00000102
    kernel32 = ctypes.windll.kernel32
    handle = kernel32.OpenProcess(SYNCHRONIZE, False, int(pid))
    if not handle:
        return False
    try:
        return kernel32.WaitForSingleObject(handle, 0) == WAIT_TIMEOUT
    finally:
        kernel32.CloseHandle(handle)


def _wait_process_exit(pid: int, timeout: float) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if not _process_alive_windows(pid):
            return True
        time.sleep(.25)
    return not _process_alive_windows(pid)


def _validate_request(req: NativeUpdateRequest) -> None:
    for value in (req.current, req.pending, req.backup, req.health, req.log, req.updates_dir, req.helper, req.request_path):
        if not Path(value).is_absolute():
            raise ValueError("Updaterul a primit o cale care nu este absolută.")
    if not req.current.lower().endswith(".exe") or not req.pending.lower().endswith(".exe"):
        raise ValueError("Fișierele de update trebuie să fie EXE.")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", req.expected_sha256):
        raise ValueError("SHA-256 invalid în cererea updaterului.")


def write_health_marker(path: str | Path, version: str) -> None:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    tmp = p.with_suffix(".tmp")
    tmp.write_text(f"{version}\n{datetime.now(timezone.utc).isoformat()}\n", encoding="utf-8")
    os.replace(tmp, p)


def health_matches(path: str | Path, version: str) -> bool:
    try:
        first = Path(path).read_text(encoding="utf-8").splitlines()[0].strip()
        return first == str(version).strip()
    except Exception:
        return False


def _wait_health(path: str | Path, version: str, timeout: float = 35.0) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if health_matches(path, version):
            return True
        time.sleep(.4)
    return False


def _cleanup_old(updates: Path, keep_backup: Path | None = None, keep_helper: Path | None = None) -> None:
    backup_dir = updates / "backup"
    if backup_dir.exists():
        for p in backup_dir.glob("*.exe"):
            if keep_backup and p.resolve() == keep_backup.resolve():
                continue
            p.unlink(missing_ok=True)
    for p in updates.iterdir() if updates.exists() else []:
        if p.is_dir():
            continue
        if keep_helper and p.resolve() == keep_helper.resolve():
            continue
        low = p.name.lower()
        if low.startswith("cinecalendar.updater_") or low.endswith((".download", ".copying", ".replacing")):
            p.unlink(missing_ok=True)


def run_native_updater(request_path: str) -> int:
    if os.name != "nt":
        return 70
    try:
        payload = json.loads(Path(request_path).read_text(encoding="utf-8"))
        req = NativeUpdateRequest(**payload)
        _validate_request(req)
    except Exception:
        return 65
    log = Path(req.log)
    _log(log, f"Updater nativ pornit pentru {req.expected_version}.")
    if not _wait_process_exit(req.parent_pid, 30):
        _log(log, "Procesul principal nu s-a închis în 30s; nu modific EXE-ul.")
        return 7

    current = Path(req.current); pending = Path(req.pending); backup = Path(req.backup)
    updates = Path(req.updates_dir); helper = Path(req.helper); health = Path(req.health)
    try:
        _cleanup_old(updates, keep_helper=helper)
        _durable_copy(current, backup)
        _durable_copy(pending, current)
        got = sha256_path(current)
        if got.lower() != req.expected_sha256.lower():
            raise RuntimeError("SHA-256 diferit după înlocuirea EXE-ului.")
    except Exception as exc:
        _log(log, f"Înlocuire eșuată: {exc}; încerc rollback.")
        try:
            if backup.exists(): _durable_copy(backup, current)
            _popen([str(current)])
        except Exception as rex:
            _log(log, f"ROLLBACK EȘUAT: {rex}")
            return 6
        return 3

    health.unlink(missing_ok=True)
    try:
        child = _popen([str(current), POST_UPDATE_MODE, str(health), req.expected_version])
    except Exception as exc:
        _log(log, f"Versiunea nouă nu pornește: {exc}; rollback.")
        _durable_copy(backup, current); _popen([str(current)])
        return 4

    if _wait_health(health, req.expected_version, 35):
        _log(log, "Health-check reușit; update confirmat.")
        try:
            _popen([str(current), CLEANUP_MODE, str(os.getpid()), str(updates), str(backup), str(helper)])
        except Exception as exc:
            _log(log, f"Cleanup amânat: {exc}")
        return 0

    _log(log, "Health-check eșuat; rollback automat.")
    try:
        child.terminate()
        try: child.wait(timeout=8)
        except Exception: child.kill()
    except Exception:
        pass
    try:
        _durable_copy(backup, current)
        _popen([str(current)])
        _log(log, "Rollback terminat; versiunea anterioară a fost repornită.")
        return 5
    except Exception as exc:
        _log(log, f"ROLLBACK EȘUAT: {exc}")
        return 6


def run_cleanup(parent_pid: int, updates_dir: str, keep_backup: str, helper: str) -> int:
    try:
        updates = Path(updates_dir).resolve(); backup = Path(keep_backup).resolve(); helper_path = Path(helper).resolve()
        if not updates.is_absolute() or not backup.is_absolute() or not helper_path.is_absolute():
            return 65
        _wait_process_exit(parent_pid, 30)
        _cleanup_old(updates, keep_backup=backup)
        for name in ("CineCalendar.pending.exe", "apply_update.json", "health.ok"):
            (updates / name).unlink(missing_ok=True)
        helper_path.unlink(missing_ok=True)
        return 0
    except Exception:
        return 65


def parse_special_startup(argv: list[str]) -> tuple[int | None, tuple[str, str] | None]:
    """Return (exit_code, post_update_health). exit_code=None means continue normal UI."""
    if len(argv) >= 3 and argv[1] == UPDATER_MODE:
        return run_native_updater(argv[2]), None
    if len(argv) >= 6 and argv[1] == CLEANUP_MODE:
        try:
            return run_cleanup(int(argv[2]), argv[3], argv[4], argv[5]), None
        except Exception:
            return 65, None
    if len(argv) >= 4 and argv[1] == POST_UPDATE_MODE:
        return None, (argv[2], argv[3])
    return None, None
