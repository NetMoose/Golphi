package ui

import (
	"reflect"
	"strings"
	"testing"
)

type eventHarness struct {
	running   bool
	queue     []func()
	normalize func(string) string
}

func (r *eventHarness) eventsRunning() bool   { return r.running }
func (r *eventHarness) enqueueEvent(f func()) { r.queue = append(r.queue, f) }
func (r *eventHarness) syncText(c *component, s string) string {
	if r.normalize != nil {
		s = r.normalize(s)
	}
	c.nativeText(s)
	return s
}
func (r *eventHarness) drain() {
	for len(r.queue) > 0 {
		f := r.queue[0]
		r.queue = r.queue[1:]
		f()
	}
}
func eventEdit(t *testing.T) (*Edit, *eventHarness) {
	t.Helper()
	f := NewForm()
	e := NewEdit(f)
	if err := e.SetParent(f); err != nil {
		t.Fatal(err)
	}
	r := &eventHarness{running: true}
	e.runtime = r
	return e, r
}

func TestEventsEffectiveAndSilent(t *testing.T) {
	e, r := eventEdit(t)
	var got []string
	e.SetOnChange(func() { got = append(got, e.Text()) })
	r.running = false
	e.SetText("prepare")
	r.running = true
	e.SetText("prepare")
	r.normalize = strings.ToUpper
	e.SetText("привет")
	e.SetText("привет")
	e.nativeText("Мир")
	e.nativeText("Мир")
	if !reflect.DeepEqual(got, []string{"ПРИВЕТ", "Мир"}) {
		t.Fatal(got)
	}
	if err := e.SetParent(nil); err != nil {
		t.Fatal(err)
	}
	e.SetText("detached")
	if err := e.SetParent(e.Owner()); err != nil {
		t.Fatal(err)
	}
	r.drain()
	if len(got) != 2 {
		t.Fatal(got)
	}
}
func TestEventsCoalescing(t *testing.T) {
	for _, end := range []string{"A", "C"} {
		t.Run(end, func(t *testing.T) {
			e, r := eventEdit(t)
			var got []string
			inside := false
			e.SetOnChange(func() {
				if inside {
					t.Fatal("recursive")
				}
				inside = true
				defer func() { inside = false }()
				got = append(got, e.Text())
				if len(got) == 1 {
					e.SetText("B")
					if e.Text() != "B" {
						t.Fatal(e.Text())
					}
					e.SetText(end)
				}
			})
			e.SetText("A")
			if len(got) != 1 {
				t.Fatal(got)
			}
			r.drain()
			want := []string{"A"}
			if end != "A" {
				want = append(want, end)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got)
			}
		})
	}
}
func TestEventsCancellationAndReplacement(t *testing.T) {
	for _, mode := range []string{"replace", "nil", "nil-inside", "reset", "destroy", "cascade", "detach", "failed-detach", "stop"} {
		t.Run(mode, func(t *testing.T) {
			e, r := eventEdit(t)
			var got []string
			next := func() { got = append(got, "next:"+e.Text()) }
			e.SetOnChange(func() {
				got = append(got, e.Text())
				e.SetText("B")
				switch mode {
				case "nil-inside":
					e.SetOnChange(nil)
					e.SetOnChange(next)
				case "reset":
					e.SetOnChange(nil)
					e.SetOnChange(next)
					e.SetText("A")
				case "destroy":
					e.Destroy()
				case "cascade":
					e.Owner().Destroy()
				default:
					e.SetOnChange(next)
				}
			})
			e.SetText("A")
			switch mode {
			case "nil":
				e.SetOnChange(nil)
				e.SetOnChange(next)
			case "detach":
				p := e.Parent()
				_ = e.SetParent(nil)
				_ = e.SetParent(p)
			case "failed-detach":
				p := NewForm()
				p.Destroy()
				if e.SetParent(p) == nil {
					t.Fatal("expected error")
				}
			case "stop":
				r.running = false
			}
			r.drain()
			want := []string{"A"}
			switch mode {
			case "replace", "failed-detach":
				want = append(want, "next:B")
			case "reset":
				want = append(want, "next:A")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
		})
	}
}
func TestClickReplacementAndDestroyedGuard(t *testing.T) {
	f := NewForm()
	b := NewButton(f)
	_ = b.SetParent(f)
	r := &eventHarness{running: true}
	b.runtime = r
	n := 0
	b.SetOnClick(func() { n++ })
	b.SetOnClick(func() { n += 2 })
	b.nativeClick()
	b.SetOnClick(nil)
	b.nativeClick()
	if n != 2 {
		t.Fatal(n)
	}
	b.Destroy()
	defer func() {
		if recover() == nil {
			t.Fatal("no destroyed panic")
		}
	}()
	b.SetOnClick(nil)
}

func TestEventsResetAfterDisabledEdit(t *testing.T) {
	e, r := eventEdit(t)
	var got []string
	e.SetOnChange(func() {
		got = append(got, e.Text())
		e.SetOnChange(nil)
		e.SetText("B")
		e.SetOnChange(func() { got = append(got, e.Text()) })
		e.SetText("A")
	})
	e.SetText("A")
	r.drain()
	if !reflect.DeepEqual(got, []string{"A", "A"}) {
		t.Fatal(got)
	}
}
func TestEventsAssignmentAfterDisabledEditIsSilent(t *testing.T) {
	e, r := eventEdit(t)
	n := 0
	e.SetOnChange(func() { n++; e.SetOnChange(nil); e.SetText("B"); e.SetOnChange(func() { n++ }) })
	e.SetText("A")
	r.drain()
	if n != 1 {
		t.Fatal(n)
	}
}
func TestEventsPendingChangesUseFinalValue(t *testing.T) {
	for _, end := range []string{"A", "D"} {
		t.Run(end, func(t *testing.T) {
			e, r := eventEdit(t)
			var got []string
			e.SetOnChange(func() {
				got = append(got, e.Text())
				if len(got) == 1 {
					e.SetText("B")
				}
			})
			e.SetText("A")
			e.SetText("C")
			e.SetText(end)
			r.drain()
			want := []string{"A"}
			if end != "A" {
				want = append(want, end)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got)
			}
		})
	}
}
func TestEventsDestroyedSettersGuardFirst(t *testing.T) {
	e, _ := eventEdit(t)
	e.Destroy()
	for _, f := range []func(){func() { e.SetOnChange(nil) }, func() { e.SetText("x") }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			f()
		}()
	}
}
func TestEventsCancelledQueueDoesNotCancelNewChange(t *testing.T) {
	e, r := eventEdit(t)
	var got []string
	e.SetOnChange(func() {
		got = append(got, e.Text())
		if len(got) == 1 {
			e.SetText("B")
		}
	})
	e.SetText("A")
	e.SetOnChange(nil)
	e.SetOnChange(func() { got = append(got, e.Text()) })
	e.SetText("C")
	r.drain()
	if !reflect.DeepEqual(got, []string{"A", "C"}) {
		t.Fatal(got)
	}
}
