## [8.5.42] — 2026-09-06

### GoFile — sesiune guest durabilă și protecție reală la rate-limit
- Tokenul guest creat de DDG este păstrat în `data/cache/gofile_guest_state.json` și reutilizat după restart sau update, în loc să fie recreat la fiecare pornire.
- Un token configurat prin `GOFILE_TOKEN`, `GOFILE_API_TOKEN` sau `GF_TOKEN` are prioritate și nu este persistat de DDG.
- `POST /accounts` nu mai este repetat după `429`: DDG salvează un cooldown și nu mai lovește endpointul până la expirarea lui; fallback-ul `gallery-dl` este oprit când cauza este rate-limit GoFile.
- Tokenul guest este invalidat numai pentru erori explicite de autentificare precum `error-wrongToken` sau `error-notAuthenticated`, nu pentru orice răspuns HTTP 401.
- Pentru metadata, DDG încearcă mai întâi candidații de website-token cu același account token și abia apoi decide dacă sesiunea trebuie refăcută.
- Valoarea implicită folosită de generatorul website-token este actualizată la `12af056dacea0b`, iar override-ul `GOFILE_WT_SALT` rămâne disponibil.
- Interogarea de metadata este aliniată la clientul web (`sortField=createTime`, `sortDirection=-1`), iar User-Agent-ul browser-like este folosit consecvent în formula WT și în cereri.
- Testele acoperă persistența tokenului, token configurat ne-persistent, cooldown-ul 429, fallback-ul website-token cu același cont și separarea erorilor de autentificare de cele de acces.

### Compatibilitate
- MEGA Preview și semantica de matching nu sunt modificate.

## [8.5.41] - 2026-09-06
- Repară timeout-ul la crearea tokenului guest GoFile: cererea `POST /accounts` folosește acum antetele web curente `X-Website-Token` și `X-BL`, plus corp JSON explicit.
- Tokenul web pentru crearea contului este calculat cu account token gol, exact pentru etapa de guest-account, folosind același User-Agent ca cererile de metadata.
- Timeout-urile de transport și răspunsurile temporare 408/425/429/5xx sunt reîncercate controlat de până la 3 ori; `429` respectă `Retry-After`.
- Fiecare încercare are timeout propriu de 15 secunde, astfel încât o singură conexiune blocată nu mai aruncă imediat scanarea în fallback.
- Tokenurile GoFile rămân numai în memorie; nu sunt persistate pe disc.
- MEGA Preview, matching-ul și celelalte providere nu sunt modificate.

## [8.5.40] - 2026-09-06
- Repară `GoFile API HTTP 401`: DDG folosește saltul curent pentru `X-Website-Token`, sincronizat cu protocolul GoFile actual.
- La `401`, tokenul guest din RAM este invalidat și refăcut o singură dată înainte de a declara scanarea eșuată.
- La `429 Too Many Requests`, DDG nu mai cade imediat în fallback: respectă `Retry-After` când este furnizat și reîncearcă controlat de până la 5 ori.
- Cererile native GoFile sunt distanțate cu minimum 500 ms pentru a reduce declanșarea rate-limit-ului pe foldere și subfoldere.
- Saltul GoFile poate fi suprascris prin `GOFILE_WT_SALT` fără recompilare dacă providerul îl rotește din nou.
- MEGA Preview, matching-ul și celelalte providere nu sunt modificate.

## [8.5.39] - 2026-09-06
- GoFile folosește acum un adapter nativ pe API-ul curent pentru listarea folderelor, cu token web dinamic și metadata directă; `gallery-dl` rămâne fallback, nu dependență principală.
- Scanarea GoFile are timeout controlat și jurnal explicit pe etape, astfel încât un eșec nu mai poate rămâne fără rezultat și fără eroare.
- Contextul de autentificare GoFile rămâne numai în memorie; tokenurile/cookie-urile nu sunt salvate în `RemoteItem` sau în istoricul persistent.
- Repară oprirea noii versiuni la câteva secunde după update: procesul `--ddg-native-updater-cleanup` nu mai poate închide fereastra DDG proaspăt pornită.
- Păstrează cleanup-ul automat al update-urilor vechi și un singur backup pentru rollback.
- MEGA Preview și semantica de matching nu sunt modificate.

# Jurnal actualizări — Duplicate Download Guard PRO

