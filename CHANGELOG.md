# Jurnal actualizări — Duplicate Download Guard PRO

Acest fișier păstrează schimbările importante pentru fiecare versiune publicată. Pentru fiecare release nou trebuie adăugată o secțiune `## [x.y.z]`; pipeline-ul de release verifică existența ei înainte de publicare.

## [8.5.2] — 2026-09-04

### Updater & lifecycle fereastră
- Reparat cazul în care după update pornea versiunea nouă, dar fereastra Edge-app a versiunii vechi rămânea deschisă.
- La o pornire confirmată ca handoff de updater, noua versiune închide nativ fereastra DDG rămasă înainte să-și deschidă propria interfață.
- Cleanup-ul este activ numai cât timp există markerul real `apply_update.json`; pornirile normale ale aplicației nu închid ferestre.
- Potrivirea ferestrei este strictă pe titlul dedicat `Duplicate Download Guard Pro`, pentru a evita închiderea unei ferestre Edge/Chrome obișnuite care ar putea conține alte taburi.
- Updaterul păstrează în continuare backup, verificare SHA-256, health-check și rollback automat.

## [8.5.1] — 2026-09-04

### Monitor operațional & interfață
- HUD operațional nou în colțul din dreapta sus: status live, operația curentă/ultima operație și stare colorată clar.
- Panou extensibil imersiv cu etapă, progres, durată, fișiere/date procesate, momentul pornirii/finalizării și detaliul tehnic raportat de backend.
- Indicator discret animat numai cât timp rulează o operație; succesul, eroarea și anularea au stări vizuale distincte.
- Durata ultimei operații este înghețată la finalizare în interfață, iar momentul finalizării este păstrat pentru sesiunea curentă.
- Acțiune contextuală în panou: explică ce urmează și oferă acces rapid la Dashboard și Jurnal.
- HUD-ul folosește exclusiv endpointul local `/api/status`; nu introduce trafic extern și nu modifică motorul de detecție/download.
- Layout responsive: rămâne compact pe ferestre înguste și se deschide într-un panou adaptat fără să acopere inutil interfața.

## [8.5.0] — 2026-09-04

### Detecție duplicate și variante media
- Smart Media Guard nou: separă duplicatele exacte de același conținut recodat/redimensionat, alte versiuni și potrivirile doar probabile.
- Statusuri mai intuitive în interfață: `AI DEJA`, `ACELAȘI CONȚINUT`, `ALTĂ VERSIUNE`, `PARE ACELAȘI`, `POSIBIL DUPLICAT`, `NU ÎL AI`, `DESCĂRCAT DEJA` și `NU S-A PUTUT VERIFICA`.
- Fișierele identice sunt confirmate prin hash/conținut indiferent de nume.
- Imaginile folosesc semnături perceptuale mai robuste, cu structură, culoare și variație de luminanță; sunt tratate și WEBP/BMP/AVIF prin fallback FFmpeg.
- Videoclipurile folosesc 7 cadre distribuite, elimină cadrele negre/fade/foarte uniforme și pot compensa intro/outro prin aliniere temporală.
- Verificare audio Chromaprint pentru a evita blocarea unui video cu aceleași imagini, dar altă limbă, comentariu sau soundtrack.
- Protecție contra rezultatelor ambigue: doi candidați aproape egali nu pot produce automat un verdict de duplicat sigur.
- Căutarea video este extinsă în colecție până când poate exclude re-encode-uri complet redenumite; un candidat intermediar de 85–93% nu mai oprește căutarea prea devreme.
- Pentru media cu mărime necunoscută sau aproximativă se încearcă verificarea perceptuală înainte de a declara rezultatul neverificabil.

### Calitate și recomandări
- Pentru aceeași sursă media, aplicația poate recomanda `REMOTE E MAI BUN` sau `AI DEJA VERSIUNEA MAI BUNĂ` folosind rezoluția și bitrate-ul numai când comparația este relevantă.
- Diferențele de durată și audio pot coborî verdictul la `ALTĂ VERSIUNE`/review în loc de blocare automată.

