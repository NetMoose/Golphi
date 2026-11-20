//go:build windows && amd64

package win32

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// retainedInputSpy delegates every message to the original native Edit proc.
// Counting is enabled only around the transaction, never selection setup.
func retainedInputSpy(t *testing.T, d *driver, hwnd uintptr) (*int, *bool) {
	t.Helper()
	count, enabled := new(int), new(bool)
	var original uintptr
	hook := syscall.NewCallback(func(h uintptr, msg uint32, w, l uintptr) uintptr {
		if *enabled && msg == 0xb1 {
			*count++
		} // EM_SETSEL
		result, _, _ := d.api["CallWindowProcW"].Call(original, h, uintptr(msg), w, l)
		return result
	})
	var err error
	original, err = d.call("SetWindowLongPtrW", hwnd, ^uintptr(3), hook)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.api["SetWindowLongPtrW"].Call(hwnd, ^uintptr(3), original); runtime.KeepAlive(hook) })
	return count, enabled
}

type retainedFailAfterPlacement struct {
	d      *driver
	placed func()
}

func (a retainedFailAfterPlacement) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	tx, err := a.d.PrepareTree(c)
	if err != nil {
		return tx, err
	}
	return &retainedFailTransaction{TreeTransaction: tx, placed: a.placed}, nil
}

type retainedFailTransaction struct {
	backend.TreeTransaction
	placed func()
}

func (tx *retainedFailTransaction) Commit() (backend.TreeResult, error) {
	result, err := tx.TreeTransaction.Commit()
	if err != nil {
		return result, err
	}
	tx.placed()
	return backend.TreeResult{}, errors.New("test: injected failure after native placement")
}

