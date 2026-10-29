package backend

import (
	"errors"
	"reflect"
	"testing"
)

// This fake supplies native actions only; ApplyTree supplies the real ordering,
// publication, rollback classification and independent panic-safe cleanup.
type nativeTreeFake struct {
	prepareErr, commitErr, restoreErr error
	result                            TreeResult
	steps                             []CleanupStep
	log                               []string
	fenced                            bool
}

func (d *nativeTreeFake) PrepareTree(TreeChange) (TreeTransaction, error) {
	d.log = append(d.log, "prepare")
	d.fenced = true
	return d, d.prepareErr
}
func (d *nativeTreeFake) Commit() (TreeResult, error) {
	d.log = append(d.log, "commit")
	return d.result, d.commitErr
}
func (d *nativeTreeFake) Rollback() error        { d.log = append(d.log, "rollback"); return d.restoreErr }
func (d *nativeTreeFake) Cleanup() []CleanupStep { return d.steps }
func (d *nativeTreeFake) Finish()                { d.log = append(d.log, "finish"); d.fenced = false }

func TestApplyTreePublishesWholeReadyResultBeforeUnfencing(t *testing.T) {
	// Removing publication, or moving it before commit/after Finish, breaks this.
	d := &nativeTreeFake{result: TreeResult{Bindings: []TreeBinding{
		{ID: 1, Peer: 11, Generation: 7},
		{ID: 2, Peer: 12, Generation: 8, Input: &InputState{Text: "А😀Б𐐷В", Anchor: 1, Caret: 4, Focused: true}},
	}}}
	published := false
	err := ApplyTree(d, TreeChange{FromAttached: true, ToAttached: true}, func(r TreeResult) {
		if !d.fenced || !reflect.DeepEqual(d.log, []string{"prepare", "commit"}) {
			t.Fatal("publication outside committed fence", d.log)
		}
		if len(r.Bindings) != 2 || r.Bindings[0].Peer != 11 || r.Bindings[1].ID != 2 || r.Bindings[1].Peer != 12 || r.Bindings[1].Generation != 8 {
			t.Fatalf("partial or altered binding result: %#v", r)
		}
		input := r.Bindings[1].Input
		if input == nil || input.Text != "А😀Б𐐷В" || input.Anchor != 1 || input.Caret != 4 || !input.Focused {
			t.Fatalf("altered input: %#v", input)
		}
		published = true
		d.log = append(d.log, "publish")
	}, func(*TreeFailure) { t.Fatal("success stopped runtime") })
	if err != nil || !published || d.fenced || !reflect.DeepEqual(d.log, []string{"prepare", "commit", "publish", "finish"}) {
		t.Fatalf("publication=%v failure=%v log=%v", published, err, d.log)
	}
}

func TestApplyTreeFailureRetainsDiagnosticsAndContinuesCleanup(t *testing.T) {
	// Dropping rollback, treating cleanup failure as fatal, or letting cleanup
	// panic skip the next resource all break externally observed results.
	original := errors.New("create edit")
	cleanup := errors.New("release panel")
	restore := errors.New("restore focus")
	for _, phase := range []string{"prepare", "commit"} {
		for _, broken := range []bool{false, true} {
			d := &nativeTreeFake{}
			if phase == "prepare" {
				d.prepareErr = original
			} else {
				d.commitErr = original
			}
			if broken {
				d.restoreErr = restore
			}
			fatalCalled := false
			var retained *TreeFailure
			d.steps = []CleanupStep{
				{"panel", func() error {
					if broken && !fatalCalled {
						t.Fatal("cleanup before fatal admission closure")
					}
					d.log = append(d.log, "release-panel")
					return cleanup
				}},
				{"edit", func() error { d.log = append(d.log, "release-edit"); panic("native release") }},
				{"label", func() error { d.log = append(d.log, "release-label"); return nil }},
			}
			got := ApplyTree(d, TreeChange{}, func(TreeResult) { t.Fatal("failed tree published") }, func(e *TreeFailure) {
				if !d.fenced {
					t.Fatal("fatal outside fence")
				}
				fatalCalled = true
				retained = e
			})
			if got == nil {
				t.Fatal("lost operation failure")
			}
			if !errors.Is(got, original) || !errors.Is(got, cleanup) || got.Fatal() != broken || fatalCalled != broken {
				t.Fatalf("bad classification: %#v", got)
			}
			if broken && (!errors.Is(got, restore) || retained != got) {
				t.Fatal("lost fatal diagnostics")
			}
			if len(got.Cleanup) != 2 || got.Cleanup[1].Panic == nil || got.Cleanup[1].Panic.Value != "native release" || len(got.Cleanup[1].Panic.Stack) == 0 {
				t.Fatalf("lost cleanup panic: %#v", got.Cleanup)
			}
			want := []string{"prepare"}
			if phase == "commit" {
				want = append(want, "commit")
			}
			want = append(want, "rollback", "release-panel", "release-edit", "release-label", "finish")
			if d.fenced || !reflect.DeepEqual(d.log, want) {
				t.Fatalf("log %v, want %v", d.log, want)
			}
		}
	}
}

func TestApplyTreeUnsupportedDoesNotPublish(t *testing.T) {
	got := ApplyTree(UnsupportedTree{}, TreeChange{}, func(TreeResult) { t.Fatal("unsupported published") }, func(*TreeFailure) { t.Fatal("unsupported fatal") })
	if got == nil || !errors.Is(got, ErrTreeUnsupported) || got.Fatal() {
		t.Fatalf("got %#v", got)
	}
}
