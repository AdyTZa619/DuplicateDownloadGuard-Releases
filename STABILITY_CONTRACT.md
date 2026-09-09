# DDG Stability Contract

Baseline stabil: TEST 8.5.49-test.104.

Scop: functiile care au fost confirmate ca merg nu se modifica pentru a adauga functii noi. Orice functie noua se scrie separat si comunica cu nucleul prin puncte de integrare explicite.

## Reguli obligatorii

1. Nu se suprascriu functii globale existente din UI (`window.showDetail`, `window.localPreviewHTML`, `window.loadRemotePreview`, `window.remotePreviewError` etc.).
2. Modulele noi folosesc `addEventListener`, elemente proprii, clase/ID-uri proprii si namespace propriu.
3. Un modul LOCAL nu are voie sa modifice REMOTE/MEGA/JDownloader/selectii; un modul provider nu are voie sa modifice LOCAL etc.
4. Modulele noi nu folosesc direct `App.mu` si nu tin mutexul global pentru I/O, log, preview sau retea. Starea noua primeste mutex/stare proprie.
5. Preview media nou foloseste listener/transport separat cand traficul poate bloca API-ul principal.
6. Nu se fac patch-uri "timeout -> fallback" peste o cale care functioneaza fara dovada din diagnostic.
7. Orice schimbare de nucleu se face numai ca `core-migration`, separat de feature work, cu test de regresie pentru functiile deja stabile.
8. Un feature PR trebuie sa adauge fisiere noi sau sa modifice numai punctul de integrare desemnat. Daca atinge un fisier protejat, CI trebuie sa esueze.

## Zone stabile/protejate

Lista executabila este in `stability/protected_paths.txt`. Ea include nucleul UI, MEGA, JDownloader, state engine si preview LOCAL confirmat in .104.

## Mod de lucru pentru functii noi

- backend: fisier(e) noi cu prefix `feature_` sau `provider_` dedicat, stare/mutex propriu;
- frontend: fisier nou in `web/features/` sau modul JS separat, fara monkey-patching al functiilor globale;
- integrare: doar prin evenimente, API-uri/route dedicate si adaptoare minime;
- test: feature-ul trebuie sa poata fi dezactivat/eliminat fara sa schimbe comportamentul nucleului.

Daca o functie stabila chiar trebuie schimbata, se opreste feature work-ul si se face o migrare de nucleu separata, cu motiv, impact si rollback clar.
