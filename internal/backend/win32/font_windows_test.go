//go:build windows && amd64

package win32

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

const wmGetFont = 0x0031

var getObjectW = syscall.NewLazyDLL("gdi32.dll").NewProc("GetObjectW")

func TestWindowsFontABI(t *testing.T) {
	if unsafe.Sizeof(logFont{}) != 92 || unsafe.Offsetof(logFont{}.faceName) != 28 ||
		unsafe.Sizeof(nonClientMetrics{}) != 504 || unsafe.Offsetof(nonClientMetrics{}.messageFont) != 408 ||
		unsafe.Offsetof(nonClientMetrics{}.paddedBorderWidth) != 500 {
		t.Fatal("LOGFONTW or NONCLIENTMETRICSW ABI mismatch")
	}
}

func newFontTestDriver(t *testing.T) *driver {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	d := New().(*driver)
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	return d
}

func assertNativeFont(t *testing.T, d *driver, h uintptr, dpi uint32, live bool) {
	t.Helper()
	var got logFont
	size := uintptr(unsafe.Sizeof(got))
	n, _, nativeErr := getObjectW.Call(h, size, uintptr(unsafe.Pointer(&got)))
	if !live {
		if n != 0 {
			t.Fatalf("HFONT %x still readable after Close", h)
		}
		return
	}
	if n != size {
		t.Fatalf("GetObjectW(HFONT %x): %d %v", h, n, nativeErr)
	}
	var want nonClientMetrics
	want.size = uint32(unsafe.Sizeof(want))
	ok, _, nativeErr := d.api["SystemParametersInfoForDpi"].Call(0x29, uintptr(want.size), uintptr(unsafe.Pointer(&want)), 0, uintptr(dpi))
	if ok == 0 {
		t.Fatalf("SystemParametersInfoForDpi(%d): %v", dpi, nativeErr)
	}
	gotFace := syscall.UTF16ToString(got.faceName[:])
	wantFace := syscall.UTF16ToString(want.messageFont.faceName[:])
	// Ignore padding beyond the NUL terminator of the face-name string.
	got.faceName, want.messageFont.faceName = [32]uint16{}, [32]uint16{}
	if gotFace != wantFace || got != want.messageFont {
		t.Fatalf("HFONT %x at %d DPI: got %q %+v, want %q %+v", h, dpi, gotFace, got, wantFace, want.messageFont)
	}
}

func TestWindowsFontAtRequestedDPI(t *testing.T) {
	for _, dpi := range []uint32{96, 144} {
		t.Run(fmt.Sprint(dpi), func(t *testing.T) {
			d := newFontTestDriver(t)
			if err := d.load(); err != nil {
				t.Fatal(err)
			}
			d.dpi = dpi
			var err error
			d.font, err = d.createSystemFont()
			if err != nil {
				t.Fatal(err)
			}
			assertNativeFont(t, d, d.font, dpi, true)
		})
	}
}

