package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runMegaTimedPreviewV8514 is used only by the interactive MEGA preview path.
// MegaClient can leave inherited stdout/stderr pipe handles alive in MEGAcmdServer
// on Windows. exec.Cmd.WaitDelay puts a hard bound on waiting for those pipes,
// so a 1.5s/4s preview command cannot turn into a ~30s UI stall.
func runMegaTimedPreviewV8514(parent context.Context, timeout time.Duration, exe string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	cmd.Env = os.Environ()
	cmd.WaitDelay = 500 * time.Millisecond

	b, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(b))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("timeout preview MEGA după %s", timeout.Round(time.Millisecond))
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return out, fmt.Errorf("MEGA preview: pipe rămas deschis după terminarea comenzii")
	}
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, out)
	}
	return out, nil
}
