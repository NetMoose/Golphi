//go:build linux && amd64

package gtk4

import (
	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/ebitengine/purego"
	"os"
	"runtime"
	"testing"
	"time"
)

// Opt in: this creates a self-closing native window on the selected display.
func TestNative(t *testing.T) {
	if os.Getenv("GOLPHI_GTK_NATIVE") != "1" {
		t.Skip("set GOLPHI_GTK_NATIVE=1 for native GTK test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	var events []backend.Event
	var failed *backend.Panic
	if err := d.Init(backend.Callbacks{Event: func(e backend.Event) { events = append(events, e) }, Failure: func(p *backend.Panic) { failed = p }}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	create := func(c backend.Config) backend.Peer {
		id, err := d.Create(c)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	form := create(backend.Config{Kind: backend.Form, Text: "Проверка GTK 🌍", Bounds: backend.Bounds{Width: 400, Height: 180}})
	edit := create(backend.Config{Kind: backend.Edit, Parent: form, Text: "Привет 🌍", Bounds: backend.Bounds{X: 16, Y: 16, Width: 368, Height: 28}})
	button := create(backend.Config{Kind: backend.Button, Parent: form, Text: "Кнопка", Bounds: backend.Bounds{X: 16, Y: 56, Width: 160, Height: 32}})
	label := create(backend.Config{Kind: backend.Label, Parent: form, Text: "Метка", Bounds: backend.Bounds{X: 16, Y: 104, Width: 368, Height: 32}})
	if len(events) != 0 {
		t.Fatal("construction delivered events", events)
	}
	for _, id := range []backend.Peer{form, edit, button, label} {
		if err := d.Show(id); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	var resizable func(uintptr) int32
	var emit func(uintptr, string)
	var width, height func(uintptr) int32
	purego.RegisterLibFunc(&resizable, lib, "gtk_window_get_resizable")
	purego.RegisterLibFunc(&emit, lib, "g_signal_emit_by_name")
	purego.RegisterLibFunc(&width, lib, "gtk_widget_get_width")
	purego.RegisterLibFunc(&height, lib, "gtk_widget_get_height")
	if resizable(d.peers[form].handle) != 0 {
		t.Fatal("window resizable")
	}
	q := backend.NewQueue(d.Wake)
	q.Open()
	ran := false
	if err := q.Post(func() {
		ran = true
		if err := d.SetText(edit, "Изменено 🙂"); err != nil {
			t.Fatal(err)
		}
		emit(d.peers[button].handle, "clicked")
	}); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("inline post")
	}
	done := make(chan struct{})
	go func() { time.Sleep(120 * time.Millisecond); _ = q.Post(func() { close(done); d.Quit() }) }()
	if p, err := d.Run(q.Dispatch); err != nil || p != nil {
		t.Fatalf("run: %v %v", p, err)
	}
	<-done
	q.Stop()
	if !ran || failed != nil {
		t.Fatal("dispatch or failure", ran, failed)
	}
	if s, err := d.Text(edit); err != nil || s != "Изменено 🙂" {
		t.Fatal(s, err)
	}
	if len(events) < 2 || events[len(events)-1].Kind != backend.Click {
		t.Fatal("signals", events)
	}
	if w, h := width(d.peers[form].fixed), height(d.peers[form].fixed); w != 400 || h != 180 {
		t.Fatalf("client allocation=%dx%d", w, h)
	}
	var allocatedWidth, allocatedHeight func(uintptr) int32
	purego.RegisterLibFunc(&allocatedWidth, lib, "gtk_widget_get_allocated_width")
	purego.RegisterLibFunc(&allocatedHeight, lib, "gtk_widget_get_allocated_height")
	// SetBounds sizes are minimums under the user-approved MVP policy. Native
	// appearance is preserved; positions and the form client size remain exact.
	type rect struct{ X, Y, Width, Height float32 }
	var computeBounds func(uintptr, uintptr, *rect) int32
	purego.RegisterLibFunc(&computeBounds, lib, "gtk_widget_compute_bounds")
	previousBottom := float64(0)
	for _, geometry := range []struct {
		id            backend.Peer
		x, y          float64
		width, height int32
	}{{edit, 16, 16, 368, 28}, {button, 16, 56, 160, 32}, {label, 16, 104, 368, 32}} {
		w, h := allocatedWidth(d.peers[geometry.id].handle), allocatedHeight(d.peers[geometry.id].handle)
		var bounds rect
		if computeBounds(d.peers[geometry.id].handle, d.peers[form].fixed, &bounds) == 0 {
			t.Fatal("cannot compute widget bounds")
		}
		x, y := float64(bounds.X), float64(bounds.Y)
		if bounds.Width != float32(w) || bounds.Height != float32(h) {
			t.Errorf("computed bounds differ from widget allocation: peer=%d bounds=%+v allocated=%dx%d", geometry.id, bounds, w, h)
		}
		t.Logf("accepted geometry peer=%d kind=%d requested=(%.0f,%.0f,%d,%d) actual=(%.0f,%.0f,%d,%d)", geometry.id, d.peers[geometry.id].kind, geometry.x, geometry.y, geometry.width, geometry.height, x, y, w, h)
		if w < geometry.width || h < geometry.height {
			t.Errorf("native widget smaller than requested minimum: peer=%d actual=%dx%d minimum=%dx%d", geometry.id, w, h, geometry.width, geometry.height)
		}
		if x != geometry.x || y != geometry.y {
			t.Errorf("native widget moved: peer=%d actual=(%v,%v) expected=(%v,%v)", geometry.id, x, y, geometry.x, geometry.y)
		}
		if x < 0 || y < 0 || x+float64(w) > 400 || y+float64(h) > 180 {
			t.Errorf("native widget clipped by client rectangle: peer=%d", geometry.id)
		}
		if y < previousBottom {
			t.Errorf("native widgets overlap: peer=%d", geometry.id)
		}
		previousBottom = y + float64(h)
	}
	d.quit = false
	var closeWindow func(uintptr)
	purego.RegisterLibFunc(&closeWindow, lib, "gtk_window_close")
	closeWindow(d.peers[form].handle)
	if events[len(events)-1].Kind != backend.CloseRequest {
		t.Fatal("native close request missing", events)
	}
	sentinel := &struct{}{}
	d.quit = false
	d.callbacks.Event = func(backend.Event) { panic(sentinel) }
	if err := d.SetText(edit, "Паника"); err != nil {
		t.Fatal(err)
	}
	if failed == nil || failed.Value != sentinel || len(failed.Stack) == 0 {
		t.Fatal("FFI panic not latched immediately", failed)
	}
	if p, err := d.Run(nil); err != nil || p != failed {
		t.Fatal("Run lost FFI panic", p, err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if len(d.peers) != 0 {
		t.Fatal("peers retained")
	}
	registry.Lock()
	n := len(registry.entries)
	registry.Unlock()
	if n != 0 {
		t.Fatal("callback roots retained", n)
	}
}
