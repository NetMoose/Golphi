//go:build linux && amd64

package gtk4

import (
	"errors"
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
	"unicode/utf8"
)

func validInput(text string, s *backend.InputState) error {
	n := utf8.RuneCountInString(text)
	if s.Text != text || s.Anchor < 0 || s.Caret < 0 || s.Anchor > n || s.Caret > n || s.Anchor > 1<<31-1 || s.Caret > 1<<31-1 {
		return errors.New("invalid GTK input state")
	}
	return nil
}
func (d *driver) form(p *peer) *peer {
	for p != nil && p.kind != backend.Form {
		p = d.peers[p.parent]
	}
	return p
}
func (d *driver) descendant(h, ancestor uintptr) bool {
	for h != 0 {
		if h == ancestor {
			return true
		}
		h = d.api.widgetParent(h)
	}
	return false
}
func (d *driver) SnapshotInput(id backend.Peer) (backend.InputState, error) {
	p, err := d.lookup(id)
	if err != nil {
		return backend.InputState{}, err
	}
	if p.kind != backend.Edit {
		return backend.InputState{}, errors.New("GTK input requires Edit")
	}
	s := backend.InputState{Text: d.api.getEdit(p.handle), Caret: int(d.api.position(d.api.editableDelegate(p.handle)))}
	s.Anchor = s.Caret
	var lo, hi int32
	if d.api.selection(p.handle, &lo, &hi) != 0 {
		s.Anchor = int(lo)
		if s.Caret == int(lo) {
			s.Anchor = int(hi)
		}
	}
	if f := d.form(p); f != nil {
		s.Focused = d.descendant(d.api.windowFocus(f.handle), p.handle)
	}
	return s, validInput(s.Text, &s)
}
func (d *driver) invalidate(p *peer) {
	registry.Lock()
	delete(registry.entries, p.token)
	registry.Unlock()
	if p.signal != 0 {
		d.api.disconnect(p.handle, p.signal)
		p.signal = 0
	}
	p.token = 0
}

// Run calls this only after native iteration or dispatch has returned. In
// particular the Go callback epilogue is NOT a native callback exit boundary.
func (d *driver) releaseRetired() {
	if d.callbackDepth != 0 {
		return
	}
	ids := d.retired
	d.retired = nil
	for i := len(ids) - 1; i >= 0; i-- {
		if d.peers[ids[i]] != nil {
			_ = d.Destroy(ids[i])
		}
	}
}

type treeTransaction struct {
	d                            *driver
	change                       backend.TreeChange
	inputs                       map[backend.Peer]backend.InputState
	created                      []backend.Peer
	recreate                     bool
	replacements                 map[backend.Peer]backend.Peer
	oldParent                    backend.Peer
	root                         backend.Peer
	form, focus                  uintptr
	moved, committed, rolledBack bool
}

