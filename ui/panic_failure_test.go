package ui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// The fake substitutes only native resources. Run, event delivery, Post and
// diagnostic collection are real. Cleanup inside the native callback is a bug.
type panicFailureDriver struct {
	runFake
	body      func(func() *backend.Panic)
	primary   *backend.Panic
	cleanup   bool
	e1, e2    error
	value     any
	attempted map[backend.Peer]int
	closed    int
}

func (d *panicFailureDriver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	d.body(dispatch)
	return d.primary, nil
}
func (d *panicFailureDriver) probe() {
	d.check()
	require(!d.inside)
	require(Post(func() { panic("cleanup Post ran") }) != nil)
	done := make(chan error)
	go func() { done <- Post(func() { panic("cleanup worker Post ran") }) }()
	require(<-done != nil)
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "cleanup"})
}
func (d *panicFailureDriver) Destroy(p backend.Peer) error {
	d.probe()
	d.attempted[p]++
	if d.cleanup {
		if p == 4 {
			return d.e1
		}
		if p == 3 {
			panic(d.value)
		}
	}
	return nil
}
func (d *panicFailureDriver) Close() error {
	d.probe()
	d.closed++
	require(len(d.attempted) == 4)
	if d.cleanup {
		return d.e2
	}
	return nil
}

// Walk the internal error tree without introducing a public diagnostic API.
func panicDiagnostics(err error) []*panicDiagnostic {
	if p, ok := err.(*panicDiagnostic); ok {
		return []*panicDiagnostic{p}
	}
	var result []*panicDiagnostic
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			result = append(result, panicDiagnostics(child)...)
		}
	} else if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		result = append(result, panicDiagnostics(wrapped.Unwrap())...)
	}
	return result
}
func primaryPanic(value any) { panic(value) }

func panicFailureScenario(source string, cleanup bool) {
	value := &struct{ Message string }{"primary V"}
	cleanupValue := &struct{ Message string }{"cleanup W"}
	d := &panicFailureDriver{cleanup: cleanup, e1: errors.New("E1"), e2: errors.New("E2"), value: cleanupValue, attempted: make(map[backend.Peer]int)}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b, edit, sibling := NewButton(f), NewEdit(f), NewLabel(f)
	require(b.SetParent(f) == nil && edit.SetParent(f) == nil && sibling.SetParent(f) == nil)
	changes, clicks, tasks := 0, 0, 0
	edit.SetOnChange(func() { changes++; edit.SetText("pending repeat") })
	crash := func() {
		edit.SetText("first")
		require(changes == 1 && edit.pending)
		require(Post(func() { tasks++ }) == nil)
		if source == "destroyed-post" {
			sibling.Destroy()
			_ = sibling.Text()
		}
		primaryPanic(value)
	}
	b.SetOnClick(func() { clicks++; crash() })
	d.body = func(dispatch func() *backend.Panic) {
		d.inside = true
		// No recover in the fake: a rethrow across this boundary fails the scenario.
		if source == "handler" {
			d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		} else {
			require(Post(crash) == nil)
			d.primary = dispatch()
		}
		require(len(d.attempted) == 0 && d.closed == 0)
		require(!edit.pending && !edit.baseValid)
		require(Post(func() { tasks++ }) != nil)
		d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "late"})
		d.primary = dispatch()
		require(d.primary != nil)
		// Preserve the very same captured stack through Run's repeated failure path.
		d.inside = false
	}
	var result error
	require(backend.Capture(func() { result = f.Run() }) == nil)
	require(result != nil)
	diagnostics := panicDiagnostics(result)
	require(len(diagnostics) == 1 || cleanup && len(diagnostics) == 2)
	first := diagnostics[0]
	require(first.phase == "callback" && first.panic == d.primary)
	if source == "destroyed-post" {
		require(strings.Contains(string(first.panic.Stack), "guardAlive"))
	} else {
		require(first.panic.Value == value)
		require(strings.Contains(string(first.panic.Stack), "primaryPanic"))
	}
	if cleanup {
		require(len(diagnostics) == 2 && errors.Is(result, d.e1) && errors.Is(result, d.e2))
		second := diagnostics[1]
		require(second.panic.Value == cleanupValue && second.phase == "destroy")
		require(strings.Contains(string(second.panic.Stack), "panicFailureDriver).Destroy"))
	}
	require(tasks == 0 && changes == 1)
	if source == "handler" {
		require(clicks == 1)
	} else {
		require(clicks == 0)
	}
	require(d.closed == 1 && len(d.attempted) == 4)
	for _, n := range d.attempted {
		require(n == 1)
	}
	require(postQueue.Load() == nil && Post(func() {}) != nil)
}

func destructionCompletionScenario() {
	d := &runFake{}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b := NewButton(f)
	require(b.SetParent(f) == nil)
	resumed, taskRan, tailRan := false, false, false
	b.SetOnClick(func() {
		b.Destroy()
		require(b.Destroyed())
		require(Post(func() {
			taskRan = true
			if !b.Destroyed() {
				_ = b.Text()
			}
		}) == nil)
		require(Post(func() { tailRan = true }) == nil)
		resumed = true
	})
	d.loop = func() {
		d.inside = true
		d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		require(resumed && !taskRan)
		d.inside = false
	}
	require(f.Run() == nil)
	require(resumed && taskRan && tailRan)
}
func init() {
	for _, source := range []string{"handler", "post", "destroyed-post"} {
		source := source
		extraScenarios["panic-"+source] = func() { panicFailureScenario(source, false) }
		extraScenarios["panic-"+source+"-cleanup"] = func() { panicFailureScenario(source, true) }
	}
	extraScenarios["destroy-completion"] = destructionCompletionScenario
}
func TestPanicFailurePath(t *testing.T) {
	for _, scenario := range []string{"panic-handler", "panic-post", "panic-destroyed-post", "panic-handler-cleanup", "panic-post-cleanup", "panic-destroyed-post-cleanup", "destroy-completion"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST="+scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}
