package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// main.go still has a few process-level log.Fatal exits around listener setup
// and Serve failures. Keep stderr behavior, but mirror those fatal messages to
// a persistent portable file so the next OFFLINE incident has an exact cause.
func init() {
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	appDir, err := portableDataDir()
	if err != nil {
		return
	}
	path := filepath.Join(appDir, "backend_fatal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}
