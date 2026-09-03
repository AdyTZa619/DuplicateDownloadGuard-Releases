# Duplicate Download Guard PRO

Repository folosit de updaterul direct al aplicației.

## Flux

- codul sursă este în ramura `main`;
- orice modificare a sursei pornește GitHub Actions;
- workflow-ul rulează testele și build-ul Windows x64;
- executabilul curent este publicat în `releases/DuplicateDownloadGuard_PRO_LATEST.exe`;
- `update.json` este generat automat cu versiune, URL și SHA-256;
- aplicația instalată citește direct `update.json` și poate face update fără configurare manuală.

## Protecția v8.4 ExactGuard AI

- indexul local este actualizat înainte de comparație, ca fișierele mutate sau adăugate recent să nu devină fals `MISSING`;
- numele cu sufixuri de coliziune precum `-D3558`, `(1)` și `copy` sunt corelate, dar aceeași mărime cu nume fără legătură nu mai este afișată ca `POSSIBLE`;
- înaintea oricărui download, candidații locali sunt verificați prin hash complet sau mostre deterministe, în funcție de modul ExactGuard;
- AI/Ollama și fingerprint-ul media rămân consultative și nu pot declara singure un duplicat exact;
- sesiunea folderului MEGA rămâne pregătită temporar după scanare, reducând timpul de pornire al preview-ului;
- downloadul afișează mereu stadiul, rezultatul și erorile acționabile: cotă, autentificare, cheie/link, fișier absent, acces, spațiu, limitare, timeout și rețea;
- un job nu poate fi declarat finalizat dacă fișierul rezultat nu există și nu trece verificarea;
- procesele console ale motoarelor rulează ascuns, MEGA are un singur job simultan, iar coada oferă `Pauză TOT` și `STOP TOT`.

Nu șterge `.github/workflows/build-release.yml`, `update.json` sau folderul `releases/` dacă folosești updaterul integrat.
