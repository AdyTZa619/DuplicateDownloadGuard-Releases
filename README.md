# Duplicate Download Guard PRO

Repository folosit de updaterul direct al aplicației.

## Flux
- codul sursă este în ramura `main`;
- orice modificare a sursei pornește GitHub Actions;
- workflow-ul rulează testele și build-ul Windows x64;
- executabilul curent este publicat în `releases/DuplicateDownloadGuard_PRO_LATEST.exe`;
- `update.json` este generat automat cu versiune, URL și SHA-256;
- aplicația instalată citește direct `update.json` și poate face update fără configurare manuală.

## Protecția v8.3 ExactGuard AI

- înaintea oricărui download, aplicația rescanează live locațiile locale și folderul de descărcare;
- candidații cu aceeași mărime sunt verificați indiferent de nume sau sufix (`-D3558`, `(1)`, `copy` etc.);
- fișierele mici sunt confirmate prin SHA-256 integral, iar mostrele fișierelor mari pot produce numai `REVIEW`, niciodată un verdict exact;
- AI/Ollama și fingerprint-ul media sunt consultative și nu pot declara singure un duplicat exact;
- duplicatele confirmate sunt blocate și în backend, inclusiv pentru cozi vechi, reluare și export JDownloader;
- procesele console ale motoarelor rulează ascuns, MEGA are un singur job simultan, iar coada oferă `Pauză TOT` și `STOP TOT`.

Nu șterge `.github/workflows/build-release.yml`, `update.json` sau folderul `releases/` dacă folosești updaterul integrat.
