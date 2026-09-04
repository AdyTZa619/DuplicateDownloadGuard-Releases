$ErrorActionPreference = 'Stop'

function Replace-Once([string]$text, [string]$old, [string]$new, [string]$label) {
  $first = $text.IndexOf($old, [System.StringComparison]::Ordinal)
  if ($first -lt 0) { throw "Pattern lipsă: $label" }
  if ($text.IndexOf($old, $first + $old.Length, [System.StringComparison]::Ordinal) -ge 0) { throw "Pattern duplicat: $label" }
  return $text.Substring(0, $first) + $new + $text.Substring($first + $old.Length)
}

$mainPath = 'main.go'
$main = Get-Content -Raw $mainPath
$main = Replace-Once $main "`ta.keepMegaSessionWarm(exe, link, oldSession)" @'
	if err := a.prepareMegaWarmRootAfterScanV86(ctx, exe, link, oldSession); err != nil {
		a.logf("MEGA Fast Preview: WebDAV root nu a putut fi pregătit (%v); păstrez fallback-ul existent", err)
		a.keepMegaSessionWarm(exe, link, oldSession)
	}
'@ 'warm-root after scan'

$needle = @'
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == remoteRef && a.preview.StreamURL != "" {
'@
$replacement = @'
	// Fast path: the scan already serves the whole public folder through one
	// warm WebDAV root. Derive the selected child URL locally and avoid a new
	// MEGAcmd webdav command for every row click.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == megaWarmRootRefV86 && a.preview.StreamURL != "" {
		if streamURL, ok := warmRootPreviewURLV86(a.preview, item); ok {
			a.resetPreviewTTLLocked()
			a.logf("MEGA Fast Preview hit: %s -> %s", item.Path, streamURL)
			return streamURL, nil
		}
		a.logf("MEGA Fast Preview miss pentru %s; folosesc WebDAV per fișier", item.Path)
	}

	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == remoteRef && a.preview.StreamURL != "" {
'@
$main = Replace-Once $main $needle $replacement 'warm-root fast path'
[System.IO.File]::WriteAllText((Resolve-Path $mainPath), $main, [System.Text.UTF8Encoding]::new($false))

gofmt -w main.go
if ($LASTEXITCODE -ne 0) { throw 'gofmt main.go failed' }

$indexPath = 'web/index.html'
$index = Get-Content -Raw $indexPath
$oldTag = '<script defer src="/exact_guard.js"></script><script>'
$newTag = '<script defer src="/exact_guard.js"></script><script defer src="/preview_quick_v86.js"></script><script>'
$index = Replace-Once $index $oldTag $newTag 'preview quick script tag'
[System.IO.File]::WriteAllText((Resolve-Path $indexPath), $index, [System.Text.UTF8Encoding]::new($false))

Write-Host 'v8.6 preview integration patch applied.'
