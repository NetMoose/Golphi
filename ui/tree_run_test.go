package ui

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

var lateTreeError = errors.New("late child creation failed")

type treeFake struct {
	runFake
	fail            bool
	attempts, ready int
	live            map[backend.Peer]bool
	dispatch        func() *backend.Panic
	probe           func()
	phase           func(string)
	retired         []backend.Peer
	released        map[backend.Peer]int
	inputs          map[backend.Peer]backend.InputState
	focus           backend.Peer
}
type treeFakeTransaction struct {
	d           *treeFake
	change      backend.TreeChange
	allocated   []backend.Peer
	before      map[backend.Peer]backend.Config
	retired     []backend.Peer
	inputBefore map[backend.Peer]backend.InputState
	focusBefore backend.Peer
}

// The fake models native positions independently of logical component state.
func (d *treeFake) Create(c backend.Config) (backend.Peer, error) {
	p, err := d.runFake.Create(c)
	if d.inputs == nil {
		d.inputs = make(map[backend.Peer]backend.InputState)
	}
	if c.Kind == backend.Edit {
		d.inputs[p] = backend.InputState{Text: c.Text}
		if c.Input != nil {
			require(c.Input.Text == c.Text && !c.Input.Focused)
			d.inputs[p] = *c.Input
		}
	}
	return p, err
}
func (d *treeFake) SnapshotInput(p backend.Peer) (backend.InputState, error) {
	input := d.inputs[p]
	input.Text = d.configs[int(p)-1].Text
	input.Focused = d.focus == p
	return input, nil
}

