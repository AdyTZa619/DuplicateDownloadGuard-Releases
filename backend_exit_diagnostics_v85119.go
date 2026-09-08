package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var backendExitDiagMuV85119 sync.Mutex

type backendExitDiagnosticV85119 struct {
	At                 string `json:"at"`
	Version            string `json:"version"`
	Reason             string `json:"reason"`
	PID                int    `json:"pid"`
	HeartbeatAgeMS     int64  `json:"heartbeatAgeMs,omitempty"`
	ExitHintAgeMS      int64  `json:"exitHintAgeMs,omitempty"`
	MissingWindowTicks int    `json:"missingWindowTicks,omitempty"`
	WindowPresent      bool   `json:"windowPresent"`
}

func writeBackendExitDiagnosticV85119(reason string, now time.Time, heartbeatNS, hintNS int64, windowPresent bool, missingTicks int) {
	appDir, err := portableDataDir()
	if err != nil {
		return
	}
	d := backendExitDiagnosticV85119{
		At:                 now.Format(time.RFC3339Nano),
		Version:            appVersion,
		Reason:             reason,
		PID:                os.Getpid(),
		MissingWindowTicks: missingTicks,
		WindowPresent:      windowPresent,
	}
	if heartbeatNS > 0 {
		d.HeartbeatAgeMS = now.Sub(time.Unix(0, heartbeatNS)).Milliseconds()
	}
	if hintNS > 0 {
		d.ExitHintAgeMS = now.Sub(time.Unix(0, hintNS)).Milliseconds()
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return
	}
	backendExitDiagMuV85119.Lock()
	defer backendExitDiagMuV85119.Unlock()
	path := filepath.Join(appDir, "backend_last_exit.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return
	}
	_ = os.Remove(path)
	_ = os.Rename(tmp, path)
}
