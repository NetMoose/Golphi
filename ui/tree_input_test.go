package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

func assertTreeInput(d *treeFake, p backend.Peer, want backend.InputState) {
	got, err := d.SnapshotInput(p)
	if err != nil || got != want {
		panic(fmt.Sprintf("physical input: got %+v (%v), want %+v", got, err, want))
	}
}

func init() {
	for _, mode := range []string{"unchanged", "same", "changed", "changed-back", "empty", "initial", "startup"} {
		extraScenarios["tree-input-"+mode] = func() {
			f := NewForm()
			left, right, root, parking := NewPanel(f), NewPanel(f), NewPanel(f), NewPanel(f)
			e, second := NewEdit(f), NewEdit(f)
			tail := NewButton(f)
			outside := NewButton(f)
			require(left.SetParent(f) == nil && right.SetParent(f) == nil && outside.SetParent(f) == nil)
			require(e.SetParent(root) == nil && second.SetParent(root) == nil && tail.SetParent(root) == nil)
			e.SetText("А😀Б𐐷В")
			if mode != "initial" {
				require(root.SetParent(left) == nil)
				if mode == "startup" {
					e.SetText("Ж😀Б𐐷В")
				}
			}
			d := &treeFake{live: make(map[backend.Peer]bool)}
			newDriver = func() backend.Driver { return d }
			changes := 0
			e.SetOnChange(func() { changes++ })
			d.loop = func() {
				r := f.runtime.(*application)
				peer := func() backend.Peer { return r.peers[&e.component] }
				if mode == "initial" {
					require(root.SetParent(left) == nil)
					assertTreeInput(d, peer(), backend.InputState{Text: "А😀Б𐐷В", Anchor: 5, Caret: 5})
					return
				}
				if mode == "startup" {
					assertTreeInput(d, peer(), backend.InputState{Text: "Ж😀Б𐐷В", Anchor: 5, Caret: 5})
					return
				}
				// Fresh native state differs from the core's cached text. Reversed scalar
				// endpoints span Cyrillic and supplementary-plane characters.
				d.configs[int(peer())-1].Text = "Я😀Ж𐐷Ю"
				d.inputs[peer()] = backend.InputState{Anchor: 4, Caret: 1}
				secondPeer := r.peers[&second.component]
				d.configs[int(secondPeer)-1].Text = "🙂Я𐐷"
				d.inputs[secondPeer] = backend.InputState{Anchor: 1, Caret: 2}
				d.focus = peer()
				want := backend.InputState{Text: "Я😀Ж𐐷Ю", Anchor: 4, Caret: 1, Focused: true}
				require(root.SetParent(right) == nil)
				assertTreeInput(d, peer(), want)
				require(e.Text() == want.Text)
				// A failed late descendant must restore actual changed native input and
				// focus, not simply preserve a mock state that was never disturbed.
				d.fail = true
				d.probe = func() { d.inputs[peer()] = backend.InputState{}; d.focus = r.peers[&outside.component] }
				require(errors.Is(root.SetParent(left), lateTreeError))
				d.probe = nil
				d.fail = false
				assertTreeInput(d, peer(), want)
				require(root.Parent() == right)
				// Focus on a non-Edit child is likewise preserved by the
				// transaction's general focus snapshot.
				d.focus = r.peers[&tail.component]
				require(root.SetParent(left) == nil)
				require(d.focus == r.peers[&tail.component])
				require(root.SetParent(right) == nil)
				d.focus = r.peers[&outside.component]
				require(root.SetParent(left) == nil)
				want.Focused = false
				assertTreeInput(d, peer(), want)
				require(d.focus == r.peers[&outside.component])
				d.focus = peer()
				require(root.SetParent(parking) == nil)
				require(e.Text() == want.Text)
				d.focus = r.peers[&outside.component]
				switch mode {
				case "same":
					e.SetText("Я😀Ж𐐷Ю")
				case "changed":
					e.SetText("е\u0301🙂𐐷")
					want = backend.InputState{Text: "е\u0301🙂𐐷", Anchor: 4, Caret: 4}
				case "changed-back":
					e.SetText("x")
					e.SetText("Я😀Ж𐐷Ю")
					want.Anchor, want.Caret = 5, 5
				case "empty":
					e.SetText("")
					want = backend.InputState{}
				}
				require(root.SetParent(nil) == nil && root.SetParent(parking) == nil)
				// Failed materialization must not consume saved positions; retry must
				// restore them into new physical peers.
				d.fail = true
				require(errors.Is(root.SetParent(right), lateTreeError))
				require(root.Parent() == parking)
				d.fail = false
				require(root.SetParent(right) == nil)
				assertTreeInput(d, peer(), want)
				assertTreeInput(d, r.peers[&second.component], backend.InputState{Text: "🙂Я𐐷", Anchor: 1, Caret: 2})
				require(d.focus == r.peers[&outside.component] && changes == 0)
			}
			require(f.Run() == nil)
		}
	}
}

// Removing cache publication/request forwarding or the real-change scalar
// collapse breaks physical reattachment; same-value collapse breaks "same".
func TestTreeInputState(t *testing.T) {
	for _, mode := range []string{"unchanged", "same", "changed", "changed-back", "empty", "initial", "startup"} {
		t.Run(mode, func(t *testing.T) { runGraphScenario(t, "tree-input-"+mode) })
	}
}
