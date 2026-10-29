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
