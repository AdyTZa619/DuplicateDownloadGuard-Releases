//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	th32csSnapProcessV85122              = 0x00000002
	processTerminateV85122               = 0x0001
	processQueryLimitedInformationV85122 = 0x1000
)

type processEntry32V85122 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

var kernel32LifecycleV85122 = syscall.NewLazyDLL("kernel32.dll")
var createToolhelp32SnapshotV85122 = kernel32LifecycleV85122.NewProc("CreateToolhelp32Snapshot")
var process32FirstWV85122 = kernel32LifecycleV85122.NewProc("Process32FirstW")
var process32NextWV85122 = kernel32LifecycleV85122.NewProc("Process32NextW")
var queryFullProcessImageNameWV85122 = kernel32LifecycleV85122.NewProc("QueryFullProcessImageNameW")

// ddgNativeWindowDefinitelyClosedV85122 is the fast path for a real click on X.
// A renderer reload keeps the latched top-level HWND valid. If Edge genuinely
// recreates the top-level window, a tolerant title scan latches the replacement
// before we report a close.
func ddgNativeWindowDefinitelyClosedV85122() bool {
	ddgPresenceWindowV85117.mu.Lock()
	defer ddgPresenceWindowV85117.mu.Unlock()

	old := ddgPresenceWindowV85117.hwnd
	if old == 0 {
		return false
	}
	if ddgNativeWindowHandleStillValidV85117(old) {
		return false
	}
	windows := matchingDDGPresenceWindowsV85117()
	if len(windows) > 0 {
		ddgPresenceWindowV85117.hwnd = windows[0]
		return false
	}
	return true
}

func fullProcessImageNameV85122(pid uint32) string {
	if pid == 0 || int(pid) == os.Getpid() {
		return ""
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformationV85122, false, pid)
	if err != nil || h == 0 {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 32768)
	sz := uint32(len(buf))
	ok, _, _ := queryFullProcessImageNameWV85122.Call(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&sz)),
	)
	if ok == 0 || sz == 0 || int(sz) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:sz])
}

// stopRecoveryHelpersForAppDirV85122 is called only during a graceful DDG
// shutdown. Crash recovery stays available after an abnormal backend death,
// but clicking X must not leave DuplicateDownloadGuard.recovery_*.exe alive.
// We terminate only helpers whose executable is inside THIS installation's
// data\\updates directory, so another portable DDG copy is never touched.
func stopRecoveryHelpersForAppDirV85122(appDir string) {
	updatesDir := filepath.Clean(filepath.Join(appDir, "updates"))
	snap, _, _ := createToolhelp32SnapshotV85122.Call(th32csSnapProcessV85122, 0)
	if snap == 0 || snap == ^uintptr(0) {
		return
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	var entry processEntry32V85122
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, _ := process32FirstWV85122.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
		if strings.HasPrefix(name, "duplicatedownloadguard.recovery_") && strings.HasSuffix(name, ".exe") {
			full := fullProcessImageNameV85122(entry.ProcessID)
			if full != "" && strings.EqualFold(filepath.Clean(filepath.Dir(full)), updatesDir) {
				_ = terminateProcessTree(int(entry.ProcessID))
				_ = waitForProcessExit(int(entry.ProcessID), 1500*1000000)
				_ = os.Remove(full)
			}
		}
		entry = processEntry32V85122{Size: uint32(unsafe.Sizeof(entry))}
		ok, _, _ = process32NextWV85122.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
}
