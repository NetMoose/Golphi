//go:build linux && amd64

// Package gtk4 implements the MVP using the system GTK4 runtime. Importing this
// package does not load GTK. All operations except Wake belong to the UI thread.
package gtk4

import (
	"errors"
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/ebitengine/purego"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

type api struct {
	initCheck                              func() int32
	windowNew, fixedNew, entryNew          func() uintptr
	buttonNew, labelNew                    func(string) uintptr
	refSink                                func(uintptr) uintptr
	unref, windowDestroy                   func(uintptr)
	windowChild                            func(uintptr, uintptr)
	windowSize                             func(uintptr, int32, int32)
	resizable                              func(uintptr, int32)
	size                                   func(uintptr, int32, int32)
	put                                    func(uintptr, uintptr, float64, float64)
	remove                                 func(uintptr, uintptr)
	title, editText, buttonText, labelText func(uintptr, string)
	getTitle, getEdit, getButton, getLabel func(uintptr) string
	present                                func(uintptr)
	visible                                func(uintptr, int32)
	connect                                func(uintptr, string, uintptr, uintptr, uintptr, uint32) uint64
	disconnect                             func(uintptr, uint64)
	contextDefault                         func() uintptr
	contextRef                             func(uintptr) uintptr
	contextUnref, wake                     func(uintptr)
	iteration                              func(uintptr, int32) int32
}
type peer struct {
	handle, fixed uintptr
	kind          backend.Kind
	parent        backend.Peer
	signal        uint64
	token         uintptr
}
type driver struct {
	backend.UnsupportedTree // Dynamic implementation is a later adapter task.

	api           api
	peers         map[backend.Peer]*peer
	next          backend.Peer
	callbacks     backend.Callbacks
	failure       *backend.Panic
	quit          bool
	mu            sync.Mutex // fences Wake against Close, never held while calling Go sinks
	context       uintptr
	ready, closed bool
	pending       atomic.Bool
}

// New constructs an inert driver. Init performs all dynamic loading.
func New() backend.Driver { return &driver{} }
func validText(s string) error {
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return errors.New("GTK text must be valid UTF-8 without NUL")
	}
	return nil
}

var registry = struct {
	sync.Mutex
	entries map[uintptr]struct {
		d  *driver
		id backend.Peer
	}
}{entries: make(map[uintptr]struct {
	d  *driver
	id backend.Peer
})}
var sequence atomic.Uint64
var trampolineOnce sync.Once
var clickedTrampoline, changedTrampoline, closeTrampoline uintptr

