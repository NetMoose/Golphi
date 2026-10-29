//go:build (linux || windows) && amd64

package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Unlike treeFake, this observer delegates every operation to the platform driver.
// inject is explicitly fault injection at the core boundary, not a native signal.
type treeNativeObserver struct {
	nativeObserver
	inject func(backend.Event)
}

func (d *treeNativeObserver) Init(cb backend.Callbacks) error {
	d.inject = cb.Event
	return d.nativeObserver.Init(cb)
}
func init() { extraScenarios["native-tree"] = nativeTreeScenario }

func nativeTreeScenario() {
	f := NewForm()
	f.SetClientSize(640, 360)
	left, right := NewPanel(f), NewPanel(f)
	left.SetBounds(0, 0, 300, 300)
	right.SetBounds(310, 0, 300, 300)
	require(left.SetParent(f) == nil && right.SetParent(f) == nil)
	root := NewPanel(f)
	inner := NewPanel(root)
	a, b := NewEdit(root), NewEdit(inner)
	root.SetBounds(0, 0, 250, 200)
	inner.SetBounds(0, 50, 230, 100)
	a.SetBounds(0, 0, 200, 28)
	b.SetBounds(0, 0, 200, 28)
	require(inner.SetParent(root) == nil && a.SetParent(root) == nil && b.SetParent(inner) == nil)
	d := &treeNativeObserver{nativeObserver: nativeObserver{Driver: newDriver(), destroyed: make(map[backend.Peer]bool), destroyCalls: make(map[backend.Peer]int)}}
	newDriver = func() backend.Driver { return d }
	var app *application
	ac, bc := 0, 0
	phase := "move"
	a.SetOnChange(func() {
		ac++
		if phase == "move" && ac == 1 {
			a.SetText("pending-a")
		}
	})
	b.SetOnChange(func() {
		bc++
		if phase == "move" && bc == 1 {
			b.SetText("pending-b")
		}
	})
	d.start = func() {
		app = f.runtime.(*application)
		require(Post(func() {
			// First dynamic materialization must publish the entire nested subtree.
			if err := root.SetParent(left); err != nil {
				panic(fmt.Sprintf("dynamic attach: %v", err))
			}
			require(ac == 0 && bc == 0)
			ap, bp := app.peers[&a.component], app.peers[&b.component]
			require(ap != 0 && bp != 0 && app.peers[&inner.component] != 0)
			a.SetText("first-a")
			b.SetText("first-b")
			require(ac == 1 && bc == 1 && a.pending && b.pending)
			abase, bbase, ag, bg := a.base, b.base, a.generation, b.generation
			require(root.SetParent(right) == nil)
			require(a.pending && b.pending && a.base == abase && b.base == bbase && a.generation == ag && b.generation == bg)
			require(ac == 1 && bc == 1)
			readMovedText := func() {
				for _, edit := range []*Edit{a, b} {
					actual, err := d.Driver.Text(app.peers[&edit.component])
					require(err == nil && actual == edit.Text())
				}
			}
			readMovedText()
			require(Post(func() {
				require(ac == 2 && bc == 2 && a.Text() == "pending-a" && b.Text() == "pending-b")
				readMovedText()
				phase = "reattach"
				a.SetOnChange(func() {
					ac++
					if ac != 3 {
						return
					}
					require(d.callbackDepth > 0)
					old := app.peers[&a.component]
					epoch := app.bindingGenerations[old]
					a.SetText("cancel-this")
					require(root.SetParent(nil) == nil)
					require(!a.pending && !a.baseValid && !b.pending && !b.baseValid)
					// Deliberately injected old event: actual retired native handles are never touched.
					d.inject(backend.Event{Kind: backend.Change, Peer: old, Generation: epoch, Text: "stale-detached"})
					require(a.Text() == "cancel-this" && ac == 3)
					a.SetText("Б 🙂")
					require(root.SetParent(left) == nil && ac == 3 && bc == 2)
					d.inject(backend.Event{Kind: backend.Change, Peer: old, Generation: epoch, Text: "stale-reattached"})
					require(a.Text() == "Б 🙂" && ac == 3)
					// New comparison interval: original callback input A must not cancel this.
					a.SetText("A")
				})
				require(d.Driver.SetText(app.peers[&a.component], "A") == nil)
				require(ac == 3 && a.pending)
				require(Post(func() {
					require(ac == 4 && bc == 2 && a.Text() == "A")
					actual, err := d.Driver.Text(app.peers[&a.component])
					require(err == nil && actual == "A")
					// Ownership and visual-parent subtrees overlap. Destruction inside a real
					// native Change callback must stay logical until that callback unwinds.
					live := []backend.Peer{app.peers[&root.component], app.peers[&inner.component], app.peers[&a.component], app.peers[&b.component]}
					a.SetOnChange(func() {
						require(d.callbackDepth > 0)
						root.Destroy()
						inner.Destroy()
						a.Destroy()
						require(root.Destroyed() && inner.Destroyed() && a.Destroyed() && b.Destroyed())
						for _, p := range live {
							require(!d.destroyed[p])
						}
					})
					require(d.Driver.SetText(app.peers[&a.component], "destroy") == nil)
					go func() {
						time.Sleep(100 * time.Millisecond)
						require(Post(func() {
							for _, p := range live {
								require(d.destroyCalls[p] == 1)
							}
							f.Destroy()
						}) == nil)
					}()
				}) == nil)
			}) == nil)
		}) == nil)
	}
	if err := f.Run(); err != nil {
		panic(fmt.Sprintf("native tree Run: %v", err))
	}
	require(len(app.peers) == 0 && len(app.components) == 0)
	for _, count := range d.destroyCalls {
		require(count == 1)
	}
	// Inspect resource ownership without adding production test APIs.
	state := reflect.ValueOf(d.Driver).Elem()
	inspectedRegistry, inspectedCallbacks := false, false
	for _, name := range []string{"peers", "byHWND", "retired"} {
		if v := state.FieldByName(name); v.IsValid() {
			require(v.Len() == 0)
			if name == "peers" || name == "byHWND" {
				inspectedRegistry = true
			}
		}
	}
	for _, name := range []string{"callbacks", "cb"} {
		if v := state.FieldByName(name); v.IsValid() {
			require(v.FieldByName("Event").IsNil() && v.FieldByName("Failure").IsNil())
			inspectedCallbacks = true
		}
	}
	require(inspectedRegistry && inspectedCallbacks)
	fmt.Printf("native tree PASS: OnChange=%d/%d nativeChanges=%d coreReleased=%d; old-epoch events=injected\n", ac, bc, d.changes, len(d.destroyCalls))
}

func TestNativeDynamicTreeIntegration(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_TREE") != "1" {
		t.Skip("set GOLPHI_NATIVE_TREE=1 on a graphical desktop")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST=native-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native tree: context=%v process=%v\n%s", ctx.Err(), err, out)
	}
	t.Log(string(out))
}
