//go:build windows && amd64

// Console diagnostic; deliberately does not import the production UI driver.
package main

import (
	"fmt"
	"github.com/NetMoose/Golphi/internal/uithread"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var user = syscall.NewLazyDLL("user32.dll")
var gdi = syscall.NewLazyDLL("gdi32.dll")

// call is restricted to scalar values and native handles, never Go pointers.
// Pointer-bearing calls must convert inline in LazyProc.Call for syscall lifetime safety.
func call(name string, args ...uintptr) uintptr {
	r, _, _ := user.NewProc(name).Call(args...)
	return r
}
func wide(s string) *uint16 {
	p, e := syscall.UTF16PtrFromString(s)
	if e != nil {
		panic(e)
	}
	return p
}
func require(ok bool, what string) {
	if !ok {
		panic(what)
	}
	fmt.Println("PASS:", what)
}

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type msg struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}

func pump() {
	var m msg
	for {
		present, _, _ := user.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		if present == 0 {
			return
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}
func create(class, text string, parent uintptr, x, y, w, h int) uintptr {
	style := uintptr(0x10000000 | 0x02000000 | 0x04000000)
	if parent != 0 {
		style |= 0x40000000
	} else {
		style |= 0x00CF0000
	}
	if class == "EDIT" {
		style |= 0x00800000 | 0x80
	}
	className, windowText := wide(class), wide(text)
	hwnd, _, _ := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowText)), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, 0, 0, 0)
	require(hwnd != 0, "CreateWindowExW "+class)
	return hwnd
}
func client(h uintptr) rect {
	var r rect
	ok, _, _ := user.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&r)))
	require(ok != 0, "GetClientRect")
	return r
}
func state(h uintptr) editState {
	n := call("GetWindowTextLengthW", h)
	b := make([]uint16, n+1)
	user.NewProc("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&b[0])), n+1)
	s := editState{Text: syscall.UTF16ToString(b), Focus: call("GetFocus") == h}
	user.NewProc("SendMessageW").Call(h, 0xB0, uintptr(unsafe.Pointer(&s.Start)), uintptr(unsafe.Pointer(&s.End)))
	var p point
	ok, _, _ := user.NewProc("GetCaretPos").Call(uintptr(unsafe.Pointer(&p)))
	require(ok != 0, "GetCaretPos")
	s.CaretX = p.X
	s.CaretY = p.Y
	return s
}
func restore(h uintptr, s editState) {
	text := wide(s.Text)
	ok, _, _ := user.NewProc("SetWindowTextW").Call(h, uintptr(unsafe.Pointer(text)))
	require(ok != 0, "restore Unicode text")
	call("SetFocus", h)
	call("SendMessageW", h, 0xB1, uintptr(s.Start), uintptr(s.End))
	pump()
}
func checkState(label string, want editState, h uintptr) {
	got := state(h)
	fmt.Printf("STATE %s hwnd=%#x %+v\n", label, h, got)
	require(sameState(want, got) == nil, label+" text/selection/focus/caret")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("PASS: ALL native dynamic-tree Windows assertions")
}
func run() (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	fmt.Printf("ENV go=%s os=%s arch=%s startup=%d main=%d\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, uithread.Startup(), uithread.Current())
	require(uithread.IsUI(), "startup OS thread pinned")
	var v struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}
	v.Size = uint32(unsafe.Sizeof(v))
	r, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&v)))
	require(r == 0, "RtlGetVersion")
	fmt.Printf("ENV Windows=%d.%d build=%d\n", v.Major, v.Minor, v.Build)
	require(v.Major == 10 && v.Build >= 22000, "Windows 11 or newer native precondition")
	require(call("SetProcessDPIAware") != 0, "system DPI awareness")
	dpi := call("GetDpiForSystem")
	require(dpi != 0, "system DPI")
	fmt.Printf("ENV DPI=%d scale=%.1f%% system-aware\n", dpi, float64(dpi)*100/96)
	form := create("STATIC", "Golphi dynamic tree probe", 0, 80, 80, 700, 400)
	defer func() { require(call("DestroyWindow", form) != 0, "destroy form and remaining descendants") }()
	a := create("STATIC", "", form, 20, 20, 180, 130)
	b := create("STATIC", "", form, 250, 20, 180, 130)
	nested := create("STATIC", "", a, 10, 10, 120, 80)
	originalForm, originalA, originalB, originalNested := client(form), client(a), client(b), client(nested)
	oversized := create("STATIC", "overflow", nested, 5, 5, 400, 300)
	pump()
	require(client(form) == originalForm && client(a) == originalA && client(b) == originalB && client(nested) == originalNested, "oversized nested child does not grow ancestors")
	dc := call("GetDC", oversized)
	require(dc != 0, "child DC")
	inside, _, _ := gdi.NewProc("PtVisible").Call(dc, 10, 10)
	outside, _, _ := gdi.NewProc("PtVisible").Call(dc, 200, 150)
	require(call("ReleaseDC", oversized, dc) != 0, "ReleaseDC")
	require(inside != 0 && outside == 0, "native DC clipping inside nested parent")
	require(call("DestroyWindow", oversized) != 0, "remove overflow child")
	edit := create("EDIT", "А😀Б é 日本語", a, 10, 10, 140, 28)
	call("SetForegroundWindow", form)
	call("SetActiveWindow", form)
	call("SetFocus", edit)
	call("SendMessageW", edit, 0xB1, 1, 3)
	pump()
	wanted := state(edit)
	require(wanted.Text == "А😀Б é 日本語", "initial exact Unicode text")
	require(wanted.Focus && wanted.Start == 1 && wanted.End == 3, "initial Unicode surrogate-pair selection and focus")
	old := call("SetParent", edit, b)
	require(old == a && call("GetParent", edit) == b, "same-form native SetParent")
	restore(edit, wanted)
	checkState("reparent selection", wanted, edit)
	// Exercise a collapsed selection separately: it is the insertion caret.
	call("SendMessageW", edit, 0xB1, 4, 4)
	pump()
	caret := state(edit)
	require(caret.Text == "А😀Б é 日本語" && caret.Focus && caret.Start == 4 && caret.End == 4, "initial collapsed caret exactly UTF-16 offset 4")
	replacement := create("EDIT", "", a, 10, 10, 140, 28)
	restore(replacement, caret)
	checkState("recreate collapsed caret", caret, replacement)
	require(call("DestroyWindow", edit) != 0, "destroy replaced native edit")
	edit = replacement
	restore(edit, wanted)
	checkState("recreate Unicode selection", wanted, edit)
	// Real native failure, before destroying the source. A stale HWND is invalid.
	dead := create("STATIC", "", form, 0, 200, 20, 20)
	require(call("DestroyWindow", dead) != 0, "prepare stale destination")
	before := state(edit)
	source := call("GetParent", edit)
	syscall.NewLazyDLL("kernel32.dll").NewProc("SetLastError").Call(0)
	failed, _, failure := user.NewProc("SetParent").Call(edit, dead)
	fmt.Printf("FAILURE-INJECTION SetParent result=%#x error=%v\n", failed, failure)
	require(failed == 0 && failure != syscall.Errno(0), "invalid destination returns native failure")
	// If an implementation mutated state before failing, explicitly restore source state.
	require(call("GetParent", edit) == source, "failed transfer retains source parent")
	restore(edit, before)
	checkState("failed transfer restoration", before, edit)
	count := 0
	callback := syscall.NewCallback(func(h, p uintptr) uintptr {
		require(uithread.IsUI(), "EnumChildWindows native callback on startup thread")
		fmt.Printf("CALLBACK hwnd=%#x thread=%d startup=%d\n", h, uithread.Current(), uithread.Startup())
		count++
		return 1
	})
	call("EnumChildWindows", form, callback, 0)
	require(count >= 4, "native descendant callbacks observed")
	worker := make(chan uint64)
	go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); worker <- uithread.Current() }()
	require(<-worker != uithread.Startup(), "worker separated from UI thread")
	require(client(form) == originalForm && client(a) == originalA && client(b) == originalB && client(nested) == originalNested, "all mutations preserve ancestor bounds")
	return nil
}
