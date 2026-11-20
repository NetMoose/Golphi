package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

var recoverableCleanupError = errors.New("E1: release rejected")

type recoverableTreeFake struct {
	treeFake
	cleaned int
}
type recoverableTreeTransaction struct {
	backend.TreeTransaction
	d *recoverableTreeFake
}

func (d *recoverableTreeFake) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	tx, err := d.treeFake.PrepareTree(c)
	return &recoverableTreeTransaction{TreeTransaction: tx, d: d}, err
}
func (tx *recoverableTreeTransaction) Cleanup() []backend.CleanupStep {
	steps := []backend.CleanupStep{
		{Operation: "release E1", Run: func() error { return recoverableCleanupError }},
		{Operation: "release W", Run: func() error { panic("W: cleanup exploded") }},
		{Operation: "independent release", Run: func() error { tx.d.cleaned++; return nil }},
	}
	return append(steps, tx.TreeTransaction.Cleanup()...)
}

func init() {
	extraScenarios["tree-recoverable-cleanup"] = func() {
		f := NewForm()
		left, right, root := NewPanel(f), NewPanel(f), NewPanel(f)
		a, b := NewEdit(f), NewEdit(f)
		button := NewButton(f)
		require(left.SetParent(f) == nil && right.SetParent(f) == nil && root.SetParent(left) == nil)
		require(a.SetParent(root) == nil && b.SetParent(root) == nil && button.SetParent(f) == nil)
		d := &recoverableTreeFake{treeFake: treeFake{live: make(map[backend.Peer]bool)}}
		newDriver = func() backend.Driver { return d }
		ac, bc, clicks, posted := 0, 0, 0, 0
		a.SetOnChange(func() {
			ac++
			if ac == 1 {
				a.SetText("a-pending")
			}
		})
		b.SetOnChange(func() {
			bc++
			if bc == 1 {
				b.SetText("b-pending")
			}
		})
		button.SetOnClick(func() { clicks++ })
		d.loop = func() {
			r := f.runtime.(*application)
			a.SetText("a-first")
			b.SetText("b-first")
			require(a.pending && b.pending && ac == 1 && bc == 1)
			abase, bbase, ag, bg := a.base, b.base, a.generation, b.generation
			rp, ap, bp := r.peers[&root.component], r.peers[&a.component], r.peers[&b.component]
			require(Post(func() { posted++ }) == nil)
			d.probe = func() {
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: r.peers[&button.component]})
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Generation: r.bindingGenerations[ap], Text: "technical"})
				require(d.dispatch() == nil && ac == 1 && bc == 1 && clicks == 0 && posted == 0)
			}
			d.phase = func(string) { d.probe() }
			d.fail = true
			err := root.SetParent(right)
			require(errors.Is(err, lateTreeError) && errors.Is(err, recoverableCleanupError))
			require(d.cleaned == 1)
			require(root.Parent() == left && d.configs[int(rp)-1].Parent == r.peers[&left.component])
			require(r.peers[&root.component] == rp && r.peers[&a.component] == ap && r.peers[&b.component] == bp)
			require(a.pending && b.pending && a.base == abase && b.base == bbase && a.generation == ag && b.generation == bg)
			require(a.Text() == "a-pending" && b.Text() == "b-pending" && !r.failed && len(r.failures) == 0)
			// Only error's ordinary formatting is available without importing internal diagnostics.
			require(strings.Contains(err.Error(), lateTreeError.Error()) && strings.Contains(err.Error(), recoverableCleanupError.Error()))
			require(strings.Contains(err.Error(), "W: cleanup exploded") && strings.Contains(err.Error(), "goroutine ") && strings.Contains(err.Error(), "tree_recoverable_test.go:"))
			d.probe, d.phase = nil, nil
			require(Post(func() { posted++ }) == nil)
			require(d.dispatch() == nil && ac == 2 && bc == 2 && posted == 2)
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: r.peers[&button.component]})
			require(clicks == 1)
			d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Generation: r.bindingGenerations[ap], Text: "after failure"})
			require(ac == 3 && a.Text() == "after failure")
		}
		require(f.Run() == nil)
	}
}
func TestTreeRecoverableCleanup(t *testing.T) { runGraphScenario(t, "tree-recoverable-cleanup") }
