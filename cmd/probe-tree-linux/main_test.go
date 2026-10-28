//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Missing clipping, state restoration, callback disconnection or cleanup must fail the executable.
func TestNativeTreeContract(t *testing.T) {
	if os.Getenv("GOLPHI_TREE_PROBE") != "1" {
		t.Skip("requires real X11; set GOLPHI_TREE_PROBE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v\n%s", err, out)
	}
	for _, want := range []string{"clip_pick=true", "clip_pixels=true", "no_auto_scroll=true", "fixed_coordinates=true", "no_growth=true", "move_state=true", "recreate_state=true", "callback_same_thread=true", "disconnect=true", "finalized=2"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s: %s", want, out)
		}
	}
	t.Logf("%s", out)
}