func (d *treeFake) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	d.attempts++
	x := &treeFakeTransaction{d: d, change: c, before: make(map[backend.Peer]backend.Config), inputBefore: make(map[backend.Peer]backend.InputState), focusBefore: d.focus}
	for _, n := range c.Nodes {
		if n.Peer != 0 {
			x.before[n.Peer] = d.configs[int(n.Peer)-1]
			if n.Kind == backend.Edit {
				x.inputBefore[n.Peer], _ = d.SnapshotInput(n.Peer)
			}
		}
	}
	if d.phase != nil {
		d.phase("prepare")
	}
	return x, nil
}
func (x *treeFakeTransaction) Commit() (backend.TreeResult, error) {
	out := backend.TreeResult{}
	resolved := make(map[backend.NodeID]backend.Peer)
	for i, n := range x.change.Nodes {
		if x.d.fail && i == len(x.change.Nodes)-1 {
			return out, lateTreeError
		}
		parent := x.change.Target
		if i > 0 {
			parent = resolved[n.Parent]
		}
		require(n.ID != 0)
		if !x.change.ToAttached {
			if n.Peer != 0 {
				x.retired = append(x.retired, n.Peer)
			}
			var input *backend.InputState
			if n.Kind == backend.Edit {
				saved := x.inputBefore[n.Peer]
				input = &saved
			}
			if x.d.focus == n.Peer {
				x.d.focus = 0
			}
			out.Bindings = append(out.Bindings, backend.TreeBinding{ID: n.ID, Input: input})
			if x.d.probe != nil {
				x.d.probe()
			}
			continue
		}
		require(parent != 0)
		if n.Peer != 0 {
			p := n.Peer
			x.d.configs[int(p)-1].Parent = parent
			resolved[n.ID] = p
			var input *backend.InputState
			if n.Kind == backend.Edit {
				saved := x.inputBefore[p]
				input = &saved
				x.d.inputs[p] = saved
			}
			out.Bindings = append(out.Bindings, backend.TreeBinding{ID: n.ID, Peer: p, Generation: uint64(x.d.attempts), Input: input})
			if x.d.probe != nil {
				x.d.probe()
			}
			continue
		}
		p, err := x.d.Create(backend.Config{Kind: n.Kind, Text: n.Text, Bounds: n.Bounds, Parent: parent})
		if err != nil {
			return out, err
		}
		resolved[n.ID] = p
		x.allocated = append(x.allocated, p)
		x.d.live[p] = true
		var input *backend.InputState
		if n.Kind == backend.Edit {
			saved, _ := x.d.SnapshotInput(p)
			if n.Input != nil {
				require(n.Input.Text == n.Text && !n.Input.Focused)
				saved = *n.Input
			}
			x.d.inputs[p] = saved
			input = &saved
		}
		out.Bindings = append(out.Bindings, backend.TreeBinding{ID: n.ID, Peer: p, Generation: uint64(x.d.attempts), Input: input})
		if x.d.probe != nil {
			x.d.probe()
		}
	}
	x.d.ready = len(out.Bindings)
	return out, nil
}
func (x *treeFakeTransaction) Rollback() error {
	x.retired = nil
	x.d.focus = x.focusBefore
	for p, input := range x.inputBefore {
		x.d.inputs[p] = input
	}
	for p, cfg := range x.before {
		x.d.configs[int(p)-1] = cfg
	}
	if x.d.phase != nil {
		x.d.phase("rollback")
	}
	return nil
}
func (x *treeFakeTransaction) Cleanup() []backend.CleanupStep {
	var steps []backend.CleanupStep
	for _, p := range x.allocated {
		steps = append(steps, backend.CleanupStep{Operation: "attempt peer", Run: func() error { delete(x.d.live, p); return nil }})
	}
	return steps
}
func (x *treeFakeTransaction) Finish() {
	x.d.retired = append(x.d.retired, x.retired...)
	if x.d.phase != nil {
		x.d.phase("finish")
	}
}
func (d *treeFake) Destroy(p backend.Peer) error {
	if d.released == nil {
		d.released = make(map[backend.Peer]int)
	}
	d.released[p]++
	delete(d.live, p)
	return d.runFake.Destroy(p)
}
func (d *treeFake) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.dispatch = dispatch
	d.loop()
	p := dispatch()
	// The adapter owns detached peer retirement until a safe outer turn.
	for _, peer := range d.retired {
		if err := d.Destroy(peer); err != nil {
			return p, err
		}
	}
	d.retired = nil
	return p, nil
}
func init() {
	extraScenarios["tree-materialization"] = func() {
		f := NewForm()
		trigger := NewButton(f)
		require(trigger.SetParent(f) == nil)
		d := &treeFake{live: make(map[backend.Peer]bool)}
		newDriver = func() backend.Driver { return d }
		clicks, changes, posted := 0, 0, 0
		var dynamicEdit *Edit
		trigger.SetOnClick(func() { clicks++ })
		d.loop = func() {
			root := NewPanel(f)
			inner := NewPanel(root)
			edit := NewEdit(root)
			dynamicEdit = edit
			require(inner.SetParent(root) == nil && edit.SetParent(inner) == nil)
			edit.SetText("prepared")
			edit.SetBounds(1, 2, 30, 40)
			edit.SetOnChange(func() { changes++ })
			require(d.attempts == 0 && len(d.configs) == 2)
			require(Post(func() { posted++ }) == nil)
			d.probe = func() {
				require(root.Parent() == nil && edit.Parent() == inner && edit.Text() == "prepared")
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
				d.dispatch()
				require(clicks == 0 && posted == 0 && changes == 0)
			}
			d.fail = true
			require(errors.Is(root.SetParent(f), lateTreeError))
			require(root.Parent() == nil && inner.Parent() == root && edit.Parent() == inner)
			require(root.Owner() == f && inner.Owner() == root && edit.Owner() == root)
			require(edit.Text() == "prepared" && edit.bounds == (backend.Bounds{X: 1, Y: 2, Width: 30, Height: 40}))
			require(len(d.live) == 0 && d.ready == 0 && changes == 0)
			d.fail = false
			require(root.SetParent(f) == nil)
			require(root.Parent() == f && d.ready == 3 && len(d.live) == 3)
			r := f.runtime.(*application)
			require(r.peers[&root.component] != 0 && r.peers[&inner.component] != 0 && r.peers[&edit.component] != 0)
			ep := r.peers[&edit.component]
			require(d.configs[int(ep)-1].Parent == r.peers[&inner.component])
			require(d.configs[int(r.peers[&inner.component])-1].Parent == r.peers[&root.component])
			require(d.configs[int(ep)-1].Bounds == (backend.Bounds{X: 1, Y: 2, Width: 30, Height: 40}))
			require(d.configs[int(ep)-1].Text == "prepared")
			edit.SetText("ready")
			require(changes == 1)
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
			require(clicks == 1)
			d.dispatch()
			require(posted == 1)
		}
		require(f.Run() == nil)
		require(dynamicEdit.runtime == nil)
	}
}
func TestTreeMaterializationRollbackRetry(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=tree-materialization")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func init() {
	extraScenarios["tree-transitions"] = func() {
		f := NewForm()
		left, right, parking := NewPanel(f), NewPanel(f), NewPanel(f)
		root := NewPanel(f)
		inner := NewPanel(f)
		a, b := NewEdit(f), NewEdit(f)
		require(left.SetParent(f) == nil && right.SetParent(f) == nil)
		require(root.SetParent(left) == nil && inner.SetParent(root) == nil)
		require(a.SetParent(root) == nil && b.SetParent(inner) == nil)
		d := &treeFake{live: make(map[backend.Peer]bool)}
		newDriver = func() backend.Driver { return d }
		ac, bc, posted := 0, 0, 0
		a.SetOnChange(func() {
			ac++
			if ac == 1 {
				a.SetText("a-repeat")
			}
		})
		b.SetOnChange(func() {
			bc++
			if bc == 1 {
				b.SetText("b-repeat")
			}
		})
		d.loop = func() {
			r := f.runtime.(*application)
			ap, bp := r.peers[&a.component], r.peers[&b.component]
			a.SetText("a-first")
			b.SetText("b-first")
			require(ac == 1 && bc == 1 && a.pending && b.pending)
			abase, bbase, ag, bg := a.base, b.base, a.generation, b.generation
			require(Post(func() { posted++ }) == nil)
			probe := func() {
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Text: "technical", Generation: r.bindingGenerations[ap]})
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: bp, Text: "technical", Generation: r.bindingGenerations[bp]})
				require(d.dispatch() == nil)
				require(ac == 1 && bc == 1 && posted == 0 && a.Text() == "a-repeat" && b.Text() == "b-repeat")
			}
			d.probe = probe
			d.phase = func(string) { probe() }
			// A late ordinary failure must retain logical and native parents and
			// both descendants' original pending comparison state.
			d.fail = true
			require(errors.Is(root.SetParent(right), lateTreeError))
			require(root.Parent() == left && d.configs[int(r.peers[&root.component])-1].Parent == r.peers[&left.component])
			require(a.pending && b.pending && a.base == abase && b.base == bbase && a.generation == ag && b.generation == bg)
			d.fail = false
			require(root.SetParent(right) == nil)
			require(root.Parent() == right && a.pending && b.pending && a.base == abase && b.base == bbase && a.generation == ag && b.generation == bg)
			d.probe, d.phase = nil, nil
			// The old binding generation cannot route into a reused peer.
			d.cb.Event(backend.Event{Kind: backend.Change, Peer: ap, Text: "stale"})
			require(a.Text() == "a-repeat" && ac == 1)
			require(d.dispatch() == nil && ac == 2 && bc == 2 && posted == 1)
			// Create pending notifications again, then detach to a non-nil
			// panel whose complete ancestor chain does not reach the Form.
			a.SetOnChange(func() { ac++; a.SetText("cancel-a") })
			b.SetOnChange(func() { bc++; b.SetText("cancel-b") })
			a.SetText("next-a")
			b.SetText("next-b")
			require(a.pending && b.pending)
			require(root.SetParent(parking) == nil)
			require(root.Parent() == parking && inner.Parent() == root && b.Parent() == inner)
			require(!a.pending && !b.pending && !a.baseValid && !b.baseValid)
			calls, attempts := d.calls, d.attempts
			a.SetText("quiet-a")
			b.SetText("quiet-b")
			require(root.SetParent(nil) == nil && root.SetParent(parking) == nil)
			require(d.calls == calls && d.attempts == attempts && ac == 3 && bc == 3)
			require(d.dispatch() == nil && ac == 3 && bc == 3)
			// Reattach is quiet; cancellation must survive reuse of this tree.
			require(root.SetParent(left) == nil)
			require(d.dispatch() == nil && ac == 3 && bc == 3)
			// Detaching inside the current handler cancels its accumulation as
			// well as pending work belonging to another descendant.
			b.SetText("pending-again")
			require(b.pending)
			a.SetOnChange(func() {
				ac++
				a.SetText("accumulated")
				require(root.SetParent(nil) == nil)
				require(!a.baseValid && !b.pending && !b.baseValid)
			})
			a.SetText("detach-now")
			require(!a.pending && !a.baseValid && !b.pending)
			require(d.dispatch() == nil && ac == 4 && bc == 4)
		}
		require(f.Run() == nil)
		for peer := backend.Peer(1); peer <= backend.Peer(len(d.configs)); peer++ {
			require(d.released[peer] == 1)
		}
	}
}
func TestTreeAttachmentTransitions(t *testing.T) { runGraphScenario(t, "tree-transitions") }

