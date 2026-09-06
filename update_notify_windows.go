//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"syscall"
	"time"
)

var (
	kernel32UpdateNotifyV8554 = syscall.NewLazyDLL("kernel32.dll")
	procBeepUpdateNotifyV8554 = kernel32UpdateNotifyV8554.NewProc("Beep")
	user32UpdateNotifyV8554   = syscall.NewLazyDLL("user32.dll")
	procMessageBeepV8554      = user32UpdateNotifyV8554.NewProc("MessageBeep")
)

func playNativeUpdateChimeV8554() bool {
	first, _, _ := procBeepUpdateNotifyV8554.Call(uintptr(880), uintptr(170))
	time.Sleep(45 * time.Millisecond)
	second, _, _ := procBeepUpdateNotifyV8554.Call(uintptr(1175), uintptr(230))
	if first != 0 || second != 0 {
		return true
	}
	fallback, _, _ := procMessageBeepV8554.Call(uintptr(0x40))
	return fallback != 0
}

func (a *App) handleUpdateNativeNotifyV8554(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	version := extractNativeUpdateVersionV8566(strings.TrimSpace(request.Label) + " " + strings.TrimSpace(request.Key))
	played, skipped := notifyNativeUpdateVersionV8566(version)
	if version == "" {
		// Compatibility fallback for a malformed/legacy caller. New DDG builds
		// always send a version in the chip label, but keep the endpoint usable.
		played = playNativeUpdateChimeV8554()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      played || skipped,
		"native":  true,
		"played":  played,
		"skipped": skipped,
		"version": version,
	})
}
