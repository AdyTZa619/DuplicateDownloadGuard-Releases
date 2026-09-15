# DDG 9.0.1 TEST — detector MEGA/Bunkr și barieră JDownloader

Data ultimei verificări locale: 2026-09-15.

## Bază verificată

- Stable `9.0.0`: commit `01ed60d3e86475d85fc6b651df5eb75763fb680e` (`origin/main`).
- TEST de pornire `9.0.1-test.136`: commit `0e3eabfe7fd34fedb45ea22d4be9c1c9ea33ae06` (`origin/testing`).
- SHA-256 publicat pentru `.136`: `74e879988e927d16aba78c1e1ed73cdd9c790db7a559ea9aeee6cbbda4e8904e`.
- Ramură de lucru: `core-migration/9.0-detector-completion`.
- Nu s-a modificat și nu se publică Stable.

Auditul a confirmat că `.136` avea deja motorul comun `evaluateDownloadGuard`, index/candidate cache, SHA-256/MD5, range sampling, video/image/audio fingerprint și accesul MEGA Preview. Modificările de aici extind motorul existent; nu introduc un al doilea detector pentru Bunkr.

## Modificări

1. Eliminarea bypass-ului JDownloader din browser. Fluxul UI folosește exclusiv `/api/download/jdownloader-direct`; backendul reexecută guard-ul și transmite către JDownloader numai deciziile finale `DOWNLOAD` / acțiunea `DESCARCĂ` / clasificarea `LIPSĂ`.
2. `allowReview` nu mai poate introduce `DE VERIFICAT` nici în JDownloader, nici în coada internă. `ACELAȘI CONȚINUT` este normalizat intern ca `DUPLICATE`, cu acțiunea `NU DESCĂRCA`.
3. Căutarea video nu mai poate rămâne blocată în primele 64/512 intrări. Toate fingerprinturile deja cache-uite sunt comparate ieftin, iar coada necunoscută avansează în loturi până când nu mai există candidați neanalizați.
4. Căutarea imaginilor avansează progresiv prin colecția relevantă și reutilizează semnăturile calculate în trecerile anterioare; fingerprintul remote este calculat o singură dată per analiză.
5. Dacă rămâne orice candidat relevant necitit/neanalizat, verdictul este `DE VERIFICAT`, niciodată `LIPSĂ`.
6. Analiza video este adaptivă: 7 cadre inițiale, apoi 15 pentru scoruri ambigue și 25 numai dacă etapa de 15 rămâne ambiguă. Punctele noi nu repetă cadrele deja extrase.
7. Pentru trim/intro/outro, o potrivire vizuală aliniată de minimum 93 este promovată la `ACELAȘI CONȚINUT` numai cu dovadă audio independentă Chromaprint de minimum 82.
8. Rezultatul explică separat scorul intern video, scorul intern audio, cadrele compatibile, diferența de durată, metadata și calitatea tehnică. Scorul este etichetat explicit ca similaritate internă, nu probabilitate statistică.
9. Cache-ul adaptiv remote↔local este persistent și este reutilizat numai dacă versiunea remote are validator stabil, iar fișierul local păstrează size, mtime și identitatea filesystem. Schimbarea, înlocuirea, mutarea sau ștergerea invalidează folosirea intrării.
10. Timeout-ul arbitrar de 90 s per rezultat a fost eliminat din workerul detectorului; anularea explicită și limitele transportului remote rămân active.

## Fișiere schimbate

Motor și verdict: `duplicate_candidates_v90.go`, `duplicate_deep_video_v901.go`, `duplicate_evidence_v90.go`, `duplicate_image_candidates_v90.go`, `duplicate_source_v90.go`, `image_signature_cache.go`, `smart_media_guard.go`, `video_candidate_analysis.go`, `video_verdict_finalize.go`.

Barieră download/JDownloader: `jdownloader_direct_v8550.go`, `main.go`, `v7_extra.go`, `v8_extra.go`, `web/download_actions_v8545.js`, `web/exact_guard.js`, `web/jdownloader_batch_confirm_v8564.js`, `web/jdownloader_fast_v8567.js`, `web/jdownloader_final_v8551.js`, `web/jdownloader_window_capture_v8566.js`.

Explicabilitate UI: `web/duplicate_evidence_v90.js`.

Teste/corpus: `duplicate_corpus_v90_test.go`, `duplicate_evidence_v90_test.go`, `duplicate_progressive_v901_test.go`, `duplicate_source_integration_v90_test.go`, `jdownloader_batch_confirm_v8564_test.go`, `jdownloader_bridge_v8545_test.go`, `jdownloader_guard_v901_test.go`, `jdownloader_media_picker_v8566_test.go`, `smart_media_guard_test.go`, `smart_state_engine_v8569_test.go`, `web/tests/duplicate_evidence_v90.test.js`, `web/tests/jdownloader_flow_v85130.test.js`, `validation/generate_duplicate_corpus.py`.

