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
