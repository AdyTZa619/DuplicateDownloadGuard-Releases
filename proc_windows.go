//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func detachUpdaterProcess(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP. The updater must survive the
	// application process that launched it and must never steal keyboard focus.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00000200}
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		// ERROR_INVALID_PARAMETER means that the process no longer exists.
		return err == syscall.Errno(87)
	}
	defer syscall.CloseHandle(h)
	ms := timeout.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	if ms > 0xfffffffe {
		ms = 0xfffffffe
	}
	state, err := syscall.WaitForSingleObject(h, uint32(ms))
	return err == nil && state == syscall.WAIT_OBJECT_0
}
