//go:build linux

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeSignalOnStartupThread(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_PROBE") != "1" {
		t.Skip("set GOLPHI_NATIVE_PROBE=1 with a real X11 DISPLAY")
	}
	out, err := exec.Command("go", "run", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("native probe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "callback_same_thread=true") {
		t.Fatalf("missing callback evidence: %s", out)
	}
	t.Logf("%s", out)
}
