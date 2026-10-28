package ui

import (
	"fmt"
	"testing"

	"github.com/NetMoose/Golphi/internal/backend"
)

func geometryPanic(t *testing.T, call func()) any {
	t.Helper()
	var result any
	func() { defer func() { result = recover() }(); call() }()
	if result == nil {
		t.Fatal("geometry operation did not panic")
	}
	return result
}

func TestGeometryPreservesLogicalCoordinates(t *testing.T) {
	owner, parent := NewForm(), NewForm()
	parent.SetClientSize(501, 303)
	controls := []*control{&NewEdit(owner).control, &NewButton(owner).control, &NewLabel(owner).control}
	for _, c := range controls {
		if err := c.SetParent(parent); err != nil {
			t.Fatal(err)
		}
		// Extending beyond the client area is valid; no automatic clipping/layout.
		c.SetBounds(490, 0, 99, 27)
		if c.bounds != (backend.Bounds{X: 490, Y: 0, Width: 99, Height: 27}) {
			t.Fatalf("changed logical bounds: %+v", c.bounds)
		}
	}
	if parent.bounds != (backend.Bounds{Width: 501, Height: 303}) {
		t.Fatalf("changed client size: %+v", parent.bounds)
	}
}

func TestGeometryRejectsInvalidValuesWithoutMutation(t *testing.T) {
	f := NewForm()
	f.SetClientSize(400, 300)
	for _, size := range [][2]int{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		before := f.bounds
		geometryPanic(t, func() { f.SetClientSize(size[0], size[1]) })
		if f.bounds != before {
			t.Fatal("invalid client size mutated form")
		}
	}
	c := NewEdit(f)
	c.SetBounds(4, 5, 60, 20)
	for _, b := range [][4]int{{-1, 0, 1, 1}, {0, -1, 1, 1}, {0, 0, 0, 1}, {0, 0, -1, 1}, {0, 0, 1, 0}, {0, 0, 1, -1}} {
		before := c.bounds
		geometryPanic(t, func() { c.SetBounds(b[0], b[1], b[2], b[3]) })
		if c.bounds != before {
			t.Fatal("invalid bounds mutated control")
		}
	}
}

func TestGeometryFrozenWhenRunStarts(t *testing.T) {
	for _, state := range []string{"component", "owner", "parent"} {
		t.Run(state, func(t *testing.T) {
			f, parent := NewForm(), NewForm()
			c := NewButton(f)
			_ = c.SetParent(parent)
			c.SetBounds(1, 2, 3, 4)
			switch state {
			case "component":
				c.started = true
			case "owner":
				f.started = true
			case "parent":
				parent.started = true
			}
			before := c.bounds
			geometryPanic(t, func() { c.SetBounds(2, 3, 4, 5) })
			if c.bounds != before {
				t.Fatal("started geometry mutated")
			}
		})
	}
	f := NewForm()
	f.SetClientSize(400, 300)
	f.started = true
	before := f.bounds
	geometryPanic(t, func() { f.SetClientSize(500, 400) })
	if f.bounds != before {
		t.Fatal("started client size mutated")
	}
}

func TestGeometryDestroyedCheckPrecedesOtherValidation(t *testing.T) {
	f := NewForm()
	c := NewLabel(f)
	f.started, c.started = true, true
	f.Destroy()
	for _, call := range []func(){func() { f.SetClientSize(0, 0) }, func() { c.SetBounds(-1, -1, 0, 0) }} {
		if got := fmt.Sprint(geometryPanic(t, call)); got != "ui: operation on destroyed component" {
			t.Fatalf("wrong first validation: %s", got)
		}
	}
}