Media Picker, redesign-ul UI, `web/index.html` și implementarea MEGA Preview nu au fost modificate.

## Rezultate automate locale

Corpus reproductibil cu codecuri reale FFmpeg, 22 cazuri: **22 trecute, 0 eșuate**.

| Grup | Cazuri trecute |
|---|---|
| Exact | identic, redenumit, mutat |
| Video transformat | remux, H.264→H.265, 1080p→720p, bitrate, metadata, FPS, trim 2 s, intro, outro, watermark, crop minor, brightness |
| Imagine transformată | JPEG recomprimat, resize, WebP, crop minor |
| Negative | video foarte asemănător dar diferit, aceeași durată/conținut diferit, același nume/conținut diferit |

Teste suplimentare trecute:

- video-ul real din corpus găsit după 20 de decoy-uri din shortlist, cu nume și folder complet diferite;
- imaginea duplicat găsită după 130 de decoy-uri;
- cache local invalidat la schimbare, înlocuire cu același nume și aceeași mărime, mutare și ștergere;
- remote fără metadata completă rămâne `DE VERIFICAT` și nu intră în JDownloader;
- Range ignorat/invalid și validator remote schimbat sunt refuzate sigur;
- analiza întreruptă nu produce verdict descărcabil;
- un lot mixt JDownloader trimite numai elementul confirmat `LIPSĂ`;
- `go test ./...`, testele JS, verificarea sintaxei JS, `go test -race ./...`, `go vet ./...` și build cross-compiled Windows x64 au trecut local.

Buildul Windows x64 local cross-compilat după corecția JDownloader are 9.460.224 bytes și SHA-256 `a8fc4560ab866026e1d1d52a026c073651078f80f65c4df0302250099f6bcc92`. Acesta este doar un artefact local de verificare, nu buildul TEST publicat.

## Timp și trafic pe corpus

| Măsură | Prima analiză | Analiză repetată/cache |
|---|---:|---:|
| Minim | 4,376 ms | 2,375 ms |
| Mediană | 4.490,566 ms | 319,990 ms |
| Medie | 8.577,165 ms | 205,564 ms |
| Maxim | 24.826,328 ms | 388,524 ms |
| Trafic total 22 cazuri | 12.053.688 bytes | 9.120.224 bytes |

Corpusul folosește fișiere media foarte mici. Multe cereri Range acoperă astfel aproape întregul fișier; traficul de mai sus nu poate fi extrapolat la fișiere MEGA/Bunkr mari.

False positive în corpus: **0/3 cazuri negative**. False negative în corpus: **0/19 cazuri duplicate/variante**.

## Verificări reale și limite

| Mediu | Stare |
|---|---|
| Windows desktop real, foldere/HDD reale | **NEVERIFICAT** — CI `windows-latest` pentru PR #95 a trecut `go test`, `go vet`, build x64 și uploadul artefactului de validare, dar nu reprezintă utilizare într-o sesiune desktop cu HDD-uri reale. |
| MEGA real | **NEVERIFICAT** — nu există link/sesiune MEGA reală disponibilă în acest mediu. |
| Bunkr real | **NEVERIFICAT** — integrarea Bunkr a fost testată prin provider/HTTP controlat, nu pe CDN/gallery-dl Bunkr live. |
| JDownloader real | `.138` a eșuat în utilizarea reală: interceptorul `window` încărcat primul apela modulul `fast`, care trimitea direct la `/flashgot` și ocolea backendul. Corecția elimină inițiatorii direcți și are teste de click/rutare plus server local controlat, dar noul candidat **NU A FOST ÎNCĂ RETESTAT** cu o instanță JDownloader reală. |
| Regresie MEGA Preview | Fișierele preview nu au fost atinse; suitele automate existente trec. Redarea MEGA reală rămâne neverificată. |

Prin urmare, candidatul nu este declarat „validat” pe MEGA+Bunkr reale. El poate fi publicat numai ca versiune **TEST**, iar verificarea manuală reală rămâne obligatorie înainte de orice promovare Stable.

## Publicare TEST

PR #95 a trecut `DDG validation` și `DDG stability boundary` și a fost integrat în `testing` la commitul `8ecbaaae624b48ce7be9fcb456c9a8959167e502`. Buildul rezultat `9.0.1-test.138` a expus bypass-ul JDownloader descris mai sus și nu mai este considerat candidat sigur pentru acest flux. Corecția se publică într-o versiune TEST ulterioară numai după CI. Stable rămâne nemodificat.

PR #96 a eliminat bypass-ul și a fost integrat în `testing` la commitul `9aa812bc233a2916321c0a592d9b0977927f1aee`. Automatizarea a publicat `9.0.1-test.139` la commitul `9ccc36650fbdaddbc7b48a3e21a62fb94b06b171`, SHA-256 `7de160c64159016c442d71fe0b1d745e7ac106171ad463963acf583b562a00c4`.

## Stabilizare după 9.0.1-test.139

