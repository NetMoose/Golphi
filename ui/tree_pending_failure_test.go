package ui

import (
	"errors"
	"slices"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

func init() {
	extraScenarios["tree-failed-detach-pending"] = func() {
		failedDetachmentWithPendingChange(false)
	}
	extraScenarios["tree-failed-move-to-detached-pending"] = func() {
		failedDetachmentWithPendingChange(true)
	}
}

func TestTreeFailedDetachmentPreservesPendingChange(t *testing.T) {
	runGraphScenario(t, "tree-failed-detach-pending")
}

func TestTreeFailedMoveToDetachedPanelPreservesPendingChange(t *testing.T) {
	runGraphScenario(t, "tree-failed-move-to-detached-pending")
}

func failedDetachmentWithPendingChange(moveToDetachedPanel bool) {
	f := NewForm()
	source, moving := NewPanel(f), NewPanel(f)
	a, b := NewEdit(f), NewEdit(f)
	require(source.SetParent(f) == nil && moving.SetParent(source) == nil)
	require(a.SetParent(moving) == nil && b.SetParent(moving) == nil)

	var target Container
	if moveToDetachedPanel {
		target = NewPanel(f)
	}

	d := &treeFake{}
	newDriver = func() backend.Driver { return d }
	var changes []string
	a.SetOnChange(func() {
		changes = append(changes, a.Text())
		if a.Text() == "a-first" {
			a.SetText("a-pending")
		}
	})
	b.SetOnChange(func() {
		changes = append(changes, b.Text())
		if b.Text() == "b-first" {
			b.SetText("b-pending")
		}
	})
	d.loop = func() {
		r := f.runtime.(*application)
		sourcePeer, movingPeer := r.peers[&source.component], r.peers[&moving.component]
		ap, bp := r.peers[&a.component], r.peers[&b.component]
		abinding, bbinding := r.bindingGenerations[ap], r.bindingGenerations[bp]
		a.SetText("a-first")
		b.SetText("b-first")
		first := []string{"a-first", "b-first"}
		require(slices.Equal(changes, first) && a.pending && b.pending)
		ag, bg := a.generation, b.generation

		d.focus = ap
		observedPartialDetach := false
		probe := func() {
			d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Generation: abinding, Text: "technical-a"})
			d.cb.Event(backend.Event{Kind: backend.Change, Peer: bp, Generation: bbinding, Text: "technical-b"})
			require(d.dispatch() == nil && slices.Equal(changes, first))
			require(a.Text() == "a-pending" && b.Text() == "b-pending")
		}
		d.probe = func() {
			// The fake stages retirement and clears focus as it commits nodes.
			// It reaches the focused Edit before the last Edit fails.
			observedPartialDetach = observedPartialDetach || d.focus == 0
			probe()
		}
		d.phase = func(string) { probe() }
		d.fail = true
		require(errors.Is(moving.SetParent(target), lateTreeError))
		require(observedPartialDetach && d.focus == ap && len(d.retired) == 0)
		require(moving.Parent() == source && a.Parent() == moving && b.Parent() == moving)
		require(len(source.children) == 1 && len(moving.children) == 2)
		if target != nil {
			require(target.componentIdentity().parent == nil && len(target.componentIdentity().children) == 0)
		}
		require(r.peers[&moving.component] == movingPeer && r.peers[&a.component] == ap && r.peers[&b.component] == bp)
		require(d.configs[int(movingPeer)-1].Parent == sourcePeer)
		require(d.configs[int(ap)-1].Parent == movingPeer && d.configs[int(bp)-1].Parent == movingPeer)
		require(r.bindingGenerations[ap] == abinding && r.bindingGenerations[bp] == bbinding)
		require(a.pending && a.baseValid && a.base == "a-first" && a.generation == ag)
		require(b.pending && b.baseValid && b.base == "b-first" && b.generation == bg)

		d.probe, d.phase = nil, nil
		want := []string{"a-first", "b-first", "a-pending", "b-pending"}
		require(d.dispatch() == nil && slices.Equal(changes, want))
		require(d.dispatch() == nil && slices.Equal(changes, want))
		d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Generation: abinding, Text: "a-next"})
		d.cb.Event(backend.Event{Kind: backend.Change, Peer: bp, Generation: bbinding, Text: "b-next"})
		require(slices.Equal(changes, append(want, "a-next", "b-next")))
	}
	require(f.Run() == nil)
}
