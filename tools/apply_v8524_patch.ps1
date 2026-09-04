$ErrorActionPreference = 'Stop'
$path = 'main.go'
$src = Get-Content $path -Raw

$routeNeedle = 'mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)'
$routeReplacement = @'
mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)
	mux.HandleFunc("/api/remote-preview/media", a.handleMegaAsyncPreviewMediaV8524)
'@
if (-not $src.Contains($routeNeedle)) { throw 'remote-preview start route not found' }
$src = $src.Replace($routeNeedle, $routeReplacement.TrimEnd())

$old = @'
	if kind == "other" {
		http.Error(w, "Formatul nu are preview media integrat", 415)
		return
	}
	streamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote, req.ForceFallback)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{
		"url":         streamURL,
		"kind":        kind,
		"streaming":   true,
		"source":      previewMode,
		"previewMode": previewMode,
		"prepareMs":   prepareDuration.Milliseconds(),
		"note":        "Fast-path-ul UI reutilizează WebDAV-ul pregătit la scanare fără comandă MEGAcmd suplimentară. Fallback-ul per-fișier rămâne disponibil dacă nu există cache.",
	})
'@
$new = @'
	if kind == "other" {
		http.Error(w, "Formatul nu are preview media integrat", 415)
		return
	}
	job, localURL, err := a.beginMegaAsyncPreviewV8524(res.Remote, req.ForceFallback)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{
		"url":         localURL,
		"kind":        kind,
		"streaming":   true,
		"source":      "MEGA ASYNC LOCAL",
		"previewMode": "MEGA ASYNC LOCAL",
		"prepareMs":   0,
		"generation":  job.generation,
		"note":        "Playerul primește imediat URL-ul local. Pregătirea MEGA rulează separat, serializat și anulabil; Range este transmis către upstream.",
	})
'@
if (-not $src.Contains($old)) { throw 'blocking MEGA UI handler block not found' }
$src = $src.Replace($old, $new)

$stopNeedle = "func (a *App) handleRemotePreviewStop(w http.ResponseWriter, r *http.Request) {`n"
$stopReplacement = "func (a *App) handleRemotePreviewStop(w http.ResponseWriter, r *http.Request) {`n`tcancelMegaAsyncPreviewV8524()`n"
if (-not $src.Contains($stopNeedle)) { throw 'remote preview stop handler not found' }
$src = $src.Replace($stopNeedle, $stopReplacement)

Set-Content -Path $path -Value $src -Encoding utf8 -NoNewline
Write-Host 'Applied v8.5.24 async preview patch to main.go'
