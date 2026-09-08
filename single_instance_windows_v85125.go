//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const processQueryLimitedInformation = 0x1000

var (
	ddgSingleInstanceMutexV85125 syscall.Handle
	ddgKernel32V85125            = syscall.NewLazyDLL("kernel32.dll")
	ddgCreateMutexWV85125        = ddgKernel32V85125.NewProc("CreateMutexW")
	ddgQueryFullImageNameWV85125 = ddgKernel32V85125.NewProc("QueryFullProcessImageNameW")
	ddgShowWindowV85125          = uiUser32.NewProc("ShowWindow")
	ddgSetForegroundV85125       = uiUser32.NewProc("SetForegroundWindow")
)

// ddgSingleInstanceMutexNameV85126 is scoped to the exact installation path.
// A portable copy in another folder is an independent installation and must not
// block this one. It also prevents the old TEST125 global mutex from trapping a
// newly installed build during update handoff.
func ddgSingleInstanceMutexNameV85126() string {
	current, err := os.Executable()
	if err != nil || strings.TrimSpace(current) == "" {
		return `Local\DuplicateDownloadGuard_PRO_Main_v85126_fallback`
	}
	if abs, absErr := filepath.Abs(current); absErr == nil {
		current = abs
	}
	current = strings.ToLower(filepath.Clean(current))
	sum := sha256.Sum256([]byte(current))
	return `Local\DuplicateDownloadGuard_PRO_Main_` + hex.EncodeToString(sum[:12])
}

// claimDDGSingleInstanceNative prevents a second normal DDG launch from
// creating another backend/UI pair for the same portable installation.
// Updater/recovery helper modes never call this function (see ui_window_cleanup.go).
func claimDDGSingleInstanceNative() bool {
	name, err := syscall.UTF16PtrFromString(ddgSingleInstanceMutexNameV85126())
	if err != nil {
		return true // fail open: never brick startup because mutex creation failed
	}
	h, _, callErr := ddgCreateMutexWV85125.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		_ = syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	ddgSingleInstanceMutexV85125 = syscall.Handle(h)
	return true
}

func activateExistingDDGWindowNative() {
	windows := matchingDDGPresenceWindowsV85117()
	if len(windows) == 0 {
		return
	}
	const swRestore = 9
	_, _, _ = ddgShowWindowV85125.Call(windows[0], swRestore)
	_, _, _ = ddgSetForegroundV85125.Call(windows[0])
}

func processImagePathV85125(pid uint32) string {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)

	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	ok, _, _ := ddgQueryFullImageNameWV85125.Call(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 || size == 0 || int(size) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// terminateOtherDDGProcessesSameImageNative is the migration cleanup for
// versions before TEST125, which did not own a reliable per-install mutex. The
// old startup/update path could leave the localhost backend alive. Only
// processes running the exact same executable path are terminated; unrelated
// portable copies elsewhere are left alone.
func terminateOtherDDGProcessesSameImageNative() int {
	current, err := os.Executable()
	if err != nil {
		return 0
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return 0
	}
	current = filepath.Clean(current)
	selfPID := uint32(os.Getpid())

	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(snapshot)

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return 0
	}

	killed := 0
	for {
		pid := entry.ProcessID
		if pid != 0 && pid != selfPID {
			name := syscall.UTF16ToString(entry.ExeFile[:])
			if strings.EqualFold(name, filepath.Base(current)) {
				other := processImagePathV85125(pid)
				if other != "" && strings.EqualFold(filepath.Clean(other), current) {
					if terminateProcessTree(int(pid)) == nil {
						killed++
					}
				}
			}
		}
		if err := syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return killed
}
