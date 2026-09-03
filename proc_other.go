//go:build !windows

package main

import (
	"os/exec"
	"time"
)

func hideChildWindow(cmd *exec.Cmd) {}

func detachUpdaterProcess(cmd *exec.Cmd) {}

func waitForProcessExit(pid int, timeout time.Duration) bool { return true }
