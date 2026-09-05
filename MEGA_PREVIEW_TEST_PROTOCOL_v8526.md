# MEGA Preview v8.5.32-test.1 — validare Windows

Acesta este un candidat de test, nu o versiune stable. Nu schimbă modulele Duplicate Detection, Smart Guard sau Download Studio.

## Dovada din testele reale 8.5.26–8.5.27

- `webdav /` a răspuns rapid, dar MEGAcmd a normalizat `/` la numele folderului public. Parserul root l-a respins și a produs `MEGA_UNKNOWN`.
- aceeași selecție prin `webdav H:HANDLE` a fost gata în aproximativ 130–170 ms, iar primul cadru în aproximativ 1,3–2,1 s.
- 8.5.26 nu salva ruta per-fișier reușită, deci următorul click relua inutil `session → logout → login → webdav /`.
- în 8.5.27, comenzile per-fișier au răspuns normal în aproximativ 160–270 ms până la primul val de cleanup automat.
- după `webdav -d` cu timeout, MegaClient a început să returneze repetat `Failed to access server: 231`; fiecare comandă a durat aproximativ 9,2 s.
- în testul 8.5.28, DDG nu a mai produs linii `CLEANUP`, însă serviciul MEGAcmd rămas blocat de rularea veche a continuat să returneze eroarea 231 peste restartul aplicației.
- în testul 8.5.29, recuperarea a reușit: restartul țintit a durat 1,081 s, repetarea aceluiași handle 111 ms, iar următoarele 27 de comenzi MEGAcmd au răspuns în 97–432 ms (media aritmetică 139 ms). Totuși, 8 din 31 de clickuri au așteptat 1,595–13,087 s înainte să ajungă la backend (`T0→T1`).
- în testul 8.5.30, toate cele 32 de comenzi MEGAcmd au rămas rapide: media 159 ms și maximul 493 ms. Nu a reapărut eroarea 231 și nu a existat cleanup MEGAcmd. Cu toate acestea, 8 clickuri au depășit 500 ms înainte să ajungă la backend, cu `T0→T1` maxim 16,364 s; 12 cereri media au depășit 500 ms după pregătirea WebDAV, cu `T4→T5` maxim 9,524 s. Prin urmare, întârzierea rămasă este pe originea HTTP comună UI/media, nu în comanda MEGAcmd.
- în testul 8.5.31, separarea media a rezolvat partea vizată: toate cele 26 de cereri care au ajuns la player au avut `T4→T5` sub 500 ms, media 25,4 ms și maximul 389 ms. Comenzile MEGAcmd au avut media 158,8 ms și maximul 450 ms. Totuși, `T0→T1` a rămas blocat pe originea principală: 16 din 32 de clickuri au depășit 500 ms, maxim 17,077 s. Șase generații au devenit vechi înainte să instaleze media, confirmând că blocajul rămas precedă backendul și playerul.

## Arhitectura candidatului

- WebDAV root este eliminat din traseul principal.
- Fiecare fișier este expus prin handle-ul exact `H:HANDLE`.
- sesiunea publică este păstrată între clickuri; `logout/login` apare numai dacă prima probă pe handle dovedește că sesiunea nu corespunde sursei.
- URL-urile per-fișier sunt memorate, dar DDG nu mai execută deloc `webdav -d` în timpul sesiunii. Rutele sunt retrase numai printr-o schimbare normală de sesiune MEGAcmd.
- schimbarea rapidă A → B → C anulează transferul HTTP/playerul vechi, dar lasă comanda MegaClient deja pornită să termine și aruncă rezultatul dacă nu mai este selecția curentă.
- comenzile de control MegaClient nu mai folosesc `taskkill /T`; serverul MEGAcmd comun nu este omorât odată cu clientul.
- numai după răspunsul exact `Failed to access server: 231`, DDG oprește țintit `MEGAcmdServer.exe`, îl pornește din nou ascuns și repetă fișierul curent. Recuperarea este permisă o singură dată pentru sursa curentă, ca să nu poată deveni buclă.
- înaintea unei selecții noi, elementul video/audio vechi primește `pause → remove src → load → remove`, iar imaginea progresivă primește un `src` local minuscul înainte de eliminare. Astfel browserul închide cererea media veche înainte să trimită noul `/start`, iar evenimentele elementului retras nu mai pot modifica preview-ul curent.
- proxy-ul media rulează pe un al doilea listener `127.0.0.1` cu port dinamic, separat de listenerul UI/API. Doar `/api/remote-preview/media` este expus pe acel port. Astfel streamurile lungi și cererile Range nu mai pot ocupa conexiunile originii pe care circulă selecția următoare, statusul și jurnalul. La schimbarea fișierului, DDG închide explicit conexiunile media ale generației vechi; conexiunile browserului nu sunt reutilizate între preview-uri.
- `/start`, status, event, stop și timings rulează pe un al treilea listener `127.0.0.1`, dedicat controlului MEGA Preview. CORS permite numai originea exactă a interfeței DDG; celelalte API-uri nu sunt expuse acolo.
- evenimentele normale nu mai folosesc `fetch keepalive`, iar T11/T12 nu mai sunt trimise din browser înainte de următorul `/start`. Backendul înregistrează aceste două etape când anulează generația veche și închide conexiunea.
- o selecție nouă anulează cererea `/start` precedentă dacă răspunsul ei nu a sosit încă; astfel clickurile rapide nu pot umple listenerul de control cu selecții deja depășite.
- Range/206 și T0–T12 rămân instrumentate. Eroarea Windows 231 are clasificare separată în jurnal și interfață.

