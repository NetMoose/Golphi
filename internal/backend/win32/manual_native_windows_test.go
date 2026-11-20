//go:build windows && amd64

package win32

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

// This is an operator workflow, not an automatic visual/focus proof. It uses
// the production driver, explicit confirmations and a bounded 180-second run.
func TestNativeDynamicTreeManual(t *testing.T) {
	if os.Getenv("GOLPHI_NATIVE_TREE_MANUAL") != "1" {
		t.Skip("set GOLPHI_NATIVE_TREE_MANUAL=1 for the interactive Windows workflow")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := New().(*driver)
	if err := d.Init(backend.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	}()
	user := d.dlls[0]
	for _, name := range []string{"GetForegroundWindow", "GetWindowThreadProcessId", "GetClientRect"} {
		p, err := user.FindProc(name)
		if err != nil {
			t.Fatal(err)
		}
		d.api[name] = p
	}
	create := func(k backend.Kind, parent backend.Peer, text string, b backend.Bounds) backend.Peer {
		t.Helper()
		id, err := d.Create(backend.Config{Kind: k, Parent: parent, Text: text, Bounds: b})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := create(backend.Form, 0, "Golphi manual tree: 180 seconds", backend.Bounds{Width: 660, Height: 380})
	create(backend.Label, f, "Nested clipping: child must stop at x=240 / y=160", backend.Bounds{X: 20, Y: 20, Width: 620, Height: 25})
	outer := create(backend.Panel, f, "", backend.Bounds{X: 40, Y: 70, Width: 220, Height: 110})
	inner := create(backend.Panel, outer, "", backend.Bounds{X: 20, Y: 20, Width: 180, Height: 70})
	create(backend.Button, inner, "OVERFLOW: right and bottom must be clipped", backend.Bounds{X: 20, Y: 15, Width: 350, Height: 150})
	create(backend.Label, f, "Area outside nested panels must stay clear", backend.Bounds{X: 280, Y: 90, Width: 350, Height: 35})
	left := create(backend.Panel, f, "", backend.Bounds{X: 30, Y: 240, Width: 260, Height: 90})
	right := create(backend.Panel, f, "", backend.Bounds{X: 330, Y: 240, Width: 260, Height: 90})
	edit := create(backend.Edit, left, "Move while Notepad stays foreground: Я🌍", backend.Bounds{X: 5, Y: 5, Width: 240, Height: 30})
	d.cb.Event = func(e backend.Event) {
		if e.Kind == backend.CloseRequest {
			d.Quit()
		}
	}
	if err := d.Show(f); err != nil {
		t.Fatal(err)
	}
	var initialOuter, initialInner rect
	readRect := func(id backend.Peer, r *rect) error {
		_, err := d.call("GetClientRect", d.peers[id].hwnd, uintptr(unsafe.Pointer(r)))
		return err
	}
	if err := readRect(outer, &initialOuter); err != nil {
		t.Fatal(err)
	}
	if err := readRect(inner, &initialInner); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("AUTO_SETUP outer=%+v inner=%+v dpi=%d; screenshot required; no visual PASS inferred\n", initialOuter, initialInner, d.dpi)
	fmt.Println("MANUAL: inspect clipping and save screenshot. Type clip-pass or fail.")
	fmt.Println("MANUAL: type arm <Notepad PID>, then activate that Notepad window. The move waits for that exact foreground PID.")
	fmt.Println("MANUAL: after MOVE log, confirm Notepad stayed active with focus-pass. Type done to finish. Timeout: 180s.")
	commands := make(chan string, 16)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			select {
			case commands <- scanner.Text():
				_ = d.Wake()
			case <-stop:
				return
			}
		}
	}()
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				_ = d.Wake()
			case <-stop:
				return
			}
		}
	}()
	deadline := time.Now().Add(180 * time.Second)
	var armedPID uint32
	clipConfirmed, focusConfirmed, moved, completed := false, false, false, false
	var workflowErr error
	p, runErr := d.Run(func() *backend.Panic {
		if time.Now().After(deadline) {
			workflowErr = fmt.Errorf("manual workflow timed out without completion")
			d.Quit()
			return nil
		}
		select {
		case raw := <-commands:
			fields := strings.Fields(raw)
			if len(fields) == 0 {
				return nil
			}
			switch fields[0] {
			case "clip-pass":
				clipConfirmed = true
				fmt.Println("USER_CONFIRM clipping=yes (operator observation; screenshot required)")
			case "focus-pass":
				if moved {
					focusConfirmed = true
					fmt.Println("USER_CONFIRM foreground_stayed=yes (operator observation)")
				} else {
					fmt.Println("REJECT focus-pass: no completed move")
				}
			case "arm":
				if len(fields) != 2 {
					fmt.Println("REJECT use: arm <PID>")
					break
				}
				pid, err := strconv.ParseUint(fields[1], 10, 32)
				if err != nil || pid == 0 || pid == uint64(os.Getpid()) {
					fmt.Println("REJECT invalid/executable own PID")
					break
				}
				armedPID = uint32(pid)
				fmt.Printf("ARMED target_pid=%d; activate that external application now\n", armedPID)
			case "fail":
				workflowErr = fmt.Errorf("operator rejected manual scenario")
				d.Quit()
			case "done":
				if clipConfirmed && focusConfirmed && moved {
					completed = true
					d.Quit()
				} else {
					fmt.Println("REJECT done: need clip-pass, completed move, focus-pass")
				}
			default:
				fmt.Println("COMMANDS: clip-pass | arm <PID> | focus-pass | done | fail")
			}
		default:
		}
		if armedPID != 0 && !moved {
			before, _, _ := d.api["GetForegroundWindow"].Call()
			var pid uint32
			d.api["GetWindowThreadProcessId"].Call(before, uintptr(unsafe.Pointer(&pid)))
			if pid == armedPID {
				c := backend.TreeChange{FromAttached: true, ToAttached: true, Target: right, Nodes: []backend.TreeNode{{ID: 1, Peer: edit, Kind: backend.Edit, Text: "Move while Notepad stays foreground: Я🌍", Bounds: backend.Bounds{X: 5, Y: 5, Width: 240, Height: 30}}}}
				failure := backend.ApplyTree(d, c, func(backend.TreeResult) {}, func(f *backend.TreeFailure) { workflowErr = f })
				after, _, _ := d.api["GetForegroundWindow"].Call()
				fmt.Printf("AUTO_MOVE pid=%d foreground_before=%#x foreground_after=%#x failure=%v\n", pid, before, after, failure)
				if failure != nil {
					workflowErr = failure
					d.Quit()
					return nil
				}
				if before != after {
					workflowErr = fmt.Errorf("move changed foreground HWND")
					d.Quit()
					return nil
				}
				var out, in rect
				if err := readRect(outer, &out); err != nil {
					workflowErr = err
					d.Quit()
					return nil
				}
				if err := readRect(inner, &in); err != nil {
					workflowErr = err
					d.Quit()
					return nil
				}
				if out != initialOuter || in != initialInner {
					workflowErr = fmt.Errorf("panel client size grew")
					d.Quit()
					return nil
				}
				moved = true
				fmt.Println("AUTO_CHECK foreground HWND unchanged; panel client sizes unchanged. Return to console and type focus-pass, then done.")
			}
		}
		return nil
	})
	if p != nil || runErr != nil || workflowErr != nil {
		t.Fatalf("manual run: panic=%v runtime=%v workflow=%v", p, runErr, workflowErr)
	}
	if !completed {
		t.Fatal("manual workflow closed before explicit completion")
	}
	fmt.Println("MANUAL_WORKFLOW_COMPLETE: confirmations recorded; automated PASS is not pixel clipping proof")
}
