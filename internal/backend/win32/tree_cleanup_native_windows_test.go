//go:build windows && amd64

package win32

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Inject a failing DestroyWindow argument at the actual production CleanupStep.
// Restore the tracked handle afterwards so final Close can release the real
// resource. No production hook or replacement cleanup implementation is needed.
type cleanupFaultDriver struct {
	*driver
	failAfterShown bool
	attemptRoot    backend.Peer
	attemptHWND    uintptr
	cleanupCalls   int
}

type cleanupFaultTransaction struct {
	*treeTransaction
	owner *cleanupFaultDriver
}

func (d *cleanupFaultDriver) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	tx, err := d.driver.PrepareTree(c)
	if tx == nil {
		return nil, err
	}
	return &cleanupFaultTransaction{treeTransaction: tx.(*treeTransaction), owner: d}, err
}

var errAfterShown = errors.New("injected failure after new subtree shown")

func (tx *cleanupFaultTransaction) Commit() (backend.TreeResult, error) {
	r, err := tx.treeTransaction.Commit()
	if err == nil && tx.owner.failAfterShown {
		return backend.TreeResult{}, errAfterShown
	}
	return r, err
}

func (tx *cleanupFaultTransaction) Cleanup() []backend.CleanupStep {
	steps := tx.treeTransaction.Cleanup()
	for i := range steps {
		run := steps[i].Run
		steps[i].Run = func() error {
			d := tx.owner
			d.cleanupCalls++
			d.attemptRoot = tx.created[0]
			p := d.peers[d.attemptRoot]
			d.attemptHWND = p.hwnd
			broken := p
			broken.hwnd = 0 // DestroyWindow(NULL) fails without destroying the real HWND.
			d.peers[d.attemptRoot] = broken
			defer func() { d.peers[d.attemptRoot] = p }()
			return run()
		}
	}
	return steps
}

func TestNativeAbortedTreeCleanupWin32(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_TREE") != "1" {
		t.Skip("set GOLPHI_NATIVE_TREE=1 on interactive Windows desktop")
	}
	for _, tc := range []struct {
		name                 string
		callback, afterShown bool
	}{
		{name: "late-child"},
		{name: "late-child-in-callback", callback: true},
		{name: "shown-root-in-callback", callback: true, afterShown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			d := &cleanupFaultDriver{driver: New().(*driver), failAfterShown: tc.afterShown}
			if err := d.Init(backend.Callbacks{}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := d.Close(); err != nil {
					t.Error(err)
				}
				if len(d.peers) != 0 || len(d.byHWND) != 0 {
					t.Error("final Close retained resources")
				}
			}()
			create := func(kind backend.Kind, parent backend.Peer) backend.Peer {
				id, err := d.Create(backend.Config{Kind: kind, Parent: parent, Bounds: backend.Bounds{Width: 200, Height: 80}})
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			form := create(backend.Form, 0)
			button := create(backend.Button, form)
			buttonHWND := d.peers[button].hwnd
			if err := d.Show(form); err != nil {
				t.Fatal(err)
			}
			bad := backend.TreeChange{ToAttached: true, Target: form, Nodes: []backend.TreeNode{
				{ID: 1, Kind: backend.Panel, Bounds: backend.Bounds{Width: 100, Height: 50}},
				{ID: 2, Parent: 1, Kind: backend.Edit, Text: "bad\x00text", Bounds: backend.Bounds{Width: 80, Height: 20}},
			}}
			if tc.afterShown {
				bad.Nodes = bad.Nodes[:1]
			}
			attempts := 0
			run := func() {
				attempts++
				if tc.callback && d.callbackDepth == 0 {
					t.Error("test did not enter native callback")
				}
				fail := backend.ApplyTree(d, bad, func(backend.TreeResult) { t.Error("published failed attempt") }, func(*backend.TreeFailure) { t.Error("cleanup failure became fatal") })
				if fail == nil || fail.Cause == nil || fail.Fatal() || len(fail.Cleanup) != 1 || !strings.Contains(fail.Cleanup[0].Error(), "DestroyWindow") {
					t.Errorf("missing ordinary operation/cleanup diagnostics: %+v", fail)
				}
				if tc.afterShown && (fail == nil || !errors.Is(fail, errAfterShown)) {
					t.Error("original cause lost")
				}
				if d.fenced || d.deliveryStopped || d.quitting {
					t.Error("restored runtime stopped")
				}
				if d.cleanupCalls != 1 || d.attemptRoot == 0 {
					t.Error("attempt cleanup did not execute synchronously")
				}
				if len(d.retired) != 0 {
					t.Error("aborted attempt queued for successful-detach retirement")
				}
				if _, ok := d.byHWND[d.attemptHWND]; ok {
					t.Error("aborted event route retained")
				}
				if alive, _, _ := d.api["IsWindow"].Call(buttonHWND); alive == 0 {
					t.Error("active callback HWND destroyed")
				}
				visible, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("IsWindowVisible").Call(d.attemptHWND)
				if visible != 0 {
					t.Error("aborted subtree remained visible")
				}
			}
			if tc.callback {
				d.cb.Event = func(backend.Event) { run() }
				d.api["SendMessageW"].Call(buttonHWND, 0xf5, 0, 0) // BM_CLICK
			} else {
				run()
			}
			if attempts != 1 {
				t.Fatalf("attempt count %d", attempts)
			}
			if err := d.releaseRetired(); err != nil {
				t.Error(err)
			}
			if _, ok := d.peers[d.attemptRoot]; !ok {
				t.Error("failed release retried at next loop turn")
			}
			if alive, _, _ := d.api["IsWindow"].Call(d.attemptHWND); alive == 0 {
				t.Error("failed resource unexpectedly released before final Close")
			}
			clicks := 0
			d.cb.Event = func(backend.Event) { clicks++ }
			d.api["SendMessageW"].Call(buttonHWND, 0xf5, 0, 0)
			if clicks != 1 {
				t.Error("old live tree notifications did not continue")
			}
			good := bad
			good.Nodes = good.Nodes[:1]
			if fail := backend.ApplyTree(d.driver, good, func(backend.TreeResult) {}, func(*backend.TreeFailure) { t.Error("later operation fatal") }); fail != nil {
				t.Errorf("later valid attachment: %v", fail)
			}
		})
	}
}
