//go:build windows && amd64

package win32

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Diagnostic only. The operator supplies real keyboard input. No EM_SETSEL,
// SnapshotInput, restoreInput, or synthetic keyboard input is used.
func TestNativeEditKeyboardSelectionProbe(t *testing.T) {
	if os.Getenv("GOLPHI_EDIT_KEYBOARD_PROBE") != "1" {
		t.Skip("set GOLPHI_EDIT_KEYBOARD_PROBE=1 on interactive Windows amd64")
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
	for _, name := range []string{"PostMessageW", "GetKeyState"} {
		p, err := d.dlls[0].FindProc(name)
		if err != nil {
			t.Fatal(err)
		}
		d.api[name] = p
	}
	create := func(k backend.Kind, parent backend.Peer, text string, b backend.Bounds) backend.Peer {
		id, err := d.Create(backend.Config{Kind: k, Parent: parent, Text: text, Bounds: b})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := create(backend.Form, 0, "Keyboard selection probe: 180 seconds", backend.Bounds{Width: 740, Height: 260})
	create(backend.Label, f, "Click ABCDEF. End; Left once; Shift+Left three times; release Shift; F8 to move.", backend.Bounds{X: 10, Y: 10, Width: 710, Height: 30})
	status := create(backend.Label, f, "After LATER log: Shift+Right ONCE, release Shift, then F9. See console.", backend.Bounds{X: 10, Y: 45, Width: 710, Height: 30})
	left := create(backend.Panel, f, "", backend.Bounds{X: 10, Y: 100, Width: 320, Height: 80})
	right := create(backend.Panel, f, "", backend.Bounds{X: 370, Y: 100, Width: 320, Height: 80})
	edit := create(backend.Edit, left, "ABCDEF", backend.Bounds{X: 5, Y: 5, Width: 280, Height: 35})
	hwnd := d.peers[edit].hwnd
	const fence = 0x8000 + 177
	type event struct {
		kind     string
		shift    bool
		count    int
		repeated bool
	}
	var events []event
	var oldProc uintptr
	var rightShift bool
	var rightCount int
	var rightRepeated bool
	hook := syscall.NewCallback(func(h uintptr, msg uint32, w, l uintptr) uintptr {
		// Always delegate first, including our fence; observations and mutations run
		// after DispatchMessageW returns, in d.Run's application dispatch.
		if msg == 0x100 && w != 0x10 && w != 0x27 && w != 0x78 {
			events = append(events, event{kind: "other-key"})
		}
		if msg == 0x100 && w == 0x27 {
			focus, _, _ := d.api["GetFocus"].Call()
			shift, _, _ := d.api["GetKeyState"].Call(0x10)
			rightShift = focus == hwnd && shift&0x8000 != 0
			rightCount++
			if l&(1<<30) != 0 || l&0xffff != 1 {
				rightRepeated = true
			}
		}
		result, _, _ := d.api["CallWindowProcW"].Call(oldProc, h, uintptr(msg), w, l)
		focus, _, _ := d.api["GetFocus"].Call()
		if msg == 0x7 || msg == 0x8 {
			events = append(events, event{kind: fmt.Sprintf("focus-message-%#x", msg)})
		}
		if msg == fence {
			events = append(events, event{kind: "fence"})
		}
		if msg == 0x101 && focus == hwnd {
			switch w {
			case 0x77:
				events = append(events, event{kind: "move"})
			case 0x78:
				events = append(events, event{kind: "finish"})
			case 0x27:
				events = append(events, event{kind: "right", shift: rightShift, count: rightCount, repeated: rightRepeated})
				rightCount = 0
				rightRepeated = false
			}
		}
		return result
	})
	var err error
	oldProc, err = d.call("SetWindowLongPtrW", hwnd, ^uintptr(3), hook)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { d.api["SetWindowLongPtrW"].Call(hwnd, ^uintptr(3), oldProc); runtime.KeepAlive(hook) }()
	type observation struct {
		lo, hi         uint32
		focused, lower bool
		text           string
	}
	var observationErr error
	observe := func(phase string) observation {
		lo, hi := d.selection(hwnd)
		focus, _, _ := d.api["GetFocus"].Call()
		parent, _, _ := d.api["GetParent"].Call(hwnd)
		var cp point
		ok, _, _ := d.api["GetCaretPos"].Call(uintptr(unsafe.Pointer(&cp)))
		low, _, _ := d.api["SendMessageW"].Call(hwnd, 0xd6, uintptr(lo), 0)
		high, _, _ := d.api["SendMessageW"].Call(hwnd, 0xd6, uintptr(hi), 0)
		text, textErr := d.Text(edit)
		if textErr != nil {
			observationErr = textErr
		}
		if focus == hwnd && ok == 0 {
			observationErr = fmt.Errorf("%s: focused Edit GetCaretPos failed", phase)
		}
		lower := ok != 0 && uint32(low) != 0xffffffff && uint32(high) != 0xffffffff && int16(low) != int16(high) && cp.x == int32(int16(low))
		fmt.Printf("EDIT_KEYBOARD_PROBE phase=%s hwnd=%#x peerHWND=%#x parent=%#x focus=%#x rangeUTF16=[%d,%d] caretOK=%d caretXY=(%d,%d) lowRaw=%#x highRaw=%#x lower=%t text=%q textErr=%v\n", phase, hwnd, d.peers[edit].hwnd, parent, focus, lo, hi, ok, cp.x, cp.y, uint32(low), uint32(high), lower, text, textErr)
		return observation{lo, hi, focus == hwnd, lower, text}
	}
	d.cb.Event = func(e backend.Event) {
		if e.Kind == backend.CloseRequest {
			d.Quit()
		}
	}
	if err := d.Show(f); err != nil {
		t.Fatal(err)
	}
	fmt.Println("EDIT_KEYBOARD_PROBE instructions: click ABCDEF; End; Left once; Shift+Left THREE times; release Shift; F8. Wait for phase=later-dispatched-fence, then Shift+Right ONCE; release Shift; F9. Do not click or type into console. If focus is lost after move, record that fact; clicking Edit to continue contaminates continuation evidence.")
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				_ = d.Wake()
			case <-stop:
				return
			}
		}
	}()
	deadline := time.Now().Add(180 * time.Second)
	moved, later, continued, completed := false, false, false, false
	var workflowErr error
	var continuation observation
	focusEventsAfterMove := 0
	p, runErr := d.Run(func() *backend.Panic {
		fail := func(e error) { workflowErr = e; d.Quit() }
		if time.Now().After(deadline) {
			fail(fmt.Errorf("180s timeout: incomplete keyboard experiment"))
			return nil
		}
		pending := events
		events = nil
		for _, e := range pending {
			switch e.kind {
			case "other-key":
				if moved && later {
					fail(fmt.Errorf("extra keyboard input after fence invalidates continuation"))
					return nil
				}
			case "focus-message-0x7", "focus-message-0x8":
				if moved {
					focusEventsAfterMove++
					observe(e.kind)
				}
			case "move":
				if moved {
					continue
				}
				before := observe("before")
				if observationErr != nil {
					fail(observationErr)
					return nil
				}
				if before.text != "ABCDEF" || before.lo != 2 || before.hi != 5 || !before.focused || !before.lower {
					fmt.Println("EDIT_KEYBOARD_PROBE_REJECT F8 requires native ABCDEF range=[2,5], focused caret at lower endpoint; retry End then Left once then Shift+Left three times")
					continue
				}
				if err := d.place(edit, right, backend.Bounds{X: 5, Y: 5, Width: 280, Height: 35}); err != nil {
					fail(err)
					return nil
				}
				moved = true
				observe("immediate")
				// A posted message dispatched by the real pump, not a sleep/timer, fences
				// the later observation. Timer wakes only enforce the bounded deadline.
				if _, err := d.call("PostMessageW", hwnd, fence, 0, 0); err != nil {
					fail(err)
					return nil
				}
			case "fence":
				if moved && !later {
					later = true
					after := observe("later-dispatched-fence")
					if observationErr != nil {
						fail(observationErr)
						return nil
					}
					if !after.focused {
						completed = true
						fmt.Println("EDIT_KEYBOARD_SELECTION_PROBE_COMPLETE diagnostic_only=true production_acceptance=false continuation_performed=false reason=focus-lost-after-move")
						d.Quit()
						return nil
					}
					if err := d.SetText(status, "Move/fence observed. NOW Shift+Right ONCE; release Shift; F9 to finish."); err != nil {
						fail(err)
						return nil
					}
					fmt.Println("EDIT_KEYBOARD_PROBE continue: Shift+Right ONCE, release Shift, F9")
				}
			case "right":
				if moved && later {
					if continued || !e.shift || e.count != 1 || e.repeated {
						fail(fmt.Errorf("continuation requires exactly one real Shift+Right after fence"))
						return nil
					}
					continued = true
					continuation = observe("after-real-shift-right-keyup")
					if observationErr != nil {
						fail(observationErr)
						return nil
					}
				}
			case "finish":
				if moved && later && continued {
					observe("final-F9-keyup")
					if observationErr != nil {
						fail(observationErr)
						return nil
					}
					completed = true
					fmt.Printf("EDIT_KEYBOARD_SELECTION_PROBE_COMPLETE diagnostic_only=true production_acceptance=false before_anchor=5 before_caret=2 continuation_performed=true expected_continuation_range=[3,5] expected_continuation_caret=3 focus_events_after_move=%d continuation_matches=%t\n", focusEventsAfterMove, continuation.lo == 3 && continuation.hi == 5 && continuation.lower && continuation.focused && continuation.text == "ABCDEF")
					d.Quit()
				} else {
					fmt.Println("EDIT_KEYBOARD_PROBE_REJECT F9: need valid F8, dispatched fence, one Shift+Right")
				}
			}
		}
		return nil
	})
	if p != nil || runErr != nil || workflowErr != nil {
		t.Fatalf("keyboard probe panic=%v runtime=%v workflow=%v", p, runErr, workflowErr)
	}
	if !completed {
		t.Fatal("keyboard experiment closed before complete observations")
	}
}
