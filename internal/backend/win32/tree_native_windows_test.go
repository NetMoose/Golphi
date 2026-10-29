//go:build windows && amd64

package win32

import (
	"github.com/NetMoose/Golphi/internal/backend"
	"os"
	"runtime"
	"testing"
	"unsafe"
)

func TestNativeDynamicTreeWin32(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_TREE") != "1" {
		t.Skip("set GOLPHI_NATIVE_TREE=1 on interactive Windows desktop")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	events := 0
	if err := d.Init(backend.Callbacks{Event: func(backend.Event) { events++ }}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
		if len(d.peers) != 0 || len(d.byHWND) != 0 {
			t.Error("native registries retained")
		}
	}()
	create := func(k backend.Kind, p backend.Peer, text string, input *backend.InputState) backend.Peer {
		t.Helper()
		id, err := d.Create(backend.Config{Kind: k, Parent: p, Text: text, Input: input, Bounds: backend.Bounds{Width: 200, Height: 80}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := create(backend.Form, 0, "", nil)
	a := create(backend.Panel, f, "", nil)
	b := create(backend.Panel, f, "", nil)
	want := backend.InputState{Text: "А🌍Б𐐷В", Anchor: 4, Caret: 1}
	e := create(backend.Edit, a, want.Text, &want)
	got, err := d.SnapshotInput(e)
	if err != nil || got != want {
		t.Fatalf("startup input: %+v %v", got, err)
	}
	node := backend.TreeNode{ID: 1, Peer: e, Kind: backend.Edit, Text: want.Text, Bounds: backend.Bounds{Width: 200, Height: 80}}
	apply := func(c backend.TreeChange) backend.TreeResult {
		t.Helper()
		var out backend.TreeResult
		fail := backend.ApplyTree(d, c, func(r backend.TreeResult) { out = r }, func(f *backend.TreeFailure) { t.Errorf("fatal: %v", f) })
		if fail != nil {
			t.Fatal(fail)
		}
		return out
	}
	r := apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{node}})
	if r.Bindings[0].Peer != e || r.Bindings[0].Generation == 0 {
		t.Fatal(r)
	}
	if p, _, _ := d.api["GetParent"].Call(d.peers[e].hwnd); p != d.peers[b].hwnd {
		t.Fatal("native parent unchanged")
	}
	got, err = d.SnapshotInput(e)
	if err != nil || got != want {
		t.Fatalf("move input: %+v %v", got, err)
	}
	// Invalid late child forces rollback after hidden resources were created.
	before := len(d.peers)
	bad := backend.TreeChange{ToAttached: true, Target: b, Nodes: []backend.TreeNode{{ID: 1, Kind: backend.Panel, Bounds: backend.Bounds{Width: 100, Height: 50}}, {ID: 2, Parent: 1, Kind: backend.Edit, Text: "bad\x00text", Bounds: backend.Bounds{Width: 80, Height: 20}}}}
	if fail := backend.ApplyTree(d, bad, func(backend.TreeResult) { t.Fatal("published failed attempt") }, func(f *backend.TreeFailure) { t.Fatal(f) }); fail == nil {
		t.Fatal("accepted invalid late child")
	}
	if err := d.releaseRetired(); err != nil {
		t.Fatal(err)
	}
	if len(d.peers) != before {
		t.Fatal("failed attempt leaked peers")
	}
	r = apply(backend.TreeChange{FromAttached: true, Nodes: []backend.TreeNode{node}})
	if r.Bindings[0].Peer != 0 || r.Bindings[0].Input == nil {
		t.Fatal(r)
	}
	if _, ok := d.byHWND[d.peers[e].hwnd]; ok {
		t.Fatal("detached route retained")
	}
	if err := d.releaseRetired(); err != nil {
		t.Fatal(err)
	}
	node.Peer = 0
	node.Input = r.Bindings[0].Input
	r = apply(backend.TreeChange{ToAttached: true, Target: a, Nodes: []backend.TreeNode{node}})
	got, err = d.SnapshotInput(r.Bindings[0].Peer)
	if err != nil || got != want {
		t.Fatalf("reattach input: %+v %v", got, err)
	}
	// An outside Button owns focus: moving the Edit must leave it there.
	outside := create(backend.Button, b, "outside", nil)
	if err := d.Show(f); err != nil {
		t.Fatal(err)
	}
	d.api["SetFocus"].Call(d.peers[outside].hwnd)
	focus, _, _ := d.api["GetFocus"].Call()
	if focus != d.peers[outside].hwnd {
		t.Fatal("outside focus setup failed")
	}
	node.Peer = r.Bindings[0].Peer
	node.Input = nil
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{node}})
	focus, _, _ = d.api["GetFocus"].Call()
	if focus != d.peers[outside].hwnd {
		t.Fatal("move stole outside focus")
	}
	// Focused reverse selection uses native caret direction, not ordered EM_GETSEL.
	logNativeInputEvidence(t, d, node.Peer, "before-SetFocus")
	d.api["SetFocus"].Call(d.peers[node.Peer].hwnd)
	logNativeInputEvidence(t, d, node.Peer, "after-SetFocus-before-EM_SETSEL")
	if err := d.restoreInput(node.Peer, want); err != nil {
		t.Fatal(err)
	}
	logNativeInputEvidence(t, d, node.Peer, "after-EM_SETSEL-before-ApplyTree")
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: []backend.TreeNode{node}})
	logNativeInputEvidence(t, d, node.Peer, "after-ApplyTree-before-SnapshotInput")
	focused, err := d.SnapshotInput(node.Peer)
	expected := want
	expected.Focused = true
	if err != nil || focused != expected {
		t.Fatalf("focused reverse move: %+v %v", focused, err)
	}
	// Native forward/reverse selections reaching the end of text exercise the
	// trusted explicit EM_SETSEL observation when EM_POSFROMCHAR(end) is unavailable.
	for _, ends := range [][2]int{{1, 5}, {5, 1}} {
		endState := want
		endState.Anchor = ends[0]
		endState.Caret = ends[1]
		if err := d.restoreInput(node.Peer, endState); err != nil {
			t.Fatal(err)
		}
		actual, err := d.SnapshotInput(node.Peer)
		endState.Focused = true
		if err != nil || actual != endState {
			t.Fatalf("text-end input %+v: %+v %v", ends, actual, err)
		}
	}
	if err := d.restoreInput(node.Peer, want); err != nil {
		t.Fatal(err)
	}
	// A dead native target forces failure after the transaction has snapshotted input.
	dead := create(backend.Panel, f, "", nil)
	deadPeer := d.peers[dead]
	if err := d.Destroy(dead); err != nil {
		t.Fatal(err)
	}
	d.peers[dead] = deadPeer
	fail := backend.ApplyTree(d, backend.TreeChange{FromAttached: true, ToAttached: true, Target: dead, Nodes: []backend.TreeNode{node}}, func(backend.TreeResult) { t.Fatal("published dead-parent attempt") }, func(f *backend.TreeFailure) { t.Fatalf("unexpected fatal rollback: %v", f) })
	delete(d.peers, dead)
	if fail == nil {
		t.Fatal("dead-parent operation succeeded")
	}
	got, err = d.SnapshotInput(node.Peer)
	if err != nil || got != expected {
		t.Fatalf("failed move did not restore input: %+v %v", got, err)
	}
	if events != 0 {
		t.Fatalf("technical events delivered: %d", events)
	}
	// A delayed event retains its old epoch and is rejected after another move.
	staleGeneration := d.peers[node.Peer].generation
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{node}})
	d.emit(backend.Event{Kind: backend.Change, Peer: node.Peer, Generation: staleGeneration, Text: want.Text})
	if events != 0 {
		t.Fatal("old epoch delivered")
	}
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: []backend.TreeNode{node}})
	// A genuine synchronous native button notification still reaches the sink.
	buttonNode := backend.TreeNode{ID: 2, Peer: outside, Kind: backend.Button, Bounds: backend.Bounds{Width: 200, Height: 80}}
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: []backend.TreeNode{buttonNode}})
	clicks := 0
	d.cb.Event = func(e backend.Event) {
		if e.Kind == backend.Click && e.Peer == outside && e.Generation == d.peers[outside].generation {
			clicks++
		}
	}
	d.api["SendMessageW"].Call(d.peers[outside].hwnd, 0xf5, 0, 0) // BM_CLICK
	if clicks != 1 {
		t.Fatalf("genuine native button notification count %d", clicks)
	}
	// Detach inside a real nested WM_COMMAND callback keeps the HWND alive until
	// the outer native call returns, while its event route is already removed.
	handle := d.peers[node.Peer].hwnd
	callbacks := 0
	d.cb.Event = func(event backend.Event) {
		if event.Kind != backend.Change {
			return
		}
		callbacks++
		apply(backend.TreeChange{FromAttached: true, Nodes: []backend.TreeNode{node}})
		if alive, _, _ := d.api["IsWindow"].Call(handle); alive == 0 {
			t.Error("released active callback HWND")
		}
		if err := d.releaseRetired(); err != nil {
			t.Error(err)
		}
		if alive, _, _ := d.api["IsWindow"].Call(handle); alive == 0 {
			t.Error("release bypassed callback guard")
		}
	}
	d.api["SendMessageW"].Call(d.peers[a].hwnd, wmCommand, 0x300<<16, handle)
	if callbacks != 1 {
		t.Fatalf("nested edit callback count %d", callbacks)
	}
	if err := d.releaseRetired(); err != nil {
		t.Fatal(err)
	}
	if alive, _, _ := d.api["IsWindow"].Call(handle); alive != 0 {
		t.Error("retired HWND still alive")
	}
	if len(d.retired) != 0 {
		t.Fatal("retirement registry retained")
	}

}

