//go:build windows && amd64

package win32

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

const (
	wmClose    = 0x10
	wmCommand  = 0x111
	formStyle  = 0x00c80000 | 0x00080000 | 0x00020000
	childStyle = 0x40000000 | 0x10000000
)

type rect struct{ left, top, right, bottom int32 }
type point struct{ x, y int32 }
type message struct {
	hwnd    uintptr
	msg     uint32
	w, l    uintptr
	time    uint32
	pt      point
	private uint32
}
type windowClass struct {
	size, style                        uint32
	proc                               uintptr
	clsExtra, wndExtra                 int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}
type peer struct {
	hwnd   uintptr
	kind   backend.Kind
	parent backend.Peer
}
type driver struct {
	api         map[string]*syscall.Proc
	dlls        []*syscall.DLL
	cb          backend.Callbacks
	peers       map[backend.Peer]peer
	byHWND      map[uintptr]backend.Peer
	next        backend.Peer
	instance    uintptr
	class       []uint16
	atom        uintptr
	dpi         uint32
	previousDPI uintptr
	failure     *backend.Panic
	quitting    bool
	// Serializes Wake with destruction of its HWND, including failed Init cleanup.
	wakeMu sync.Mutex
	wake   uintptr
}

var trampoline struct {
	sync.Once
	address uintptr
}
var registry struct {
	sync.RWMutex
	owner *driver
}
var _ backend.Driver = (*driver)(nil)

// New only allocates Go state; Init performs all native loading.
func New() backend.Driver {
	return &driver{api: make(map[string]*syscall.Proc), peers: make(map[backend.Peer]peer), byHWND: make(map[uintptr]backend.Peer)}
}
func (d *driver) call(name string, args ...uintptr) (uintptr, error) {
	r, _, e := d.api[name].Call(args...)
	if r == 0 {
		if e != syscall.Errno(0) {
			return r, fmt.Errorf("%s: %w", name, e)
		}
		return r, fmt.Errorf("%s failed", name)
	}
	return r, nil
}
func ptr(p *uint16) uintptr { return uintptr(unsafe.Pointer(p)) }
func (d *driver) Init(cb backend.Callbacks) error {
	d.cb = cb
	for _, lib := range []struct {
		name    string
		symbols []string
	}{
		{"user32.dll", []string{"RegisterClassExW", "UnregisterClassW", "CreateWindowExW", "DestroyWindow", "DefWindowProcW", "PeekMessageW", "MsgWaitForMultipleObjectsEx", "TranslateMessage", "DispatchMessageW", "ShowWindow", "SetWindowTextW", "GetWindowTextLengthW", "GetWindowTextW", "AdjustWindowRectExForDpi", "GetDpiForSystem", "SetThreadDpiAwarenessContext", "LoadCursorW"}},
		{"kernel32.dll", []string{"GetModuleHandleW", "SetLastError", "CreateEventW", "SetEvent", "ResetEvent", "CloseHandle", "WaitForSingleObject"}}} {
		dll, err := syscall.LoadDLL(lib.name)
		if err != nil {
			return err
		}
		d.dlls = append(d.dlls, dll)
		for _, name := range lib.symbols {
			p, err := dll.FindProc(name)
			if err != nil {
				return err
			}
			d.api[name] = p
		}
	}
	// System-aware context fixes scaling at startup; per-monitor transitions are outside MVP.
	previous, err := d.call("SetThreadDpiAwarenessContext", ^uintptr(1))
	if err != nil {
		return err
	}
	d.previousDPI = previous
	dpi, err := d.call("GetDpiForSystem")
	if err != nil {
		return err
	}
	d.dpi = uint32(dpi)
	d.instance, err = d.call("GetModuleHandleW", 0)
	if err != nil {
		return err
	}
	registry.Lock()
	if registry.owner != nil {
		registry.Unlock()
		return errors.New("win32: backend already initialized")
	}
	registry.owner = d
	registry.Unlock()
	trampoline.Do(func() { trampoline.address = syscall.NewCallback(wndProc) })
	d.class, _ = encodeText(fmt.Sprintf("GolphiMVP_%x", d.instance))
	cursor, _ := d.call("LoadCursorW", 0, 32512)
	wc := windowClass{size: uint32(unsafe.Sizeof(windowClass{})), proc: trampoline.address, instance: d.instance, cursor: cursor, background: 6, name: &d.class[0]}
	d.atom, err = d.call("RegisterClassExW", uintptr(unsafe.Pointer(&wc)))
	runtime.KeepAlive(wc)
	if err != nil {
		return err
	}
	h, err := d.call("CreateEventW", 0, 1, 0, 0)
	if err != nil {
		return err
	}
	d.wakeMu.Lock()
	d.wake = h
	d.wakeMu.Unlock()
	return nil
}

