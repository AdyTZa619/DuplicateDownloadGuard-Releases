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
// so a short preview command cannot turn into a ~30s UI stall.
func runMegaTimedPreviewV8514(parent context.Context, timeout time.Duration, exe string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	started := time.Now()
	safeArgs := megaPreviewSafeArgsV8514(args)
	megaPreviewDiagfV8514("CMD START  %s  budget=%s", safeArgs, timeout)

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	cmd.Env = os.Environ()
	cmd.WaitDelay = 500 * time.Millisecond

	b, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(b))
	elapsed := time.Since(started)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("timeout preview MEGA după %s", timeout.Round(time.Millisecond))
		megaPreviewDiagfV8514("CMD END    %s  elapsed=%s  TIMEOUT  output=%q", safeArgs, elapsed.Round(time.Millisecond), megaPreviewShortOutputV8514(out))
		return out, err
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		err = fmt.Errorf("MEGA preview: pipe rămas deschis după terminarea comenzii")
		megaPreviewDiagfV8514("CMD END    %s  elapsed=%s  WAIT_DELAY  output=%q", safeArgs, elapsed.Round(time.Millisecond), megaPreviewShortOutputV8514(out))
		return out, err
	}
	if err != nil {
		megaPreviewDiagfV8514("CMD END    %s  elapsed=%s  ERROR=%v  output=%q", safeArgs, elapsed.Round(time.Millisecond), err, megaPreviewShortOutputV8514(out))
		return out, fmt.Errorf("%w: %s", err, out)
	}
	megaPreviewDiagfV8514("CMD END    %s  elapsed=%s  OK  output=%q", safeArgs, elapsed.Round(time.Millisecond), megaPreviewShortOutputV8514(out))
	return out, nil
}
