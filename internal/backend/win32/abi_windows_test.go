//go:build windows && amd64

package win32

import (
	"github.com/NetMoose/Golphi/internal/backend"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsAMD64ABI(t *testing.T) {
	if unsafe.Sizeof(message{}) != 48 || unsafe.Offsetof(message{}.w) != 16 || unsafe.Sizeof(windowClass{}) != 80 || unsafe.Offsetof(windowClass{}.instance) != 24 {
		t.Fatal("Win32 AMD64 ABI mismatch")
	}
}
func TestNewDoesNotLoadNative(t *testing.T) {
	d := New().(*driver)
	if len(d.dlls) != 0 || len(d.api) != 0 || d.atom != 0 || d.wake != 0 {
		t.Fatal("New loaded native runtime")
	}
	if err := d.Wake(); err == nil {
		t.Fatal("wake accepted before Init")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

// This is a native Windows regression, compiled here and runnable on target:
// golphi-win32.test.exe -test.v
func TestWakeSurvivesNativeMessageDrain(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	defer func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	if err := d.Wake(); err != nil {
		t.Fatal(err)
	}
	// Native modal loops can drain every posted message. They cannot consume the
	// manual-reset event awaited only by our outer safe dispatch loop.
	var m message
	for {
		r, _, _ := d.api["PeekMessageW"].Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		if r == 0 {
			break
		}
		d.api["DispatchMessageW"].Call(uintptr(unsafe.Pointer(&m)))
	}
	if r, _, _ := d.api["WaitForSingleObject"].Call(d.wake, 0); r != 0 {
		t.Fatalf("wake lost: wait=%x", r)
	}
	if _, err := d.call("ResetEvent", d.wake); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := d.api["WaitForSingleObject"].Call(d.wake, 0); r != 258 {
		t.Fatalf("reset did not clear event: %x", r)
	}
	q := backend.NewQueue(d.Wake)
	q.Open()
	defer q.Stop()
	posted := make(chan error, 1)
	started := false
	if p, err := d.Run(func() *backend.Panic {
		if !started {
			started = true
			go func() { posted <- q.Post(func() { d.Quit() }) }()
		}
		return q.Dispatch()
	}); p != nil || err != nil {
		t.Fatalf("Run: %v %v", p, err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
}
func TestDestroyFormRetiresChildren(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	defer func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	form, err := d.Create(backend.Config{Kind: backend.Form, Bounds: backend.Bounds{Width: 400, Height: 180}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := d.Create(backend.Config{Kind: backend.Edit, Parent: form, Text: "Привет 🌍", Bounds: backend.Bounds{Width: 100, Height: 28}})
	if err != nil {
		t.Fatal(err)
	}
	if text, err := d.Text(child); err != nil || text != "Привет 🌍" {
		t.Fatal(text, err)
	}
	if err := d.Destroy(form); err != nil {
		t.Fatal(err)
	}
	if len(d.peers) != 0 || len(d.byHWND) != 0 {
		t.Fatal("cascaded native children retained in registry")
	}
}

func TestCreateOverflowBeforeNativeAllocation(t *testing.T) {
	d := New().(*driver)
	d.dpi = 192
	// No native APIs have been loaded. Any native access before rejection would
	// panic, and peer identity/resource maps must remain untouched.
	if _, err := d.Create(backend.Config{Kind: backend.Form, Bounds: backend.Bounds{Width: int(^uint(0) >> 1), Height: 180}}); err == nil {
		t.Fatal("accepted overflowing size")
	}
	if d.next != 0 || len(d.peers) != 0 || len(d.byHWND) != 0 {
		t.Fatal("invalid size mutated identity/resources")
	}
}
