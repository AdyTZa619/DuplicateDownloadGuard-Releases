# Jurnal actualizări — Duplicate Download Guard PRO

Pentru istoricul complet până la 8.5.45 vezi `CHANGELOG_HISTORY_8.5.45.md`.

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
