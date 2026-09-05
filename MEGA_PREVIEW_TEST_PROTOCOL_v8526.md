# MEGA Preview v8.5.26-test.1 — validare Windows

Acesta este un candidat de test, nu o versiune stable. Nu schimbă modulele Duplicate Detection, Smart Guard sau Download Studio.

## Cauzele reparate în candidat

- 8.5.10 considera suficient faptul că vechiul port WebDAV răspundea după restart; asta nu dovedea că portul deservea încă aceeași sesiune și același root.
- 8.5.17–8.5.25 tratau un `previewMu` ocupat drept cache miss, deci puteau reporni inutil ruta MEGAcmd.
- workerul global al preview-ului serializa cererile: B și C puteau rămâne în spatele procesului A chiar după anularea selecției A.
- cleanup-ul playerului și lifecycle-ul serviciului MEGA erau amestecate. Candidatul păstrează un singur serviciu per sursă și anulează separat numai transferul HTTP vechi.

## Test minim

1. Pornește aplicația și deschide rezultatele ultimei scanări MEGA.
2. Selectează 20 de imagini/video/audio consecutive, inclusiv o secvență rapidă A → B → C.
3. Revino la un fișier deja previzualizat.
4. Fă o scanare MEGA nouă și repetă primele trei preview-uri.
5. Repornește aplicația și testează primul preview.

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
