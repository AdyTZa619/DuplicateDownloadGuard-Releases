$ErrorActionPreference = 'Stop'
$path = 'main.go'
$text = (Get-Content -Raw $path).Replace("`r`n", "`n")

function Replace-Exact([string]$label, [string]$old, [string]$new) {
    $old = $old.Replace("`r`n", "`n")
    $new = $new.Replace("`r`n", "`n")
    if (-not $script:text.Contains($old)) {
        throw "Nu am găsit blocul pentru $label"
    }
    $script:text = $script:text.Replace($old, $new)
}

$oldMux = @'
	mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)
	mux.HandleFunc("/api/remote-preview/stop", a.handleRemotePreviewStop)
'@
$newMux = @'
	mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)
	mux.HandleFunc("/api/provider-preview/media", a.handleProviderPreviewMediaV86)
	mux.HandleFunc("/api/remote-preview/stop", a.handleRemotePreviewStop)
'@
Replace-Exact 'ruta provider preview' $oldMux $newMux

$oldStart = @'
	if !strings.EqualFold(res.Remote.Source, "MEGA") {
		previewURL := strings.TrimSpace(res.Remote.DirectURL)
		if previewURL == "" {
			previewURL = res.Remote.URL
		}
		pu, err := url.Parse(previewURL)
		if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
			http.Error(w, "Sursa remote nu poate fi previzualizată direct", 400)
			return
		}
		jsonOut(w, map[string]any{"url": previewURL, "kind": kind, "streaming": true, "source": res.Remote.Source})
		return
	}
'@
$newStart = @'
	if !strings.EqualFold(res.Remote.Source, "MEGA") {
		if kind == "other" {
			http.Error(w, "Formatul nu are preview media integrat", 415)
			return
		}
		if _, err := providerPreviewTargetV86(res.Remote); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		previewURL := providerPreviewPathV86(req.ID)
		jsonOut(w, map[string]any{
			"url":         previewURL,
			"kind":        kind,
			"streaming":   true,
			"source":      res.Remote.Source,
			"previewMode": "provider-proxy",
			"note":        "Preview-ul trece prin proxy-ul local DDG pentru a păstra Range și contextul HTTP al providerului fără a expune cookie-uri sau tokenuri în browser.",
		})
		return
	}
'@
Replace-Exact 'preview non-MEGA' $oldStart $newStart

$oldPlayer = @'
	target := strings.TrimSpace(res.Remote.DirectURL)
	if target == "" {
		target = res.Remote.URL
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		var err error
		target, err = a.startMegaPreview(res.Remote)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
'@
$newPlayer = @'
	target := strings.TrimSpace(res.Remote.DirectURL)
	if target == "" {
		target = res.Remote.URL
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		var err error
		target, err = a.startMegaPreview(res.Remote)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		if remoteMediaKind(res.Remote.Name) == "other" {
			http.Error(w, "Formatul nu are preview media integrat", 415)
			return
		}
		if _, err := providerPreviewTargetV86(res.Remote); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		target = providerPreviewURLForRequestV86(r, req.ID)
	}
'@
Replace-Exact 'player non-MEGA' $oldPlayer $newPlayer

[System.IO.File]::WriteAllText((Resolve-Path $path), $text, [System.Text.UTF8Encoding]::new($false))
gofmt -w main.go provider_preview_proxy.go provider_preview_url.go provider_preview_proxy_test.go
