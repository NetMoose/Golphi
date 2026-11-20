package ui

import (
	"fmt"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// A native container destroys its descendants, invalidating their peer IDs.
type cascadingTreeFake struct {
	treeFake
	gone map[backend.Peer]bool
}

func (d *cascadingTreeFake) Destroy(p backend.Peer) error {
	if d.gone[p] {
		return fmt.Errorf("already released peer %d", p)
	}
	var release func(backend.Peer)
	release = func(parent backend.Peer) {
		d.gone[parent] = true
		for i, cfg := range d.configs {
			child := backend.Peer(i + 1)
			if cfg.Parent == parent && !d.gone[child] {
				release(child)
			}
		}
	}
	release(p)
	return d.treeFake.Destroy(p)
}

func init() {
	for _, mode := range []string{"close", "callback"} {
		extraScenarios["moved-release-"+mode] = func() {
			f := NewForm()
			child := NewButton(f)
			require(child.SetParent(f) == nil)
			d := &cascadingTreeFake{treeFake: treeFake{live: make(map[backend.Peer]bool)}, gone: make(map[backend.Peer]bool)}
			newDriver = func() backend.Driver { return d }
			d.loop = func() {
				panel := NewPanel(f)
				require(panel.SetParent(f) == nil && child.SetParent(panel) == nil)
				if mode == "callback" {
					child.SetOnClick(func() {
						panel.Destroy()
						require(child.Destroyed() && len(d.gone) == 0)
					})
					r := f.runtime.(*application)
					p := r.peers[&child.component]
					d.cb.Event(backend.Event{Kind: backend.Click, Peer: p, Generation: r.bindingGenerations[p]})
					require(d.dispatch() == nil && r.running && len(d.gone) == 2)
				}
				d.cb.Event(backend.Event{Kind: backend.CloseRequest, Peer: 1})
			}
			if err := f.Run(); err != nil {
				panic(err)
			}
			require(len(d.released) == 3)
		}
	}
	extraScenarios["transient-lifetime"] = func() {
		f := NewForm()
		survivor := NewButton(f)
		require(survivor.SetParent(f) == nil)
		clicks := 0
		survivor.SetOnClick(func() { clicks++ })
		d := &treeFake{live: make(map[backend.Peer]bool)}
		newDriver = func() backend.Driver { return d }
		d.loop = func() {
			r := f.runtime.(*application)
			for i := 0; i < 100; i++ {
				transient := NewPanel(f)
				dormant := NewEdit(transient)
				require(transient.SetParent(f) == nil)
				transient.Destroy()
				require(d.dispatch() == nil && transient.runtime == nil && dormant.runtime == nil)
				require(len(r.created) == 2 && len(r.participants) == 2 && len(r.peers) == 2)
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: r.peers[&survivor.component]})
			}
		}
		require(f.Run() == nil && clicks == 100)
	}
	extraScenarios["destroy-worker-guard"] = func() {
		f := NewForm()
		d := &runFake{}
		newDriver = func() backend.Driver { return d }
		d.loop = func() {
			for _, op := range []func(*Button){
				func(b *Button) { _ = b.SetParent(nil) },
				func(b *Button) { b.SetBounds(0, 0, 5, 5) },
			} {
				b := NewButton(f)
				begin, done := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(done)
					<-begin
					defer func() {
						v := recover()
						require(v == "ui: operation requires the UI thread during Run" || v == "ui: operation on destroyed component")
					}()
					op(b)
				}()
				close(begin)
				b.Destroy()
				<-done
			}
		}
		require(f.Run() == nil)
	}
}
func TestMovedPeerRelease(t *testing.T) {
	for _, mode := range []string{"close", "callback"} {
		t.Run(mode, func(t *testing.T) { runGraphScenario(t, "moved-release-"+mode) })
	}
}
func TestTransientLifetime(t *testing.T)  { runGraphScenario(t, "transient-lifetime") }
func TestDestroyWorkerGuard(t *testing.T) { runGraphScenario(t, "destroy-worker-guard") }

// Model backend retirement at native callback exit, before core dispatch.
type retiringCascadeFake struct{ cascadingTreeFake }

func (d *retiringCascadeFake) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.dispatch = dispatch
	d.loop()
	for _, p := range d.retired {
		if !d.gone[p] {
			if err := d.Destroy(p); err != nil {
				return nil, err
			}
		}
	}
	return dispatch(), nil
}

func init() {
	for _, mode := range []string{"move", "detach"} {
		extraScenarios["pending-child-"+mode] = func() {
			f := NewForm()
			ancestor := NewPanel(f)
			child := NewButton(f)
			require(ancestor.SetParent(f) == nil && child.SetParent(ancestor) == nil)
			d := &retiringCascadeFake{cascadingTreeFake{treeFake: treeFake{live: make(map[backend.Peer]bool)}, gone: make(map[backend.Peer]bool)}}
			newDriver = func() backend.Driver { return d }
			var destination Container
			child.SetOnClick(func() {
				child.Destroy()
				require(ancestor.SetParent(destination) == nil)
				ancestor.Destroy()
				require(len(d.gone) == 0)
			})
			d.loop = func() {
				if mode == "move" {
					panel := NewPanel(f)
					require(panel.SetParent(f) == nil)
					destination = panel
				}
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: 3})
				require(child.Destroyed() && ancestor.Destroyed())
			}
			if err := f.Run(); err != nil {
				panic(err)
			}
			require(len(d.gone) == len(d.configs))
		}
	}
}
func TestPendingChildRelease(t *testing.T) {
	for _, mode := range []string{"move", "detach"} {
		t.Run(mode, func(t *testing.T) { runGraphScenario(t, "pending-child-"+mode) })
	}
}