func TestNativeRetainedHWNDTransferWin32(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_TREE") != "1" {
		t.Skip("set GOLPHI_NATIVE_TREE=1 on interactive Windows amd64")
	}
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	d := New().(*driver)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	// Cleanup order restores subclass procedures before destroying their HWNDs.
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	create := func(kind backend.Kind, parent backend.Peer, text string) backend.Peer {
		t.Helper()
		id, err := d.Create(backend.Config{Kind: kind, Parent: parent, Text: text, Bounds: backend.Bounds{Width: 300, Height: 100}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := create(backend.Form, 0, "Retained HWND acceptance")
	a, b := create(backend.Panel, f, ""), create(backend.Panel, f, "")
	nested := create(backend.Panel, a, "")
	edit := create(backend.Edit, nested, "А🌍Б𐐷В")
	outside := create(backend.Button, f, "outside")
	if err := d.Show(f); err != nil {
		t.Fatal(err)
	}
	hwnd := d.peers[edit].hwnd
	count, enabled := retainedInputSpy(t, d, hwnd)
	want := backend.InputState{Text: "А🌍Б𐐷В", Anchor: 1, Caret: 4, Focused: true}
	d.api["SetFocus"].Call(hwnd)
	if err := d.restoreInput(edit, want); err != nil {
		t.Fatal(err)
	}
	assertInput := func(expected backend.InputState) {
		t.Helper()
		got, err := d.SnapshotInput(edit)
		if err != nil || got != expected {
			t.Fatalf("input got=%+v want=%+v err=%v", got, expected, err)
		}
		lo, hi := d.selection(hwnd)
		if lo != 1 || hi != 6 {
			t.Fatalf("native UTF16 range=[%d,%d], want [1,6]", lo, hi)
		}
		if d.peers[edit].hwnd != hwnd {
			t.Fatal("retained HWND replaced")
		}
	}
	assertInput(want)
	editNode := backend.TreeNode{ID: 2, Peer: edit, Kind: backend.Edit, Bounds: backend.Bounds{X: 5, Y: 5, Width: 280, Height: 35}}
	apply := func(c backend.TreeChange) {
		t.Helper()
		*count, *enabled = 0, true
		published := false
		var publishedBindings []backend.TreeBinding
		priorGenerations := make(map[backend.Peer]uint64)
		for _, n := range c.Nodes {
			priorGenerations[n.Peer] = d.peers[n.Peer].generation
		}
		failure := backend.ApplyTree(d, c, func(r backend.TreeResult) {
			published = true
			publishedBindings = append([]backend.TreeBinding(nil), r.Bindings...)
			for _, binding := range r.Bindings {
				if binding.Generation <= priorGenerations[binding.Peer] || binding.Peer == 0 {
					t.Error("retained binding generation did not advance", binding)
				}
			}
		}, func(f *backend.TreeFailure) { t.Error("fatal", f) })
		*enabled = false
		if *count != 0 {
			t.Fatalf("transaction sent %d EM_SETSEL", *count)
		}
		if failure != nil || !published {
			t.Fatalf("move published=%t failure=%v", published, failure)
		}
		for _, binding := range publishedBindings {
			if got := d.peers[binding.Peer].generation; got != binding.Generation {
				t.Fatalf("Finish generation peer=%d got=%d published=%d", binding.Peer, got, binding.Generation)
			}
		}
		parent, _, _ := d.api["GetParent"].Call(d.peers[c.Nodes[0].Peer].hwnd)
		if parent != d.peers[c.Target].hwnd {
			t.Fatal("native root parent did not change")
		}
		assertInput(want)
	}
	// Focused Edit changes native parent, preserving scalar and UTF16 endpoints.
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: a, Nodes: []backend.TreeNode{editNode}})
	editNode.Parent = 1
	// Move a nested Panel subtree with the focused Edit as a descendant.
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{
		{ID: 1, Peer: a, Kind: backend.Panel, Bounds: backend.Bounds{Width: 300, Height: 100}},
		{ID: 3, Parent: 1, Peer: nested, Kind: backend.Panel, Bounds: backend.Bounds{Width: 300, Height: 100}}, editNode,
	}})
	d.api["SetFocus"].Call(d.peers[outside].hwnd)
	want.Focused = false
	editNode.Parent = 0
	apply(backend.TreeChange{FromAttached: true, ToAttached: true, Target: nested, Nodes: []backend.TreeNode{editNode}})
	focus, _, _ := d.api["GetFocus"].Call()
	if focus != d.peers[outside].hwnd {
		t.Fatal("outside focus stolen")
	}
	d.api["SetFocus"].Call(hwnd)
	want.Focused = true
	assertInput(want)
	old := d.peers[edit]
	reachedPlacement := false
	adapter := retainedFailAfterPlacement{d: d, placed: func() {
		reachedPlacement = true
		parent, _, _ := d.api["GetParent"].Call(hwnd)
		if parent != d.peers[b].hwnd {
			t.Error("fault did not follow native placement")
		}
		assertInput(want)
	}}
	*count, *enabled = 0, true
	failure := backend.ApplyTree(adapter, backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{editNode}}, func(backend.TreeResult) { t.Error("published injected failure") }, func(f *backend.TreeFailure) { t.Error("fatal rollback", f) })
	*enabled = false
	if failure == nil || failure.Fatal() || !reachedPlacement {
		t.Fatalf("rollback failure=%v placed=%t", failure, reachedPlacement)
	}
	if *count != 0 {
		t.Fatalf("commit/rollback sent %d EM_SETSEL", *count)
	}
	parent, _, _ := d.api["GetParent"].Call(hwnd)
	if parent != d.peers[old.parent].hwnd || d.peers[edit].parent != old.parent || d.peers[edit].bounds != old.bounds {
		t.Fatal("rollback did not restore native/logical placement")
	}
	assertInput(want)

	// A detach Commit actually hides the focused Edit before the injected
	// failure. Rollback must Show it, restore focus, and certify original input.
	visibleProc, err := d.dlls[0].FindProc("IsWindowVisible")
	if err != nil {
		t.Fatal(err)
	}
	hidden := false
	detachAdapter := retainedFailAfterPlacement{d: d, placed: func() {
		visible, _, _ := visibleProc.Call(hwnd)
		hidden = visible == 0
		if !hidden {
			t.Error("detach fault did not follow native hide")
		}
	}}
	*count, *enabled = 0, true
	failure = backend.ApplyTree(detachAdapter, backend.TreeChange{FromAttached: true, Nodes: []backend.TreeNode{editNode}}, func(backend.TreeResult) { t.Error("published failed detach") }, func(f *backend.TreeFailure) { t.Error("fatal detach rollback", f) })
	*enabled = false
	if failure == nil || failure.Fatal() || !hidden {
		t.Fatalf("detach rollback failure=%v hidden=%t", failure, hidden)
	}
	if *count != 0 {
		t.Fatalf("detach rollback sent %d EM_SETSEL", *count)
	}
	visible, _, _ := visibleProc.Call(hwnd)
	if visible == 0 {
		t.Fatal("detach rollback left Edit hidden")
	}
	if routed, ok := d.byHWND[hwnd]; !ok || routed != edit {
		t.Fatal("detach rollback lost retained route")
	}
	assertInput(want)

	// Prepare snapshots the valid Edit before discovering a bogus later peer.
	// The returned partial transaction must rollback without changing input.
	*count, *enabled = 0, true
	failure = backend.ApplyTree(d, backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{
		editNode, {ID: 99, Parent: editNode.ID, Peer: ^backend.Peer(0), Kind: backend.Edit},
	}}, func(backend.TreeResult) { t.Error("published partial Prepare failure") }, func(f *backend.TreeFailure) { t.Error("fatal partial Prepare rollback", f) })
	*enabled = false
	if failure == nil || failure.Fatal() {
		t.Fatalf("partial Prepare rollback failure=%v", failure)
	}
	if *count != 0 {
		t.Fatalf("partial Prepare rollback sent %d EM_SETSEL", *count)
	}
	parent, _, _ = d.api["GetParent"].Call(hwnd)
	if parent != d.peers[old.parent].hwnd || d.peers[edit].parent != old.parent || d.peers[edit].bounds != old.bounds {
		t.Fatal("partial Prepare changed placement")
	}
	assertInput(want)

	// Final negative case: inject native selection corruption AFTER Prepare's
	// snapshot. This is deliberate test input, outside the counted transaction
	// operations. Neither Commit nor Rollback may repair it with EM_SETSEL or
	// certify a state that differs from the original snapshot.
	tx, err := d.PrepareTree(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{editNode}})
	if err != nil {
		if tx != nil {
			tx.Finish()
		}
		t.Fatal(err)
	}
	d.api["SendMessageW"].Call(hwnd, 0xb1, 0, 1) // injected forward range [0,1]
	lo, hi := d.selection(hwnd)
	if lo != 0 || hi != 1 {
		tx.Finish()
		t.Fatalf("selection corruption setup=[%d,%d]", lo, hi)
	}
	*count, *enabled = 0, true
	_, commitErr := tx.Commit()
	rollbackErr := tx.Rollback()
	tx.Finish()
	*enabled = false
	if commitErr == nil || rollbackErr == nil {
		t.Fatalf("corrupted retained input was certified: commit=%v rollback=%v", commitErr, rollbackErr)
	}
	if *count != 0 {
		t.Fatalf("corruption guard attempted %d EM_SETSEL repairs", *count)
	}
	if !d.deliveryStopped || d.fenced {
		t.Fatalf("Finish reopened fatal delivery or retained fence: stopped=%t fenced=%t", d.deliveryStopped, d.fenced)
	}
	unavailable, prepareErr := d.PrepareTree(backend.TreeChange{FromAttached: true, ToAttached: true, Target: b, Nodes: []backend.TreeNode{editNode}})
	if unavailable != nil || prepareErr == nil {
		t.Fatal("fatal delivery latch allowed a new transaction")
	}
}
