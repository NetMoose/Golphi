//go:build windows && amd64

package win32

import (
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Diagnostic only: PASS means every observation was collected, not that the
// dynamic-tree selection contract passed. No SnapshotInput/restoreInput calls
// are made here. The raw arm has no Golphi edit subclass or selection cache.
func TestNativeEditSelectionProbe(t *testing.T) {
	if os.Getenv("GOLPHI_EDIT_SELECTION_PROBE") != "1" {
		t.Skip("set GOLPHI_EDIT_SELECTION_PROBE=1 on interactive Windows 11 amd64")
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
	form, err := d.Create(backend.Config{Kind: backend.Form, Text: "Golphi selection diagnostic", Bounds: backend.Bounds{Width: 400, Height: 180}})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Show(form); err != nil {
		t.Fatal(err)
	}
	// CommCtrl.h: ECM_FIRST=0x1500; EM_SETCARETINDEX=+17, EM_GETCARETINDEX=+18.
	// https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/um/CommCtrl.h
	// Microsoft documents these messages from Windows 10 1809; a zero result alone
	// cannot establish support. Collapsed positions 1 and high calibrate readback.
	const setCaretIndex, getCaretIndex = 0x1511, 0x1512
	type step struct {
		name      string
		msg, w, l uintptr
	}
	completed := 0
	for _, sample := range []struct {
		name, text string
		high       uintptr
	}{{"ascii", "ABCDE", 4}, {"surrogates", "А🌍Б𐐷В", 6}} {
		low, high := uintptr(1), sample.high
		for _, scenario := range []struct {
			name  string
			steps []step
		}{
			{"calibration", []step{{"collapse-low", 0xb1, low, low}, {"collapse-high", 0xb1, high, high}}},
			{"forward", []step{{"forward", 0xb1, low, high}}},
			{"reverse", []step{{"reverse", 0xb1, high, low}}},
			{"same-range-reverse", []step{{"forward", 0xb1, low, high}, {"reverse", 0xb1, high, low}}},
			{"collapsed-reverse", []step{{"forward", 0xb1, low, high}, {"collapse-anchor", 0xb1, high, high}, {"reverse", 0xb1, high, low}}},
			{"explicit-caret-index", []step{{"forward", 0xb1, low, high}, {"set-index-low", setCaretIndex, low, 0}, {"set-index-high", setCaretIndex, high, 0}}},
		} {
			for _, mode := range []string{"raw", "golphi"} {
				// Each arm gets a new EDIT and identical parent/style/geometry/text. Native
				// child destruction before the next arm prevents cross-case selection state.
				var hwnd uintptr
				var id backend.Peer
				if mode == "golphi" {
					id, err = d.Create(backend.Config{Kind: backend.Edit, Parent: form, Text: sample.text, Bounds: backend.Bounds{X: 10, Y: 10, Width: 300, Height: 40}})
					if err != nil {
						t.Fatal(err)
					}
					hwnd = d.peers[id].hwnd
				} else {
					class, _ := encodeText("EDIT")
					text, _ := encodeText(sample.text)
					var geometry [4]uintptr
					for i, logical := range []int{10, 10, 300, 40} {
						physical, scaleErr := scale(logical, d.dpi)
						if scaleErr != nil {
							t.Fatal(scaleErr)
						}
						geometry[i] = uintptr(physical)
					}
					hwnd, err = d.call("CreateWindowExW", 0, ptr(&class[0]), ptr(&text[0]), uintptr(childStyle|0x00800000|0x00010000|0x80), geometry[0], geometry[1], geometry[2], geometry[3], d.peers[form].hwnd, 0, d.instance, 0)
					runtime.KeepAlive(class)
					runtime.KeepAlive(text)
					if err != nil {
						t.Fatal(err)
					}
				}
				// Always destroy even if focus/setup assertions abort the test.
				func() {
					defer func() {
						if id != 0 {
							err = d.Destroy(id)
						} else {
							_, err = d.call("DestroyWindow", hwnd)
						}
						if err != nil {
							t.Error(err)
						}
					}()
					d.api["SetFocus"].Call(hwnd)
					focus, _, _ := d.api["GetFocus"].Call()
					if focus != hwnd {
						t.Fatalf("probe focus setup: mode=%s sample=%s case=%s want=%#x got=%#x", mode, sample.name, scenario.name, hwnd, focus)
					}
					observe := func(phase string) {
						lo, hi := d.selection(hwnd)
						focus, _, _ := d.api["GetFocus"].Call()
						var cp point
						ok, _, caretErr := d.api["GetCaretPos"].Call(uintptr(unsafe.Pointer(&cp)))
						lower, _, _ := d.api["SendMessageW"].Call(hwnd, 0xd6, uintptr(lo), 0)
						upper, _, _ := d.api["SendMessageW"].Call(hwnd, 0xd6, uintptr(hi), 0)
						index, _, _ := d.api["SendMessageW"].Call(hwnd, getCaretIndex, 0, 0)
						t.Logf("EDIT_PROBE mode=%s sample=%s case=%s phase=%s hwnd=%#x focus=%#x dpi=%d rangeUTF16=[%d,%d] caretOK=%d caretErr=%v caretXY=(%d,%d) lowRaw=%#x lowXY=(%d,%d) highRaw=%#x highXY=(%d,%d) caretIndexRaw=%d", mode, sample.name, scenario.name, phase, hwnd, focus, d.dpi, lo, hi, ok, caretErr, cp.x, cp.y, uint32(lower), int16(lower), int16(lower>>16), uint32(upper), int16(upper), int16(upper>>16), index)
						if id != 0 {
							p := d.peers[id]
							t.Logf("EDIT_PROBE_CACHE anchorUTF16=%d caretUTF16=%d known=%t observation=%+v", p.selectionAnchor, p.selectionCaret, p.selectionKnown, p.selectionObservation)
						}
						if focus != hwnd || ok == 0 {
							t.Fatalf("probe observation invalid: focus=%#x hwnd=%#x caretOK=%d", focus, hwnd, ok)
						}
					}
					observe("focused-initial")
					for _, s := range scenario.steps {
						t.Logf("EDIT_PROBE_SEND mode=%s sample=%s case=%s phase=%s msg=%#x w=%d l=%d", mode, sample.name, scenario.name, s.name, s.msg, s.w, s.l)
						d.api["SendMessageW"].Call(hwnd, s.msg, s.w, s.l)
						observe(s.name)
					}
					if d.failure != nil {
						t.Fatalf("backend callback failure: %+v", d.failure)
					}
					completed++
				}()
			}
		}
	}
	if completed != 24 {
		t.Fatalf("incomplete probe: %d/24", completed)
	}
	t.Logf("EDIT_SELECTION_PROBE_COMPLETE cases=%d diagnostic-only; not dynamic-tree acceptance", completed)
}
