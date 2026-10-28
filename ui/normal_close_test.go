package ui

import (
	"os"
	"os/exec"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// A close delivered reentrantly while application code is on the stack must
// stop admission immediately, but resource release must wait for native return.
type normalCloseDriver struct {
	runFake
	destroyed int
	closed    bool
	resumed   bool
}

func (d *normalCloseDriver) Destroy(p backend.Peer) error {
	require(!d.inside)
	require(Post(func() { panic("cleanup task resumed") }) != nil)
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "cleanup"})
	d.destroyed++
	return d.runFake.Destroy(p)
}
func (d *normalCloseDriver) Close() error {
	require(!d.inside && d.destroyed == 3)
	d.closed = true
	return d.runFake.Close()
}
func (d *normalCloseDriver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	d.inside = true
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	require(d.resumed && d.destroyed == 0 && !d.closed)
	// Even a backend which delivers stale events and wakes after Quit cannot
	// restart application delivery or execute the canceled repeat notification.
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	d.cb.Event(backend.Event{Kind: backend.Change, Peer: 3, Text: "late"})
	require(dispatch() == nil)
	require(d.destroyed == 0)
	d.inside = false
	return dispatch(), nil
}
func normalCloseScenario() {
	d := &normalCloseDriver{}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b := NewButton(f)
	e := NewEdit(f)
	require(b.SetParent(f) == nil && e.SetParent(f) == nil)
	clicks, changes := 0, 0
	e.SetOnChange(func() { changes++; e.SetText("repeat") })
	b.SetOnClick(func() {
		clicks++
		e.SetText("first")
		require(changes == 1 && e.pending)
		require(Post(func() { panic("canceled task ran") }) == nil)
		d.cb.Event(backend.Event{Kind: backend.CloseRequest, Peer: 1})
		require(!e.pending && !e.baseValid)
		require(Post(func() { panic("post after close") }) != nil)
		require(d.destroyed == 0 && !d.closed)
		// Current application code remains free to finish after the close request.
		d.resumed = true
	})
	require(f.Run() == nil)
	require(clicks == 1 && changes == 1 && d.destroyed == 3 && d.closed)
	require(!f.Destroyed() && f.runtime == nil && e.runtime == nil)
	require(Post(func() { panic("post after Run") }) != nil)
}
func init() { extraScenarios["normal-close"] = normalCloseScenario }
func TestNormalCloseCancelsPendingDelivery(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=normal-close")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