Auditul de defecte efectuat după `.139` a identificat și corectat următoarele probleme, fără a adăuga funcții noi:

1. Backendul JDownloader construia pentru Bunkr URL-ul `/f/<handle>`; acum folosește endpointul descărcabil `/d/<handle>`. Același format este păstrat și în helper-ele UI legacy.
2. Smart State nu mai pierdea evidența elementelor trimise după mutarea handoff-ului în backend: răspunsul backend include exact elementele acceptate, iar UI memorează numai acele transferuri.
3. Statusurile legacy/provizorii `MISSING` și `DIFFERENT` nu mai sunt afișate drept verdict final `LIPSĂ`. Atât Smart State, cât și popup-urile JDownloader cer combinația `guardVerdict=DOWNLOAD` + `detector.classification=LIPSĂ`; altfel rezultatul rămâne `DE VERIFICAT`.
4. Un folder/HDD local configurat dar offline, lipsă sau necitibil nu mai este eliminat silențios din index. Scanarea este marcată incompletă, nu este reutilizată ca snapshot proaspăt și nu poate produce `LIPSĂ`. Un duplicat deja demonstrat într-o altă rădăcină rămâne totuși blocat corect.
5. Fișierele video/imagine din cache care nu mai pot fi citite sau decodate rămân candidați neanalizați. Ele forțează `DE VERIFICAT` în loc să fie excluse din colecția relevantă.
6. Răspunsul backend JDownloader enumeră numai elementele efectiv transmise după eliminarea linkurilor duplicate, astfel încât reconcilierea ulterioară nu poate marca rezultate netrimise ca fiind în JDownloader.

Fișierele modificate în această stabilizare: `download_guard.go`, `download_guard_fresh_cache.go`, `duplicate_candidates_v90.go`, `duplicate_image_candidates_v90.go`, `image_signature_cache.go`, `media_meta_cache.go`, `smart_media_guard.go`, `jdownloader_bridge_v8545.go`, `jdownloader_direct_v8550.go`, `web/jdownloader_batch_confirm_v8564.js`, `web/jdownloader_fast_v8567.js`, `web/jdownloader_window_capture_v8566.js`, `web/smart_state_engine_v8569.js` și testele aferente. Media Picker, redesign-ul UI și MEGA Preview nu au fost modificate.

Verificări locale după ultima modificare:

- `go test ./... -count=1`: trecut în 9,879 s;
- race țintit pentru JDownloader/guard/duplicate/index/media: trecut în 1,340 s;
- `go vet ./...`: trecut;
- verificarea sintaxei tuturor fișierelor JS: trecută;
- `node --test web/tests/*.test.js`: 11/11 trecute în 171,975 ms;
- `git diff --check`: trecut;
- build Windows x64 cross-compilat: 13.333.504 bytes, SHA-256 `272929fc474764726883cad219288d82fb336c292f96e28dc6a93bbe0ddaeb6a`.

Acest build de stabilizare nu a consumat trafic MEGA/Bunkr real: **0 bytes**. Nu au fost măsurați timpi reali de primă analiză/cache pe HDD-uri reale în această rundă. Testele folosesc filesystem temporar, HTTP/provider controlat și un server JDownloader fals local. Windows desktop real, JDownloader real, MEGA real și Bunkr real rămân **NEVERIFICATE** pentru noul candidat; în consecință poate fi publicat numai ca TEST și nu este declarat validat end-to-end.

## Corecție stare provizorie după 9.0.1-test.140

O verificare vizuală reală a arătat `Detector: 1/76`, `Posibile: 75`, dar interfața afișa `De confirmat: 76`. Cauza a fost confundarea dovezii provizorii create înaintea workerului cu verdictul final `DE VERIFICAT`.

Corecția separă explicit:

- `ÎN ANALIZĂ`: workerul nu a terminat încă rezultatul (`guardAt=0`, dovadă provizorie);
- `DE VERIFICAT`: workerul a terminat, dar dovezile au rămas insuficiente;
- `AI DEJA`: `IDENTIC` sau `ACELAȘI CONȚINUT` cu dovadă locală actuală;
- `LIPSEȘTE`: exclusiv `guardVerdict=DOWNLOAD`, `detector.classification=LIPSĂ` și analiză finalizată.

Filtrul, sumarul, tabelul, selecția inteligentă și calculul bytes pentru download folosesc aceeași separare. Pentru cazul observat, sumarul așteptat în timpul progresului este `75 În analiză` și `1 De verificat`, nu `76 De confirmat`. Bariera JDownloader nu este relaxată.

Verificări locale ale corecției: `go test ./...` trecut în 10,119 s; race țintit trecut în 1,595 s; `go vet ./...` trecut; 11/11 teste JS trecute; build Windows x64 cross-compilat 13.326.336 bytes, SHA-256 `1ee774e45c64ccc1c3339822c31007ef2c9b5907be3204afce23f69ac39dd986`. Testarea pe instalarea Windows din captură rămâne necesară.
