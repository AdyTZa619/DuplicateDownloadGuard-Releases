# MEGA Preview v8.5.28-test.1 — validare Windows

Acesta este un candidat de test, nu o versiune stable. Nu schimbă modulele Duplicate Detection, Smart Guard sau Download Studio.

## Dovada din testele reale 8.5.26–8.5.27

- `webdav /` a răspuns rapid, dar MEGAcmd a normalizat `/` la numele folderului public. Parserul root l-a respins și a produs `MEGA_UNKNOWN`.
- aceeași selecție prin `webdav H:HANDLE` a fost gata în aproximativ 130–170 ms, iar primul cadru în aproximativ 1,3–2,1 s.
- 8.5.26 nu salva ruta per-fișier reușită, deci următorul click relua inutil `session → logout → login → webdav /`.
- în 8.5.27, comenzile per-fișier au răspuns normal în aproximativ 160–270 ms până la primul val de cleanup automat.
- după `webdav -d` cu timeout, MegaClient a început să returneze repetat `Failed to access server: 231`; fiecare comandă a durat aproximativ 9,2 s.

## Arhitectura candidatului

- WebDAV root este eliminat din traseul principal.
- Fiecare fișier este expus prin handle-ul exact `H:HANDLE`.
- sesiunea publică este păstrată între clickuri; `logout/login` apare numai dacă prima probă pe handle dovedește că sesiunea nu corespunde sursei.
- URL-urile per-fișier sunt memorate, dar DDG nu mai execută deloc `webdav -d` în timpul sesiunii. Rutele sunt retrase numai printr-o schimbare normală de sesiune MEGAcmd.
- schimbarea rapidă A → B → C anulează transferul HTTP/playerul vechi, dar lasă comanda MegaClient deja pornită să termine și aruncă rezultatul dacă nu mai este selecția curentă.
- comenzile de control MegaClient nu mai folosesc `taskkill /T`; serverul MEGAcmd comun nu este omorât odată cu clientul.
- Range/206 și T0–T12 rămân instrumentate. Eroarea Windows 231 are clasificare separată în jurnal și interfață.

## Test minim

1. Pornește aplicația și deschide rezultatele ultimei scanări MEGA.
2. Selectează 20 de imagini/video/audio consecutive, inclusiv o secvență rapidă A → B → C.
3. Revino la un fișier deja previzualizat.
4. Fă o scanare MEGA nouă și repetă primele trei preview-uri.
5. Repornește aplicația și testează primul preview.

În jurnal nu trebuie să apară nicio linie `CLEANUP` și nicio comandă `webdav -d` în secvența de preview.

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
