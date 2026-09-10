//go:build !windows && !linux

package main

import "os"

// Unsupported file identities disable digest reuse, never validation.
func hashFileIdentityV90(_ *os.File, _ os.FileInfo) (string, bool) { return "", false }