func makeFontPeer(t *testing.T, d *driver, kind backend.Kind, parent backend.Peer) backend.Peer {
	t.Helper()
	id, err := d.Create(backend.Config{Kind: kind, Parent: parent, Text: "Ёжик, щука 🌍", Bounds: backend.Bounds{Width: 200, Height: 60}})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertPeerFont(t *testing.T, d *driver, id backend.Peer) {
	t.Helper()
	h := d.peers[id].hwnd
	f, _, _ := d.api["SendMessageW"].Call(h, wmGetFont, 0, 0)
	if f == 0 || f != d.font {
		t.Fatalf("peer %d: WM_GETFONT = %x, want %x", id, f, d.font)
	}
	if s, err := d.Text(id); s != "Ёжик, щука 🌍" || err != nil {
		t.Fatalf("peer %d: text %q, %v", id, s, err)
	}
	assertNativeFont(t, d, f, d.dpi, true)
}

func TestWindowsFontLifetime(t *testing.T) {
	d := newFontTestDriver(t)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	t.Logf("FONT_DPI=%d", d.dpi)
	if expect := os.Getenv("GOLPHI_WIN01_EXPECT_DPI"); expect != "" {
		dpi, err := strconv.ParseUint(expect, 10, 32)
		if err != nil || dpi != uint64(d.dpi) {
			t.Fatalf("startup DPI = %d, expected %q", d.dpi, expect)
		}
	}
	h := d.font
	form := makeFontPeer(t, d, backend.Form, 0)
	left := makeFontPeer(t, d, backend.Panel, form)
	right := makeFontPeer(t, d, backend.Panel, form)
	apply := func(c backend.TreeChange) backend.TreeBinding {
		t.Helper()
		var b backend.TreeBinding
		if fail := backend.ApplyTree(d, c, func(r backend.TreeResult) { b = r.Bindings[0] }, func(f *backend.TreeFailure) { t.Fatal(f) }); fail != nil {
			t.Fatal(fail)
		}
		return b
	}
	for _, kind := range []backend.Kind{backend.Button, backend.Edit, backend.Label} {
		id := makeFontPeer(t, d, kind, left)
		assertPeerFont(t, d, id)
		node := backend.TreeNode{ID: 1, Peer: id, Kind: kind, Text: "Ёжик, щука 🌍", Bounds: backend.Bounds{Width: 200, Height: 60}}
		move := apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: right, Nodes: []backend.TreeNode{node}})
		if move.Peer != id {
			t.Fatal("retained move replaced peer")
		}
		assertPeerFont(t, d, id)
		detach := apply(backend.TreeChange{FromAttached: true, Nodes: []backend.TreeNode{node}})
		assertPeerFont(t, d, id)
		if err := d.releaseRetired(); err != nil {
			t.Fatal(err)
		}
		assertNativeFont(t, d, h, d.dpi, true)
		node.Peer, node.Input = 0, detach.Input
		fresh := apply(backend.TreeChange{ToAttached: true, Target: left, Nodes: []backend.TreeNode{node}})
		if fresh.Peer == id {
			t.Fatal("reattachment reused retired peer")
		}
		assertPeerFont(t, d, fresh.Peer)
		if err := d.Destroy(fresh.Peer); err != nil {
			t.Fatal(err)
		}
		assertNativeFont(t, d, h, d.dpi, true)
	}

	before := len(d.peers)
	bad := backend.TreeChange{ToAttached: true, Target: left, Nodes: []backend.TreeNode{
		{ID: 1, Kind: backend.Panel, Bounds: backend.Bounds{Width: 200, Height: 60}},
		{ID: 2, Parent: 1, Kind: backend.Edit, Bounds: backend.Bounds{Width: 200, Height: 60}},
		{ID: 3, Parent: 1, Kind: backend.Label, Text: "invalid\x00text", Bounds: backend.Bounds{Width: 200, Height: 60}},
	}}
	fail := backend.ApplyTree(d, bad, func(backend.TreeResult) { t.Fatal("published failed attachment") }, func(f *backend.TreeFailure) { t.Fatal(f) })
	if fail == nil || fail.Fatal() || len(fail.Cleanup) != 0 || len(d.peers) != before {
		t.Fatalf("failed attachment did not clean up its new peers: %v", fail)
	}
	assertNativeFont(t, d, h, d.dpi, true)

	// Exercise the hidden allocation path directly, before Commit's show.
	d.fenced = true
	hidden := makeFontPeer(t, d, backend.Edit, left)
	assertPeerFont(t, d, hidden)
	style := syscall.NewLazyDLL("user32.dll").NewProc("GetWindowLongPtrW")
	got, _, _ := style.Call(d.peers[hidden].hwnd, ^uintptr(15)) // GWL_STYLE
	if got == 0 || got&0x10000000 != 0 {
		t.Fatalf("fenced Edit style = %x, want valid style with WS_VISIBLE clear", got)
	}
	d.fenced = false

	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if d.font != 0 || len(d.peers) != 0 {
		t.Fatal("Close retained font or font users")
	}
	assertNativeFont(t, d, h, 0, false)
	d.api["DeleteObject"] = nil
	if err := d.Close(); err != nil {
		t.Fatalf("repeat Close: %v", err)
	}
}

