//go:build !windows

package main

import "fmt"

func recoverMegaControlServerV8529(exe string) (string, error) {
	return "", fmt.Errorf("recuperarea automată MEGAcmd este disponibilă numai pe Windows")
}
