package win32

import (
	"strings"
	"testing"
)

func TestSelectionScalarUTF16(t *testing.T) {
	const s = "Я🌍е́Z"
	for scalar, units := range []int{0, 1, 3, 4, 5, 6} {
		got, err := scalarToUTF16(s, scalar)
		if err != nil || got != units {
			t.Fatalf("scalar %d: %d %v", scalar, got, err)
		}
		back, err := utf16ToScalar(s, units)
		if err != nil || back != scalar {
			t.Fatalf("units %d: %d %v", units, back, err)
		}
	}
	for _, n := range []int{-1, 2, 7} {
		if _, err := utf16ToScalar(s, n); err == nil {
			t.Fatalf("accepted invalid UTF16 boundary %d", n)
		}
	}
	for _, n := range []int{-1, 6} {
		if _, err := scalarToUTF16(s, n); err == nil {
			t.Fatalf("accepted scalar %d", n)
		}
	}
}

func TestSelectionDirectionRequiresEvidence(t *testing.T) {
	low := caretObservation{X: 10, Y: 2, Valid: true}
	high := caretObservation{X: 40, Y: 2, Valid: true}
	for _, tc := range []struct {
		name                 string
		caret, lo, hi, cache caretObservation
		cachedReverse        bool
		wantReverse          bool
		wantError            bool
	}{
		{"forward", high, low, high, caretObservation{}, false, false, false},
		{"reverse", low, low, high, caretObservation{}, false, true, false},
		{"neither", caretObservation{X: 25, Y: 2, Valid: true}, low, high, caretObservation{}, false, false, true},
		{"ambiguous", low, low, low, caretObservation{}, false, false, true},
		{"end unavailable", high, low, caretObservation{}, caretObservation{}, false, false, true},
		{"end trusted", high, low, caretObservation{}, high, false, false, false},
		{"reverse end trusted", low, low, caretObservation{}, low, true, true, false},
		{"stale cache", high, low, caretObservation{}, low, false, false, true},
		{"cache contradicts known low", low, low, caretObservation{}, low, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectionDirection(tc.caret, tc.lo, tc.hi, tc.cache, tc.cachedReverse)
			if (err != nil) != tc.wantError || err == nil && got != tc.wantReverse {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}

func TestSelectionDirectionDiagnosticPreservesObservations(t *testing.T) {
	_, err := selectionDirection(caretObservation{X: 17, Y: 3, Valid: true}, caretObservation{X: 10, Y: 2, Valid: true}, caretObservation{X: 40, Y: 2, Valid: true}, caretObservation{X: 16, Y: 3, Valid: true}, true)
	if err == nil {
		t.Fatal("ambiguous observations accepted")
	}
	for _, part := range []string{"caret={X:17 Y:3 Valid:true}", "low={X:10 Y:2 Valid:true}", "high={X:40 Y:2 Valid:true}", "cache={X:16 Y:3 Valid:true}", "cachedReverse=true"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("diagnostic missing %q: %v", part, err)
		}
	}
}

func TestSingleLineDirectionObservedWindowsRegression(t *testing.T) {
	low := caretObservation{X: 10, Y: 0, Valid: true}
	high := caretObservation{X: 35, Y: 0, Valid: true}
	for _, tc := range []struct {
		name                            string
		caret, lo, hi, cache            caretObservation
		reverse, wantReverse, wantError bool
	}{
		{"observed high contradicts reverse cache", caretObservation{X: 35, Y: 2, Valid: true}, low, high, caretObservation{X: 35, Y: 2, Valid: true}, true, false, false},
		{"observed low despite vertical offset", caretObservation{X: 10, Y: 2, Valid: true}, low, high, caretObservation{}, false, true, false},
		{"equal X different Y remains ambiguous", caretObservation{X: 10, Y: 2, Valid: true}, low, caretObservation{X: 10, Y: 4, Valid: true}, caretObservation{}, false, false, true},
		{"neither X", caretObservation{X: 20, Y: 2, Valid: true}, low, high, caretObservation{}, false, false, true},
		{"unknown end without trusted observation", caretObservation{X: 50, Y: 2, Valid: true}, low, caretObservation{}, caretObservation{}, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := singleLineSelectionDirection(tc.caret, tc.lo, tc.hi, tc.cache, tc.reverse)
			if (err != nil) != tc.wantError || err == nil && got != tc.wantReverse {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}

func TestSingleLineDirectionMissingEndObservedWindowsRegression(t *testing.T) {
	for _, x := range []int32{181, 208} {
		// User input selected [25,31] / [29,36], invalidating the old
		// collapsed cache. EM_POSFROMCHAR(high) returned 0xffffffff.
		caret := caretObservation{X: x, Y: 2, Valid: true}
		low := caretObservation{X: x, Y: 0, Valid: true}
		reverse, err := singleLineSelectionDirection(caret, low, caretObservation{}, caretObservation{}, false)
		if err != nil || !reverse {
			t.Errorf("observed x=%d: reverse=%t err=%v", x, reverse, err)
		}
		// Endpoint readback wins over an old forward label; it does not
		// manufacture a new explicit-set observation for later snapshots.
		reverse, err = singleLineSelectionDirection(caret, low, caretObservation{}, caret, false)
		if err != nil || !reverse {
			t.Errorf("observed x=%d with contradictory cache: reverse=%t err=%v", x, reverse, err)
		}
	}
}

func TestSingleLineDirectionMissingEndRejectsInsufficientEvidence(t *testing.T) {
	low := caretObservation{X: 181, Valid: true}
	for _, tc := range []struct {
		name             string
		caret, low, high caretObservation
	}{
		{"forward end cannot be inferred from nonmatch", caretObservation{X: 210, Y: 2, Valid: true}, low, caretObservation{}},
		{"missing caret", caretObservation{}, low, caretObservation{}},
		{"both ends missing", caretObservation{X: 181, Y: 2, Valid: true}, caretObservation{}, caretObservation{}},
		{"known equal X remains ambiguous", caretObservation{X: 181, Y: 2, Valid: true}, low, caretObservation{X: 181, Y: 4, Valid: true}},
		{"missing low is not the observed text end case", caretObservation{X: 181, Y: 2, Valid: true}, caretObservation{}, low},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := singleLineSelectionDirection(tc.caret, tc.low, tc.high, caretObservation{}, false); err == nil {
				t.Fatal("accepted insufficient endpoint evidence")
			}
		})
	}
}

func TestShiftEndNativeTransition(t *testing.T) {
	caret := caretObservation{X: 223, Y: 2, Valid: true}
	for _, tc := range []struct {
		name                                string
		anchor, beforeCaret, lo, hi, length uint32
		known                               bool
		observed                            caretObservation
		want                                bool
	}{
		{"observed collapsed to end", 16, 16, 16, 31, 31, true, caret, true},
		{"reverse to forward end", 16, 10, 16, 31, 31, true, caret, true},
		{"unknown prior direction", 16, 16, 16, 31, 31, false, caret, false},
		{"wrong anchor", 16, 16, 15, 31, 31, true, caret, false},
		{"not text end", 16, 16, 16, 30, 31, true, caret, false},
		{"collapsed at end", 31, 16, 31, 31, 31, true, caret, false},
		{"same range reverse is not a range transition", 31, 16, 16, 31, 31, true, caret, false},
		{"missing caret", 16, 16, 16, 31, 31, true, caretObservation{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := shiftEndObservation(tc.anchor, tc.beforeCaret, tc.known, tc.lo, tc.hi, tc.length, tc.observed)
			if cache.Valid != tc.want {
				t.Fatalf("cache=%+v want valid=%t", cache, tc.want)
			}
			if tc.want {
				reverse, err := singleLineSelectionDirection(caret, caretObservation{X: 123, Valid: true}, caretObservation{}, cache, false)
				if err != nil || reverse {
					t.Fatalf("Shift+End snapshot: reverse=%t err=%v", reverse, err)
				}
			}
		})
	}
}

func TestSelectionObservationLifetime(t *testing.T) {
	for _, tc := range []struct {
		name       string
		msg        uint32
		w          uintptr
		invalidate bool
	}{
		{"End keyup", 0x101, 0x23, false},
		{"Shift keyup", 0x101, 0x10, false},
		{"hover", 0x200, 0, false},
		{"drag", 0x200, 1, true},
		{"navigation", 0x100, 0x25, true},
		{"selection", 0xb1, 0, true},
		{"text", 0xc, 0, true},
		{"replace", 0xc2, 0, true},
		{"paste", 0x302, 0, true},
		{"cut", 0x300, 0, true},
		{"undo", 0x304, 0, true},
		{"focus rechecks native range and caret", 7, 0, false},
		{"move hotkey", 0x100, 0x77, false},
		{"shift down", 0x100, 0x10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := invalidatesSelectionObservation(tc.msg, tc.w); got != tc.invalidate {
				t.Fatalf("invalidate=%t want %t", got, tc.invalidate)
			}
		})
	}
}
