//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func hideChildWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func detachUpdaterProcess(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP. The updater must survive the
	// application process that launched it and must never steal keyboard focus.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00000200}
}
