package win32

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestUTF16(t *testing.T) {
	s := "Привет 🌍"
	got, err := encodeText(s)
	if err != nil || string(utf16.Decode(got[:len(got)-1])) != s || got[len(got)-1] != 0 {
		t.Fatalf("%v %v", got, err)
	}
	if !reflect.DeepEqual(got[len(got)-3:], []uint16{0xd83c, 0xdf0d, 0}) {
		t.Fatal(got)
	}
	for _, bad := range []string{"x\x00y", string([]byte{0xff})} {
		if _, err := encodeText(bad); err == nil {
			t.Fatal("accepted invalid text")
		}
	}
}
func TestStartupScale(t *testing.T) {
	for _, v := range []struct {
		n    int
		dpi  uint32
		want int
	}{{400, 96, 400}, {28, 144, 42}, {1, 120, 1}, {16, 192, 32}} {
		if got, err := scale(v.n, v.dpi); got != v.want || err != nil {
			t.Fatal(got, v)
		}
	}
}

func TestScaleRejectsOverflowBeforeMultiplication(t *testing.T) {
	for _, v := range []struct {
		n   int
		dpi uint32
	}{{int(^uint(0) >> 1), 192}, {int(maxNativeInt), 192}, {100, ^uint32(0)}, {-1, 96}, {1, 0}} {
		if _, err := scale(v.n, v.dpi); err == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	if got, err := scale(int(maxNativeInt), 96); err != nil || int64(got) != maxNativeInt {
		t.Fatal(got, err)
	}
	if got, err := scale(int(maxNativeInt/2), 192); err != nil || int64(got) != maxNativeInt-1 {
		t.Fatal(got, err)
	}
}
func TestAdjustedFrameExtentRejectsOverflow(t *testing.T) {
	for _, v := range [][2]int32{{-8, 2147483647}, {-2147483648, 2147483647}, {10, 10}, {10, 9}} {
		if _, err := frameExtent(v[0], v[1]); err == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	if got, err := frameExtent(-8, 408); err != nil || got != 416 {
		t.Fatal(got, err)
	}
}
