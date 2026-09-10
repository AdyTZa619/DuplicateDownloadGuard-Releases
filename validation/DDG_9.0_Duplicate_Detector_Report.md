# DDG — detector de duplicate, TEST local bazat pe 9.0

Bază: Stable 9.0.0, commit `01ed60d`. Cod verificat: `f720bc2e172d29b68e91a72d405343acbcaafb2d`. Executabil: `DuplicateDownloadGuard_PRO_9.0.1-test.0.exe`.

SHA-256 executabil: `c5ec36564d9e9fc92bdf4500e12f0d7bfd781cea47c67c8d70afac805618cfaf`. Windows x64, Go 1.23.12. Eticheta TEST local a fost schimbată doar în copia pentru build.

**Rezultat pe corpusul reproductibil:** 19/19 cazuri trecute: 3 duplicate exacte, 13 variante ale aceluiași conținut și 3 fișiere diferite. Baseline: 18/19; crop-ul video minor era ratat. Niciunul dintre cele trei exemple negative valide nu a fost declarat duplicat.

Fișierele sunt sintetice, codate și transformate efectiv cu FFmpeg 6.1.1. Rezultatele nu sunt o rată de precizie pentru colecții reale. Sursa vizuală generată are 320×180; fișierele video de bază sunt codate la 1920×1080. Nu pretind că această sursă oferă detaliul unui material filmat nativ la 1080p.

**Măsurători totale pentru cele 19 cazuri** (aceleași intrări; fără pauze între teste incluse):

| Măsură | 9.0.0 | Detector TEST |
|---|---:|---:|
| Indexare inițială — metadate filesystem | 84.122 ms | 241.814 ms |
| Comparații la prima rulare | 204.224 s | 138.223 s |
| Comparații repetate | 130.302 s | 0.922 s |
| Payload HTTP — prima rulare | 126,382,440 bytes | 8,305,984 bytes |
| Payload HTTP — repetare | 126,662,594 bytes | 1,847,083 bytes |

Reducerea timpului total de repetare: `1 − 921.931/130301.727 = 99.29%`. Payload-ul HTTP reprezintă octeții de corp scriși de serverul de test, fără antete/TCP. Citirea integrală pentru cele trei fișiere mici identice rămâne necesară confirmării SHA-256 și se repetă.

Cache remote video: 12/12 verificări video repetate au reutilizat amprenta după revalidarea antetelor HTTP. Pentru acestea, payload-ul video la repetare este zero; durata observată este 4.895–48.377 ms.

În prima rulare au fost calculate amprente pentru 16 fișiere media locale (12 verificări video și 4 de imagine); la repetare, 0. Acest contor nu include hashingul complet al celor 3 duplicate exacte.

**Cazuri individuale** — scorul media este un scor calculat de similaritate; nu este probabilitate statistică. Scorul de clasificare este limitat la 99 pentru rezultate non-exacte.

| Caz | Baseline | TEST | Scor media TEST | Prima rulare TEST | Repetare TEST |
|---|---|---|---:|---:|---:|
| A identical | trecut | trecut | SHA-256 | 48.9 ms | 14.2 ms |
| B renamed | trecut | trecut | SHA-256 | 9.4 ms | 9.9 ms |
| C moved | trecut | trecut | SHA-256 | 10.4 ms | 7.8 ms |
| D remux | trecut | trecut | 100 | 9229.3 ms | 10.6 ms |
| E H264-H265 | trecut | trecut | 100 | 10616.6 ms | 5.3 ms |
| F 1080-720 | trecut | trecut | 100 | 7918.2 ms | 4.9 ms |
| G bitrate | trecut | trecut | 99 | 9519.0 ms | 8.5 ms |
| H metadata | trecut | trecut | 100 | 9590.4 ms | 11.3 ms |
| I trim-2s | trecut | trecut | 93 | 26807.2 ms | 13.7 ms |
| J watermark | trecut | trecut | 99 | 15253.9 ms | 48.4 ms |
| K similar but different | trecut | trecut | — | 9985.0 ms | 6.8 ms |
| L JPEG recompression | trecut | trecut | 99 | 125.0 ms | 75.0 ms |
| M image resize | trecut | trecut | 99 | 45.8 ms | 10.8 ms |
| N different same duration | trecut | trecut | — | 9362.1 ms | 8.4 ms |
| O same name different content | trecut | trecut | — | 9238.7 ms | 10.2 ms |
| P minor crop | ratat | trecut | 93 | 9896.5 ms | 8.3 ms |
| Q brightness | trecut | trecut | 99 | 9792.4 ms | 6.9 ms |
| R WebP conversion | trecut | trecut | 98 | 697.3 ms | 635.8 ms |
| S image crop | trecut | trecut | 95 | 77.2 ms | 25.2 ms |

