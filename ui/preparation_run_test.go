package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

func init() {
	extraScenarios["logical-preparation"] = func() {
		f := NewForm()
		attached := NewButton(f)
		require(attached.SetParent(f) == nil)
		outer, inner := NewPanel(f), NewPanel(f)
		require(inner.SetParent(outer) == nil)
		detached := NewEdit(f)
		require(detached.SetParent(inner) == nil)
		dead := NewPanel(f)
		dead.Destroy()
		d := &runFake{}
		newDriver = func() backend.Driver { return d }
		d.loop = func() {
			beforeCalls, beforeOrder, beforeOwned := d.calls, nextComponentOrder, len(f.owned)
			worker := make(chan any)
			operations := []func(){
				func() { NewEdit(f) }, func() { NewButton(f) }, func() { NewLabel(f) }, func() { NewPanel(f) }, func() { NewForm() },
				func() { _ = attached.SetParent(dead) }, func() { _ = detached.SetParent(f) },
				func() { attached.SetBounds(0, 0, 0, 0) },
				func() { _ = attached.SetParent(f) }, func() { _ = detached.SetParent(inner) }, func() { _ = outer.SetParent(nil) },
				func() { detached.SetBounds(1, 2, 30, 40) },
			}
			for _, operation := range operations {
				go func(call func()) { var p any; func() { defer func() { p = recover() }(); call() }(); worker <- p }(operation)
				p := <-worker
				require(p != nil && strings.Contains(fmt.Sprint(p), "UI thread"))
			}
			require(d.calls == beforeCalls && nextComponentOrder == beforeOrder && len(f.owned) == beforeOwned)
			require(detached.bounds == (backend.Bounds{}) && detached.Parent() == inner && outer.Parent() == nil)
			go func() { var p any; func() { defer func() { p = recover() }(); _ = dead.SetParent(nil) }(); worker <- p }()
			require(fmt.Sprint(<-worker) == "ui: operation on destroyed component")
			go func() {
				var p any
				func() { defer func() { p = recover() }(); dead.SetBounds(-1, -1, 0, 0) }()
				worker <- p
			}()
			require(fmt.Sprint(<-worker) == "ui: operation on destroyed component")
			posted := make(chan error)
			go func() {
				posted <- Post(func() {
					calls := d.calls
					p := NewPanel(f)
					e, b, l := NewEdit(p), NewButton(p), NewLabel(p)
					require(p.Owner() == f && p.Parent() == nil && e.Owner() == p && !p.started)
					require(e.SetParent(p) == nil && b.SetParent(p) == nil && l.SetParent(p) == nil)
					e.SetOnChange(func() { panic("detached preparation delivered event") })
					e.SetText("prepared")
					require(p.SetParent(outer) == nil && p.SetParent(outer) == nil)
					require(p.SetParent(nil) == nil && p.SetParent(nil) == nil)
					// Startup already latched this detached subtree: geometry ignores started.
					require(detached.started && inner.started && outer.started)
					detached.SetBounds(1, 2, 30, 40)
					require(detached.bounds == (backend.Bounds{X: 1, Y: 2, Width: 30, Height: 40}))
					// Explicit fixture represents a previously displayed, now detached tree;
					// native detach is intentionally not implemented by this task.
					e.started, p.started = true, true
					e.SetBounds(5, 6, 70, 80)
					require(e.bounds.Width == 70)
					expectPanic(func() { attached.SetBounds(1, 2, 3, 4) })
					expectPanic(func() { f.SetClientSize(300, 200) })
					require(p.SetParent(f) != nil && attached.SetParent(nil) != nil)
					require(d.calls == calls)
				})
			}()
			require(<-posted == nil)
		}
		require(f.Run() == nil)
		require(detached.bounds.Width == 30)
	}
}

func TestLogicalPreparationRunningThreadGuards(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=logical-preparation")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
