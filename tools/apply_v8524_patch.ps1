$ErrorActionPreference = 'Stop'
$path = 'main.go'
$src = Get-Content $path -Raw
$src = $src -replace "`r`n", "`n"

function Replace-Required([string]$needle, [string]$replacement, [string]$label) {
  if (-not $script:src.Contains($needle)) { throw "$label not found" }
  $script:src = $script:src.Replace($needle, $replacement)
}

Replace-Required 'mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)' "mux.HandleFunc(`"/api/remote-preview/start`", a.handleRemotePreviewStart)`n`tmux.HandleFunc(`"/api/remote-preview/media`", a.handleMegaAsyncPreviewMediaV8524)" 'remote preview route'

Replace-Required 'streamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote, req.ForceFallback)' 'job, localURL, err := a.beginMegaAsyncPreviewV8524(res.Remote, req.ForceFallback)' 'blocking MEGA start call'
Replace-Required '"url":         streamURL,' '"url":         localURL,' 'MEGA response url'
Replace-Required '"source":      previewMode,' '"source":      "MEGA ASYNC LOCAL",' 'MEGA response source'
Replace-Required '"previewMode": previewMode,' '"previewMode": "MEGA ASYNC LOCAL",' 'MEGA response mode'
Replace-Required '"prepareMs":   prepareDuration.Milliseconds(),' "`"prepareMs`":   0,`n`t`t`"generation`":  job.generation," 'MEGA response prepareMs'
Replace-Required '"note":        "Fast-path-ul UI reutilizează WebDAV-ul pregătit la scanare fără comandă MEGAcmd suplimentară. Fallback-ul per-fișier rămâne disponibil dacă nu există cache.",' '"note":        "Playerul primește imediat URL-ul local. Pregătirea MEGA rulează separat, serializat și anulabil; Range este transmis către upstream.",' 'MEGA response note'

Replace-Required "func (a *App) handleRemotePreviewStop(w http.ResponseWriter, r *http.Request) {`n" "func (a *App) handleRemotePreviewStop(w http.ResponseWriter, r *http.Request) {`n`tcancelMegaAsyncPreviewV8524()`n" 'remote preview stop handler'

Set-Content -Path $path -Value $src -Encoding utf8 -NoNewline
Write-Host 'Applied v8.5.24 async preview patch to main.go'