**Audit și modificări:**

- Detectorul vechi avea SHA-256/MD5, eșantionare HTTP, șapte cadre dHash în Smart Guard, dar trei cadre în verificarea vizuală manuală. Aceste trasee folosesc acum aceeași logică.
- Hash-ul local este revalidat prin fișier deschis, dimensiune, mtime și identitate filesystem. Înlocuirea unui fișier și schimbarea sa în timpul hashingului nu pot reutiliza digestul vechi.
- Semnătura media combină dHash, pHash DCT și corelație structurală: ponderi 20%, 30%, 50%, cu praguri de informație vizuală, durată și aliniere temporală. Amprentele de aliniere sunt cache-uite.
- Candidații exacți folosesc bucket-uri de dimensiune; video folosește durate sortate și benzi pHash, imaginile benzi pHash. Numele este un semnal auxiliar. Bugetul video este de maximum 12 candidați pentru etapa finală; rezultatele neterminate rămân marcate pentru verificare.
- Indexurile de benzi păstrează referințe numerice la fișiere, nu câte o copie a tuturor metadatelor pentru fiecare bandă.
- Citirea FFmpeg remote trece printr-un reader separat cu blocuri de 256 KiB și plafon de 16 MiB/analiză video. Răspunsurile Range ignorate/invalide și schimbarea validatorului sunt refuzate. Nu s-au modificat implementările preview MEGA.
- Cache-urile video/imagine ale TEST-ului au fișiere separate de cele Stable. Cache-ul remote video persistă numai cu validator HTTP și este revalidat înaintea reutilizării. Semnăturile fără validator nu sunt reutilizate persistent.
- Raportul și detaliile rezultatului includ clasificarea, scorul, motivele, datele online/local și indiciul tehnic de calitate. Rezoluția/bitrate-ul sunt indicii tehnice, nu o evaluare vizuală garantată.
- Nu am identificat în sursa auditată un modul separat numit «Still All Dupp». Am modificat componentele reale existente, fără a pretinde o integrare care nu există.

**Benchmark separat: 10.000 intrări în memorie**, trei iterații, Intel Xeon Platinum 8370C. Nu măsoară viteza HDD-ului utilizatorului. Snapshotul nou este construit o dată pe preflight; comparația veche reprezintă scanarea/rankarea candidaților pentru o singură intrare remote. Rezultatele nu sunt un benchmark al tuturor etapelor pe 10.000 fișiere reale.

```text
goos: linux
goarch: amd64
pkg: ddgpro
cpu: Intel(R) Xeon(R) Platinum 8370C CPU @ 2.80GHz
BenchmarkDetectorCandidateSnapshotV90-9   	       3	  24021186 ns/op	 8252757 B/op	      36 allocs/op
BenchmarkLegacyMediaCandidateScanV90-9    	       3	 123171428 ns/op	10309824 B/op	  459979 allocs/op
PASS
ok  	ddgpro	0.690s
```

**Verificări efectuate:** `go test ./...`, teste de concurență `-race` pe detector/cache/candidați, `go vet` pentru Windows, build Windows x64, sintaxa JavaScript și 8 teste UI Node. Endpointul de status răspunde în timp ce preflightul așteaptă date remote. Teste de reload persistent, fișier schimbat/înlocuit, candidat redenumit în afara shortlistului vechi și limită Range/buget.

**Limite de validare:** executabilul nu a fost rulat într-o sesiune Windows reală; integrarea vizuală completă, HDD-urile utilizatorului, contul MEGA și JDownloader real nu au fost validate în această sesiune. CI Windows/publicarea nu au putut fi pornite deoarece controlul automat de aprobare a respins push-ul. Nu prezint build-ul drept validare completă a aplicației.

Media Picker, MEGA preview, lifecycle și JDownloader nu au diferențe față de baza 9.0 în fișierele lor. Singura modificare în HTML-ul general încarcă modulul de dovezi pentru detector.

**Reproducere:**

```bash
python3 validation/generate_duplicate_corpus.py /tmp/ddg-corpus
DDG_MEDIA_CORPUS=/tmp/ddg-corpus DDG_CORPUS_STRICT=1 go test -run TestDuplicateCorpusV90 -v
go test ./...
go test -race -run "TestDetector|TestCachedRenamed|TestUnexamined|TestDigest|TestRemoteFingerprint"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
```

Două fișiere de test negative au fost constatate invalide în prima pregătire. Au fost regenerate și validate cu ffprobe; cazurile N/O au fost remăsurate și în baseline cu executabilul original de test. Tabelul de mai sus folosește numai aceste intrări valide. Datele sintetice și generarea lor nu provin din colecția utilizatorului.