func (d *driver) PrepareTree(c backend.TreeChange) (backend.TreeTransaction, error) {
	if !d.ready || d.closed || d.fenced {
		return nil, errors.New("GTK tree unavailable or nested")
	}
	d.fenced = true
	tx := &treeTransaction{d: d, change: c, inputs: make(map[backend.Peer]backend.InputState)}
	if len(c.Nodes) == 0 {
		return tx, errors.New("empty GTK subtree")
	}
	var f *peer
	if c.FromAttached {
		p, err := d.lookup(c.Nodes[0].Peer)
		if err != nil {
			return tx, err
		}
		tx.root = c.Nodes[0].Peer
		tx.oldParent = p.parent
		f = d.form(p)
	} else if c.ToAttached {
		p, err := d.lookup(c.Target)
		if err != nil {
			return tx, err
		}
		f = d.form(p)
	}
	if f != nil {
		tx.form = f.handle
		tx.focus = d.api.windowFocus(f.handle)
	}
	for _, n := range c.Nodes {
		if err := validDimensions(n.Bounds); err != nil {
			return tx, err
		}
		if err := validText(n.Text); err != nil {
			return tx, err
		}
		if c.FromAttached && n.Kind == backend.Edit {
			s, err := d.SnapshotInput(n.Peer)
			if err != nil {
				return tx, err
			}
			tx.inputs[n.Peer] = s
			if c.ToAttached {
				tx.recreate = true
			}
		}
	}
	return tx, nil
}
func (tx *treeTransaction) restoreInput(replacements map[backend.Peer]backend.Peer) error {
	for id, s := range tx.inputs {
		if replacements != nil {
			id = replacements[id]
		}
		p, err := tx.d.lookup(id)
		if err != nil {
			return err
		}
		if tx.d.api.getEdit(p.handle) != s.Text {
			tx.d.api.editText(p.handle, s.Text)
		}
		tx.d.api.selectRegion(p.handle, int32(s.Anchor), int32(s.Caret))
		actual, err := tx.d.SnapshotInput(id)
		if err != nil {
			return err
		}
		if actual.Text != s.Text || actual.Anchor != s.Anchor || actual.Caret != s.Caret {
			return errors.New("GTK input restoration failed")
		}
	}
	return nil
}
func (tx *treeTransaction) restoreFocus(replacements map[backend.Peer]backend.Peer) error {
	focus := tx.focus
	for old, id := range replacements {
		p := tx.d.peers[old]
		if p.kind != backend.Edit && p.kind != backend.Button {
			continue
		}
		if tx.d.descendant(focus, p.handle) {
			q := tx.d.peers[id]
			focus = q.handle
			if q.kind == backend.Edit {
				focus = tx.d.api.editableDelegate(q.handle)
			}
			break
		}
	}
	if tx.form != 0 {
		tx.d.api.setFocus(tx.form, focus)
		if tx.d.api.windowFocus(tx.form) != focus {
			return errors.New("GTK focus restoration failed")
		}
	}
	return nil
}
func (tx *treeTransaction) move(parent backend.Peer) error {
	d := tx.d
	p, err := d.lookup(tx.root)
	if err != nil {
		return err
	}
	if old := d.peers[p.parent]; old != nil {
		d.api.remove(old.fixed, p.handle)
	}
	p.parent = parent
	if parent != 0 {
		q, err := d.lookup(parent)
		if err != nil {
			return err
		}
		d.api.put(q.fixed, p.handle, float64(p.bounds.X), float64(p.bounds.Y))
	}
	return nil
}
func (tx *treeTransaction) Commit() (backend.TreeResult, error) {
	d, c := tx.d, tx.change
	r := backend.TreeResult{Bindings: make([]backend.TreeBinding, 0, len(c.Nodes))}
	ids := make(map[backend.NodeID]backend.Peer)
	if tx.recreate {
		tx.replacements = make(map[backend.Peer]backend.Peer)
	}
	if c.FromAttached && !tx.recreate {
		tx.moved = true
		if err := tx.move(c.Target); err != nil {
			return r, err
		}
	}
	for _, n := range c.Nodes {
		id := n.Peer
		if (!c.FromAttached || tx.recreate) && c.ToAttached {
			parent := c.Target
			if n.Parent != 0 {
				parent = ids[n.Parent]
			}
			var err error
			text, input := n.Text, n.Input
			if saved, ok := tx.inputs[n.Peer]; ok {
				text = saved.Text
				input = &saved
			}
			id, err = d.Create(backend.Config{Kind: n.Kind, Parent: parent, Text: text, Bounds: n.Bounds, Input: input})
			if err != nil {
				return r, err
			}
			tx.created = append(tx.created, id)
			if tx.recreate {
				tx.replacements[n.Peer] = id
			}
		}
		ids[n.ID] = id
		binding := backend.TreeBinding{ID: n.ID}
		if c.ToAttached {
			binding.Peer = id
		} else if s, ok := tx.inputs[id]; ok {
			s.Focused = false
			binding.Input = &s
		} else if n.Input != nil {
			s := *n.Input
			s.Focused = false
			binding.Input = &s
		}
		r.Bindings = append(r.Bindings, binding)
	}
	// Make the complete subtree visible under the delivery fence, then ask
	// its root GtkFixed layout manager to synchronously allocate descendants.
	if c.ToAttached {
		for _, n := range c.Nodes {
			if err := d.Show(ids[n.ID]); err != nil {
				return r, err
			}
		}
		tx.layout()
		for _, n := range c.Nodes {
			p := d.peers[ids[n.ID]]
			if d.api.width(p.handle) <= 0 || d.api.height(p.handle) <= 0 {
				return r, errors.New("GTK subtree allocation not ready")
			}
		}
		if err := tx.restoreFocus(tx.replacements); err != nil {
			return r, err
		}
		if err := tx.restoreInput(tx.replacements); err != nil {
			return r, err
		}
	}
	for i, n := range c.Nodes {
		if c.ToAttached && n.Kind == backend.Edit {
			s, err := d.SnapshotInput(ids[n.ID])
			if err != nil {
				return r, err
			}
			r.Bindings[i].Input = &s
		}
	}
	if tx.recreate {
		old := d.peers[tx.root]
		d.api.visible(old.handle, 0)
		d.api.remove(d.peers[old.parent].fixed, old.handle)
		old.parent = 0
	}
	// No fallible operation follows epoch replacement. Rollback therefore never
	// has to resurrect an invalidated route.
	for i, n := range c.Nodes {
		if c.ToAttached {
			id := ids[n.ID]
			p := d.peers[id]
			d.invalidate(p)
			d.generation++
			p.generation = d.generation
			d.connect(id, p)
			r.Bindings[i].Generation = p.generation
		}
	}
	tx.committed = true
	return r, nil
}
func (tx *treeTransaction) layout() {
	d := tx.d
	for _, f := range d.peers {
		if f.kind == backend.Form && f.handle == tx.form {
			w, h := d.api.width(f.fixed), d.api.height(f.fixed)
			if w <= 0 {
				w = int32(f.bounds.Width)
			}
			if h <= 0 {
				h = int32(f.bounds.Height)
			}
			// GtkWindow positions its content by translation, including the CSD
			// titlebar offset. Preserve that parent-assigned origin while invoking
			// widget allocation (which also completes resize bookkeeping).
			var bounds [4]float32
			if d.api.computeBounds(f.fixed, f.handle, &bounds) == 0 {
				return
			}
			origin := [2]float32{bounds[0], bounds[1]}
			transform := d.api.translate(0, &origin)
			// gtk_widget_allocate takes ownership of the GskTransform.
			d.api.allocate(f.fixed, w, h, -1, transform)
			return
		}
	}
}
func (tx *treeTransaction) Rollback() error {
	d := tx.d
	var err error
	if tx.moved {
		err = tx.move(tx.oldParent)
	}
	if err == nil {
		tx.layout()
		if tx.change.FromAttached {
			for _, n := range tx.change.Nodes {
				p := d.peers[n.Peer]
				if p == nil || d.api.width(p.handle) <= 0 || d.api.height(p.handle) <= 0 {
					err = errors.New("GTK rollback allocation not ready")
					break
				}
			}
		}
	}
	if err == nil {
		err = tx.restoreFocus(nil)
	}
	if err == nil {
		err = tx.restoreInput(nil)
	}
	// Failed first attachments may already have been shown before a readiness
	// check fails. Restore the visible tree synchronously, retaining references
	// only for later callback-safe physical release.
	if len(tx.created) > 0 {
		p := d.peers[tx.created[0]]
		d.api.visible(p.handle, 0)
		if parent := d.peers[p.parent]; parent != nil {
			d.api.remove(parent.fixed, p.handle)
		}
		p.parent = 0
	}
	tx.rolledBack = true
	if err != nil {
		d.quit = true
		return fmt.Errorf("GTK rollback: %w", err)
	}
	return nil
}
func (tx *treeTransaction) Cleanup() []backend.CleanupStep {
	// Keep all attempts until the outer safe turn, including attempts created
	// from a currently executing signal. Finish transfers ownership to driver.
	return nil
}
func (tx *treeTransaction) Finish() {
	d := tx.d
	if tx.committed && tx.change.FromAttached && (!tx.change.ToAttached || tx.recreate) {
		for _, n := range tx.change.Nodes {
			d.invalidate(d.peers[n.Peer])
			d.retired = append(d.retired, n.Peer)
		}
	}
	if !tx.committed {
		for _, id := range tx.created {
			d.invalidate(d.peers[id])
			d.retired = append(d.retired, id)
		}
	}
	d.fenced = false
}
