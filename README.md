# Duplicate Download Guard PRO

Repository folosit de updaterul direct al aplicației.

## Flux
- codul sursă este în ramura `main`;
- orice modificare a sursei pornește GitHub Actions;
- workflow-ul rulează testele și build-ul Windows x64;
- executabilul curent este publicat în `releases/DuplicateDownloadGuard_PRO_LATEST.exe`;
- `update.json` este generat automat cu versiune, URL și SHA-256;
- aplicația instalată citește direct `update.json` și poate face update fără configurare manuală.

Nu șterge `.github/workflows/build-release.yml`, `update.json` sau folderul `releases/` dacă folosești updaterul integrat.
