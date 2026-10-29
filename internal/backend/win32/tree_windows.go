//go:build windows && amd64

package win32

import (
	"errors"
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
)

type treeTransaction struct {
	d                  *driver
	change             backend.TreeChange
	input              map[backend.Peer]backend.InputState
	focus              uintptr
	oldRoot            peer
	created            []backend.Peer
	result             backend.TreeResult
	mutated, committed bool
}

func (d *driver) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	if d.fenced || d.deliveryStopped {
		return nil, errors.New("win32: tree transaction unavailable")
	}
	if len(c.Nodes) == 0 {
		return nil, errors.New("win32: empty tree")
	}
	if c.ToAttached {
		p, err := d.get(c.Target)
		if err != nil {
			return nil, err
		}
		if p.kind != backend.Panel && p.kind != backend.Form {
			return nil, errors.New("win32: target is not a container")
		}
	}
	d.fenced = true
	tx := &treeTransaction{d: d, change: c, input: make(map[backend.Peer]backend.InputState)}
	tx.focus, _, _ = d.api["GetFocus"].Call()
	if c.FromAttached {
		for i, n := range c.Nodes {
			p, err := d.get(n.Peer)
			if err != nil {
				return tx, err
			}
			if i == 0 {
				tx.oldRoot = p
			}
			if p.kind == backend.Edit {
				s, err := d.SnapshotInput(n.Peer)
				if err != nil {
					return tx, err
				}
				tx.input[n.Peer] = s
			}
		}
	}
	return tx, nil
}
func (tx *treeTransaction) Commit() (backend.TreeResult, error) {
	d, c := tx.d, tx.change
	ids := make(map[backend.NodeID]backend.Peer)
	for i, n := range c.Nodes {
		id := n.Peer
		if !c.FromAttached && c.ToAttached {
			parent := c.Target
			if i != 0 {
				parent = ids[n.Parent]
			}
			var err error
			id, err = d.Create(backend.Config{Kind: n.Kind, Parent: parent, Text: n.Text, Bounds: n.Bounds, Input: n.Input})
			if err != nil {
				return backend.TreeResult{}, err
			}
			tx.created = append(tx.created, id)
		}
		ids[n.ID] = id
		binding := backend.TreeBinding{ID: n.ID}
		if s, ok := tx.input[n.Peer]; ok {
			copy := s
			binding.Input = &copy
		}
		if c.ToAttached {
			binding.Peer = id
			d.generation++
			binding.Generation = d.generation
			if n.Kind == backend.Edit {
				s, err := d.SnapshotInput(id)
				if err != nil {
					return backend.TreeResult{}, err
				}
				binding.Input = &s
			}
		}
		tx.result.Bindings = append(tx.result.Bindings, binding)
	}
	if c.FromAttached {
		root := c.Nodes[0].Peer
		tx.mutated = true
		if c.ToAttached {
			if err := d.place(root, c.Target, c.Nodes[0].Bounds); err != nil {
				return backend.TreeResult{}, err
			}
		} else {
			d.api["ShowWindow"].Call(tx.oldRoot.hwnd, 0)
		}
	}
	if c.ToAttached {
		for _, id := range tx.created {
			d.api["ShowWindow"].Call(d.peers[id].hwnd, 4)
		} // SW_SHOWNOACTIVATE
		if err := tx.restoreFocus(false); err != nil {
			return backend.TreeResult{}, err
		}
	}
	if c.FromAttached && c.ToAttached {
		for id, s := range tx.input {
			if err := d.restoreInput(id, s); err != nil {
				return backend.TreeResult{}, err
			}
		}
	}
	tx.committed = true
	return tx.result, nil
}
func (d *driver) place(id, parent backend.Peer, b backend.Bounds) error {
	p, err := d.get(id)
	if err != nil {
		return err
	}
	target, err := d.get(parent)
	if err != nil {
		return err
	}
	x, err := scale(b.X, d.dpi)
	if err != nil {
		return err
	}
	y, err := scale(b.Y, d.dpi)
	if err != nil {
		return err
	}
	if _, err = d.call("SetParent", p.hwnd, target.hwnd); err != nil {
		return err
	}
	// SetParent uses window coordinates; reapply logical client coordinates.
	if _, err = d.call("SetWindowPos", p.hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x1|0x4|0x10); err != nil {
		return err
	}
	p.parent = parent
	p.bounds = b
	d.peers[id] = p
	return nil
}
func (tx *treeTransaction) restoreFocus(rollback bool) error {
	d := tx.d
	// Attachment must not reacquire focus from a previously detached edit.
	focus, _, _ := d.api["GetFocus"].Call()
	if focus == tx.focus {
		return nil
	}
	if !rollback && !tx.change.ToAttached {
		return nil
	}
	if tx.focus != 0 {
		valid, _, _ := d.api["IsWindow"].Call(tx.focus)
		if valid == 0 {
			return errors.New("win32: original focus target disappeared")
		}
	}
	d.api["SetFocus"].Call(tx.focus)
	focus, _, _ = d.api["GetFocus"].Call()
	if focus != tx.focus {
		return errors.New("win32: focus restoration failed")
	}
	return nil
}
func (tx *treeTransaction) Rollback() (err error) {
	tx.committed = false
	defer func() {
		if err != nil {
			tx.d.deliveryStopped = true
		}
	}()
	d := tx.d
	var errs []error
	if tx.mutated {
		root := tx.change.Nodes[0].Peer
		if e := d.place(root, tx.oldRoot.parent, tx.oldRoot.bounds); e != nil {
			errs = append(errs, e)
		}
		d.api["ShowWindow"].Call(tx.oldRoot.hwnd, 4)
		for id, s := range tx.input {
			if e := d.restoreInput(id, s); e != nil {
				errs = append(errs, e)
			}
		}
	}
	if e := tx.restoreFocus(true); e != nil {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
func (tx *treeTransaction) Cleanup() []backend.CleanupStep { return nil } // all attempt HWNDs retained until outside native callback
func (tx *treeTransaction) Finish() {
	d := tx.d
	if tx.committed {
		if tx.change.FromAttached && !tx.change.ToAttached {
			for _, n := range tx.change.Nodes {
				delete(d.byHWND, d.peers[n.Peer].hwnd)
			}
			d.retired = append(d.retired, tx.change.Nodes[0].Peer)
		} else if tx.change.ToAttached {
			for _, b := range tx.result.Bindings {
				p := d.peers[b.Peer]
				p.generation = b.Generation
				d.peers[b.Peer] = p
			}
		}
	} else {
		for _, id := range tx.created {
			delete(d.byHWND, d.peers[id].hwnd)
		}
		if len(tx.created) > 0 {
			d.retired = append(d.retired, tx.created[0])
		}
	}
	d.fenced = false
}
func (d *driver) subtree(id backend.Peer) []backend.Peer {
	ids := []backend.Peer{id}
	for i := 0; i < len(ids); i++ {
		for child, p := range d.peers {
			if p.parent == ids[i] {
				ids = append(ids, child)
			}
		}
	}
	return ids
}
func (d *driver) releaseRetired() error {
	if d.callbackDepth != 0 || d.fenced {
		return nil
	}
	ids := d.retired
	d.retired = nil
	var errs []error
	for _, id := range ids {
		if _, ok := d.peers[id]; ok {
			if err := d.Destroy(id); err != nil {
				d.retired = append(d.retired, id)
				errs = append(errs, fmt.Errorf("retired peer %d: %w", id, err))
			}
		}
	}
	return errors.Join(errs...)
}