## Test minim

1. Pornește aplicația și deschide rezultatele ultimei scanări MEGA. Nu este necesară închiderea manuală a proceselor MEGAcmd rămase din testul vechi.
2. Deschide un fișier care înainte returna eroarea 231. Dacă serviciul este deja sănătos, nu trebuie să apară o recuperare nouă; dacă eroarea revine, trebuie să apară cel mult o comandă `MEGAcmdServer recovery`, urmată de `per-file-after-server-restart`.
3. Selectează 20 de imagini/video/audio consecutive, inclusiv o secvență rapidă A → B → C.
4. Revino la un fișier deja previzualizat.
5. Fă o scanare MEGA nouă și repetă primele trei preview-uri.
6. Repornește aplicația și testează primul preview.

Prima generație din jurnal trebuie să conțină `ARCH control=http://127.0.0.1:PORT media=http://127.0.0.1:PORT separated=true`, cu două porturi diferite. În jurnal nu trebuie să apară nicio linie `CLEANUP`, nicio comandă `webdav -d` și mai mult de o linie `MEGAcmdServer recovery` pentru aceeași sursă. Pentru fiecare click, `T1` trebuie să rămână sub 500 ms; această valoare măsoară strict timpul dintre clickul din UI și primirea cererii de către backend, înaintea comenzii MEGAcmd. După `T4`, prima cerere media `T5` trebuie de asemenea să pornească fără pauzele periodice de mai multe secunde.

Jurnalul complet se scrie în `data\MEGA_Preview_Diagnostic.log`. Timpii structurați sunt disponibili și la `http://127.0.0.1:PORT/api/remote-preview/timings` cât timp aplicația rulează.

## Criteriu

Nu se declară rezolvat până când testul real confirmă că după 20 de schimbări nu apar pauze periodice de 10–30 s și nu cresc necontrolat streamurile/procesele.

| Test | Fișier | T0→T4 | T4→T8 | T8→T10 | Total | Rezultat |
|---:|---|---:|---:|---:|---:|---|
| 1 |  |  |  |  |  |  |
| 2 |  |  |  |  |  |  |
| 3 |  |  |  |  |  |  |
| 4 |  |  |  |  |  |  |
| 5 |  |  |  |  |  |  |
| 6 |  |  |  |  |  |  |
| 7 |  |  |  |  |  |  |
| 8 |  |  |  |  |  |  |
| 9 |  |  |  |  |  |  |
| 10 |  |  |  |  |  |  |
| 11 |  |  |  |  |  |  |
| 12 |  |  |  |  |  |  |
| 13 |  |  |  |  |  |  |
| 14 |  |  |  |  |  |  |
| 15 |  |  |  |  |  |  |
| 16 |  |  |  |  |  |  |
| 17 |  |  |  |  |  |  |
| 18 |  |  |  |  |  |  |
| 19 |  |  |  |  |  |  |
| 20 |  |  |  |  |  |  |