func wndProc(hwnd uintptr, msg uint32, w, l uintptr) (result uintptr) {
	registry.RLock()
	d := registry.owner
	registry.RUnlock()
	if d == nil {
		return 0
	}
	failure := backend.Capture(func() {
		if msg == wmClose {
			if id, ok := d.byHWND[hwnd]; ok {
				d.emit(backend.Event{Kind: backend.CloseRequest, Peer: id})
				return
			}
		}
		if msg == wmCommand && l != 0 {
			if id, ok := d.byHWND[l]; ok && d.failure == nil {
				p := d.peers[id]
				code := uint16(w >> 16)
				if p.kind == backend.Button && code == 0 {
					d.emit(backend.Event{Kind: backend.Click, Peer: id})
				}
				if p.kind == backend.Edit && code == 0x300 {
					s, err := d.Text(id)
					if err != nil {
						panic(err)
					}
					d.emit(backend.Event{Kind: backend.Change, Peer: id, Text: s})
				}
			}
		}
		result, _, _ = d.api["DefWindowProcW"].Call(hwnd, uintptr(msg), w, l)
	})
	if failure != nil {
		d.captureFailure(failure)
		return 0
	}
	return result
}
func (d *driver) captureFailure(p *backend.Panic) {
	if d.failure == nil {
		d.failure = p
		d.Quit()
		if d.cb.Failure != nil {
			d.cb.Failure(p)
		}
	}
}
func (d *driver) emit(e backend.Event) {
	if d.failure == nil && !d.quitting && d.cb.Event != nil {
		d.cb.Event(e)
	}
}
func (d *driver) Create(c backend.Config) (backend.Peer, error) {
	if c.Bounds.Width <= 0 || c.Bounds.Height <= 0 {
		return 0, errors.New("win32: invalid bounds")
	}
	x, err := scale(c.Bounds.X, d.dpi)
	if err != nil {
		return 0, err
	}
	y, err := scale(c.Bounds.Y, d.dpi)
	if err != nil {
		return 0, err
	}
	w, err := scale(c.Bounds.Width, d.dpi)
	if err != nil {
		return 0, err
	}
	h, err := scale(c.Bounds.Height, d.dpi)
	if err != nil {
		return 0, err
	}
	if w == 0 || h == 0 {
		return 0, errors.New("win32: scaled size must be positive")
	}
	text, err := encodeText(c.Text)
	if err != nil {
		return 0, err
	}
	var parent uintptr
	class := d.class
	style := uintptr(formStyle)
	ex := uintptr(0)
	if c.Kind != backend.Form {
		p, ok := d.peers[c.Parent]
		if !ok || p.kind != backend.Form {
			return 0, errors.New("win32: invalid parent")
		}
		parent = p.hwnd
		style = childStyle
		switch c.Kind {
		case backend.Edit:
			class, _ = encodeText("EDIT")
			style |= 0x00800000 | 0x00010000 | 0x80
		case backend.Button:
			class, _ = encodeText("BUTTON")
			style |= 0x00010000
		case backend.Label:
			class, _ = encodeText("STATIC")
		default:
			return 0, errors.New("win32: invalid kind")
		}
	} else if c.Parent != 0 {
		return 0, errors.New("win32: form cannot have a parent")
	}
	if c.Kind == backend.Form {
		r := rect{right: int32(w), bottom: int32(h)}
		if _, err = d.call("AdjustWindowRectExForDpi", uintptr(unsafe.Pointer(&r)), style, 0, ex, uintptr(d.dpi)); err != nil {
			return 0, err
		}
		w, err = frameExtent(r.left, r.right)
		if err != nil {
			return 0, err
		}
		h, err = frameExtent(r.top, r.bottom)
		if err != nil {
			return 0, err
		}
	}
	d.next++
	id := d.next
	menu := uintptr(0)
	if c.Kind != backend.Form {
		menu = uintptr(id)
	}
	handle, err := d.call("CreateWindowExW", ex, ptr(&class[0]), ptr(&text[0]), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, menu, d.instance, 0)
	runtime.KeepAlive(text)
	runtime.KeepAlive(class)
	if err != nil {
		return 0, err
	}
	d.peers[id] = peer{handle, c.Kind, c.Parent}
	d.byHWND[handle] = id
	return id, nil
}
func (d *driver) get(id backend.Peer) (peer, error) {
	p, ok := d.peers[id]
	if !ok {
		return peer{}, errors.New("win32: unknown peer")
	}
	return p, nil
}
func (d *driver) Destroy(id backend.Peer) error {
	p, err := d.get(id)
	if err != nil {
		return err
	}
	// DestroyWindow cascades to children. Disable the entire subtree first,
	// and retire every associated identity only after native destruction succeeds.
	ids := []backend.Peer{id}
	for childID, child := range d.peers {
		if child.parent == id {
			ids = append(ids, childID)
		}
	}
	for _, childID := range ids {
		delete(d.byHWND, d.peers[childID].hwnd)
	}
	_, err = d.call("DestroyWindow", p.hwnd)
	if err == nil {
		for _, childID := range ids {
			delete(d.peers, childID)
		}
	}
	return err
}
func (d *driver) SetText(id backend.Peer, s string) error {
	p, err := d.get(id)
	if err != nil {
		return err
	}
	text, err := encodeText(s)
	if err != nil {
		return err
	}
	_, err = d.call("SetWindowTextW", p.hwnd, ptr(&text[0]))
	runtime.KeepAlive(text)
	return err
}
func (d *driver) Text(id backend.Peer) (string, error) {
	p, err := d.get(id)
	if err != nil {
		return "", err
	}
	d.api["SetLastError"].Call(0)
	n, _, nativeErr := d.api["GetWindowTextLengthW"].Call(p.hwnd)
	if n == 0 && nativeErr != syscall.Errno(0) {
		return "", fmt.Errorf("GetWindowTextLengthW: %w", nativeErr)
	}
	buf := make([]uint16, n+1)
	d.api["SetLastError"].Call(0)
	read, _, nativeErr := d.api["GetWindowTextW"].Call(p.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if read == 0 && nativeErr != syscall.Errno(0) {
		return "", fmt.Errorf("GetWindowTextW: %w", nativeErr)
	}
	return string(utf16.Decode(buf[:read])), nil
}
func (d *driver) Show(id backend.Peer) error {
	p, err := d.get(id)
	if err != nil {
		return err
	}
	d.api["ShowWindow"].Call(p.hwnd, 5)
	return nil
}
func (d *driver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	for !d.quitting {
		// Reset before dispatch: a concurrent successful Wake either supplies work
		// to this dispatch (under Queue's lock) or leaves the event signaled.
		if _, err := d.call("ResetEvent", d.wake); err != nil {
			return d.failure, err
		}
		if p := dispatch(); p != nil {
			d.captureFailure(p)
			break
		}
		if d.quitting {
			break
		}
		var m message
		r, _, _ := d.api["PeekMessageW"].Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		// PeekMessage can service sent messages and nested native loops, but unlike
		// GetMessage it returns afterwards even if that nested loop drained input.
		if d.quitting {
			break
		}
		if r != 0 {
			if m.msg == 0x12 {
				break
			}
			d.api["TranslateMessage"].Call(uintptr(unsafe.Pointer(&m)))
			d.api["DispatchMessageW"].Call(uintptr(unsafe.Pointer(&m)))
			continue
		}
		event := d.wake
		r, _, err := d.api["MsgWaitForMultipleObjectsEx"].Call(1, uintptr(unsafe.Pointer(&event)), 0xffffffff, 0x04ff, 4)
		if r == 0xffffffff {
			return d.failure, fmt.Errorf("MsgWaitForMultipleObjectsEx: %w", err)
		}
	}
	return d.failure, nil
}
func (d *driver) Quit() {
	d.quitting = true
	// The manual-reset event survives nested native message loops.
	_ = d.Wake()
}
func (d *driver) Wake() error {
	d.wakeMu.Lock()
	defer d.wakeMu.Unlock()
	if d.wake == 0 {
		return errors.New("win32: wake target unavailable")
	}
	_, err := d.call("SetEvent", d.wake)
	return err
}
func (d *driver) Close() error {
	var errs []error
	d.cb = backend.Callbacks{}
	d.quitting = true
	// Peers normally have been independently attempted by the core; retry native
	// resources whose earlier Destroy failed, with callback resolution disabled.
	// Children first avoids invalid handles when native parent destruction
	// cascades. Each independent failure still allows all remaining attempts.
	ids := make([]backend.Peer, 0, len(d.peers))
	for id, p := range d.peers {
		if p.kind != backend.Form {
			ids = append(ids, id)
		}
	}
	for id, p := range d.peers {
		if p.kind == backend.Form {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if _, ok := d.peers[id]; ok {
			if err := d.Destroy(id); err != nil {
				errs = append(errs, err)
			}
		}
	}

	d.wakeMu.Lock()
	if d.wake != 0 {
		if _, err := d.call("CloseHandle", d.wake); err != nil {
			errs = append(errs, err)
		}
		d.wake = 0
	}
	d.wakeMu.Unlock()
	if d.atom != 0 {
		if _, err := d.call("UnregisterClassW", ptr(&d.class[0]), d.instance); err != nil {
			errs = append(errs, err)
		} else {
			d.atom = 0
		}
	}
	registry.Lock()
	if registry.owner == d && d.atom == 0 {
		registry.owner = nil
	}
	registry.Unlock()
	if d.previousDPI != 0 {
		if _, err := d.call("SetThreadDpiAwarenessContext", d.previousDPI); err != nil {
			errs = append(errs, err)
		}
		d.previousDPI = 0
	}
	// If native teardown failed and the class is still registered, retain its
	// default-procedure DLL and inert registry entry. No application sink is
	// retained, and late messages remain ABI-safe rather than calling freed code.
	if d.atom != 0 {
		return errors.Join(errs...)
	}
	for _, dll := range d.dlls {
		if err := dll.Release(); err != nil {
			errs = append(errs, err)
		}
	}
	d.dlls = nil
	return errors.Join(errs...)
}
