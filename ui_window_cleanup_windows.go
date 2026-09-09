//go:build windows

package main

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const wmClose = 0x0010

var uiUser32 = syscall.NewLazyDLL("user32.dll")
var uiEnumWindows = uiUser32.NewProc("EnumWindows")
var uiGetWindowTextLengthW = uiUser32.NewProc("GetWindowTextLengthW")
var uiGetWindowTextW = uiUser32.NewProc("GetWindowTextW")
var uiPostMessageW = uiUser32.NewProc("PostMessageW")
var uiIsWindow = uiUser32.NewProc("IsWindow")
var uiShowWindow = uiUser32.NewProc("ShowWindow")

var ddgPresenceWindowV85117 struct {
	mu    sync.Mutex
	state ddgWindowLatchStateV85133
}

func enumerateDDGWindows(match func(string) bool) []uintptr {
	windows := make([]uintptr, 0, 2)
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		length, _, _ := uiGetWindowTextLengthW.Call(hwnd)
		if length == 0 {
			return 1
		}
		buf := make([]uint16, int(length)+1)
		_, _, _ = uiGetWindowTextW.Call(
			hwnd,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if match(syscall.UTF16ToString(buf)) {
			windows = append(windows, hwnd)
		}
		return 1
	})
	_, _, _ = uiEnumWindows.Call(callback, 0)
	return windows
}

func matchingDDGAppWindows() []uintptr {
	return enumerateDDGWindows(isDDGAppWindowTitle)
}

func matchingDDGPresenceWindowsV85117() []uintptr {
	return enumerateDDGWindows(isDDGAppWindowPresenceTitle)
}

func ddgNativeWindowHandleStillValidV85117(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	ok, _, _ := uiIsWindow.Call(hwnd)
	return ok != 0
}

// ddgAppWindowPresentNative is used only for backend lifetime decisions. Once
// the real DDG HWND has been seen, keep that handle latched and trust IsWindow
// instead of requiring Edge to keep an exact title every second. This survives
// minimize/suspend/renderer-title changes. If Edge genuinely recreates the
// top-level window, the tolerant title scan discovers and latches the new HWND.
func ddgAppWindowPresentNative() bool {
	ddgPresenceWindowV85117.mu.Lock()
	defer ddgPresenceWindowV85117.mu.Unlock()

	currentValid := ddgNativeWindowHandleStillValidV85117(ddgPresenceWindowV85117.state.hwnd)
	var replacement uintptr
	if !currentValid {
		windows := matchingDDGPresenceWindowsV85117()
		if len(windows) > 0 {
			replacement = windows[0]
		}
	}
	return ddgPresenceWindowV85117.state.observe(currentValid, replacement)
}

func closeDDGAppWindowsNative() int {
	windows := matchingDDGAppWindows()
	for _, hwnd := range windows {
		_, _, _ = uiPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
	if len(windows) == 0 {
		return 0
	}

	// Give Edge a short bounded interval to process WM_CLOSE. main() has not
	// opened the new DDG window yet, so every exact-title match in this period
	// belongs to the previous UI instance.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(matchingDDGAppWindows()) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return len(windows)
}

// closeDDGPresenceWindowsForHandoffNative is deliberately used only when
// apply_update.json proves that an updater handoff is in progress. The old Edge
// --app window may have a decorated title and its localhost backend may already
// be gone, so the strict normal-start matcher is not sufficient here. Close all
// tolerant DDG matches before the new build creates its own window. If Edge
// ignores WM_CLOSE, hide the stale shell so users never land on an OFFLINE old
// UI while the healthy updated instance starts.
func closeDDGPresenceWindowsForHandoffNative() int {
	windows := matchingDDGPresenceWindowsV85117()
	if len(windows) == 0 {
		return 0
	}
	for _, hwnd := range windows {
		_, _, _ = uiPostMessageW.Call(hwnd, wmClose, 0, 0)
	}

	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(matchingDDGPresenceWindowsV85117()) == 0 {
			return len(windows)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// SW_HIDE is a bounded last resort for the browser shell only; do not kill
	// Edge because it can host unrelated normal browser windows/processes.
	const swHide = 0
	for _, hwnd := range matchingDDGPresenceWindowsV85117() {
		_, _, _ = uiShowWindow.Call(hwnd, swHide)
	}
	return len(windows)
}
