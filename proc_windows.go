//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	createNoWindow        = 0x08000000
	createNewProcessGroup = 0x00000200
)

func terminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	killer := exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/T", "/F")
	killer.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := killer.Run(); err != nil {
		// The process may have exited between the cancellation signal and
		// taskkill. Treat that case as success; the direct Kill below is a
		// final fallback for a still-live child.
		if p, findErr := os.FindProcess(pid); findErr == nil {
			if killErr := p.Kill(); killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
				return nil
			}
		}
		return err
	}
	return nil
}

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}
	// exec.CommandContext normally kills only the direct child. Several download
	// engines can spawn helpers, so replace that cancellation with a whole-tree
	// termination. This is what makes STOP/Pause/Cancel stop the real work too.
	if directCancel := cmd.Cancel; directCancel != nil {
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				if err := terminateProcessTree(cmd.Process.Pid); err == nil {
					return nil
				}
			}
			err := directCancel()
			if errors.Is(err, os.ErrProcessDone) {
				return nil
			}
			return err
		}
		if cmd.WaitDelay == 0 {
			cmd.WaitDelay = 3 * time.Second
		}
	}
}

// hideControlWindow is for short-lived clients such as MegaClient.exe that
// communicate with a separate, process-wide server. On context cancellation we
// may terminate the direct client, but must never taskkill its process tree:
// that tree can include the shared MEGAcmd server used by the next preview.
func hideControlWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}
}

func detachUpdaterProcess(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP. The updater must survive the
	// application process that launched it and must never steal keyboard focus.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}
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
