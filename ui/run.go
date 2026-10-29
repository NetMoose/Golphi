package ui

import (
	"errors"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/uithread"
)

var runUsed atomic.Bool
var runActive atomic.Bool

// Run запускает единственный native UI на startup-потоке. Уничтоженная форма
// вызывает panic прежде любых проверок. Отклонённый вызов живой формы возвращает
// error без расхода допуска; прошедшая проверки попытка расходуется навсегда.
func (f *Form) Run() (err error) {
	f.guardAlive()
	if !uithread.IsUI() {
		return errors.New("ui: Run requires the startup UI thread")
	}
	if !runUsed.CompareAndSwap(false, true) {
		return errors.New("ui: Run attempt already used")
	}
	runActive.Store(true)
	defer runActive.Store(false)
	r := &application{form: f, peers: make(map[*component]backend.Peer), components: make(map[backend.Peer]*component), bindingGenerations: make(map[backend.Peer]uint64)}
	r.participants = append(r.participants, &f.component)
	// Include detached owned controls for latching and event cancellation, but
	// create peers only for the visual children of this single main window.
	seen := map[*component]bool{&f.component: true}
	var collect func(*component)
	collect = func(parent *component) {
		for _, edges := range []map[*component]struct{}{parent.owned, parent.children} {
			for c := range edges {
				if !seen[c] {
					seen[c] = true
					r.participants = append(r.participants, c)
					collect(c)
				}
			}
		}
	}
	collect(&f.component)
	sort.Slice(r.participants, func(i, j int) bool { return r.participants[i].order < r.participants[j].order })
	for _, c := range r.participants {
		c.started = true
	}
	defer func() {
		r.stop()
		if r.driver != nil {
			r.cleanupOp("quit", func() error { r.driver.Quit(); return nil })
			for i := len(r.created) - 1; i >= 0; i-- {
				r.destroyPeer(r.created[i])
			}
			r.cleanupOp("close", r.driver.Close)
		}
		for _, c := range r.participants {
			c.runtime = nil
		}
		err = errors.Join(err, errors.Join(r.failures...))
	}()
	r.driver = newDriver()
	r.queue = backend.NewQueue(r.driver.Wake)
	if err = r.driver.Init(backend.Callbacks{Event: r.event, Failure: r.failure}); err != nil {
		return err
	}
	if r.failed {
		return nil
	}
	ordered := []*component{&f.component}
	var appendChildren func(*component)
	appendChildren = func(parent *component) {
		for _, c := range r.participants {
			if c.parent != nil && c.parent.componentIdentity() == parent {
				ordered = append(ordered, c)
				appendChildren(c)
			}
		}
	}
	appendChildren(&f.component)
	for _, c := range ordered {
		bounds := c.bounds
		if bounds.Width == 0 || bounds.Height == 0 {
			if c.form != nil {
				bounds.Width, bounds.Height = 400, 180
			} else {
				bounds.Width, bounds.Height = 120, 28
			}
		}
		cfg := backend.Config{Kind: c.kind, Text: c.text, Bounds: bounds, Input: c.copySavedInput()}
		if c.parent != nil {
			cfg.Parent = r.peers[c.parent.componentIdentity()]
		}
		p, e := r.driver.Create(cfg)
		if e != nil {
			return e
		}
		if p == 0 {
			return errors.New("ui: backend returned zero peer")
		}
		r.peers[c] = p
		r.components[p] = c
		r.created = append(r.created, c)
		if r.failed {
			return nil
		}
	}
	// Native show/initial text readback may synchronously signal: stay silent.
	for _, c := range r.created {
		actual, e := r.driver.Text(r.peers[c])
		if e != nil {
			return e
		}
		c.text = actual
		c.savedInput = nil
		if r.failed {
			return nil
		}
	}
	for _, c := range r.created {
		if e := r.driver.Show(r.peers[c]); e != nil {
			return e
		}
		if r.failed {
			return nil
		}
	}
	for _, c := range r.participants {
		c.runtime = r
	}
	if r.failed {
		return nil
	}
	r.running = true
	r.queue.Open()
	postQueue.Store(r.queue)
	p, e := r.driver.Run(r.dispatch)
	if p != nil {
		r.failure(p)
	}
	return e
}

