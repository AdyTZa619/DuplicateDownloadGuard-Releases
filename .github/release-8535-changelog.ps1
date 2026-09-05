$ErrorActionPreference = 'Stop'
$path = 'CHANGELOG.md'
$text = (Get-Content -Raw $path).Replace("`r`n", "`n")
if ($text.Contains('## [8.5.35]')) {
    Write-Host '8.5.35 already present.'
    exit 0
}
$marker = '## [8.5.34]'
$idx = $text.IndexOf($marker)
if ($idx -lt 0) { throw 'Nu am găsit secțiunea 8.5.34.' }
$section = @'
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

'@
$text = $text.Insert($idx, $section.Replace("`r`n", "`n"))
[System.IO.File]::WriteAllText((Resolve-Path $path), $text, [System.Text.UTF8Encoding]::new($false))
