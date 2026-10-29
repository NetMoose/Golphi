package ui

import (
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParentValidationOwnerRootAndCycles(t *testing.T) {
	f, other := NewForm(), NewForm()
	owner := NewButton(NewPanel(f))
	a, b := NewPanel(owner), NewPanel(f)
	if err := b.SetParent(a); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		child  *Panel
		target Container
		want   string
	}{
		{"self", a, a, "itself"},
		{"cycle", a, b, "cycle"},
		{"foreign form", a, other, "different form"},
		{"foreign detached tree", a, NewPanel(NewLabel(other)), "different form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.child.SetParent(tc.target); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s error, got %v", tc.want, err)
			}
			if a.Parent() != nil || b.Parent() != a || len(a.children) != 1 || len(b.children) != 0 {
				t.Fatal("rejection changed graph")
			}
		})
	}
	if err := a.SetParent(f); err != nil {
		t.Fatal(err)
	}
	if err := a.SetParent(nil); err != nil {
		t.Fatal(err)
	}
	if err := a.SetParent(NewPanel(other)); err == nil {
		t.Fatal("detached tree lost owner root")
	}
}

func TestParentValidationRunningPreservesEvents(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=parent-validation-running")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func init() {
	extraScenarios["parent-validation-running"] = func() {
		f := NewForm()
		e := NewEdit(f)
		require(e.SetParent(f) == nil)
		foreign := NewPanel(NewForm())
		dead := NewPanel(f)
		dead.Destroy()
		d := &runFake{}
		newDriver = func() backend.Driver { return d }
		calls := 0
		e.SetOnChange(func() {
			calls++
			if calls == 1 {
				e.SetText("pending")
			}
		})
		d.loop = func() {
			e.SetText("first")
			before, nativeCalls := e.eventState, d.calls
			for _, target := range []Container{foreign, dead} {
				err := e.SetParent(target)
				require(err != nil && !strings.Contains(err.Error(), "during Run"))
				require(e.Parent() == f && e.Owner() == f && e.Text() == "pending" && e.eventState.generation == before.generation && calls == 1 && d.calls == nativeCalls)
			}
		}
		require(f.Run() == nil && calls == 2)
	}
}

func TestParentValidationDestroyedReceiverFirst(t *testing.T) {
	f := NewForm()
	p := NewPanel(f)
	p.Destroy()
	dead := NewPanel(f)
	dead.Destroy()
	runActive.Store(true)
	defer runActive.Store(false)
	for _, target := range []Container{p, dead, nil} {
		got := fmt.Sprint(geometryPanic(t, func() { _ = p.SetParent(target) }))
		if got != "ui: operation on destroyed component" {
			t.Fatal(got)
		}
	}
}