func callback(token uintptr, kind backend.EventKind) {
	var d *driver
	failure := backend.Capture(func() {
		registry.Lock()
		e, ok := registry.entries[token]
		registry.Unlock()
		if !ok {
			return
		}
		d = e.d
		if d.failure != nil || d.quit {
			return
		}
		event := backend.Event{Kind: kind, Peer: e.id}
		if kind == backend.Change {
			p, ok := d.peers[e.id]
			if !ok {
				return
			}
			event.Text = d.api.getEdit(p.handle)
		}
		if d.callbacks.Event != nil {
			d.callbacks.Event(event)
		}
	})
	if failure != nil && d != nil {
		d.latch(failure)
	}
}
func clicked(_, token uintptr)            { callback(token, backend.Click) }
func changed(_, token uintptr)            { callback(token, backend.Change) }
func closeRequest(_, token uintptr) int32 { callback(token, backend.CloseRequest); return 1 }
func (d *driver) latch(p *backend.Panic) {
	if d.failure != nil {
		return
	}
	d.failure = p
	d.quit = true
	if d.callbacks.Failure != nil {
		d.callbacks.Failure(p)
	}
}
func (d *driver) Init(c backend.Callbacks) (err error) {
	if d.ready || d.closed {
		return errors.New("GTK driver already initialized or closed")
	}
	d.callbacks = c
	d.peers = make(map[backend.Peer]*peer)
	lib, e := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		return e
	}
	// GTK installs process-wide TLS/handlers. Keep the library loaded for process
	// lifetime; Close releases owned objects and callback roots, not GTK globals.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("GTK symbol registration: %v", p)
		}
	}()
	a := &d.api
	for _, s := range []struct {
		p any
		n string
	}{
		{&a.initCheck, "gtk_init_check"}, {&a.windowNew, "gtk_window_new"}, {&a.fixedNew, "gtk_fixed_new"}, {&a.entryNew, "gtk_entry_new"}, {&a.buttonNew, "gtk_button_new_with_label"}, {&a.labelNew, "gtk_label_new"},
		{&a.refSink, "g_object_ref_sink"}, {&a.unref, "g_object_unref"}, {&a.windowDestroy, "gtk_window_destroy"}, {&a.windowChild, "gtk_window_set_child"}, {&a.windowSize, "gtk_window_set_default_size"}, {&a.resizable, "gtk_window_set_resizable"}, {&a.size, "gtk_widget_set_size_request"}, {&a.put, "gtk_fixed_put"}, {&a.remove, "gtk_fixed_remove"},
		{&a.title, "gtk_window_set_title"}, {&a.editText, "gtk_editable_set_text"}, {&a.buttonText, "gtk_button_set_label"}, {&a.labelText, "gtk_label_set_text"}, {&a.getTitle, "gtk_window_get_title"}, {&a.getEdit, "gtk_editable_get_text"}, {&a.getButton, "gtk_button_get_label"}, {&a.getLabel, "gtk_label_get_text"},
		{&a.present, "gtk_window_present"}, {&a.visible, "gtk_widget_set_visible"}, {&a.connect, "g_signal_connect_data"}, {&a.disconnect, "g_signal_handler_disconnect"}, {&a.contextDefault, "g_main_context_default"}, {&a.contextRef, "g_main_context_ref"}, {&a.contextUnref, "g_main_context_unref"}, {&a.wake, "g_main_context_wakeup"}, {&a.iteration, "g_main_context_iteration"},
	} {
		purego.RegisterLibFunc(s.p, lib, s.n)
	}
	if a.initCheck() == 0 {
		return errors.New("gtk_init_check failed (display unavailable)")
	}
	trampolineOnce.Do(func() {
		clickedTrampoline = purego.NewCallback(clicked)
		changedTrampoline = purego.NewCallback(changed)
		closeTrampoline = purego.NewCallback(closeRequest)
	})
	d.context = a.contextRef(a.contextDefault())
	if d.context == 0 {
		return errors.New("GTK default context unavailable")
	}
	d.ready = true
	return nil
}
func (d *driver) lookup(id backend.Peer) (*peer, error) {
	p := d.peers[id]
	if p == nil {
		return nil, errors.New("unknown GTK peer")
	}
	return p, nil
}
func validDimensions(b backend.Bounds) error {
	const maxNativeDimension = 1<<31 - 1
	if b.Width <= 0 || b.Height <= 0 || b.Width > maxNativeDimension || b.Height > maxNativeDimension {
		return errors.New("GTK dimensions must be positive signed 32-bit integers")
	}
	return nil
}

