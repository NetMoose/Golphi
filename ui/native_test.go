//go:build (linux || windows) && amd64

package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/uithread"
)

// These opt-in subprocesses enter Run on the startup thread pinned by ui.init.
// The wrapper observes real driver calls and signals; it never synthesizes events.
func init() {
	extraScenarios["native-core"] = func() { nativeCoreScenario(false) }
	extraScenarios["native-close"] = func() { nativeCoreScenario(true) }
}

type nativeObserver struct {
	backend.Driver
	start           func()
	callbackDepth   int
	changes, closes int
	destroyed       map[backend.Peer]bool
	destroyCalls    map[backend.Peer]int
}

func (d *nativeObserver) Init(cb backend.Callbacks) error {
	require(uithread.IsUI())
	original := cb.Event
	cb.Event = func(e backend.Event) {
		require(uithread.IsUI())
		d.callbackDepth++
		defer func() { d.callbackDepth-- }()
		if e.Kind == backend.Change {
			d.changes++
		}
		if e.Kind == backend.CloseRequest {
			d.closes++
		}
		original(e)
	}
	return d.Driver.Init(cb)
}
func (d *nativeObserver) Destroy(p backend.Peer) error {
	require(uithread.IsUI() && d.callbackDepth == 0)
	d.destroyCalls[p]++
	err := d.Driver.Destroy(p)
	if err == nil {
		d.destroyed[p] = true
	}
	return err
}
func (d *nativeObserver) Run(dispatch func() *backend.Panic) (*backend.Panic, error) {
	require(uithread.IsUI())
	d.start()
	return d.Driver.Run(dispatch)
}

func nativeCoreScenario(wmClose bool) {
	title := fmt.Sprintf("Golphi-native-integration-%d", os.Getpid())
	f := NewForm()
	f.SetTitle(title)
	f.SetClientSize(400, 180)
	e := NewEdit(f)
	require(e.SetParent(f) == nil)
	e.SetBounds(16, 16, 368, 28)
	e.SetText("Привет 🌍")
	dead := NewEdit(f)
	require(dead.SetParent(f) == nil)
	dead.SetBounds(16, 60, 368, 28)
	changes := 0
	e.SetOnChange(func() { changes++ })
	require(Post(func() {}) != nil)
	d := &nativeObserver{Driver: newDriver(), destroyed: make(map[backend.Peer]bool), destroyCalls: make(map[backend.Peer]int)}
	newDriver = func() backend.Driver { return d }
	var app *application
	var editPeer, deadPeer backend.Peer
	done := make(chan struct{})
	d.start = func() {
		require(changes == 0 && e.Text() == "Привет 🌍")
		app = e.runtime.(*application)
		editPeer, deadPeer = app.peers[&e.component], app.peers[&dead.component]
		// Give the real native loop time to block, then wake it from a worker.
		go func() {
			time.Sleep(150 * time.Millisecond)
			require(Post(func() {
				require(uithread.IsUI())
				e.SetText("Изменено 🙂")
				require(changes == 1 && d.changes > 0)
				actual, err := d.Driver.Text(editPeer)
				require(err == nil && actual == e.Text())
				e.SetText(actual)
				require(changes == 1)
				// Exercise native normalization/readback without baking in a platform limit.
				e.SetText(strings.Repeat("x", 70000))
				actual, err = d.Driver.Text(editPeer)
				require(err == nil && actual == e.Text() && changes == 2)
				fmt.Printf("native readback: requested=70000 actual=%d\n", len(actual))
				if len(actual) < 70000 {
					e.SetText(strings.Repeat("x", 70001))
					require(changes == 2 && e.Text() == actual)
				}
				count := 0
				e.SetOnChange(func() {
					count++
					if count == 1 {
						e.SetText("intermediate")
						e.SetText("final")
					}
				})
				e.SetText("begin")
				require(count == 1)
				require(Post(func() {
					require(count == 2 && e.Text() == "final")
					e.SetOnChange(func() { count++; e.SetText("canceled") })
					e.SetText("trigger")
					e.SetOnChange(nil)
					e.SetOnChange(func() { count++ })
					require(Post(func() {
						require(count == 3)
						// Native Change enters the handler directly: Destroy must be logical
						// immediately, while the peer survives until the native callback returns.
						dead.SetOnChange(func() {
							require(d.callbackDepth > 0)
							dead.Destroy()
							require(dead.Destroyed() && !d.destroyed[deadPeer])
							expectPanic(func() { dead.Text() })
						})
						require(d.Driver.SetText(deadPeer, "native change") == nil)
						require(dead.Destroyed())
						// Delay verification to another native dispatch, after deferred release.
						go func() {
							time.Sleep(150 * time.Millisecond)
							require(Post(func() {
								require(d.destroyed[deadPeer])
								if wmClose {
									go func() {
										out, err := exec.Command("wmctrl", "-F", "-c", title).CombinedOutput()
										if err != nil {
											panic(fmt.Sprintf("wmctrl: %v %s", err, out))
										}
									}()
								} else {
									f.Destroy()
									require(f.Destroyed() && e.Destroyed())
								}
								close(done)
							}) == nil)
						}()
					}) == nil)
				}) == nil)
			}) == nil)
		}()
	}
	if err := f.Run(); err != nil {
		panic(err)
	}
	<-done
	require(len(app.peers) == 0 && len(app.components) == 0 && len(d.destroyed) == 3)
	require(len(d.destroyCalls) == 3)
	for _, calls := range d.destroyCalls {
		require(calls == 1)
	}
	require(dead.Destroyed())
	if wmClose {
		require(!f.Destroyed() && !e.Destroyed())
		f.Destroy()
		require(f.Destroyed() && e.Destroyed())
	} else {
		require(f.Destroyed() && e.Destroyed())
	}
	require(Post(func() {}) != nil)
	if wmClose {
		require(d.closes == 1)
	}
	// Read-only reflection observes resource maps without introducing test APIs.
	state := reflect.ValueOf(d.Driver).Elem()
	for _, name := range []string{"peers", "byHWND"} {
		if field := state.FieldByName(name); field.IsValid() {
			require(field.Len() == 0)
		}
	}
	for _, name := range []string{"callbacks", "cb"} {
		if field := state.FieldByName(name); field.IsValid() {
			require(field.FieldByName("Event").IsNil() && field.FieldByName("Failure").IsNil())
		}
	}
	fmt.Printf("native core PASS: changes=%d closeRequests=%d released=%d\n", d.changes, d.closes, len(d.destroyed))
}

func TestNativeCoreIntegration(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_CORE") != "1" {
		t.Skip("set GOLPHI_NATIVE_CORE=1 on a graphical desktop")
	}
	scenarios := []string{"native-core"}
	if os.Getenv("GOLPHI_NATIVE_WM_CLOSE") == "1" {
		scenarios = append(scenarios, "native-close")
	}
	for _, s := range scenarios {
		t.Run(s, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0])
			cmd.Env = append(os.Environ(), "GOLPHI_RUN_TEST="+s)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %v\n%s", ctx.Err(), err, out)
			}
			t.Log(string(out))
		})
	}
}
