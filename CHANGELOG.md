# Jurnal actualizări — Duplicate Download Guard PRO

Pentru istoricul complet până la 8.5.45 vezi `CHANGELOG_HISTORY_8.5.45.md`.

## [9.0.0] — 2026-09-09

### Promovarea versiunii curente
- Promovează în Stable conținutul buildului `8.5.49-test.133`, fără alte schimbări funcționale în Media Picker.
- Include starea curentă a comparației `PE PC / LIPSĂ / DE VERIFICAT`, a previewurilor dedicate și a integrării JDownloader.
- Păstrează oprirea aplicației și gestionarea HLS exact în forma livrată în TEST `.133`.
- Include în Stable rutele și legăturile funcționale care anterior erau injectate numai în buildul TEST.
- Păstrează updaterul TEST cu manifestul și EXE-ul citite din aceeași revizie Git și verificate SHA-256.
- Păstrează `8.5.49-test.129` ca punct explicit de revenire.

## [TEST130] — 2026-09-09

### Finalizare și punct de revenire
- Păstrează `8.5.49-test.129` în branchul `checkpoint/test129-results-working-20260908` ca punct explicit de revenire.
- Întărește preview-ul pentru Erome, GoFile, Bunkr și Cyberdrop, inclusiv reîmprospătarea linkurilor CDN expirate și diagnosticele pentru erori HTTP.
- Corectează oprirea backendului, jurnalizarea fatală, cursa din controllerul MEGA Preview și verificarea exactă a versiunii după update.
- Calculează `PE PC / LIPSĂ / DE VERIFICAT` numai din dovezi locale curente și elimină deciziile persistente rămase fără fișier local.
- Menține Media Picker exclusiv pentru site-uri generice; MEGA, GoFile, Bunkr, Cyberdrop și Erome folosesc fluxurile dedicate.
- Trimite către JDownloader numai fișierele confirmate `LIPSĂ`, într-un singur request și un singur pachet; elimină bypass-ul `Trimite TOATE`.
- Extinde CI-ul cu verificare JavaScript recursivă și teste Node de comportament.

## [8.5.48] — 2026-09-06

### Updater TEST — reparare SHA-256 și revizie fixă
- Repară eroarea „SHA-256 TEST nu corespunde manifestului”: updaterul nu mai hash-uiește accidental răspunsul JSON al GitHub Contents API în locul EXE-ului.
- Manifestul TEST și EXE-ul sunt citite din aceeași revizie Git fixată înainte de download, eliminând nepotrivirile dacă ramura `testing` se schimbă în timpul instalării.
- Pentru EXE-urile mari, updaterul citește blob-ul Git base64 exact și calculează SHA-256 pe bytes-ii reali ai executabilului.
- Stable 8.5.48 este un patch de recuperare/infrastructură pentru updater; nu schimbă MEGA Preview, matching-ul sau motorul normal de download.

## [8.5.47] — 2026-09-06

### Updater TEST — compatibilitate și acces
- Repară eroarea „Canal TEST indisponibil momentan: Failed to fetch” din aplicația Windows: manifestul și EXE-ul TEST sunt citite prin GitHub REST API, care suportă CORS pentru cereri din interfața locală.
- Descărcarea TEST păstrează verificarea SHA-256 înainte ca EXE-ul să fie trimis updaterului local.
- Butoanele Stable/TEST din dreapta sus sunt montate ca elemente separate lângă monitorul operațional și nu mai pot fi șterse când HUD-ul își reconstruiește conținutul.
- Stable 8.5.47 este un patch de infrastructură pentru updater; nu modifică MEGA Preview, matching-ul sau fluxul normal de download.

## [8.5.46] — 2026-09-06

### Updater — Stable + TEST separat
- Adaugă un canal TEST separat de Stable pentru build-uri de probă, astfel încât fixurile să poată fi verificate înainte de publicarea normală.
- Când există o versiune nouă, în colțul din dreapta sus apare direct butonul de update; Stable și TEST sunt diferențiate clar.
- Butonul din colț poate instala direct versiunea disponibilă, fără să fie necesară deschiderea manuală a secțiunii Updater.
- Build-urile TEST folosesc același mecanism local de backup, health-check și rollback ca updaterul existent.
- Adaugă pipeline separat pentru ramura `testing`, cu teste, vet, verificare JavaScript, build Windows x64, SHA-256 și manifest `update-test.json`.
- Canalul Stable rămâne independent; un build TEST nu modifică `update.json` și nu este publicat automat tuturor utilizatorilor.

### Compatibilitate
- Nu modifică MEGA Preview, matching-ul sau motoarele de download existente.
