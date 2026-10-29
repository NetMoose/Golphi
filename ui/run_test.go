package ui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/uithread"
)

type runFake struct {
	backend.UnsupportedTree
	cb      backend.Callbacks
	configs []backend.Config
	calls   int
	initErr error
	loop    func()
	inside  bool
}

func (d *runFake) check() {
	if !uithread.IsUI() {
		panic("wrong native thread")
	}
	d.calls++
}
func (d *runFake) Init(c backend.Callbacks) error { d.check(); d.cb = c; return d.initErr }
func (d *runFake) Create(c backend.Config) (backend.Peer, error) {
	d.check()
	d.configs = append(d.configs, c)
	d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
	return backend.Peer(len(d.configs)), nil
}
func (d *runFake) Destroy(p backend.Peer) error {
	d.check()
	if d.inside {
		panic("destroy inside callback")
	}
	return nil
}
func (d *runFake) SetText(p backend.Peer, text string) error {
	d.check()
	d.configs[int(p)-1].Text = text
	d.cb.Event(backend.Event{Kind: backend.Change, Peer: p, Text: text})
	return nil
}
func (d *runFake) Text(p backend.Peer) (string, error) {
	d.check()
	return d.configs[int(p)-1].Text, nil
}
func (d *runFake) Show(backend.Peer) error { d.check(); return nil }
func (d *runFake) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	d.check()
	if d.loop != nil {
		d.loop()
	}
	return dispatch(), nil
}
func (d *runFake) Quit()        { d.check() }
func (d *runFake) Wake() error  { return nil }
func (d *runFake) Close() error { d.check(); return nil }

var extraScenarios = map[string]func(){}

func TestMain(m *testing.M) {
	if s := os.Getenv("GOLPHI_RUN_TEST"); s != "" {
		if fn := extraScenarios[s]; fn != nil {
			fn()
		} else if strings.HasPrefix(s, "post-") {
			postScenario(s)
		} else {
			runScenario(s)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func require(ok bool) {
	if !ok {
		panic("scenario assertion failed")
	}
}
func expectPanic(fn func()) { p := backend.Capture(fn); require(p != nil) }
func runScenario(s string) {
	d := &runFake{}
	newDriver = func() backend.Driver { return d }
	f := NewForm()
	b := NewButton(f)
	require(b.SetParent(f) == nil)
	if s == "invalid" {
		done := make(chan error)
		go func() { done <- f.Run() }()
		require(<-done != nil)
		require(d.calls == 0)
	}
	if s == "destroyed" {
		dead := NewForm()
		dead.Destroy()
		expectPanic(func() { _ = dead.Run() })
		ch := make(chan bool)
		go func() { ch <- backend.Capture(func() { _ = dead.Run() }) != nil }()
		require(<-ch)
		require(d.calls == 0)
	}
	if s == "failure" {
		d.initErr = errors.New("init failure")
	}
	edit := NewEdit(f)
	require(edit.SetParent(f) == nil)
	edit.SetText("prepared")
	detached := NewEdit(f)
	changes := 0
	edit.SetOnChange(func() {
		changes++
		if changes == 1 {
			edit.SetText("second")
		}
	})
	events := 0
	b.SetOnClick(func() {
		events++
		calls := d.calls
		require(f.Run() != nil)
		require(calls == d.calls)
		beforeOrder, beforeOwned := nextComponentOrder, len(f.owned)
		label := NewLabel(f)
		require(label.Owner() == f && label.Parent() == nil)
		expectPanic(func() { NewForm() })
		require(beforeOrder+1 == nextComponentOrder && beforeOwned+1 == len(f.owned))
		expectPanic(func() { edit.SetBounds(0, 0, 10, 10) })
		edit.SetText("first")
		require(changes == 1)
		require(b.SetParent(nil) != nil)
		require(b.Parent() == f)
		b.Destroy()
		require(b.Destroyed())
	})
	d.loop = func() {
		require(events == 0 && changes == 0)
		require(edit.Text() == "prepared")
		d.inside = true
		d.cb.Event(backend.Event{Kind: backend.Click, Peer: 2})
		d.inside = false
		require(events == 1)
	}
	err := f.Run()
	require((err != nil) == (s == "failure"))
	require(f.started && b.started && edit.started && detached.started)
	if s != "failure" {
		require(len(d.configs) == 3)
		require(changes == 2)
		require(d.configs[2].Kind == backend.Edit && d.configs[2].Parent == 1)
		require(d.configs[0].Kind == backend.Form)
		require(d.configs[1].Parent == 1)
	}
	calls := d.calls
	require(f.Run() != nil)
	require(d.calls == calls)
	f.Destroy()
	expectPanic(func() { _ = f.Run() })
	ch := make(chan bool)
	go func() { ch <- backend.Capture(func() { _ = f.Run() }) != nil }()
	require(<-ch)
	require(d.calls == calls)
}
func TestRunProcessAdmission(t *testing.T) {
	for _, s := range []string{"normal", "invalid", "failure", "destroyed"} {
		t.Run(s, func(t *testing.T) {
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST="+s)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
