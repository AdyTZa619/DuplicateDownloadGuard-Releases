# Jurnal actualizări — Duplicate Download Guard PRO

Acest fișier păstrează schimbările importante pentru fiecare versiune publicată. Pentru fiecare release nou trebuie adăugată o secțiune `## [x.y.z]`; pipeline-ul de release verifică existența ei înainte de publicare.

## [8.5.6] — 2026-09-04

### MEGA Preview — restart și cache real
- Reparată cauza principală a preview-ului care putea sta aproximativ 40–50 secunde după restart: pentru folder links, MEGAcmd era apelat cu `login <link>` și putea reconstrui folderul public de la zero.
- Preview-ul folosește acum `login <folder-link> --resume`, astfel încât MEGAcmd poate reutiliza cache-ul local al folderului public.
- La închiderea controlată a aplicației, sesiunea publică este închisă cu `logout --keep-session` înainte de restaurarea sesiunii MEGA anterioare; cache-ul necesar pentru următorul `--resume` nu mai este șters intenționat.
- După reluarea folderului, aplicația pornește o singură rădăcină WebDAV și derivă local URL-urile fișierelor; revenirea la alte fișiere din același folder nu necesită relogin.
- Dacă scanarea a lăsat deja sesiunea folderului caldă, DDG încearcă să repare doar WebDAV root fără relogin.
- Prewarm oportunist la aproximativ 2,5 secunde după pornirea UI-ului: dacă nu rulează scanare/download și există rezultate MEGA salvate, primul preview este pregătit în fundal înainte de selectarea unui rând.
- Cererile concurente de prewarm/click sunt reunite într-o singură inițializare MEGAcmd; nu mai pot porni două secvențe login/WebDAV care să se blocheze reciproc.
- Mod nou de diagnostic `MEGA FAST RESUME`, separat de `MEGA FAST ROOT`, `MEGA FAST CACHE` și `MEGA FALLBACK`.

### Validare
- Test de regresie pentru argumentele de login MEGA confirmă prezența obligatorie a `--resume`.
- Hotfixul păstrează fallback-ul per-fișier existent dacă reluarea cache-ului sau WebDAV root eșuează.

## [8.5.5] — 2026-09-04

### Download Studio — nucleu refăcut
- Joburile din coadă păstrează snapshotul complet al sursei remote și o identitate stabilă; `ResultID` nu mai este folosit ca dovadă că un rezultat dintr-o scanare nouă este același fișier.
- Joburile vechi 8.5.4 sunt migrate numai când sursa, numele și URL-ul se potrivesc; dacă identitatea nu poate fi demonstrată, transferul este oprit sigur și utilizatorul primește motiv + acțiune.
- Smart Guard verifică joburile desprinse de tabelul curent fără să poată marca/bloca accidental un rând care a reutilizat același ID după rescanare.
- Detectarea unui job deja aflat în coadă folosește identitatea remote stabilă, nu doar ID-ul rezultatului.

### Motoare de download
- `Auto` este determinist: MEGA → MEGAcmd, surse yt-dlp și HLS/DASH → yt-dlp, HTTP/direct/gallery → downloaderul intern. aria2 rămâne opțiune explicită și nu mai este ales doar pentru că este instalat.
- Motorul este validat înainte ca jobul să intre efectiv în transfer; lipsa yt-dlp/aria2/MEGAcmd, HLS/DASH pe motor incompatibil sau URL lipsă apar explicit ca `NU S-A PORNIT`, cu recomandarea concretă.
- yt-dlp folosește URL-ul paginii pentru sursele yt-dlp și URL-ul streamului direct pentru manifestele HLS/DASH.
- aria2 transmite `Referer` atunci când fișierul direct provine dintr-o pagină sursă care îl cere.

### Downloader HTTP intern
- Trimite `Referer` pentru linkurile directe provenite din pagini/gallery-dl, reducând erorile 403 ale CDN-urilor care verifică originea.
- `.part` are identitate stabilă per sursă, astfel încât două URL-uri diferite cu același nume nu se calcă și resume-ul rămâne legat de fișierul corect.
- Un `.part` deja complet după crash este verificat și finalizat fără redescărcarea inutilă a întregului fișier.
- Numele final este collision-safe (`file.ext`, `file (1).ext` etc.); un fișier existent nu mai este suprascris accidental.
- Un răspuns `text/html` primit în locul fișierului media este respins și raportat ca URL expirat/autentificare necesară, nu salvat fals ca video/imagine.
- Erorile HTTP și de motor primesc cod, titlu și acțiune distincte; cazurile nerecuperabile nu mai consumă retry-uri fără sens.

