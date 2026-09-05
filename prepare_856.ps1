$ErrorActionPreference = 'Stop'

$version = '8.5.36'
$changelogPath = 'CHANGELOG.md'
$mainPath = 'main.go'

$changelog = Get-Content -Raw $changelogPath
if (-not $changelog.Contains("## [$version]")) {
    $marker = '## [8.5.35]'
    $idx = $changelog.IndexOf($marker)
    if ($idx -lt 0) { throw 'Nu am găsit secțiunea 8.5.35 în CHANGELOG.md' }
    $section = @"
## [8.5.36] — 2026-09-06

### Sursă online — buton și feedback imediat
- Butonul „Analizează fără download” este legat direct de modulul provider, fără dependență de handlerul inline legacy; click-ul GoFile/Bunkr/Cyberdrop nu mai poate rămâne aparent inert.
- La pornirea scanării, butonul se dezactivează și afișează imediat providerul analizat; Enter în câmpul URL pornește aceeași rută, iar scanările paralele sunt blocate.
- Traseul MEGA dedicat și semantica de matching rămân neschimbate.

### Updater — reconectare și curățare automată
- Verificarea statusului și a update-ului reîncearcă automat conexiunea locală de până la trei ori, fără cache, înainte să raporteze indisponibilitatea; reduce cazul „Failed to fetch” care dispărea doar după restartul DDG.
- După un update confirmat sănătos se păstrează maximum un singur backup EXE pentru rollback; backup-urile mai vechi sunt eliminate înainte de următoarea instalare.
- Helper-ele `DuplicateDownloadGuard.Updater_*.exe`, `DuplicateDownloadGuard.pending.exe`, `apply_update.json` și temporarele `.download`, `.copying`, `.replacing` sunt curățate automat după health-check.
- Curățarea finală a helperului rulează numai după ce procesul updaterului s-a închis, evitând blocarea executabilului de către Windows; path guard-ul împiedică ștergerea în afara folderului de update.
- Testele de regresie acoperă reconnect-ul updaterului, retenția unui singur backup, protejarea fișierelor active și curățarea artefactelor vechi.

"@
    $changelog = $changelog.Substring(0, $idx) + $section + $changelog.Substring($idx)
    [System.IO.File]::WriteAllText((Resolve-Path $changelogPath), $changelog, [System.Text.UTF8Encoding]::new($false))
}

$main = Get-Content -Raw $mainPath
$pattern = 'const appVersion = "[^"]+"'
if (-not [regex]::IsMatch($main, $pattern)) { throw 'Nu am găsit appVersion în main.go' }
$main = [regex]::Replace($main, $pattern, 'const appVersion = "8.5.36 Pro Smart Media Guard"', 1)
[System.IO.File]::WriteAllText((Resolve-Path $mainPath), $main, [System.Text.UTF8Encoding]::new($false))

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git rm .github/workflows/prepare-856.yml prepare_856.ps1
git add VERSION CHANGELOG.md main.go
git commit -m 'Prepare 8.5.36 release'
git push
