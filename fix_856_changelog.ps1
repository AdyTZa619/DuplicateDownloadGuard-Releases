$ErrorActionPreference = 'Stop'
$path = 'CHANGELOG.md'
$lines = Get-Content $path
$replacement = '- Helper-ele DuplicateDownloadGuard.Updater_*.exe, DuplicateDownloadGuard.pending.exe, apply_update.json și temporarele .download, .copying, .replacing sunt curățate automat după health-check.'
$found = $false
for ($i = 0; $i -lt $lines.Count; $i++) {
    if ($lines[$i].StartsWith('- Helper-ele ')) {
        $lines[$i] = $replacement
        $found = $true
        break
    }
}
if (-not $found) { throw 'Linia Helper-ele nu a fost găsită' }
[System.IO.File]::WriteAllLines((Resolve-Path $path), $lines, [System.Text.UTF8Encoding]::new($false))

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git rm .github/workflows/fix-856-changelog.yml fix_856_changelog.ps1
git add CHANGELOG.md
git commit -m 'Fix 8.5.36 changelog formatting'
git push
