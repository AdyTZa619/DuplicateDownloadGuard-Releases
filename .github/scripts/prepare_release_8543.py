from pathlib import Path
import re

version = "8.5.43"
Path("VERSION").write_text(version + "\n", encoding="utf-8")

main_path = Path("main.go")
main = main_path.read_text(encoding="utf-8")
main, count = re.subn(
    r'const appVersion = "[^"]+"',
    f'const appVersion = "{version} Pro Smart Media Guard"',
    main,
    count=1,
)
if count != 1:
    raise SystemExit("appVersion marker not found exactly once")
main_path.write_text(main, encoding="utf-8")

changelog_path = Path("CHANGELOG.md")
changelog = changelog_path.read_text(encoding="utf-8")
marker = "## [8.5.43]"
if marker not in changelog:
    section = """## [8.5.43] — 2026-09-06

### GoFile — cold-start rapid și creare guest aliniată fluxului web
- Crearea contului guest folosește acum un singur `POST /accounts` fără corp, în loc de trei încercări consecutive de câte 15 secunde.
- Cererea de creare guest nu mai trimite headerele de metadata `X-Website-Token` / `X-BL`, nici `Content-Type: application/json` sau corpul artificial `{}`.
- Timeout-ul pentru prima creare guest este limitat la 8 secunde; la blocaj de transport DDG trece rapid la fallback-ul `gallery-dl` în loc să consume toate cele 45 de secunde ale scanării native.
- Răspunsurile reale `429` păstrează protecția de cooldown și nu pornesc fallback-uri care ar crea alte conturi guest.
- Tokenurile guest reușite continuă să fie persistate și reutilizate conform mecanismului introdus în 8.5.42.
- Testele verifică explicit requestul bodyless/fără headere metadata și faptul că un timeout de transport nu este repetat.

### Compatibilitate
- MEGA Preview și semantica de matching nu sunt modificate.

"""
    changelog_path.write_text(section + changelog, encoding="utf-8")
