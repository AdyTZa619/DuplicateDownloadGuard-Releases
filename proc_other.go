//go:build !windows

package main

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}

func detachUpdaterProcess(cmd *exec.Cmd) {}
