# Duplicate Download Guard PRO

Repository folosit de updaterul direct al aplicației.

## Flux

- codul sursă stabil este în ramura `main`;
- orice modificare a sursei pornește GitHub Actions;
- workflow-ul rulează testele, `go vet` și build-ul Windows x64;
- executabilul curent este publicat în `releases/DuplicateDownloadGuard_PRO_LATEST.exe`;
- `update.json` este generat automat cu versiune, URL și SHA-256;
- aplicația instalată citește direct `update.json` și poate face update fără configurare manuală.

## v8.4.1 Reliability

- închiderea ferestrei aplicației oprește controlat backend-ul local, salvează coada și nu lasă joburile active într-o stare falsă;
- updaterul nativ așteaptă terminarea procesului vechi înainte de înlocuirea EXE-ului și păstrează mecanismul de health-check/rollback;
- procesele externe sunt pornite fără ferestre console, iar oprirea/cancelarea curăță procesele copil și managerul aria2;
- `Pauză TOT`, `STOP TOT`, Resume și recuperarea după restart resetează coerent starea, viteza, ETA și ExactGuard-ul joburilor;
- un motor care nu produce nici fișier, nici eroare nu mai poate fi raportat ca succes;
- fișierul final trebuie să existe și, când dimensiunea remote este exactă, trebuie să aibă dimensiunea corectă; SHA-256/MD5 este verificat când sursa îl publică;
- downloaderul intern închide și sincronizează corect fișierul `.part` înainte de rename pe Windows și detectează transferurile incomplete;
- rezultatele MEGA vechi din folderul de download nu mai pot fi confundate cu fișierul descărcat în jobul curent; fallback-ul pe nume schimbat este permis numai când este recent, de aceeași mărime și neambiguu;
- scanarea MEGA, preview-ul WebDAV și downloadul MEGA folosesc exclusiv aceeași sesiune MEGAcmd, astfel încât operațiile `login/logout/webdav/get` nu se mai pot invalida reciproc;
- joburile MEGA din coadă așteaptă eliberarea sesiunii în loc să consume artificial retry-uri cât timp rulează o scanare;
- anularea MEGA este clasificată separat și nu este tratată ca eroare de rețea retryabilă;
- yt-dlp nu mai folosește istoricul vechi de archive pentru a ignora silențios o redescărcare cerută explicit;
- CI-ul ramurii de dezvoltare este doar de validare: verifică formatul Go, rulează `go test ./...`, `go vet ./...` și compilează Windows amd64 fără să rescrie automat sursa.

## Protecția v8.4 ExactGuard AI

- indexul local este actualizat înainte de comparație, ca fișierele mutate sau adăugate recent să nu devină fals `MISSING`;
- numele cu sufixuri de coliziune precum `-D3558`, `(1)` și `copy` sunt corelate, dar aceeași mărime cu nume fără legătură nu mai este afișată ca `POSSIBLE`;
- înaintea oricărui download, candidații locali sunt verificați prin hash complet sau mostre deterministe, în funcție de modul ExactGuard;
- AI/Ollama și fingerprint-ul media rămân consultative și nu pot declara singure un duplicat exact;
- sesiunea folderului MEGA poate rămâne pregătită temporar după scanare pentru preview, dar v8.4.1 o arbitrează exclusiv față de scanare și download;
- downloadul afișează stadiul, rezultatul și erorile acționabile: cotă, autentificare, cheie/link, fișier absent, acces, spațiu, limitare, timeout și rețea;
- un job nu poate fi declarat finalizat dacă fișierul rezultat nu există sau nu trece validările disponibile;
- procesele console ale motoarelor rulează ascuns, MEGA are un singur job simultan, iar coada oferă `Pauză TOT` și `STOP TOT`.

Nu șterge `.github/workflows/build-release.yml`, `update.json` sau folderul `releases/` dacă folosești updaterul integrat.
