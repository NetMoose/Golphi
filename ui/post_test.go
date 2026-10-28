package ui

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/uithread"
)

type postFake struct {
	boundary sync.WaitGroup
	accepted atomic.Int64
	executed atomic.Int64
	runFake
	stage string
	run   func(func() *backend.Panic) *backend.Panic
	wakes atomic.Int64
}

func (d *postFake) preparing(stage string) error {
	require(Post(func() { panic("startup task executed") }) != nil)
	require(Post(nil) != nil)
	ch := make(chan error)
	go func() { ch <- Post(func() { panic("worker during startup") }) }()
	require(<-ch != nil)
	if d.stage == stage {
		return errors.New("startup " + stage)
	}
	return nil
}
func (d *postFake) Init(cb backend.Callbacks) error {
	if err := d.runFake.Init(cb); err != nil {
		return err
	}
	return d.preparing("init")
}
func (d *postFake) Create(c backend.Config) (backend.Peer, error) {
	p, err := d.runFake.Create(c)
	if err != nil {
		return p, err
	}
	return p, d.preparing("create")
}
func (d *postFake) Text(p backend.Peer) (string, error) {
	text, err := d.runFake.Text(p)
	if err != nil {
		return text, err
	}
	return text, d.preparing("text")
}
func (d *postFake) Show(p backend.Peer) error {
	if err := d.runFake.Show(p); err != nil {
		return err
	}
	if err := d.preparing("show"); err != nil {
		return err
	}
	// Last Show returns while workers race the actual readiness publication.
	if p == 2 {
		for i := 0; i < 64; i++ {
			d.boundary.Add(1)
			go func() {
				defer d.boundary.Done()
				if Post(func() { require(uithread.IsUI()); d.executed.Add(1) }) == nil {
					d.accepted.Add(1)
				}
			}()
		}
	}
	return nil
}
func (d *postFake) Wake() error { d.wakes.Add(1); return nil }
func (d *postFake) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	return d.run(dispatch), nil
}
func (d *postFake) Close() error {
	require(Post(func() { panic("cleanup task executed") }) != nil)
	return d.runFake.Close()
}
func postScenario(s string) {
	require(Post(nil) != nil)
	require(Post(func() { panic("before Run") }) != nil)
	d := &postFake{stage: strings.TrimPrefix(s, "post-fail-")}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b := NewButton(f)
	require(b.SetParent(f) == nil)
	var got []int
	var inHandler bool
	b.SetOnClick(func() {
		inHandler = true
		require(Post(func() {
			require(!inHandler && uithread.IsUI())
			got = append(got, 1)
			require(Post(func() { got = append(got, 3) }) == nil)
			require(len(got) == 1)
		}) == nil)
		require(Post(func() { got = append(got, 2) }) == nil)
		require(len(got) == 0)
		inHandler = false
	})
	d.run = func(dispatch func() *backend.Panic) *backend.Panic {
		d.boundary.Wait()
		require(d.executed.Load() == 0)
		require(dispatch() == nil)
		require(d.executed.Load() == d.accepted.Load())
		// No paint has occurred: first ordinary delivery must already admit Post.
		require(Post(nil) != nil)
		d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		require(dispatch() == nil)
		require(reflect.DeepEqual(got, []int{1, 2, 3}))
		var wg sync.WaitGroup
		var executed atomic.Int64
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); require(Post(func() { require(uithread.IsUI()); executed.Add(1) }) == nil) }()
		}
		wg.Wait()
		require(executed.Load() == 0)
		require(dispatch() == nil)
		require(executed.Load() == 16)
		if s == "post-panic" {
			require(Post(func() { panic("posted failure") }) == nil)
			require(Post(func() { panic("must cancel") }) == nil)
			p := dispatch()
			require(p != nil && p.Value == "posted failure")
			require(Post(func() {}) != nil)
			return p
		}
		require(Post(func() { panic("accepted task must cancel on normal stop") }) == nil)
		// Concurrent callers may be admitted before shutdown, but none after it.
		start := make(chan struct{})
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _ = Post(func() { panic("must cancel") }) }()
		}
		close(start)
		d.cb.Event(backend.Event{Kind: backend.CloseRequest, Peer: 1})
		wg.Wait()
		require(Post(func() {}) != nil)
		require(dispatch() == nil)
		return nil
	}
	err := f.Run()
	require((err != nil) == (strings.HasPrefix(s, "post-fail-") || s == "post-panic"))
	if strings.HasPrefix(s, "post-fail-") {
		require(d.wakes.Load() == 0)
	}
	require(Post(nil) != nil)
	require(Post(func() {}) != nil)
	require(f.Run() != nil)
	require(Post(func() {}) != nil)
}
func TestPostLifecycle(t *testing.T) {
	for _, s := range []string{"post-normal", "post-panic", "post-fail-init", "post-fail-create", "post-fail-text", "post-fail-show"} {
		t.Run(s, func(t *testing.T) {
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST="+s)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