// Exercise reentry through the public runtime: each process has its own Run.
func init() {
	for _, mode := range []string{"direct", "ancestor", "quiet", "repeat", "replace", "nil", "destroy", "generation"} {
		extraScenarios["tree-reentry-"+mode] = func() {
			f := NewForm()
			panel := NewPanel(f)
			e := NewEdit(f)
			require(panel.SetParent(f) == nil && e.SetParent(panel) == nil)
			d := &treeFake{live: make(map[backend.Peer]bool)}
			newDriver = func() backend.Driver { return d }
			calls, replacement, depth := 0, 0, 0
			var move func(Container) error
			move = e.SetParent
			if mode == "ancestor" {
				move = panel.SetParent
			}
			e.SetOnChange(func() {
				depth++
				defer func() { depth-- }()
				require(depth == 1)
				calls++
				if calls > 1 {
					require(e.Text() == "A")
					return
				}
				r := f.runtime.(*application)
				oldPeer := r.peers[&e.component]
				oldGeneration := r.bindingGenerations[oldPeer]
				e.SetText("canceled")
				require(move(nil) == nil)
				// The former peer and its actual old generation must be inert,
				// including while its native resource awaits retirement.
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: oldPeer, Generation: oldGeneration, Text: "stale-detached"})
				require(e.Text() == "canceled" && calls == 1)
				e.SetText("B")
				parent := Container(panel)
				if mode == "ancestor" {
					parent = f
				}
				require(move(parent) == nil && calls == 1)
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: oldPeer, Generation: oldGeneration, Text: "stale-reattached"})
				require(e.Text() == "B" && calls == 1)
				if mode == "quiet" {
					return
				}
				e.SetText("A")
				if mode == "repeat" {
					e.SetText("C")
					e.SetText("B")
					e.SetText("A")
				}
				if mode == "replace" {
					e.SetOnChange(func() { require(depth == 0 && e.Text() == "A"); replacement++ })
				}
			})
			d.loop = func() {
				e.SetText("A")
				require(calls == 1 && replacement == 0)
				if mode == "nil" {
					e.SetOnChange(nil)
					e.SetOnChange(func() { replacement++ })
				}
				if mode == "destroy" {
					e.Destroy()
				}
				if mode == "generation" {
					// Cancel a queued generation, then queue a fresh one before
					// either closure runs. The old closure must not consume it.
					require(e.SetParent(nil) == nil)
					e.SetText("B")
					require(e.SetParent(panel) == nil)
					e.SetOnChange(func() {
						replacement++
						if replacement == 1 {
							e.SetText("fresh")
						}
					})
					e.SetText("new")
					require(replacement == 1)
				}
				require(d.dispatch() == nil)
				switch mode {
				case "quiet", "nil", "destroy":
					require(calls == 1 && replacement == 0)
				case "replace":
					require(calls == 1 && replacement == 1)
				case "generation":
					require(calls == 1 && replacement == 2 && e.Text() == "fresh")
				default:
					require(calls == 2 && replacement == 0)
				}
				require(d.dispatch() == nil)
				if mode == "generation" {
					require(replacement == 2)
				} else if mode == "replace" {
					require(replacement == 1)
				} else if mode == "quiet" || mode == "nil" || mode == "destroy" {
					require(calls == 1)
				} else {
					require(calls == 2)
				}
			}
			require(f.Run() == nil)
		}
	}
}

func TestTreeReattachmentComparison(t *testing.T) {
	for _, mode := range []string{"direct", "ancestor", "quiet", "repeat", "replace", "nil", "destroy", "generation"} {
		t.Run(mode, func(t *testing.T) { runGraphScenario(t, "tree-reentry-"+mode) })
	}
}
