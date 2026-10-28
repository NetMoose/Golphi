// Package win32 implements the native Windows MVP backend.
package win32

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func encodeText(s string) ([]uint16, error) {
	if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
		return nil, errors.New("win32: text must be valid UTF-8 without NUL")
	}
	return append(utf16.Encode([]rune(s)), 0), nil
}

const maxNativeInt = int64(1<<31 - 1)

// Bound the input before multiplication: n may be a maximum-sized Go int.
func scale(n int, dpi uint32) (int, error) {
	if n < 0 || dpi == 0 || uint64(n) > uint64(maxNativeInt*96+47)/uint64(dpi) {
		return 0, errors.New("win32: scaled coordinate exceeds native signed 32-bit range")
	}
	return int((uint64(n)*uint64(dpi) + 48) / 96), nil
}

func frameExtent(low, high int32) (int, error) {
	n := int64(high) - int64(low)
	if n <= 0 || n > maxNativeInt {
		return 0, errors.New("win32: adjusted frame exceeds native signed 32-bit range")
	}
	return int(n), nil
}
