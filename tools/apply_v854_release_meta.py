from pathlib import Path

version = "8.5.4"
changelog = Path("CHANGELOG.md")
text = changelog.read_text(encoding="utf-8")
if "## [8.5.4]" in text:
    raise SystemExit("8.5.4 already present")
marker = "## [8.5.3]"
if marker not in text:
    raise SystemExit("8.5.3 marker missing")
section = '''## [8.5.4] — 2026-09-04

### Download Studio — flux refăcut
- Butonul principal este simplificat la `⬇ Descarcă selecția`; Smart Guard rulează automat în fundal înainte de coadă, fără a transforma acțiunea de download într-un flux greu de urmărit.
- Folderul și motorul alese în interfață sunt folosite imediat; nu mai este necesar să apeși mai întâi „Salvează regulile”.
- Dacă nu este ales niciun folder, aplicația folosește automat folderul portabil `downloads` și îl afișează corect în Download Studio.
- După adăugarea reușită, interfața trece direct la coadă și arată ce motor a fost ales și de ce; dacă un fișier este respins, motivul este afișat imediat.

### Alegerea motorului
- Modul `Auto` este acum determinist: MEGA → MEGAcmd; pagini/streamuri yt-dlp → yt-dlp; URL-uri HTTP directe și media extrasă din galerii → downloaderul HTTP intern cu resume.
- `aria2` rămâne disponibil ca alegere explicită de performanță, dar nu mai este ales automat doar pentru că este instalat.
- HLS/DASH (`.m3u8` / `.mpd`) nu mai poate ajunge accidental la aria2/downloaderul intern ca fișier simplu; este trimis la yt-dlp sau utilizatorul primește o eroare clară că yt-dlp este necesar.
- yt-dlp primește URL-ul stabil al paginii atunci când există, nu URL-ul CDN temporar descoperit la scanare.

### Coada persistentă
- Fiecare job nou salvează snapshotul complet al sursei remote: URL pagină, URL direct, handle MEGA, ProviderID, hash, tip media și metadatele necesare reluării.
- Un job poate fi reluat după restart sau după o nouă scanare fără să depindă de existența vechiului rând din tabelul de rezultate.
- `ResultID` nu mai este folosit ca identitate a fișierului pentru reluare, blocarea duplicatelor sau reutilizarea verdictului Guard; identitatea remote stabilă are prioritate.
- Joburile vechi care nu conțin suficiente date pentru a reconstrui sigur o sursă complexă sunt oprite cu explicație și acțiune clară, în loc să descarce un URL presupus.

### HTTP / galerii
- Downloaderul intern trimite User-Agent de browser și `Referer` pentru media extrasă din galerii/pagini, reducând erorile HTTP 403 de tip hotlink protection.
- aria2 primește același User-Agent și Referer atunci când este ales explicit.
- Resume `.part` este verificat prin `Range`; dacă serverul ignoră Range, transferul reîncepe curat de la zero în loc să concateneze date și să corupă fișierul.
- Coliziunile de nume nu mai fac downloadul să eșueze la final: se folosesc automat nume de forma `fișier (1).ext`.
- Fișierul final este sincronizat și verificat înainte de rename, iar transferurile incomplete rămân erori explicite.

### Teste de regresie
- Teste pentru selecția motorului Auto, protecția HLS/DASH, MEGA fără fallback HTTP, Referer obligatoriu, resume Range, server care ignoră Range, coliziuni de nume și snapshot persistent.
- Teste dedicate care demonstrează că reutilizarea aceluiași `ResultID` pentru alt fișier nu poate muta un job sau un verdict Guard pe sursa greșită.

'''
changelog.write_text(text.replace(marker, section + marker, 1), encoding="utf-8")
Path("VERSION").write_text(version + "\n", encoding="utf-8")
Path("tools/apply_v854_release_meta.py").unlink(missing_ok=True)
Path(".github/workflows/apply-v854-release-meta.yml").unlink(missing_ok=True)
print("v8.5.4 release metadata applied")
