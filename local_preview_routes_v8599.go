package main

import "net/http"

// registerLocalPreviewDiagnosticsV8599 replaces only the local-preview route in
// TEST builds. The MEGA/provider/JDownloader routes remain untouched.
func registerLocalPreviewDiagnosticsV8599(mux *http.ServeMux, a *App) {
	mux.HandleFunc("/api/local-preview", a.handleLocalPreviewDiagV8599)
	mux.HandleFunc("/api/local-preview-buffered", a.handleLocalPreviewBufferedV8599)
	mux.HandleFunc("/api/local-preview/trace", a.handleLocalPreviewClientTraceV8599)
}
