package uithread_test

import (
	"os/exec"
	"testing"
)

// Removing startup pinning, or loading GTK during ui import, breaks this subprocess.
func TestUIImportPinsStartupWithoutGTK(t *testing.T) {
	out, err := exec.Command("go", "run", "./testdata/startup").CombinedOutput()
	if err != nil {
		t.Fatalf("startup helper: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
