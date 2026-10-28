package ui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Independent sibling peers deliberately fail in different ways. A failed
// attempt is recorded, without pretending that its resource was freed.
type cleanupFailureFake struct {
	runFake
	e1, e2    error
	value     any
	attempted map[backend.Peer]int
	closed    int
	dispatch  func() *backend.Panic
}

func (d *cleanupFailureFake) probe() {
	d.check()
	require(postQueue.Load() == nil)
	require(Post(func() { panic("cleanup Post delivered") }) != nil)
	done := make(chan error)
	go func() { done <- Post(func() { panic("cleanup worker Post delivered") }) }()
	require(<-done != nil)
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "late"})
	require(d.dispatch() == nil) // Cleanup has no invented primary callback panic.
}
func (d *cleanupFailureFake) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	d.dispatch = dispatch
	d.loop()
	return dispatch(), nil
}
func (d *cleanupFailureFake) Quit() {
	d.check()
	// The first Quit is requested by CloseRequest; subsequent cleanup is silent.
	require(postQueue.Load() == nil)
}
func (d *cleanupFailureFake) Destroy(p backend.Peer) error {
	d.probe()
	d.attempted[p]++
	switch p {
	case 4:
		return d.e1
	case 3:
		panic(d.value)
	}
	return nil
}
func (d *cleanupFailureFake) Close() error {
	d.probe()
	d.closed++
	require(len(d.attempted) == 4)
	return d.e2
}

func cleanupFailureScenario() {
	e1, e2 := errors.New("cleanup peer E1"), errors.New("cleanup infrastructure E2")
	value := &struct{ Message string }{"cleanup W"}
	d := &cleanupFailureFake{e1: e1, e2: e2, value: value, attempted: make(map[backend.Peer]int)}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b, edit, sibling := NewButton(f), NewEdit(f), NewLabel(f)
	require(b.SetParent(f) == nil && edit.SetParent(f) == nil && sibling.SetParent(f) == nil)
	clicks, changes := 0, 0
	b.SetOnClick(func() { clicks++ })
	edit.SetOnChange(func() { changes++; edit.SetText("pending repeat") })
	d.loop = func() {
		d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "first"})
		require(changes == 1)
		require(Post(func() { panic("cancelled Post delivered") }) == nil)
		d.cb.Event(backend.Event{Kind: backend.CloseRequest, Peer: 1})
		require(d.dispatch() == nil)
	}
	var result error
	escaped := backend.Capture(func() { result = f.Run() })
	require(escaped == nil && result != nil)
	require(errors.Is(result, e1) && errors.Is(result, e2))
	var diagnostic *panicDiagnostic
	require(errors.As(result, &diagnostic))
	require(diagnostic.phase == "destroy" && diagnostic.panic.Value == value)
	require(strings.Contains(string(diagnostic.panic.Stack), "cleanupFailureFake).Destroy"))
	require(strings.Contains(result.Error(), value.Message))
	require(d.closed == 1 && len(d.attempted) == 4)
	for _, attempts := range d.attempted {
		require(attempts == 1)
	}
	require(clicks == 0 && changes == 1)
	require(postQueue.Load() == nil && Post(func() {}) != nil)
	require(!f.Destroyed() && f.component.runtime == nil && edit.component.runtime == nil)
}
func init() { extraScenarios["cleanup-failure-normal"] = cleanupFailureScenario }
func TestNormalCloseCleanupFailures(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=cleanup-failure-normal")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
}
