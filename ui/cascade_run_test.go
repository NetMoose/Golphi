package ui

import (
	"os"
	"os/exec"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

type cascadeDriver struct {
	runFake
	released map[backend.Peer]int
}

func (d *cascadeDriver) Destroy(p backend.Peer) error {
	if err := d.runFake.Destroy(p); err != nil {
		return err
	}
	d.released[p]++
	return nil
}
func (d *cascadeDriver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	d.loop()
	require(dispatch() == nil)
	require(len(d.released) == 4)
	for _, peer := range []backend.Peer{2, 3, 4, 5} {
		require(d.released[peer] == 1)
	}
	require(d.released[1] == 0 && d.released[6] == 0)
	// Another safe turn must not release any peer twice.
	require(dispatch() == nil && len(d.released) == 4)
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 6})
	return nil, nil
}
func init() {
	extraScenarios["same-parent-inert"] = func() {
		f := NewForm()
		e := NewEdit(f)
		detached := NewEdit(f)
		require(e.SetParent(f) == nil)
		d := &runFake{}
		newDriver = func() backend.Driver { return d }
		changes := 0
		e.SetOnChange(func() {
			changes++
			if changes == 1 {
				e.SetText("second")
			}
		})
		d.loop = func() {
			d.cb.Event(backend.Event{Kind: backend.Change, Peer: 2, Text: "first"})
			require(changes == 1 && e.pending && e.baseValid)
			base, generation, calls := e.base, e.generation, d.calls
			require(e.SetParent(f) == nil)
			require(detached.SetParent(nil) == nil)
			var nilPanel *Panel
			require(detached.SetParent(nilPanel) == nil)
			require(e.pending && e.baseValid && e.base == base && e.generation == generation)
			require(d.calls == calls && len(d.configs) == 2 && changes == 1)
			require(e.Parent() == f && detached.Parent() == nil)
		}
		require(f.Run() == nil)
		require(changes == 2)
	}
	extraScenarios["overlapping-cascade"] = func() {
		f := NewForm()
		p := NewPanel(f)
		child := NewButton(p)
		ownedOutside := NewLabel(p)
		visualOnly := NewEdit(f)
		detached := NewLabel(child)
		survivor := NewButton(f)
		require(p.SetParent(f) == nil && child.SetParent(p) == nil)
		require(ownedOutside.SetParent(f) == nil && visualOnly.SetParent(p) == nil)
		require(survivor.SetParent(f) == nil)
		d := &cascadeDriver{released: make(map[backend.Peer]int)}
		newDriver = func() backend.Driver { return d }
		clicks := 0
		survivorClicks := 0
		survivor.SetOnClick(func() { survivorClicks++ })
		child.SetOnClick(func() {
			clicks++
			p.Destroy()
			for _, c := range []Component{p, child, ownedOutside, visualOnly, detached} {
				require(c.Destroyed())
			}
			require(!f.Destroyed() && !survivor.Destroyed())
			p.Destroy()
			child.Destroy()
			require(len(d.released) == 0)
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: 3})
			require(clicks == 1)
		})
		d.loop = func() {
			require(len(d.configs) == 6)
			d.inside = true
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: 3})
			d.inside = false
			require(clicks == 1 && len(d.released) == 0)
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: 6})
			require(survivorClicks == 1)
		}
		require(f.Run() == nil)
		require(survivorClicks == 2)
		require(len(d.released) == 6)
		for p := backend.Peer(1); p <= 6; p++ {
			require(d.released[p] == 1)
		}
	}
}
func TestSameParentInertDuringRun(t *testing.T) { runGraphScenario(t, "same-parent-inert") }
func TestOverlappingCascadeDuringChildCallback(t *testing.T) {
	runGraphScenario(t, "overlapping-cascade")
}
func runGraphScenario(t *testing.T, scenario string) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST="+scenario)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
