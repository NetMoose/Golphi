package ui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

var (
	fatalRestoreError = errors.New("fatal placement restoration")
	fatalCleanupError = errors.New("fatal attempt cleanup")
	fatalDestroyError = errors.New("fatal peer teardown")
	fatalCloseError   = errors.New("fatal driver close")
)

type fatalTreeFake struct {
	treeFake
	checkStopped     func()
	raceStart        chan struct{}
	admitted         chan error
	callbackReturned bool
	cleanupSteps     int
	destroyed        int
	closed           bool
	held             []backend.CleanupStep
}
type fatalTreeTransaction struct {
	backend.TreeTransaction
	d *fatalTreeFake
}

func (d *fatalTreeFake) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	tx, err := d.treeFake.PrepareTree(c)
	return &fatalTreeTransaction{TreeTransaction: tx, d: d}, err
}
func (x *fatalTreeTransaction) Rollback() error {
	// Deliberately leave the partially changed physical tree unrestored. One
	// worker is known admitted before stop, others race the actual stop boundary.
	require(<-x.d.admitted == nil)
	close(x.d.raceStart)
	return fatalRestoreError
}
func (x *fatalTreeTransaction) Cleanup() []backend.CleanupStep {
	// Attempt peers model callback-sensitive resources, retained until Close.
	x.d.held = x.TreeTransaction.Cleanup()
	return []backend.CleanupStep{
		{Operation: "fatal cleanup error", Run: func() error { x.d.checkStopped(); x.d.cleanupSteps++; return fatalCleanupError }},
		{Operation: "fatal cleanup panic", Run: func() error { x.d.checkStopped(); x.d.cleanupSteps++; panic("fatal-cleanup-token") }},
		{Operation: "fatal cleanup independent", Run: func() error { x.d.checkStopped(); x.d.cleanupSteps++; return nil }},
	}
}
func (d *fatalTreeFake) Destroy(p backend.Peer) error {
	require(!d.inside && d.callbackReturned)
	d.destroyed++
	_ = d.treeFake.Destroy(p)
	return fatalDestroyError
}
func (d *fatalTreeFake) Close() error {
	require(!d.inside && d.callbackReturned)
	d.checkStopped()
	for _, step := range d.held {
		require(step.Run() == nil)
	}
	d.closed = true
	return fatalCloseError
}

// Removing fatal stop, retaining only the pre-cleanup diagnostic, or moving
// teardown into SetParent must break these public Run/SetParent scenarios.
func init() {
	for _, first := range []bool{false, true} {
		name := "tree-fatal-placement"
		if first {
			name = "tree-fatal-first-attachment"
		}
		extraScenarios[name] = func() {
			f := NewForm()
			trigger := NewButton(f)
			pending := NewEdit(f)
			left, right, root := NewPanel(f), NewPanel(f), NewPanel(f)
			child := NewEdit(root)
			require(trigger.SetParent(f) == nil && pending.SetParent(f) == nil)
			require(left.SetParent(f) == nil && right.SetParent(f) == nil && child.SetParent(root) == nil)
			if !first {
				require(root.SetParent(left) == nil)
			}
			d := &fatalTreeFake{treeFake: treeFake{live: make(map[backend.Peer]bool), fail: true}, raceStart: make(chan struct{}), admitted: make(chan error, 1)}
			newDriver = func() backend.Driver { return d }
			var tasks atomic.Int32
			clicks, changes := 0, 0
			var app *application
			var triggerPeer, pendingPeer backend.Peer
			var setErr error
			results := make(chan error, 32)
			pending.SetOnChange(func() { changes++; pending.SetText("queued-change") })
			d.checkStopped = func() {
				require(Post(func() { tasks.Add(1) }) != nil)
				require(app.primary == nil && !app.running && !pending.pending && !pending.baseValid)
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: triggerPeer})
				d.cb.Event(backend.Event{Kind: backend.Change, Peer: pendingPeer, Text: "late-change"})
				require(d.dispatch() == nil)
				require(clicks == 1 && changes == 1 && tasks.Load() == 0)
			}
			trigger.SetOnClick(func() {
				clicks++
				pending.SetText("first-change")
				require(pending.pending)
				require(Post(func() { tasks.Add(1) }) == nil)
				go func() { d.admitted <- Post(func() { tasks.Add(1) }) }()
				for i := 0; i < cap(results); i++ {
					go func() { <-d.raceStart; results <- Post(func() { tasks.Add(1) }) }()
				}
				setErr = root.SetParent(right)
				require(errors.Is(setErr, lateTreeError) && errors.Is(setErr, fatalRestoreError))
				require(errors.Is(setErr, fatalCleanupError))
				require(d.cleanupSteps == 3 && d.destroyed == 0 && !d.closed)
				d.checkStopped()
				// Ordinary return is essential: cleanup panic must never escape here.
				d.callbackReturned = true
			})
			d.loop = func() {
				app = f.runtime.(*application)
				triggerPeer, pendingPeer = app.peers[&trigger.component], app.peers[&pending.component]
				d.inside = true
				d.cb.Event(backend.Event{Kind: backend.Click, Peer: triggerPeer})
				d.inside = false
				require(d.callbackReturned && d.destroyed == 0 && !d.closed)
				for i := 0; i < cap(results); i++ {
					err := <-results
					require(err == nil || errors.Is(err, backend.ErrNotRunning))
				}
				// Finish and later outer dispatch must never reopen delivery.
				d.checkStopped()
			}
			err := f.Run()
			for _, cause := range []error{lateTreeError, fatalRestoreError, fatalCleanupError, fatalDestroyError, fatalCloseError} {
				require(errors.Is(err, cause))
			}
			var failure *backend.TreeFailure
			require(errors.As(err, &failure) && failure.Fatal() && len(failure.Cleanup) == 2)
			require(failure.Cleanup[1].Panic != nil && failure.Cleanup[1].Panic.Value == "fatal-cleanup-token" && len(failure.Cleanup[1].Panic.Stack) > 0)
			require(strings.Contains(err.Error(), "fatal-cleanup-token") && !strings.Contains(err.Error(), "callback panic"))
			wantDestroyed := 7
			if first {
				wantDestroyed = 5
			}
			require(strings.Count(err.Error(), fatalDestroyError.Error()) == wantDestroyed)
			require(d.closed && d.destroyed == wantDestroyed && len(d.live) == 0 && tasks.Load() == 0)
			for peer := backend.Peer(1); peer <= backend.Peer(wantDestroyed); peer++ {
				require(d.released[peer] == 1)
			}
			require(app.primary == nil && postQueue.Load() == nil)
			d.checkStopped()
		}
	}
}
func TestTreeFatalRollback(t *testing.T) {
	for _, scenario := range []string{"tree-fatal-placement", "tree-fatal-first-attachment"} {
		t.Run(scenario, func(t *testing.T) { runGraphScenario(t, scenario) })
	}
}
