from pathlib import Path

old = '''def ensure_once(text: str, old: str, new: str, label: str) -> str:\n    old_count = text.count(old)\n    new_count = text.count(new)\n    if old_count == 1 and new_count == 0:\n        return text.replace(old, new, 1)\n    if old_count == 0 and new_count == 1:\n        return text\n    raise SystemExit(f"{label}: unexpected state old={old_count} new={new_count}")\n'''

new = '''def ensure_once(text: str, old: str, new: str, label: str) -> str:\n    # Some replacements intentionally keep the old snippet inside the new one.\n    # Treat an already-present complete replacement as success before checking\n    # the old snippet so rerunning CI is truly idempotent.\n    new_count = text.count(new)\n    if new_count == 1:\n        return text\n    old_count = text.count(old)\n    if old_count == 1:\n        return text.replace(old, new, 1)\n    raise SystemExit(f"{label}: unexpected state old={old_count} new={new_count}")\n'''

for name in ("fixes/apply_841.py", "fixes/apply_mega_session.py"):
    path = Path(name)
    text = path.read_text(encoding="utf-8")
    if new in text:
        continue
    if old not in text:
        raise SystemExit(f"{name}: ensure_once implementation not recognized")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")
