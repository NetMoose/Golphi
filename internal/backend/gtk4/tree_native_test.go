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

func TestNativeTree(t *testing.T) {
	if os.Getenv("GOLPHI_GTK_NATIVE") != "1" {
		t.Skip("set GOLPHI_GTK_NATIVE=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	events := 0
	if err := d.Init(backend.Callbacks{Event: func(backend.Event) { events++ }}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	create := func(k backend.Kind, p backend.Peer, w, h int) backend.Peer {
		id, e := d.Create(backend.Config{Kind: k, Parent: p, Bounds: backend.Bounds{Width: w, Height: h}})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	f := create(backend.Form, 0, 640, 480)
	a := create(backend.Panel, f, 200, 120)
	b := create(backend.Panel, f, 200, 120)
	state := backend.InputState{Text: "А😀Б𐐷В", Anchor: 4, Caret: 1}
	e, err := d.Create(backend.Config{Kind: backend.Edit, Parent: a, Text: state.Text, Input: &state, Bounds: backend.Bounds{Width: 180, Height: 30}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.SnapshotInput(e)
	if err != nil || s.Text != state.Text || s.Anchor != 4 || s.Caret != 1 {
		t.Fatalf("startup snapshot=%+v err=%v", s, err)
	}
	nodes := []backend.TreeNode{{ID: 1, Peer: e, Kind: backend.Edit, Text: state.Text, Bounds: backend.Bounds{Width: 180, Height: 30}}}
	var result backend.TreeResult
	apply := func(from, to bool, target backend.Peer) {
		t.Helper()
		if fail := backend.ApplyTree(d, backend.TreeChange{FromAttached: from, ToAttached: to, Target: target, Nodes: nodes}, func(r backend.TreeResult) { result = r }, func(f *backend.TreeFailure) { t.Error(f) }); fail != nil {
			t.Fatal(fail)
		}
	}
	lib, _ := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	var grab func(uintptr) int32
	var width func(uintptr) int32
	var transformUnref func(uintptr)
	purego.RegisterLibFunc(&grab, lib, "gtk_widget_grab_focus")
	purego.RegisterLibFunc(&width, lib, "gtk_widget_get_width")
	purego.RegisterLibFunc(&transformUnref, lib, "gsk_transform_unref")
	for _, id := range []backend.Peer{f, a, b, e} {
		if err := d.Show(id); err != nil {
			t.Fatal(err)
		}
	}
	// Establish mapped native widgets with ordinary iterations before measuring
	// the synchronous transaction itself (no pumping inside SetParent).
	for width(d.peers[a].handle) == 0 {
		d.api.iteration(d.context, 1)
	}
	// Complete initial window-manager activation before establishing the
	// selection fixture. This wait is outside the transaction under test.
	activationEnd := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(activationEnd) {
		d.api.iteration(d.context, 0)
		time.Sleep(time.Millisecond)
	}
	grab(d.peers[e].handle)
	d.api.selectRegion(d.peers[e].handle, 4, 1)
	var computeBounds func(uintptr, uintptr, *[4]float32) int32
	purego.RegisterLibFunc(&computeBounds, lib, "gtk_widget_compute_bounds")
	var rootBefore, rootAfter [4]float32
	if computeBounds(d.peers[f].fixed, d.peers[f].handle, &rootBefore) == 0 {
		t.Fatal("missing root bounds")
	}
	if os.Getenv("GTK_CSD") == "1" && rootBefore[1] <= 0 {
		t.Fatal("CSD fixture has no title offset", rootBefore)
	}
	oldFocus := d.api.windowFocus(d.peers[f].handle)
	apply(true, true, b)
	if computeBounds(d.peers[f].fixed, d.peers[f].handle, &rootAfter) == 0 || rootAfter != rootBefore {
		t.Fatalf("root placement changed before=%v after=%v", rootBefore, rootAfter)
	}
	t.Logf("root placement preserved: %v", rootAfter)
	previous := e
	e = result.Bindings[0].Peer
	nodes[0].Peer = e
	oldFocus = d.api.windowFocus(d.peers[f].handle)
	if !d.descendant(oldFocus, d.peers[e].handle) {
		t.Fatal("focus not synchronously restored")
	}
	if width(d.peers[e].handle) == 0 {
		t.Fatal("moved Edit not synchronously allocated")
	}

	if e == previous || result.Bindings[0].Generation == 0 {
		t.Fatal(result)
	}
	s, err = d.SnapshotInput(e)
	if err != nil || s.Anchor != 4 || s.Caret != 1 {
		t.Fatal(s, err)
	}
	// A failed readiness step must restore the former native allocation and
	// route before returning an ordinary error.
	allocate := d.api.allocate
	skip := true
	d.api.allocate = func(h uintptr, w, hh, base int32, tr uintptr) {
		if skip {
			skip = false
			if tr != 0 {
				transformUnref(tr)
			}
			return
		}
		allocate(h, w, hh, base, tr)
	}
	failedMove := backend.ApplyTree(d, backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: nodes}, func(backend.TreeResult) { t.Error("published unallocated widget") }, func(*backend.TreeFailure) { t.Error("fatal rollback") })
	d.api.allocate = allocate
	if failedMove == nil || failedMove.Fatal() {
		t.Fatalf("readiness failure=%v", failedMove)
	}
	if width(d.peers[e].handle) == 0 || d.peers[e].parent != b {
		t.Fatal("rollback did not restore working geometry")
	}
	// Fail after the replacement has taken focus/PRIMARY. Rollback must give
	// ownership back to the old realized entry before hiding the replacement.
	position := d.api.position
	lastReplacement := d.next
	d.api.position = func(h uintptr) int32 {
		for id, p := range d.peers {
			if id > lastReplacement && p.kind == backend.Edit && d.api.editableDelegate(p.handle) == h {
				return 0
			}
		}
		return position(h)
	}
	failedFocusedMove := backend.ApplyTree(d, backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: nodes}, func(backend.TreeResult) { t.Error("published bad replacement input") }, func(*backend.TreeFailure) { t.Error("replacement rollback fatal") })
	d.api.position = position
	if failedFocusedMove == nil || failedFocusedMove.Fatal() {
		t.Fatal(failedFocusedMove)
	}
	restored, restoreErr := d.SnapshotInput(e)
	if restoreErr != nil || restored.Anchor != 4 || restored.Caret != 1 || !restored.Focused {
		t.Fatalf("replacement rollback input: %+v %v", restored, restoreErr)
	}
	// Let unrelated frame work run only after asserting synchronous readiness;
	// later GTK work must not undo the restored selection or focus.
	until := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(until) {
		d.api.iteration(d.context, 0)
		time.Sleep(time.Millisecond)
	}
	if computeBounds(d.peers[f].fixed, d.peers[f].handle, &rootAfter) == 0 || rootAfter != rootBefore {
		t.Fatalf("later frame changed root placement: %v => %v", rootBefore, rootAfter)
	}
	s, err = d.SnapshotInput(e)
	if err != nil || s.Anchor != 4 || s.Caret != 1 || d.api.windowFocus(d.peers[f].handle) != oldFocus {
		t.Fatalf("latent focus/input change: %+v %v", s, err)
	}
	button := create(backend.Button, a, 80, 32)
	_ = d.Show(button)
	(&treeTransaction{d: d, form: d.peers[f].handle}).layout()
	grab(d.peers[button].handle)
	buttonFocus := d.api.windowFocus(d.peers[f].handle)
	savedNodes := nodes
	nodes = []backend.TreeNode{{ID: 2, Peer: button, Kind: backend.Button, Bounds: backend.Bounds{Width: 80, Height: 32}}}
	apply(true, true, b)
	if buttonFocus == 0 || d.api.windowFocus(d.peers[f].handle) != buttonFocus {
		t.Fatal("button focus not restored")
	}
	nodes = savedNodes
	apply(true, false, 0)
	if result.Bindings[0].Peer != 0 || result.Bindings[0].Input.Anchor != 4 {
		t.Fatal(result)
	}
	nodes[0].Peer = 0
	nodes[0].Input = result.Bindings[0].Input
	apply(false, false, 0)
	apply(false, true, a)
	if result.Bindings[0].Peer == 0 || result.Bindings[0].Peer == e {
		t.Fatal(result)
	}
	newID := result.Bindings[0].Peer
	if width(d.peers[newID].handle) == 0 {
		t.Fatal("attached Edit not synchronously allocated")
	}
	oldToken := d.peers[newID].token
	apply(false, false, 0)
	if events != 0 {
		t.Fatal("technical events", events)
	}
	// A partial new subtree must be removed on ordinary native allocation error.
	entryNew := d.api.entryNew
	calls := 0
	d.api.entryNew = func() uintptr {
		calls++
		if calls == 2 {
			return 0
		}
		return entryNew()
	}
	d.releaseRetired()
	count := len(d.peers)
	bad := backend.TreeChange{ToAttached: true, Target: a, Nodes: []backend.TreeNode{
		{ID: 10, Kind: backend.Panel, Bounds: backend.Bounds{Width: 100, Height: 100}},
		{ID: 11, Parent: 10, Kind: backend.Edit, Bounds: backend.Bounds{Width: 80, Height: 30}},
		{ID: 12, Parent: 10, Kind: backend.Edit, Bounds: backend.Bounds{Width: 80, Height: 30}}}}
	fail := backend.ApplyTree(d, bad, func(backend.TreeResult) { t.Error("published partial subtree") }, func(*backend.TreeFailure) { t.Error("fatal") })
	d.api.entryNew = entryNew
	if fail == nil || fail.Fatal() {
		t.Fatal(fail)
	}
	d.releaseRetired()
	// Partial-attempt peers must all have been released.
	if len(d.peers) != count {
		t.Fatalf("attempt leak peers=%d before=%d", len(d.peers), count)
	}
	// A first-attachment failure after showing the attempt must synchronously
	// remove it from the native parent, before deferred physical cleanup.
	nativeWidth := d.api.width
	last := d.next
	d.api.width = func(h uintptr) int32 {
		for id, p := range d.peers {
			if id > last && p.handle == h {
				return 0
			}
		}
		return nativeWidth(h)
	}
	failedAttach := backend.ApplyTree(d, backend.TreeChange{ToAttached: true, Target: a, Nodes: []backend.TreeNode{{ID: 20, Kind: backend.Edit, Bounds: backend.Bounds{Width: 80, Height: 30}}}}, func(backend.TreeResult) { t.Error("published failed attachment") }, func(*backend.TreeFailure) { t.Error("fatal attach") })
	d.api.width = nativeWidth
	if failedAttach == nil || failedAttach.Fatal() {
		t.Fatal(failedAttach)
	}
	for id, p := range d.peers {
		if id > last && d.api.widgetParent(p.handle) != 0 {
			t.Fatal("failed visible attempt remains parented")
		}
	}
	d.releaseRetired()
	callback(oldToken, backend.Click)
	if events != 1 {
		t.Fatal("current native route lost", events)
	}
	// Weak notification proves actual finalization occurs after the callback,
	// not merely registry removal. Trigger a real native changed signal.
	var weak func(uintptr, uintptr, uintptr)
	purego.RegisterLibFunc(&weak, lib, "g_object_weak_ref")
	finalized := 0
	gone := purego.NewCallback(func(uintptr, uintptr) { finalized++ })
	weak(d.peers[newID].handle, gone, 0)
	d.callbacks.Event = func(ev backend.Event) {
		if ev.Peer != newID {
			return
		}
		change := backend.TreeChange{FromAttached: true, Nodes: []backend.TreeNode{{ID: 1, Peer: newID, Kind: backend.Edit, Bounds: backend.Bounds{Width: 180, Height: 30}}}}
		if f := backend.ApplyTree(d, change, func(backend.TreeResult) {}, func(*backend.TreeFailure) { t.Error("fatal detach") }); f != nil {
			t.Error(f)
		}
		if finalized != 0 {
			t.Error("released within native callback")
		}
	}
	if err := d.SetText(newID, "callback"); err != nil {
		t.Fatal(err)
	}
	if finalized != 0 {
		t.Fatal("released at Go callback epilogue")
	}
	d.releaseRetired()
	if finalized != 1 {
		t.Fatal("retired native object not finalized exactly once", finalized)
	}
	before := events
	callback(oldToken, backend.Change)
	if events != before {
		t.Fatal("stale token delivered")
	}
	// Persistently failing native allocation cannot certify ordinary rollback.
	d.api.allocate = func(_ uintptr, _ int32, _ int32, _ int32, tr uintptr) {
		if tr != 0 {
			transformUnref(tr)
		}
	}
	fatalCalled := false
	fatalMove := backend.ApplyTree(d, backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: []backend.TreeNode{{ID: 30, Peer: button, Kind: backend.Button, Bounds: backend.Bounds{Width: 80, Height: 32}}}}, func(backend.TreeResult) { t.Error("published unusable tree") }, func(*backend.TreeFailure) {
		fatalCalled = true
		if !d.quit {
			t.Error("fatal native delivery not latched")
		}
	})
	d.api.allocate = allocate
	if fatalMove == nil || !fatalMove.Fatal() || !fatalCalled {
		t.Fatalf("unrestored allocation must be fatal: %v hook=%v", fatalMove, fatalCalled)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	registry.Lock()
	remaining := len(registry.entries)
	registry.Unlock()
	if len(d.peers) != 0 || len(d.retired) != 0 || remaining != 0 {
		t.Fatalf("cleanup peers=%d retired=%d registry=%d", len(d.peers), len(d.retired), remaining)
	}
}

// Removing Panel's scrolled viewport would let the huge child enlarge its
// ancestors or paint/hit outside the requested rectangle.
func TestNativePanelClipping(t *testing.T) {
	if os.Getenv("GOLPHI_GTK_NATIVE") != "1" {
		t.Skip("set GOLPHI_GTK_NATIVE=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	create := func(c backend.Config) backend.Peer {
		id, e := d.Create(c)
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	f := create(backend.Config{Kind: backend.Form, Bounds: backend.Bounds{Width: 400, Height: 300}})
	outer := create(backend.Config{Kind: backend.Panel, Parent: f, Bounds: backend.Bounds{Width: 260, Height: 180}})
	inner := create(backend.Config{Kind: backend.Panel, Parent: outer, Bounds: backend.Bounds{X: 10, Y: 10, Width: 140, Height: 90}})
	lib, _ := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	var cssNew, display func() uintptr
	var cssLoad func(uintptr, string, int64)
	var cssAdd func(uintptr, uintptr, uint32)
	var cssRemove func(uintptr, uintptr)
	purego.RegisterLibFunc(&cssNew, lib, "gtk_css_provider_new")
	purego.RegisterLibFunc(&display, lib, "gdk_display_get_default")
	purego.RegisterLibFunc(&cssLoad, lib, "gtk_css_provider_load_from_data")
	purego.RegisterLibFunc(&cssAdd, lib, "gtk_style_context_add_provider_for_display")
	purego.RegisterLibFunc(&cssRemove, lib, "gtk_style_context_remove_provider_for_display")
	css := cssNew()
	cssLoad(css, "scrolledwindow {background-color: #ffffff;}", -1)
	cssAdd(display(), css, 600)
	defer d.api.unref(css)
	defer cssRemove(display(), css)
	var drawing func() uintptr
	var drawFunc func(uintptr, uintptr, uintptr, uintptr)
	var rgb func(uintptr, float64, float64, float64)
	var paint func(uintptr)
	var pick func(uintptr, float64, float64, uint32) uintptr
	purego.RegisterLibFunc(&drawing, lib, "gtk_drawing_area_new")
	purego.RegisterLibFunc(&drawFunc, lib, "gtk_drawing_area_set_draw_func")
	purego.RegisterLibFunc(&rgb, lib, "cairo_set_source_rgb")
	purego.RegisterLibFunc(&paint, lib, "cairo_paint")
	purego.RegisterLibFunc(&pick, lib, "gtk_widget_pick")
	for _, id := range []backend.Peer{f, outer, inner} {
		_ = d.Show(id)
	}
	for d.api.width(d.peers[inner].handle) == 0 {
		d.api.iteration(d.context, 1)
	}
	// Rendering is allowed to await paint; none of the transaction-readiness
	// assertions above use this wait.
	pump := func() {
		end := time.Now().Add(250 * time.Millisecond)
		for time.Now().Before(end) {
			d.api.iteration(d.context, 0)
			time.Sleep(time.Millisecond)
		}
	}
	pump()
	before, err := renderPixels(lib, d.peers[f].handle, d.peers[outer].handle, 260, 180, t.TempDir()+"/before.png")
	if err != nil {
		t.Fatal(err)
	}
	huge := drawing()
	d.api.refSink(huge)
	defer d.api.unref(huge)
	draw := purego.NewCallback(func(_ uintptr, cr uintptr, _ int32, _ int32, _ uintptr) { rgb(cr, 1, 0, 1); paint(cr) })
	drawFunc(huge, draw, 0, 0)
	d.api.size(huge, 1200, 600)
	d.api.put(d.peers[inner].fixed, huge, 100, 60)
	d.api.visible(huge, 1)
	(&treeTransaction{d: d, form: d.peers[f].handle}).layout()
	pump()
	if d.api.width(d.peers[outer].handle) != 260 || d.api.height(d.peers[outer].handle) != 180 || d.api.width(d.peers[inner].handle) != 140 || d.api.height(d.peers[inner].handle) != 90 {
		t.Fatal("child grew panel")
	}
	if d.descendant(pick(d.peers[outer].handle, 155, 80, 0), huge) || !d.descendant(pick(d.peers[inner].handle, 110, 70, 0), huge) {
		t.Fatalf("panel hit clipping huge=%x inside=%x outside=%x huge-size=%dx%d", huge, pick(d.peers[inner].handle, 110, 70, 0), pick(d.peers[outer].handle, 155, 80, 0), d.api.width(huge), d.api.height(huge))
	}
	after, err := renderPixels(lib, d.peers[f].handle, d.peers[outer].handle, 260, 180, t.TempDir()+"/after.png")
	if err != nil {
		t.Fatal(err)
	}
	inside, outside := 0, 0
	for y := 0; y < 180; y++ {
		for x := 0; x < 260; x++ {
			i := (y*260 + x) * 4
			if string(before[i:i+4]) != string(after[i:i+4]) {
				if x >= 10 && x < 150 && y >= 10 && y < 100 {
					inside++
				} else {
					outside++
				}
			}
		}
	}
	if inside == 0 || outside != 0 {
		t.Fatalf("pixel clipping inside=%d outside=%d", inside, outside)
	}
	t.Logf("pixel clipping: inside=%d outside=%d", inside, outside)
	d.api.remove(d.peers[inner].fixed, huge)
}

// Mixed-subtree replacement must preserve the focused logical node (including
// Button), leave external focus alone, and retire every old peer exactly once.
func TestNativeReplacementSelection(t *testing.T) {
	if os.Getenv("GOLPHI_GTK_NATIVE") != "1" {
		t.Skip("set GOLPHI_GTK_NATIVE=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	events := 0
	if err := d.Init(backend.Callbacks{Event: func(backend.Event) { events++ }}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	create := func(k backend.Kind, parent backend.Peer, w, h int) backend.Peer {
		id, e := d.Create(backend.Config{Kind: k, Parent: parent, Bounds: backend.Bounds{Width: w, Height: h}})
		if e != nil {
			t.Fatal(e)
		}
		_ = d.Show(id)
		return id
	}
	f := create(backend.Form, 0, 640, 480)
	target := create(backend.Panel, f, 300, 220)
	external := create(backend.Button, f, 80, 32)
	panel := create(backend.Panel, f, 220, 160)
	inner := create(backend.Panel, panel, 200, 130)
	edit := create(backend.Edit, inner, 180, 30)
	button := create(backend.Button, inner, 80, 32)
	_ = d.SetText(edit, "А😀Б𐐷В")
	lib, _ := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	var grab func(uintptr) int32
	var emit func(uintptr, string)
	var weak func(uintptr, uintptr, uintptr)
	purego.RegisterLibFunc(&grab, lib, "gtk_widget_grab_focus")
	purego.RegisterLibFunc(&emit, lib, "g_signal_emit_by_name")
	purego.RegisterLibFunc(&weak, lib, "g_object_weak_ref")
	finalized := map[backend.Peer]int{}
	gone := purego.NewCallback(func(token, _ uintptr) { finalized[backend.Peer(token)]++ })
	pump := func() {
		end := time.Now().Add(80 * time.Millisecond)
		for time.Now().Before(end) {
			d.api.iteration(d.context, 0)
			time.Sleep(time.Millisecond)
		}
	}
	pump()
	events = 0
	for i := 0; i < 9; i++ {
		grab(d.peers[edit].handle)
		d.api.selectRegion(d.peers[edit].handle, 4, 1)
		focusID := edit
		if i%3 == 1 {
			focusID = button
		}
		if i%3 == 2 {
			focusID = external
		}
		grab(d.peers[focusID].handle)
		// grab_focus may select all only on entering Edit; fixture is explicit.
		d.api.selectRegion(d.peers[edit].handle, 4, 1)
		old := []backend.Peer{panel, inner, edit, button}
		oldButton := d.peers[button].handle
		for _, id := range old {
			weak(d.peers[id].handle, gone, uintptr(id))
		}
		targetID := target
		if i%2 == 1 {
			targetID = f
		}
		change := backend.TreeChange{FromAttached: true, ToAttached: true, Target: targetID, Nodes: []backend.TreeNode{
			{ID: 1, Peer: panel, Kind: backend.Panel, Bounds: backend.Bounds{Width: 220, Height: 160}},
			{ID: 2, Parent: 1, Peer: inner, Kind: backend.Panel, Bounds: backend.Bounds{Width: 200, Height: 130}},
			{ID: 3, Parent: 2, Peer: edit, Kind: backend.Edit, Text: "stale cache", Bounds: backend.Bounds{Width: 180, Height: 30}},
			{ID: 4, Parent: 2, Peer: button, Kind: backend.Button, Bounds: backend.Bounds{Width: 80, Height: 32}}}}
		var result backend.TreeResult
		if fail := backend.ApplyTree(d, change, func(r backend.TreeResult) { result = r }, func(*backend.TreeFailure) { t.Error("fatal") }); fail != nil {
			t.Fatal(fail)
		}
		if len(result.Bindings) != 4 {
			t.Fatal("incomplete subtree", result)
		}
		for n, b := range result.Bindings {
			if b.ID != backend.NodeID(n+1) || b.Peer == 0 || b.Peer == old[n] || b.Generation == 0 {
				t.Fatal("invalid complete replacement", result)
			}
		}
		panel, inner, edit, button = result.Bindings[0].Peer, result.Bindings[1].Peer, result.Bindings[2].Peer, result.Bindings[3].Peer
		expectedFocus := edit
		if i%3 == 1 {
			expectedFocus = button
		}
		if i%3 == 2 {
			expectedFocus = external
		}
		check := func() {
			s, e := d.SnapshotInput(edit)
			if e != nil || s.Text != "А😀Б𐐷В" || s.Anchor != 4 || s.Caret != 1 || !d.descendant(d.api.windowFocus(d.peers[f].handle), d.peers[expectedFocus].handle) {
				t.Fatalf("iteration %d input/focus %+v %v", i, s, e)
			}
		}
		check()
		emit(oldButton, "clicked")
		if events != 0 {
			t.Fatal("retired real native signal delivered", events)
		}
		d.releaseRetired()
		for _, id := range old {
			if finalized[id] != 1 || d.peers[id] != nil {
				t.Fatalf("peer %d finalizations=%d", id, finalized[id])
			}
		}
		pump()
		check()
		if events != 0 {
			t.Fatal("late technical event delivered", events)
		}
	}
}