Acest fișier păstrează schimbările importante pentru fiecare versiune publicată. Pentru fiecare release nou trebuie adăugată o secțiune `## [x.y.z]`; pipeline-ul de release verifică existența ei înainte de publicare.

## [8.5.38] — 2026-09-06

### GoFile / Bunkr / Cyberdrop — scanare fără blocaj înainte de rezultate
- Scanarea inițială nu mai execută câte un HEAD/Range HTTP pentru fiecare fișier după ce gallery-dl a furnizat deja metadata necesară comparației locale.
- GoFile, Bunkr și Cyberdrop trimit lista extrasă direct către comparator; detaliile HTTP de transport sunt rezolvate ulterior numai când sunt necesare pentru preview, verificare sau download.
- Elimină cazul în care un folder cu multe fișiere părea că nu face nimic și nu afișa nici rezultat, nici eroare, din cauza timeout-urilor de până la 25 secunde pe probele CDN.
- Sursele gallery-dl generice păstrează fallback-ul de îmbogățire HTTP atunci când metadata extractorului nu este suficientă.

### Compatibilitate
- MEGA Preview și semantica de matching rămân neschimbate.
- Testele verifică explicit că GoFile/Bunkr/Cyberdrop nu mai intră pe calea de probe HTTP blocante înainte de rezultate.

## [8.5.37] — 2026-09-06

### GoFile / Bunkr / Cyberdrop — rezultate compatibile gallery-dl
- Parserul provider acceptă acum atât JSON Lines (`output.jsonl`) cât și documentul JSON clasic/pretty-print folosit de versiuni mai vechi de gallery-dl; o extracție validă nu mai poate ajunge la 0 rezultate doar din cauza formatului de ieșire.
- Sunt transformate în fișiere numai mesajele gallery-dl de tip URL (`Message.Url == 3`); mesajele Directory/Queue rămân control/metadata și nu generează rezultate false.
- Sunt păstrate numele reale, mărimile, ProviderID-ul și URL-ul direct înainte de comparația locală; mesajul de eroare diferențiază acum extractorul fără fișiere de un parser incompatibil.
- Testele acoperă GoFile în JSONL, JSON clasic pretty-printed și ignorarea mesajelor Queue.

### Updater — cleanup retroactiv după pornire sănătoasă
- După pornirea normală, DDG așteaptă health marker-ul versiunii curente înainte să curețe fișierele updaterului; nu șterge nimic înainte ca noua versiune să fie confirmată sănătoasă.
- Cleanup-ul funcționează și când update-ul a fost executat de helperul versiunii anterioare, astfel încât resturile deja existente pot fi eliminate la prima pornire a versiunii noi.
- Se păstrează cel mai nou backup EXE pentru rollback; backup-urile mai vechi, helper-ele, pending/request și temporarele updaterului sunt eliminate automat.
- Pe Windows, cleanup-ul se repetă după câteva secunde pentru helperul care putea fi încă blocat la prima încercare.

### Compatibilitate
- MEGA Preview și semantica de matching nu sunt modificate în această versiune.

## [8.5.36] — 2026-09-06

### Sursă online — buton și feedback imediat
- Butonul „Analizează fără download” este legat direct de modulul provider, fără dependență de handlerul inline legacy; click-ul GoFile/Bunkr/Cyberdrop nu mai poate rămâne aparent inert.
- La pornirea scanării, butonul se dezactivează și afișează imediat providerul analizat; Enter în câmpul URL pornește aceeași rută, iar scanările paralele sunt blocate.
- Traseul MEGA dedicat și semantica de matching rămân neschimbate.

### Updater — reconectare și curățare automată
- Verificarea statusului și a update-ului reîncearcă automat conexiunea locală de până la trei ori, fără cache, înainte să raporteze indisponibilitatea; reduce cazul „Failed to fetch” care dispărea doar după restartul DDG.
- După un update confirmat sănătos se păstrează maximum un singur backup EXE pentru rollback; backup-urile mai vechi sunt eliminate înainte de următoarea instalare.
- Helper-ele DuplicateDownloadGuard.Updater_*.exe, DuplicateDownloadGuard.pending.exe, apply_update.json și temporarele .download, .copying, .replacing sunt curățate automat după health-check.
- Curățarea finală a helperului rulează numai după ce procesul updaterului s-a închis, evitând blocarea executabilului de către Windows; path guard-ul împiedică ștergerea în afara folderului de update.
- Testele de regresie acoperă reconnect-ul updaterului, retenția unui singur backup, protejarea fișierelor active și curățarea artefactelor vechi.
## [8.5.35] — 2026-09-06