func TestWindowsFontStartupFaults(t *testing.T) {
	for _, symbol := range []string{"SystemParametersInfoForDpi", "CreateFontIndirectW", "CreateEventW"} {
		t.Run(symbol, func(t *testing.T) {
			d := newFontTestDriver(t)
			if err := d.load(); err != nil {
				t.Fatal(err)
			}
			// None of SPI_GETNONCLIENTMETRICS, a LOGFONTW pointer, or
			// CreateEventW's NULL LPSECURITY_ATTRIBUTES is a valid HWND.
			d.api[symbol] = d.api["IsWindow"]
			err := d.initNative()
			if err == nil || !strings.Contains(err.Error(), symbol) {
				t.Fatalf("Init error = %v, want %s failure", err, symbol)
			}
			if d.wake != 0 || d.Wake() == nil {
				t.Fatal("failed startup left a native wake target")
			}
			h := d.font
			if (h != 0) != (symbol == "CreateEventW") {
				t.Fatalf("unexpected acquired HFONT %x", h)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			if d.font != 0 || d.atom != 0 || d.previousDPI != 0 || len(d.dlls) != 0 {
				t.Fatal("partial startup retained resources")
			}
			if h != 0 {
				assertNativeFont(t, d, h, 0, false)
			}
		})
	}
}

func TestWindowsFontCleanupFaults(t *testing.T) {
	for _, fault := range []string{"delete-error", "delete-panic", "event-panic", "live-edit"} {
		t.Run(fault, func(t *testing.T) {
			d := newFontTestDriver(t)
			if err := d.Init(backend.Callbacks{}); err != nil {
				t.Fatal(err)
			}
			form := makeFontPeer(t, d, backend.Form, 0)
			edit := makeFontPeer(t, d, backend.Edit, form)
			h, hwnd, event := d.font, d.peers[edit].hwnd, d.wake
			del, destroy, closeHandle := d.api["DeleteObject"], d.api["DestroyWindow"], d.api["CloseHandle"]
			defer func() {
				d.api["DeleteObject"], d.api["DestroyWindow"], d.api["CloseHandle"] = del, destroy, closeHandle
			}()
			switch fault {
			case "delete-error":
				d.api["DeleteObject"] = d.api["IsWindow"]
			case "delete-panic":
				d.api["DeleteObject"] = nil
			case "event-panic":
				d.api["CloseHandle"] = nil
				defer func() {
					if err := syscall.CloseHandle(syscall.Handle(event)); err != nil {
						t.Errorf("release injected event: %v", err)
					}
				}()
			case "live-edit":
				d.api["DestroyWindow"] = nil
				// A working DeleteObject would release the font if Close
				// failed to honor its live users.
			}
			err := d.Close()
			var failure backend.CleanupFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Close error = %v, want cleanup diagnostic", err)
			}
			switch fault {
			case "delete-error":
				if failure.Operation != "DeleteObject" || failure.Err == nil || failure.Panic != nil {
					t.Fatal(failure)
				}
			case "delete-panic", "event-panic", "live-edit":
				if failure.Panic == nil || failure.Panic.Value == nil || !strings.Contains(string(failure.Panic.Stack), "(*driver).Close") {
					t.Fatal(failure)
				}
			}
			if d.failure != nil || d.previousDPI != 0 || d.wake != 0 {
				t.Fatal("cleanup invented a callback panic or skipped an independent release")
			}
			if fault == "live-edit" {
				if live, _, _ := d.api["IsWindow"].Call(hwnd); live == 0 || !strings.Contains(err.Error(), "font") {
					t.Fatalf("surviving Edit or its diagnostic missing: %v", err)
				}
				assertPeerFont(t, d, edit)
			}
			assertNativeFont(t, d, h, d.dpi, fault != "event-panic")
			if fault != "event-panic" && (d.font != h || len(d.dlls) == 0) {
				t.Fatal("Close discarded the live font or its DLL")
			}
		})
	}
}

func TestWindowsFontPreservesCallbackFailure(t *testing.T) {
	for _, source := range []string{"handler", "Post"} {
		t.Run(source, func(t *testing.T) {
			d := newFontTestDriver(t)
			primary := errors.New("primary " + source)
			var observed *backend.Panic
			if err := d.Init(backend.Callbacks{
				Event:   func(backend.Event) { panic(primary) },
				Failure: func(p *backend.Panic) { observed = p },
			}); err != nil {
				t.Fatal(err)
			}
			form := makeFontPeer(t, d, backend.Form, 0)
			button := makeFontPeer(t, d, backend.Button, form)
			q := backend.NewQueue(d.Wake)
			q.Open()
			defer q.Stop()
			if source == "Post" {
				if err := q.Post(func() { panic(primary) }); err != nil {
					t.Fatal(err)
				}
			}
			p, err := d.Run(func() *backend.Panic {
				if source == "handler" {
					// BN_CLICKED with the HWND of our actual native Button.
					d.api["SendMessageW"].Call(d.peers[form].hwnd, wmCommand, uintptr(button), d.peers[button].hwnd)
				}
				return q.Dispatch()
			})
			q.Stop()
			if err != nil || p == nil || p.Value != primary || p != observed || len(p.Stack) == 0 || d.callbackDepth != 0 {
				t.Fatalf("Run panic = %+v, callback = %+v, err = %v", p, observed, err)
			}
			stack := string(p.Stack)
			del := d.api["DeleteObject"]
			defer func() { d.api["DeleteObject"] = del }()
			d.api["DeleteObject"] = nil
			var cleanup backend.CleanupFailure
			err = d.Close()
			if !errors.As(err, &cleanup) || cleanup.Operation != "DeleteObject" || cleanup.Panic == nil || !strings.Contains(string(cleanup.Panic.Stack), "deleteSystemFont") {
				t.Fatalf("Close lost cleanup panic: %v", err)
			}
			if d.failure != p || p.Value != primary || string(p.Stack) != stack {
				t.Fatal("cleanup replaced the callback panic")
			}
			if len(d.peers) != 0 || d.wake != 0 || d.atom != 0 || d.previousDPI != 0 {
				t.Fatal("cleanup panic interrupted independent native release")
			}
		})
	}
}

