//go:build !windows

package main

import "net/http"

// The released app is Windows-only. This test-only adapter lets the platform-
// independent detector tests run on Linux; it never replaces the real Windows
// updater handler or participates in a production build.
func (a *App) handleUpdateNativeNotifyV8554(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Windows updater is outside this test runtime", http.StatusNotImplemented)
}
