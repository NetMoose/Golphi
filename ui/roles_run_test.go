package ui

import (
	"github.com/NetMoose/Golphi/internal/backend"
	"os"
	"os/exec"
	"testing"
)

func init() {
	extraScenarios["nested-roles"] = func() {
		f := NewForm()
		inner := NewPanel(f) // Created before its visual parent.
		outer := NewPanel(f)
		child := NewButton(inner)
		detached := NewLabel(child) // Recursive Owner edge without visual placement.
		require(inner.SetParent(outer) == nil)
		require(outer.SetParent(f) == nil)
		require(child.SetParent(inner) == nil)
		d := &runFake{}
		newDriver = func() backend.Driver { return d }
		require(f.Run() == nil)
		require(len(d.configs) == 4)
		require(d.configs[0].Kind == backend.Form && d.configs[0].Parent == 0)
		require(d.configs[1].Kind == backend.Panel && d.configs[1].Parent == 1)
		require(d.configs[2].Kind == backend.Panel && d.configs[2].Parent == 2)
		require(d.configs[3].Kind == backend.Button && d.configs[3].Parent == 3)
		require(detached.started)
	}
}

// Catch startup silently omitting nested children or creating them before parents.
func TestNestedRolesStartup(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=nested-roles")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
