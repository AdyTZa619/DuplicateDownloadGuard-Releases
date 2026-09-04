//go:build windows

package main

import (
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

func matchingDDGAppWindows() []uintptr {
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
		if isDDGAppWindowTitle(syscall.UTF16ToString(buf)) {
			windows = append(windows, hwnd)
		}
		return 1
	})
	_, _, _ = uiEnumWindows.Call(callback, 0)
	return windows
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