### Surse online universale — GoFile, Bunkr și Cyberdrop
- Câmpul de sursă online poate ruta explicit GoFile, Bunkr și Cyberdrop prin motorul universal, fără să modifice traseul MEGA stabil.
- `gallery-dl` este folosit în mod metadata/JSON pentru a enumera fișierele fără download integral și păstrează numele reale, mărimile, ID-urile providerului și structura albumelor înainte de comparația locală.
- Contextul HTTP sensibil al providerului — cookie-uri, token guest, Referer și Origin — este ținut numai în RAM și nu este serializat în rezultate, coadă, istoric sau loguri.
- Scanarea HTTP păstrează capabilitățile reale ale sursei și folosește probe HEAD/Range numai pentru completări precum mărime exactă, Content-Type, ETag, hash headers și suport Range.

### Preview remote non-MEGA
- Preview-ul GoFile/Bunkr/Cyberdrop trece printr-un proxy local DDG legat de ID-ul rezultatului, în loc să expună direct URL-ul CDN în browser.
- Proxy-ul păstrează cererile `Range` și răspunsurile `206 Partial Content`, astfel încât seek-ul și citirea parțială nu transformă preview-ul într-un download integral.
- `Content-Disposition: attachment` nu este propagat către suprafața de preview, evitând descărcarea accidentală în locul redării inline.
- La 401/403/404/410, sursele gallery-dl pot fi reextrase o singură dată; fișierul este reidentificat în ordinea ProviderID → cale → nume, iar potrivirile ambigue sunt refuzate.
- URL-ul remote reîmprospătat este păstrat în rezultatul din RAM pentru următoarele accesări, fără a persista credențiale sau tokenuri.
- Playerul extern non-MEGA folosește aceeași rută locală DDG, astfel încât primește același context HTTP ca preview-ul integrat.

### Compatibilitate și validare
- MEGA Preview rămâne pe controllerul dedicat validat în 8.5.33/8.5.34; nu a fost rescris și nu folosește proxy-ul providerilor.
- Downloaderul HTTP intern continuă să folosească transportul provider-aware și păstrează Range/resume; aria2 rămâne opțiune explicită.
- Testele acoperă contextul HTTP, expirarea lui, cookies pe domeniu/path, parsarea JSON gallery-dl, identitatea GoFile/Bunkr/Cyberdrop, Range → 206, refuzul MEGA și eliminarea headerului attachment.
- Pachetul este validat prin `gofmt`, verificarea JavaScript, toate testele Go, `go vet` și build Windows x64 înainte de publicarea stable.
## [8.5.34] — 2026-09-05

### Updater — trecere corectă de la test la stable
- Compararea versiunilor respectă acum precedența SemVer: pentru același nucleu numeric, versiunea stable este mai nouă decât orice prerelease `-test.N`.
- Repară cazul concret în care `8.5.33-test.1` era considerată mai nouă decât `8.5.33` și updaterul afișa greșit „sunt la zi”.
- Testele acoperă trecerea test → stable, ordinea între două candidate test, egalitatea stable și intrările invalide.
- MEGA Preview rămâne identic cu arhitectura 8.5.33 validată real pe Windows.

## [8.5.33] — 2026-09-05

### MEGA Preview — rută rapidă validată pe Windows
- Preview-ul folosește un singur controller pe durata aplicației, cu listener separat pentru control și media; operațiile lente MEGAcmd nu mai blochează serverul principal.
- Selecțiile rapide sunt comasate timp de 90 ms, iar numai ultima selecție pornește preview-ul; cererile depășite sunt anulate fără a fi raportate ca erori MEGA.
- Fallback-ul per-fișier rămâne disponibil și nu mai concurează cu fallback-uri JavaScript duplicate.
- Jurnalul tehnic notează pentru fiecare click traseul, comenzile, timpii, starea root-ului, anulările și motivul fallback-ului.