// Reads only: diagnostics must not change focus/selection, pump messages, sleep,
// or replace the original operation under investigation.
func logNativeInputEvidence(t *testing.T, d *driver, id backend.Peer, phase string) {
	t.Helper()
	p := d.peers[id]
	lo, hi := d.selection(p.hwnd)
	focus, _, _ := d.api["GetFocus"].Call()
	var cp point
	ok, _, caretErr := d.api["GetCaretPos"].Call(uintptr(unsafe.Pointer(&cp)))
	low, _, _ := d.api["SendMessageW"].Call(p.hwnd, 0xd6, uintptr(lo), 0)
	high, _, _ := d.api["SendMessageW"].Call(p.hwnd, 0xd6, uintptr(hi), 0)
	t.Logf("NATIVE_INPUT phase=%s peer=%d hwnd=%#x focus=%#x dpi=%d utf16=[%d,%d] caretOK=%d caretErr=%v caret=(%d,%d) lowRaw=%#x low=(%d,%d) highRaw=%#x high=(%d,%d) cached=[%d,%d] known=%t cache=%+v", phase, id, p.hwnd, focus, d.dpi, lo, hi, ok, caretErr, cp.x, cp.y, uint32(low), int16(low), int16(low>>16), uint32(high), int16(high), int16(high>>16), p.selectionAnchor, p.selectionCaret, p.selectionKnown, p.selectionObservation)
}
