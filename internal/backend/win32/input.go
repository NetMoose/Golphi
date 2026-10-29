package win32

import "fmt"

func scalarToUTF16(s string, offset int) (int, error) {
	units, scalars := 0, 0
	for _, r := range s {
		if scalars == offset {
			return units, nil
		}
		scalars++
		units++
		if r > 0xffff {
			units++
		}
	}
	if scalars == offset {
		return units, nil
	}
	return 0, fmt.Errorf("win32: scalar offset %d outside text", offset)
}
func utf16ToScalar(s string, offset int) (int, error) {
	units, scalars := 0, 0
	for _, r := range s {
		if units == offset {
			return scalars, nil
		}
		units++
		if r > 0xffff {
			units++
		}
		scalars++
	}
	if units == offset {
		return scalars, nil
	}
	return 0, fmt.Errorf("win32: UTF-16 offset %d is not a scalar boundary", offset)
}

// A valid cache observation comes only from an explicit EM_SETSEL followed by
// native caret readback, and is invalidated before subsequent input mutation.
type caretObservation struct {
	X, Y  int32
	Valid bool
}

func selectionDirection(caret, low, high, cache caretObservation, cachedReverse bool) (bool, error) {
	if caret.Valid && low.Valid && high.Valid && low != high {
		if caret == low {
			return true, nil
		}
		if caret == high {
			return false, nil
		}
	}
	if caret.Valid && (!low.Valid || !high.Valid) && cache.Valid && caret == cache &&
		!(cachedReverse && low.Valid && caret != low) &&
		!(!cachedReverse && high.Valid && caret != high) &&
		!(cachedReverse && high.Valid && caret == high) &&
		!(!cachedReverse && low.Valid && caret == low) {
		return cachedReverse, nil
	}
	return false, fmt.Errorf("win32: selection direction cannot be established unambiguously: caret=%+v low=%+v high=%+v cache=%+v cachedReverse=%t", caret, low, high, cache, cachedReverse)
}

func singleLineSelectionDirection(caret, low, high, cache caretObservation, cachedReverse bool) (bool, error) {
	// Golphi creates only single-line EDIT (no ES_MULTILINE). The caret
	// bitmap and glyph origins can have different Y offsets (observed 2 vs 0
	// at 96 DPI); direction is determined by distinct positions on this one line.
	rawCaret, rawLow, rawHigh, rawCache := caret, low, high, cache
	caret.Y = 0
	low.Y = 0
	high.Y = 0
	cache.Y = 0
	reverse, err := selectionDirection(caret, low, high, cache, cachedReverse)
	if err != nil {
		return false, fmt.Errorf("single-line rawCaret=%+v rawLow=%+v rawHigh=%+v rawCache=%+v: %w", rawCaret, rawLow, rawHigh, rawCache, err)
	}
	return reverse, nil
}