func (d *driver) Create(c backend.Config) (backend.Peer, error) {
	if !d.ready || d.closed {
		return 0, errors.New("GTK driver not ready")
	}
	if err := validText(c.Text); err != nil {
		return 0, err
	}
	if err := validDimensions(c.Bounds); err != nil {
		return 0, err
	}
	var parent *peer
	if c.Kind != backend.Form {
		var err error
		parent, err = d.lookup(c.Parent)
		if err != nil || parent.kind != backend.Form {
			return 0, errors.New("GTK control requires form parent")
		}
	} else if c.Parent != 0 {
		return 0, errors.New("GTK form cannot have parent")
	}
	p := &peer{kind: c.Kind, parent: c.Parent}
	a := &d.api
	switch c.Kind {
	case backend.Form:
		p.handle = a.windowNew()
	case backend.Edit:
		p.handle = a.entryNew()
	case backend.Button:
		p.handle = a.buttonNew(c.Text)
	case backend.Label:
		p.handle = a.labelNew(c.Text)
	default:
		return 0, errors.New("unknown GTK kind")
	}
	if p.handle == 0 {
		return 0, errors.New("GTK widget creation failed")
	}
	a.refSink(p.handle)
	d.next++
	id := d.next
	d.peers[id] = p
	if c.Kind == backend.Form {
		p.fixed = a.fixedNew()
		if p.fixed == 0 {
			_ = d.Destroy(id)
			return 0, errors.New("GTK fixed creation failed")
		}
		a.windowChild(p.handle, p.fixed)
		a.windowSize(p.handle, int32(c.Bounds.Width), int32(c.Bounds.Height))
		a.resizable(p.handle, 0)
	} else {
		a.size(p.handle, int32(c.Bounds.Width), int32(c.Bounds.Height))
		a.put(parent.fixed, p.handle, float64(c.Bounds.X), float64(c.Bounds.Y))
	}
	if err := d.SetText(id, c.Text); err != nil {
		_ = d.Destroy(id)
		return 0, err
	}
	var signal string
	var cb uintptr
	switch c.Kind {
	case backend.Form:
		signal = "close-request"
		cb = closeTrampoline
	case backend.Edit:
		signal = "changed"
		cb = changedTrampoline
	case backend.Button:
		signal = "clicked"
		cb = clickedTrampoline
	}
	if cb != 0 {
		p.token = uintptr(sequence.Add(1))
		registry.Lock()
		registry.entries[p.token] = struct {
			d  *driver
			id backend.Peer
		}{d, id}
		registry.Unlock()
		p.signal = a.connect(p.handle, signal, cb, p.token, 0, 0)
	}
	return id, nil
}
func (d *driver) Destroy(id backend.Peer) error {
	p, err := d.lookup(id)
	if err != nil {
		return err
	}
	delete(d.peers, id)
	registry.Lock()
	delete(registry.entries, p.token)
	registry.Unlock()
	if p.signal != 0 {
		d.api.disconnect(p.handle, p.signal)
	}
	if p.kind == backend.Form {
		// Fence all child callback roots before destroying their native parent.
		for child, q := range d.peers {
			if q.parent == id {
				_ = d.Destroy(child)
			}
		}
		d.api.windowDestroy(p.handle)
	} else if parent := d.peers[p.parent]; parent != nil {
		d.api.remove(parent.fixed, p.handle)
	}
	d.api.unref(p.handle)
	return nil
}
func (d *driver) SetText(id backend.Peer, s string) error {
	if err := validText(s); err != nil {
		return err
	}
	p, err := d.lookup(id)
	if err != nil {
		return err
	}
	switch p.kind {
	case backend.Form:
		d.api.title(p.handle, s)
	case backend.Edit:
		d.api.editText(p.handle, s)
	case backend.Button:
		d.api.buttonText(p.handle, s)
	case backend.Label:
		d.api.labelText(p.handle, s)
	}
	return nil
}
func (d *driver) Text(id backend.Peer) (string, error) {
	p, err := d.lookup(id)
	if err != nil {
		return "", err
	}
	switch p.kind {
	case backend.Form:
		return d.api.getTitle(p.handle), nil
	case backend.Edit:
		return d.api.getEdit(p.handle), nil
	case backend.Button:
		return d.api.getButton(p.handle), nil
	default:
		return d.api.getLabel(p.handle), nil
	}
}
func (d *driver) Show(id backend.Peer) error {
	p, err := d.lookup(id)
	if err != nil {
		return err
	}
	if p.kind == backend.Form {
		d.api.present(p.handle)
	} else {
		d.api.visible(p.handle, 1)
	}
	return nil
}
func (d *driver) Wake() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.ready || d.closed {
		return errors.New("GTK wake target unavailable")
	}
	d.pending.Store(true)
	d.api.wake(d.context)
	return nil
}
func (d *driver) Quit() { d.quit = true }
func (d *driver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	if !d.ready || d.closed {
		return nil, errors.New("GTK driver not ready")
	}
	for !d.quit {
		d.pending.Swap(false)
		if dispatch != nil {
			if p := dispatch(); p != nil {
				d.latch(p)
			}
		}
		if d.quit {
			break
		}
		if d.pending.Load() {
			continue
		}
		// wakeup is sticky through the next iteration, closing the check/sleep race.
		d.api.iteration(d.context, 1)
	}
	return d.failure, nil
}
func (d *driver) Close() error {
	if d.closed {
		return nil
	}
	d.mu.Lock()
	d.closed = true
	d.ready = false
	d.mu.Unlock()
	for id := range d.peers {
		_ = d.Destroy(id)
	}
	if d.context != 0 {
		d.api.contextUnref(d.context)
		d.context = 0
	}
	d.callbacks = backend.Callbacks{}
	return nil
}