### Validare reală
- Candidata 8.5.33-test.1 a fost validată pe Windows cu MEGAcmd în 61 schimbări de preview: pregătirea serverului a avut media 107 ms și maxim 158 ms, comenzile per-fișier media 163 ms și maxim 277 ms.
- Nu au existat erori MEGA, cleanup concurent, `webdav -d` sau degradare după schimbări repetate; utilizatorul a confirmat că preview-ul funcționează bine.
- Codul publicat este aceeași arhitectură testată și trece verificările Go, JavaScript, vet și build Windows x64.

## [8.5.10] — 2026-09-04

### MEGA Preview — root persistent și eliminarea blocajelor recurente
- Un fallback pentru un singur fișier nu mai execută `webdav -d /`; WebDAV root rămâne activ pentru fișierele următoare, în loc ca primul child problematic să împingă toată sesiunea pe ruta per-fișier lentă.
- Fallback-ul per-fișier este temporar când root-ul este sănătos: URL-ul alternativ este folosit numai pentru fișierul care a eșuat, iar starea canonică a preview-ului rămâne root-ul.
- După un login/cold resume, DDG încearcă să pornească un singur WebDAV root și derivă local URL-urile tuturor fișierelor; `webdav <fișier>` rămâne doar fallback de compatibilitate dacă root-ul nu poate fi expus.
- La închiderea normală, dacă DDG este proprietarul sesiunii publice și nu există o sesiune MEGA anterioară de restaurat, root-ul WebDAV nu mai este oprit. URL-ul local al root-ului este păstrat pentru restart.
- La pornirea următoare, DDG verifică doar listenerul local și construiește child URL-ul fără `session`, `logout`, `login` sau `webdav`; dacă listenerul nu mai există, cache-ul este invalidat și se revine la mecanismul sigur de resume.
- Cache-ul persistent acceptă exclusiv endpointuri loopback (`127.0.0.1`, `localhost`, `::1`) și nu salvează niciodată tokenul/session ID-ul MEGA sau credențiale de cont.
- Cele două straturi JavaScript de fallback nu mai pot porni două încercări per-fișier succesive pentru aceeași eroare; fallback-ul nou marchează eroarea finală înainte de a delega handlerului vechi.
- Pregătirea root-ului după scanare are propriul context scurt, astfel încât un context de scanare aproape expirat nu mai poate trimite primul click direct pe ruta lentă.

### Validare
- Testele de regresie interzic explicit `webdav -d /` în fallback-ul unui child și verifică faptul că URL-ul fallback rămâne diferit de URL-ul root.
- Teste noi verifică round-trip-ul root-ului persistent, respingerea URL-urilor non-loopback și păstrarea root-ului la shutdown fără comandă de teardown.
- Candidata funcțională a trecut `gofmt`, verificarea tuturor fișierelor JavaScript, toate testele Go, `go vet` și build-ul Windows x64 înainte de bump-ul final la 8.5.10; release-ul este revalidat după bump.

## [8.5.9] — 2026-09-04

### MEGA Preview — cold-start și blocaje după mai multe vizualizări
- Reparată concurența dintre clickurile de preview și comenzile `webdav -d` de cleanup pornite în fundal; cleanup-ul vechi nu mai poate ocoli arbitrul global al sesiunii MEGA.
- Curățarea unui nod WebDAV vechi este acum strict low-priority: încearcă gate-ul MEGA doar pentru o fereastră foarte scurtă și renunță dacă există activitate foreground, în loc să stea în coadă înaintea playerului.
- Cleanup-ul este limitat la un timeout scurt și verifică să nu oprească nodul devenit între timp din nou activ.
- Dacă DDG nu a înlocuit o sesiune MEGA existentă, la închiderea controlată oprește WebDAV-ul dar păstrează sesiunea folderului public activă; astfel restartul nu mai trebuie să execute obligatoriu `session` → `logout` → `login <folder> --resume` înainte de primul preview.
- Se salvează numai un hint local non-secret cu URL-ul folderului public; DDG nu persistă tokenul/session ID-ul MEGA.
- La următoarea pornire, pentru aceeași sursă, DDG încearcă mai întâi `webdav <fișier>` direct pe sesiunea păstrată. Dacă hintul este stale sau sesiunea a fost schimbată extern, tentativa expiră rapid, hintul este invalidat și se revine sigur la ruta `--resume`.
- După un login `--resume` reușit fără sesiune anterioară de restaurat, hintul este actualizat pentru restartul următor.