func TestWindowsFontVisualStyles(t *testing.T) {
	d := newFontTestDriver(t)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	form := makeFontPeer(t, d, backend.Form, 0)
	button := makeFontPeer(t, d, backend.Button, form)
	assertPeerFont(t, d, button)

	user32 := syscall.NewLazyDLL("user32.dll")
	updateWindow := user32.NewProc("UpdateWindow")
	d.api["ShowWindow"].Call(d.peers[form].hwnd, 4) // SW_SHOWNOACTIVATE
	updateWindow.Call(d.peers[form].hwnd)
	updateWindow.Call(d.peers[button].hwnd)

	uxtheme := syscall.NewLazyDLL("uxtheme.dll")
	app, _, _ := uxtheme.NewProc("IsAppThemed").Call()
	active, _, _ := uxtheme.NewProc("IsThemeActive").Call()
	buttonTheme, _, _ := uxtheme.NewProc("GetWindowTheme").Call(d.peers[button].hwnd)
	t.Logf("IS_APP_THEMED=%d IS_THEME_ACTIVE=%d BUTTON_THEME=%#x", app, active, buttonTheme)
	switch expect := os.Getenv("GOLPHI_WIN01_EXPECT_STYLES"); expect {
	case "":
	case "0":
		if app != 0 {
			t.Fatal("expected classic style")
		}
	case "1":
		if app == 0 || active == 0 || buttonTheme == 0 {
			t.Fatal("expected active visual styles and a themed native Button")
		}
	default:
		t.Fatalf("invalid GOLPHI_WIN01_EXPECT_STYLES %q", expect)
	}
}

func TestWindowsFontEditCreationPublishesOriginalProc(t *testing.T) {
	d := newFontTestDriver(t)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	form := makeFontPeer(t, d, backend.Form, 0)

	type callWindowProcMessage struct {
		lParam, wParam uintptr
		message        uint32
		hwnd           uintptr
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	nextHook := user32.NewProc("CallNextHookEx")
	if err := nextHook.Find(); err != nil {
		t.Fatal(err)
	}
	var observed, missing bool
	proc := syscall.NewCallback(func(code int32, wParam uintptr, m *callWindowProcMessage) uintptr {
		const emGetSel = 0x00b0
		if code >= 0 && m.message == emGetSel {
			p := d.peers[d.byHWND[m.hwnd]]
			if p.kind == backend.Edit {
				observed = true
				missing = missing || p.oldProc == 0
			}
		}
		r, _, _ := nextHook.Call(0, uintptr(code), wParam, uintptr(unsafe.Pointer(m)))
		return r
	})
	threadID, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId").Call()
	const whCallWndProc = 4
	hook, _, err := user32.NewProc("SetWindowsHookExW").Call(whCallWndProc, proc, 0, threadID)
	if hook == 0 {
		t.Fatalf("SetWindowsHookExW: %v", err)
	}
	defer func() {
		if ok, _, err := user32.NewProc("UnhookWindowsHookEx").Call(hook); ok == 0 {
			t.Errorf("UnhookWindowsHookEx: %v", err)
		}
	}()

	makeFontPeer(t, d, backend.Edit, form)
	if !observed || missing {
		t.Fatalf("Edit creation EM_GETSEL observed=%v, original window procedure missing=%v", observed, missing)
	}
}
