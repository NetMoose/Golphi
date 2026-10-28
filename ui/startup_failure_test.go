package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// startupFailureFake models Driver's ownership contract: a failed Create
// rolls back its own temporary allocation and returns no published peer.
type startupFailureFake struct {
	runFake
	phase                          string
	original, destroyErr, closeErr error
	panicValue                     any
	attempts, rolledBack           int
	live                           map[backend.Peer]bool
	destroyed                      []backend.Peer
	operations                     []string
}

func (d *startupFailureFake) probe(operation string) {
	d.check()
	d.operations = append(d.operations, operation)
	require(postQueue.Load() == nil)
	require(Post(func() { panic("startup Post executed") }) != nil)
	done := make(chan error)
	go func() { done <- Post(func() { panic("startup worker Post executed") }) }()
	require(<-done != nil)
	// Synchronous startup/cleanup signals must remain silent.
	if d.cb.Event != nil {
		d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "must stay silent"})
	}
}
func (d *startupFailureFake) Init(cb backend.Callbacks) error {
	d.cb = cb
	d.probe("init")
	if d.phase != "create" {
		return fmt.Errorf("%s: %w", d.phase, d.original)
	}
	return nil
}
func (d *startupFailureFake) Create(c backend.Config) (backend.Peer, error) {
	d.probe("create")
	d.attempts++
	if d.attempts == 4 {
		// This operation allocated a temporary resource, then rolled it back.
		d.rolledBack++
		return 0, fmt.Errorf("create failed: %w", d.original)
	}
	if c.Parent != 0 {
		require(d.live[c.Parent])
	}
	p := backend.Peer(d.attempts)
	d.live[p] = true
	return p, nil
}
func (d *startupFailureFake) Text(backend.Peer) (string, error) {
	panic("readback reached after startup failure")
}
func (d *startupFailureFake) Show(backend.Peer) error { panic("show reached after startup failure") }
func (d *startupFailureFake) Run(func() *backend.Panic) (*backend.Panic, error) {
	panic("native loop reached after startup failure")
}
func (d *startupFailureFake) Wake() error { panic("startup queue was published") }
func (d *startupFailureFake) Quit() {
	d.probe("quit")
	if d.phase == "create" {
		panic(d.panicValue)
	}
}
func (d *startupFailureFake) Destroy(p backend.Peer) error {
	d.probe("destroy")
	require(d.live[p])
	delete(d.live, p)
	d.destroyed = append(d.destroyed, p)
	if p == 3 {
		return d.destroyErr
	}
	return nil
}
func (d *startupFailureFake) Close() error {
	d.probe("close")
	require(len(d.live) == 0)
	return d.closeErr
}

func startupFailureScenario(phase string) {
	original := errors.New("startup sentinel " + phase)
	e1, e2 := errors.New("peer cleanup sentinel"), errors.New("infrastructure cleanup sentinel")
	value := &struct{ Message string }{"cleanup panic sentinel"}
	d := &startupFailureFake{phase: phase, original: original, destroyErr: e1, closeErr: e2, panicValue: value, live: make(map[backend.Peer]bool)}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b := NewButton(f)
	edit := NewEdit(f)
	label := NewLabel(f)
	last := NewButton(f)
	detached := NewEdit(f)
	require(b.SetParent(f) == nil && edit.SetParent(f) == nil && label.SetParent(f) == nil && last.SetParent(f) == nil)
	b.SetOnClick(func() { panic("startup click delivered") })
	edit.SetOnChange(func() { panic("startup change delivered") })
	err := f.Run()
	require(err != nil && errors.Is(err, original) && errors.Is(err, e2))
	if phase == "create" {
		require(d.attempts == 4 && d.rolledBack == 1)
		require(reflect.DeepEqual(d.destroyed, []backend.Peer{3, 2, 1}))
		require(errors.Is(err, e1))
		var diagnostic *panicDiagnostic
		require(errors.As(err, &diagnostic))
		require(diagnostic.panic.Value == value && len(diagnostic.panic.Stack) > 0)
		require(strings.Contains(string(diagnostic.panic.Stack), "startupFailureFake).Quit"))
		require(strings.Contains(err.Error(), value.Message))
	} else {
		require(d.attempts == 0 && len(d.destroyed) == 0)
		require(reflect.DeepEqual(d.operations, []string{"init", "quit", "close"}))
	}
	require(postQueue.Load() == nil && Post(func() {}) != nil)
	require(f.started && b.started && edit.started && label.started && last.started && detached.started)
	require(f.component.runtime == nil && edit.component.runtime == nil)
	calls := d.calls
	require(f.Run() != nil && d.calls == calls)
	other := NewForm()
	require(other.Run() != nil && d.calls == calls)
}

func init() {
	for _, phase := range []string{"missing-library", "missing-symbol", "backend-init", "create"} {
		phase := phase
		extraScenarios["startup-failure-"+phase] = func() { startupFailureScenario(phase) }
	}
}
func TestStartupFailure(t *testing.T) {
	for _, phase := range []string{"missing-library", "missing-symbol", "backend-init", "create"} {
		t.Run(phase, func(t *testing.T) {
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=startup-failure-"+phase)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}