### Validare
- Teste noi verifică round-trip-ul hintului, izolarea între două URL-uri MEGA și faptul că fast path-ul de restart nu execută `session`, `logout` sau `login` când sesiunea păstrată este validă.
- Ramura a trecut formatarea, verificarea tuturor fișierelor JavaScript, toate testele Go, `go vet` și build-ul Windows x64 înainte de bump-ul final la 8.5.9; release-ul este revalidat după bump.

## [8.5.8] — 2026-09-04

### MEGA Preview — reparare regresie de latență
- Eliminat prewarm-ul MEGA automat de la pornirea interfeței; primul click al utilizatorului nu mai poate rămâne în spatele unei inițializări MEGAcmd pornite în fundal.
- După restart, preview-ul nu mai pornește mai întâi WebDAV pentru întreg folderul public. Folosește `login <folder-link> --resume`, apoi deschide direct fișierul cerut prin WebDAV per-fișier.
- Warm-root-ul rămâne disponibil după o scanare MEGA, unde sesiunea folderului este deja deschisă și costul inițializării a fost plătit.
- Fallback-ul declanșat de player este acum un fallback real: oprește warm-root-ul și pornește explicit `webdav <handle/cale fișier>`, astfel încât nu mai poate întoarce același URL FAST ROOT care tocmai a eșuat.
- Fallback-ul per-fișier folosește și el `--resume`; nu mai există login rece ascuns pe această cale.
- Erorile playerului pentru modurile `MEGA FAST ROOT` și `MEGA DIRECT RESUME` pot declanșa retry-ul per-fișier o singură dată, evitând buclele de retry.
- Diagnosticul expune separat `MEGA DIRECT RESUME` și `MEGA TRUE FALLBACK` pentru a vedea exact traseul folosit.

### Validare
- Teste dedicate verifică secvența `webdav -d /` → `webdav <fișier>`, faptul că URL-ul fallback este diferit de warm-root și faptul că startup prewarm rămâne eliminat.
- Test static de regresie confirmă că restart preview este rutat prin direct per-file resume, nu prin whole-folder warm-root.
- Candidata a trecut verificarea formatării, toate fișierele JavaScript, toate testele Go, `go vet` și build-ul Windows x64 înainte de bump-ul final la 8.5.8; release-ul este revalidat după bump.

## [8.5.7] — 2026-09-04

### Foldere locale — actualizare automată și rezultate corecte
- Schimbările listei de foldere locale sunt detectate automat prin heartbeat-ul interfeței; după adăugarea sau eliminarea unei locații este programată o singură reindexare/recomparare, fără să fie necesară o nouă scanare remote.
- Actualizarea așteaptă operațiile foreground deja active și se serializează cu Smart Guard, astfel încât indexarea, scanarea MEGA și verificarea înainte de download nu se calcă reciproc.
- Rezultatele deja afișate sunt recalculate după adăugarea unui folder; un fișier existent în noua locație poate fi găsit imediat fără restart.
- Eliminarea unui folder curăță din index intrările care aparțin exclusiv locației scoase, prevenind verdictul fals `AI DEJA` pentru fișiere care nu mai fac parte din colecția activă.
- Dacă un folder eliminat conține un subfolder care rămâne configurat sau folderul activ de download, fișierele încă acoperite de acea locație validă nu sunt șterse din index.
- Actualizarea funcționează atât cu `Live refresh înainte de comparație` activ, cât și dezactivat.

### Download Studio și documentație
- Tabelul de ajutor pentru motorul `Auto` este aliniat cu nucleul introdus în 8.5.5: HTTP/CDN direct folosește downloaderul intern în modul Auto; aria2 rămâne opțiune explicită.

### Validare și protecții de release
- CI verifică acum cu `node --check` toate fișierele JavaScript separate din `web/*.js`, nu doar `exact_guard.js`.
- Pipeline-ul de release verifică `gofmt` pentru toate fișierele Go și sintaxa tuturor fișierelor JavaScript separate înainte de teste și build.
- Teste de regresie pentru adăugare de folder cu fișier redenumit de tip `original-D3558.jpg`, pentru `LiveRefreshCompare` activ/dezactivat, eliminarea unui folder și păstrarea corectă a unui subfolder încă activ.
- Pachetul a trecut `gofmt`, verificarea JavaScript, toate testele Go, `go vet` și build Windows x64 înainte de pregătirea release-ului.

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
