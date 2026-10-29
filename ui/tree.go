package ui

import (
	"errors"
	"sort"

	"github.com/NetMoose/Golphi/internal/backend"
)

// setParent keeps logical state untouched until the shared coordinator has
// synchronously committed every native node. No user code runs in publication.
func (r *application) setParent(root *component, parent Container) error {
	if !r.running || r.treeChanging {
		return errors.New("ui: tree transaction unavailable")
	}
	nodes := []*component{}
	var visit func(*component)
	visit = func(c *component) {
		nodes = append(nodes, c)
		children := make([]*component, 0, len(c.children))
		for child := range c.children {
			children = append(children, child)
		}
		sort.Slice(children, func(i, j int) bool { return children[i].order < children[j].order })
		for _, child := range children {
			visit(child)
		}
	}
	visit(root)
	change := backend.TreeChange{FromAttached: root.attached(), ToAttached: parent != nil && parent.componentIdentity().attached()}
	if change.ToAttached {
		change.Target = r.peers[parent.componentIdentity()]
	}
	for i, c := range nodes {
		n := backend.TreeNode{ID: backend.NodeID(c.order), Peer: r.peers[c], Kind: c.kind, Text: c.text, Bounds: c.bounds}
		if n.Peer == 0 {
			n.Input = c.copySavedInput()
		}
		if i != 0 {
			n.Parent = backend.NodeID(c.parent.componentIdentity().order)
		}
		if n.Bounds.Width == 0 || n.Bounds.Height == 0 {
			n.Bounds.Width, n.Bounds.Height = 120, 28
		}
		change.Nodes = append(change.Nodes, n)
	}
	// Prepare replacement maps and membership before entering the backend. A
	// failed operation leaves the current maps and all component state untouched.
	peers := make(map[*component]backend.Peer, len(r.peers)+len(nodes))
	components := make(map[backend.Peer]*component, len(r.components)+len(nodes))
	generations := make(map[backend.Peer]uint64, len(r.bindingGenerations)+len(nodes))
	for c, p := range r.peers {
		peers[c] = p
	}
	for p, c := range r.components {
		components[p] = c
	}
	for p, g := range r.bindingGenerations {
		generations[p] = g
	}
	created := append([]*component(nil), r.created...)
	known := make(map[*component]bool, len(created))
	for _, c := range created {
		known[c] = true
	}
	for _, c := range nodes {
		if !known[c] {
			created = append(created, c)
		}
	}
	var children map[*component]struct{}
	if parent != nil {
		children = make(map[*component]struct{}, len(parent.componentIdentity().children)+1)
		for c := range parent.componentIdentity().children {
			children[c] = struct{}{}
		}
		children[root] = struct{}{}
	}
	byID := make(map[backend.NodeID]*component, len(nodes))
	for _, c := range nodes {
		byID[backend.NodeID(c.order)] = c
	}
	r.treeChanging = true
	defer func() { r.treeChanging = false }()
	failure := backend.ApplyTree(r.driver, change, func(result backend.TreeResult) {
		for _, c := range nodes {
			p := peers[c]
			delete(peers, c)
			delete(components, p)
			delete(generations, p)
		}
		for _, b := range result.Bindings {
			c := byID[b.ID]
			if b.Peer != 0 {
				peers[c] = b.Peer
				components[b.Peer] = c
				generations[b.Peer] = b.Generation
			}
			c.savedInput = nil
			if b.Input != nil {
				c.text = b.Input.Text
				if !change.ToAttached {
					saved := *b.Input
					saved.Focused = false
					c.savedInput = &saved
				}
			}
			if !change.FromAttached || !change.ToAttached {
				c.cancelTextNotification()
			}
			c.runtime = r
			if change.ToAttached {
				c.started = true
			}
		}
		if root.parent != nil {
			delete(root.parent.componentIdentity().children, root)
		}
		root.parent = parent
		if parent != nil {
			parent.componentIdentity().children = children
		}
		r.peers, r.components, r.bindingGenerations = peers, components, generations
		r.created = created
	}, func(failure *backend.TreeFailure) {
		r.failed = true
		r.stop()
		r.failures = append(r.failures, failure)
		r.driver.Quit()
	})
	if failure != nil {
		return failure
	}
	return nil
}

// Each request owns its snapshot; an aborted backend attempt cannot alter the
// logical cache. A saved focus observation never requests focus on attachment.
func (c *component) copySavedInput() *backend.InputState {
	if c.savedInput == nil {
		return nil
	}
	saved := *c.savedInput
	saved.Focused = false
	return &saved
}