### Istoric de download
- Registru persistent `download_history.json`, separat de lista curentă și de coadă.
- `DESCĂRCAT DEJA` verifică și faptul că fișierul rezultat local nu a fost înlocuit ulterior.
- Amprenta locală poate fi SHA-256 integral pentru fișiere mici sau mostre distribuite pentru fișiere mari.
- Identitate stabilă pentru MEGA și yt-dlp; URL-urile CDN temporare nu mai sunt baza istoricului.
- Aceeași sursă yt-dlp cu altă mărime/calitate devine `DESCĂRCAT ÎNAINTE` + verificare manuală, fără a bloca automat un posibil upgrade 4K.

### Performanță
- Cache persistent pentru metadate ffprobe, fingerprint video, semnături imagini și segmente audio Chromaprint.
- Cache-urile se invalidează automat la schimbarea dimensiunii sau timestamp-ului fișierului.
- Negative-cache pentru fișiere media corupte/necitibile, ca FFmpeg/ffprobe să nu fie relansate inutil la fiecare verificare.
- Pre-index media gradual, cu un singur worker și buget redus, care nu concurează cu scanarea sau Download Guard.
- Fingerprint-ul remote video este calculat o singură dată și reutilizat pentru candidații locali.

### MEGA și player
- Schimbarea preview-ului WebDAV folosește `pornește noul stream → confirmă URL-ul → oprește vechiul stream`; dacă noul stream eșuează, cel vechi rămâne activ.
- Debounce la navigarea rapidă prin rezultatele MEGA pentru a evita porniri/opriri WebDAV inutile.
- Clasificare explicită pentru cotă, rate-limit, autentificare, link/cheie invalidă, indisponibilitate, spațiu insuficient și rețea.
- Sunt recunoscute `API_EOVERQUOTA`, HTTP 509 și HTTP 429; dacă MEGA oferă un timp de retry, acesta este afișat utilizatorului.

### Fiabilitate și recuperare
- Salvarea cozii folosește înlocuire atomică, iar istoricul se sincronizează imediat după salvarea durabilă a unui job finalizat.
- Recovery pentru `config.json`, index, rezultate și decizii manuale din `.tmp` valid sau ultimul backup valid după crash/power-loss.
- Un `.tmp` complet și mai nou poate fi promovat chiar dacă fișierul principal vechi este încă valid.
- Cache-urile media au schemă/versionare și protecții de consistență pentru scrieri concurente.

### Interfață
- Verdicturile Smart Guard sunt afișate coerent în raport, detalii, coadă și tabelul principal, păstrând compatibilitatea internă cu filtrele/API existente.
- `LIMITĂ / COTĂ`, `INDISPONIBIL` și acțiunile recomandate sunt afișate separat de erorile tehnice generice.

## [8.4.1] — 2026-09-04

### Fiabilitate
- Închiderea controlată salvează coada și evită joburile rămase fals `running`.
- Updater cu backup, health-check, rollback și așteptarea închiderii procesului vechi înainte de înlocuirea EXE-ului.
- Procesele externe sunt ascunse și curățate la Stop/Cancel/închiderea aplicației.
- `Pauză TOT`, `STOP TOT`, reluare și recovery de coadă păstrează coerent starea, viteza și ETA.
- Un engine care nu produce nici fișier, nici eroare nu mai poate fi raportat ca succes.
- Fișierul final este verificat după dimensiune și, când sursa oferă checksum, după SHA-256/MD5.
- Downloaderul intern închide/sincronizează fișierul `.part` înainte de rename și detectează transferurile incomplete.

### MEGA și surse
- Scanarea, preview-ul și downloadul MEGA reutilizează aceeași sesiune MEGAcmd și sunt arbitrate pentru a evita conflictele login/logout/webdav/get.
- Joburile MEGA așteaptă sesiunea în loc să consume inutil retry-uri cât timp rulează o scanare.
- Anularea MEGA este separată de erorile de rețea retryable.
- Fișiere MEGA vechi din destinație nu mai sunt confundate cu rezultatul jobului curent.
- yt-dlp nu mai sare silențios peste un redownload explicit doar din cauza istoricului archive.
