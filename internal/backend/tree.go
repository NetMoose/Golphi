package backend

import (
	"errors"
	"fmt"
)

// NodeID identifies a logical component for one tree request, independently of
// native peer identity. Zero is reserved for the root's external parent.
type NodeID uint64

// InputState uses Unicode scalar offsets, never UTF-8 bytes or UTF-16 units.
// Anchor and Caret describe the selection's endpoints in the effective Text.
// Focused is observed state, not a request to steal focus on attachment.
type InputState struct {
	Text          string
	Anchor, Caret int
	Focused       bool
}

// TreeNode describes one node in parent-before-child order. Peer is the old
// peer, or zero for an unmaterialized node. Input is only meaningful for Edit.
// Parent is zero on the subtree root and otherwise refers to an earlier NodeID.
type TreeNode struct {
	ID, Parent NodeID
	Peer       Peer
	Kind       Kind
	Text       string
	Bounds     Bounds
	Input      *InputState
}

// TreeChange is validated by the core before any native operation. Target is
// the external parent peer when ToAttached is true, otherwise zero. Nodes
// contains the whole visual subtree, not the independent ownership graph.
type TreeChange struct {
	FromAttached, ToAttached bool
	Target                   Peer
	Nodes                    []TreeNode
}

// TreeBinding is the complete post-operation binding. Detached nodes have no
// Peer or Generation but retain their effective input state. Every attached
// binding has a nonzero generation, including when the old peer is reused.
type TreeBinding struct {
	ID         NodeID
	Peer       Peer
	Generation uint64
	Input      *InputState
}
type TreeResult struct{ Bindings []TreeBinding }

// CleanupStep is an independent safe release attempt. Unsafe releases (e.g.
// active native callback roots) must stay owned by the adapter for a safe turn.
type CleanupStep struct {
	Operation string
	Run       func() error
}

// TreeTransaction owns all attempted resources until Finish. Prepare fences
// application delivery before any native work. Commit returns only after the
// entire subtree and input state are synchronously ready (without paint wait).
// Any Commit failure requires Rollback, even if no mutation appeared to occur.
// Rollback returning nil certifies the entire old working state is restored;
// resource-release failures belong to Cleanup, not Rollback.
// Cleanup returns independent safe release steps for an aborted attempt only.
// Finish never calls user code or performs fallible physical release: it ends
// the fence after core publication/rollback and retains unsafe retirements for
// safe-loop teardown. Failed Rollback latches an irreversible delivery stop;
// Finish MUST NOT reopen that stop. It is called exactly once, including after
// failed Prepare.
// All methods are UI-thread-only. Transactions are single-use, not nestable.
type TreeTransaction interface {
	Commit() (TreeResult, error)
	Rollback() error
	Cleanup() []CleanupStep
	Finish()
}

// CleanupFailure preserves an individual cleanup error or panic with its stack.
type CleanupFailure struct {
	Operation string
	Err       error
	Panic     *Panic
}

func (e CleanupFailure) Error() string {
	if e.Panic != nil {
		return fmt.Sprintf("%s: cleanup panic: %v\n%s", e.Operation, e.Panic.Value, e.Panic.Stack)
	}
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}
func (e CleanupFailure) Unwrap() error { return e.Err }

// TreeFailure is internal diagnostics, not a promised public error type.
// Cleanup failure alone does not make an otherwise restored operation fatal.
type TreeFailure struct {
	Cause   error
	Restore error
	Cleanup []CleanupFailure
}

func (e *TreeFailure) Fatal() bool   { return e.Restore != nil }
func (e *TreeFailure) Error() string { return errors.Join(e.Unwrap()...).Error() }
func (e *TreeFailure) Unwrap() []error {
	all := []error{e.Cause}
	if e.Restore != nil {
		all = append(all, e.Restore)
	}
	for _, c := range e.Cleanup {
		all = append(all, c)
	}
	return all
}

var ErrTreeUnsupported = errors.New("backend dynamic tree operations are not implemented")

// UnsupportedTree is temporary compatibility scaffolding for static adapters.
// It must be removed when implementing native transactions, never treated as a
// fallback which provides dynamic behavior.
type UnsupportedTree struct{}

func (UnsupportedTree) PrepareTree(TreeChange) (TreeTransaction, error) {
	return nil, ErrTreeUnsupported
}
func (UnsupportedTree) SnapshotInput(Peer) (InputState, error) {
	return InputState{}, ErrTreeUnsupported
}

// ApplyTree coordinates the internal boundary only; it neither changes the
// logical graph nor implements event coalescing. publish must atomically install
// the prevalidated core graph/bindings/event state and must not panic, fail or
// call user code. fatal must synchronously close Post/delivery before cleanup,
// retain the diagnostic pointer (cleanup is appended later), and never panic.
// No fallible step follows publish: successful retirements stay adapter-owned
// until safe teardown, whose errors belong to that teardown's diagnostics.
func ApplyTree(d interface {
	PrepareTree(TreeChange) (TreeTransaction, error)
}, change TreeChange, publish func(TreeResult), fatal func(*TreeFailure)) *TreeFailure {
	tx, err := d.PrepareTree(change)
	if tx == nil {
		// No transaction means Prepare guarantees that nothing was changed and
		// that it retained responsibility for any internally rejected allocation.
		if err == nil {
			panic("backend: PrepareTree returned no transaction and no error")
		}
		return &TreeFailure{Cause: err}
	}
	defer tx.Finish()
	if err == nil {
		var result TreeResult
		result, err = tx.Commit()
		if err == nil {
			publish(result)
			return nil
		}
	}
	failure := &TreeFailure{Cause: err, Restore: tx.Rollback()}
	if failure.Fatal() {
		fatal(failure)
	}
	for _, step := range tx.Cleanup() {
		var cleanupErr error
		p := Capture(func() { cleanupErr = step.Run() })
		if cleanupErr != nil || p != nil {
			failure.Cleanup = append(failure.Cleanup, CleanupFailure{Operation: step.Operation, Err: cleanupErr, Panic: p})
		}
	}
	return failure
}