type application struct {
	form                  *Form
	driver                backend.Driver
	queue                 *backend.Queue
	participants, created []*component
	peers                 map[*component]backend.Peer
	components            map[backend.Peer]*component
	bindingGenerations    map[backend.Peer]uint64
	treeChanging          bool
	running, failed       bool
	primary               *backend.Panic
	failures              []error
}
type panicDiagnostic struct {
	phase string
	panic *backend.Panic
}

func (e *panicDiagnostic) Error() string {
	return fmt.Sprintf("ui: %s panic: %v\n%s", e.phase, e.panic.Value, e.panic.Stack)
}
func (r *application) stop() {
	// Queue.Stop is the admission boundary, including workers holding a loaded
	// pointer. Publication is cleared only after its native wakes are fenced.
	if r.queue != nil {
		r.queue.Stop()
		postQueue.CompareAndSwap(r.queue, nil)
	}
	r.running = false
	for _, c := range r.participants {
		c.cancelTextNotification()
	}
}
func (r *application) failure(p *backend.Panic) {
	r.failed = true
	r.stop()
	if p != nil && r.primary == nil {
		r.primary = p
		r.failures = append(r.failures, &panicDiagnostic{phase: "callback", panic: p})
	}
}
func (r *application) cleanupOp(name string, fn func() error) {
	var err error
	p := backend.Capture(func() { err = fn() })
	if err != nil {
		r.failures = append(r.failures, fmt.Errorf("ui: %s: %w", name, err))
	}
	if p != nil {
		r.failures = append(r.failures, &panicDiagnostic{phase: name, panic: p})
	}
}
func (r *application) destroyPeer(c *component) {
	p := r.peers[c]
	if p == 0 {
		return
	}
	delete(r.peers, c)
	delete(r.components, p)
	delete(r.bindingGenerations, p)
	r.cleanupOp("destroy", func() error { return r.driver.Destroy(p) })
}
func (r *application) componentDestroyed(c *component) {
	if c == &r.form.component {
		r.stop()
		r.driver.Quit()
	}
	// Resource release happens in outer dispatch or after native Run returns.
}
func (r *application) event(e backend.Event) {
	if !r.running || r.treeChanging {
		return
	}
	if e.Kind == backend.CloseRequest {
		r.stop()
		r.driver.Quit()
		return
	}
	c := r.components[e.Peer]
	if c == nil || c.destroyed || r.bindingGenerations[e.Peer] != e.Generation {
		return
	}
	if p := backend.Capture(func() {
		switch e.Kind {
		case backend.Click:
			c.nativeClick()
		case backend.Change:
			c.nativeText(e.Text)
		}
	}); p != nil {
		r.failure(p)
		r.driver.Quit()
	}
}
func (r *application) dispatch() *backend.Panic {
	if !r.running || r.treeChanging {
		return r.primary
	}
	if p := r.queue.Dispatch(); p != nil {
		r.failure(p)
		r.driver.Quit()
		return p
	}
	for i := len(r.created) - 1; i >= 0; i-- {
		c := r.created[i]
		if c.destroyed {
			r.destroyPeer(c)
		}
	}
	if len(r.failures) > 0 {
		r.stop()
		r.driver.Quit()
	}
	return r.primary
}
func (r *application) eventsRunning() bool { return r.running && !r.treeChanging }
func (r *application) enqueueEvent(fn func()) {
	if err := r.queue.Post(fn); err != nil && r.running {
		panic(err)
	}
}
func (r *application) syncText(c *component, text string) string {
	p := r.peers[c]
	if p == 0 {
		return text
	}
	if err := r.driver.SetText(p, text); err != nil {
		panic(err)
	}
	actual, err := r.driver.Text(p)
	if err != nil {
		panic(err)
	}
	return actual
}
