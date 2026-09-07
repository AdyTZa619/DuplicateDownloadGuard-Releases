//go:build !windows

package main

func closeDDGAppWindowsNative() int { return 0 }

// DDG is a Windows app. On non-Windows builds/tests there is no native Edge
// app window to inspect, so fail safe and never let the watchdog infer that the
// UI vanished from native-window state alone.
func ddgAppWindowPresentNative() bool { return true }