### Interfață
- Butonul principal este simplificat la `Descarcă selectate`; Smart Guard continuă să ruleze automat înainte de transfer.
- Folderul și motorul selectate în Download Studio sunt trimise direct backendului; folderul portabil `downloads\` rămâne fallback dacă nu este ales nimic.
- Fișierele respinse înainte de coadă sunt afișate cu `NU S-A PORNIT`, motorul, cauza și ce trebuie făcut; dacă există joburi acceptate, interfața deschide automat Download Studio.

### Validare
- Teste HTTP reale pentru server care cere `Referer`, coliziuni de nume, resume `.part`, răspuns HTML în loc de media, alegerea deterministă a motorului și identitatea jobului după rescanare.
- Nucleul final a trecut verificarea JavaScript, toate testele Go, `go vet` și build Windows x64 pe CI independent înainte de pregătirea release-ului.

## [8.5.4] — 2026-09-04

### MEGA Preview — hotfix latență
- Eliminată verificarea `HEAD` sincronă din calea rapidă a preview-ului MEGA. După ce scanarea a pregătit WebDAV root, URL-ul fișierului este construit local și trimis imediat playerului.
- Un cache hit `MEGA FAST ROOT` sau `MEGA FAST CACHE` nu mai așteaptă mutexul global MEGA și nu mai lansează nicio comandă MEGAcmd.
- Endpointul de preview raportează modul folosit și timpul de pregătire în milisecunde, pentru diagnosticarea clară a cazurilor care cad pe fallback.
- Dacă URL-ul root rapid nu poate fi consumat de browser, interfața încearcă automat o singură dată fallback-ul WebDAV per-fișier înainte să ofere player extern/MEGA.
- Debounce-ul navigării prin rezultate a fost redus de la 320 ms la 140 ms; rămâne suficient pentru a evita comutări WebDAV inutile când se navighează rapid.
- Analizele Smart Guard, ffprobe și fingerprint rămân pe traseele lor verificate; optimizarea zero-command este limitată la preview-ul interactiv din Compare Studio.

### Validare
- Test nou care demonstrează că un cache hit root nu execută nicio cerere de rețea și se rezolvă local.
- Teste pentru reutilizarea nodului WebDAV per-fișier și izolarea între două surse MEGA diferite.
- Hotfixul a trecut `gofmt`, verificarea JavaScript, toate testele Go, `go vet` și build Windows x64 înainte de release.

## [8.5.3] — 2026-09-04

### MEGA Fast Preview
- După scanarea unui folder MEGA public, aplicația încearcă să pornească o singură rădăcină WebDAV și o păstrează temporar „caldă” pentru verificarea rapidă a fișierelor.
- Pentru fișierele din același folder, URL-ul de preview este derivat local din rădăcina WebDAV; nu mai este necesară pornirea unui nod WebDAV nou la fiecare rând atunci când fast-path-ul este disponibil.
- Fast-path-ul face doar o verificare locală foarte scurtă a URL-ului rezultat; dacă structura WebDAV a instalării MEGAcmd nu corespunde, aplicația cade automat pe mecanismul per-fișier existent.
- Schimbarea dintre două streamuri per-fișier nu mai așteaptă oprirea WebDAV-ului vechi: URL-ul nou este returnat imediat, iar cleanup-ul vechi rulează în fundal după un scurt handoff.
- Pornirea și fallback-urile WebDAV au timeout-uri mai mici pentru a evita blocarea aparentă a Compare Studio.
- Corectată construirea URL-urilor WebDAV pentru nume cu spații și caractere care necesită escaping; calea nu mai este dublu codificată.

### Compare Studio
- Rezumat compact nou direct lângă `REMOTE` și `LOCAL`, astfel încât informațiile principale să fie vizibile fără scroll până la MediaInfo.
- Pentru video/audio sunt afișate automat durata disponibilă din player; pentru video și imagini apare rezoluția încărcată.
- Rezumatul include scorul perceptual/general disponibil și verdictul curent (`ACELAȘI CONȚINUT`, `ALTĂ VERSIUNE`, `PARE ACELAȘI`, `AI DEJA`, `NU ÎL AI` etc.).
- Metadatele rapide sunt citite din elementele media deja încărcate în interfață; nu pornesc un ffprobe suplimentar și nu introduc transfer remote separat.

### Validare
- Teste dedicate pentru construirea URL-urilor WebDAV child, fast-path/fallback și cleanup-ul asincron.
- Test de regresie care confirmă că modulul de rezumat REMOTE/LOCAL este inclus efectiv în EXE și încărcat de interfață.

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