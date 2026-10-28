//go:build linux && amd64

package gtk4

import (
	"github.com/NetMoose/Golphi/internal/backend"
	"testing"
)

func TestTextValidation(t *testing.T) {
	for _, s := range []string{"a\x00b", "\xff"} {
		if validText(s) == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if err := validText("Привет 🌍"); err != nil {
		t.Fatal(err)
	}
}
func TestClosedWake(t *testing.T) {
	d := New()
	if d.Wake() == nil {
		t.Fatal("wake before init")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

var _ backend.Driver = New()

func TestCreateRejectsUnrepresentableDimensionsBeforeAllocation(t *testing.T) {
	for _, bounds := range []backend.Bounds{
		{Width: 1 << 31, Height: 100},
		{Width: 100, Height: 1 << 31},
		{Width: (1 << 32) + 480, Height: 260},
		{Width: 480, Height: (1 << 32) + 260},
	} {
		allocations := 0
		d := &driver{ready: true, peers: make(map[backend.Peer]*peer)}
		d.api.windowNew = func() uintptr { allocations++; return 0 }
		id, err := d.Create(backend.Config{Kind: backend.Form, Bounds: bounds})
		if err == nil || id != 0 || allocations != 0 || d.next != 0 || len(d.peers) != 0 {
			t.Fatalf("bounds=%+v id=%d err=%v allocations=%d next=%d peers=%d", bounds, id, err, allocations, d.next, len(d.peers))
		}
	}
}

func TestNativeDimensionBoundary(t *testing.T) {
	for _, bounds := range []backend.Bounds{
		{Width: 1, Height: 1},
		{Width: 1<<31 - 1, Height: 1<<31 - 1},
	} {
		if err := validDimensions(bounds); err != nil {
			t.Fatalf("valid bounds=%+v: %v", bounds, err)
		}
	}
	for _, bounds := range []backend.Bounds{
		{Width: 0, Height: 1}, {Width: 1, Height: 0},
		{Width: -1, Height: 1}, {Width: 1, Height: -1},
		{Width: 1 << 31, Height: 1}, {Width: 1, Height: 1 << 31},
	} {
		if err := validDimensions(bounds); err == nil {
			t.Fatalf("accepted bounds=%+v", bounds)
		}
	}
}
