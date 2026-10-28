//go:build windows && amd64

// probe-windows is an unattended native Win32 startup-thread experiment.
package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/uithread"
	_ "github.com/NetMoose/Golphi/ui"
)

var user32 = syscall.NewLazyDLL("user32.dll")

func main() {
	if err := run(); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: native window creation, EnumWindows callbacks, startup/main/callback identity, worker separation, and cleanup")
}

func run() (err error) {
	fmt.Printf("Golphi Windows native probe; go=%s target=%s/%s CGO_ENABLED=0 build required\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	// RtlGetVersion avoids compatibility-manifest version virtualization.
	var version struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}
	version.Size = uint32(unsafe.Sizeof(version))
	status, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&version)))
	if status != 0 {
		return fmt.Errorf("RtlGetVersion NTSTATUS=0x%x", status)
	}
	fmt.Printf("Windows version=%d.%d build=%d\n", version.Major, version.Minor, version.Build)
	// Set awareness before querying DPI; otherwise Windows may return virtualized 96 DPI.
	aware, _, e := user32.NewProc("SetProcessDPIAware").Call()
	if aware == 0 {
		return fmt.Errorf("SetProcessDPIAware: %v", e)
	}
	dpiProc := user32.NewProc("GetDpiForSystem")
	if e := dpiProc.Find(); e != nil {
		return fmt.Errorf("GetDpiForSystem (Windows 10 1607+ required): %w", e)
	}
	dpi, _, _ := dpiProc.Call()
	if dpi == 0 {
		return fmt.Errorf("GetDpiForSystem returned zero")
	}
	fmt.Printf("system DPI=%d scale=%.1f%% awareness=system-aware (primary/system scale; not every monitor)\n", dpi, float64(dpi)*100/96)
	startup, mainThread := uithread.Startup(), uithread.Current()
	workerResult := make(chan uint64)
	go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); workerResult <- uithread.Current() }()
	worker := <-workerResult
	fmt.Printf("thread startup=%d main=%d worker=%d isUI=%v\n", startup, mainThread, worker, uithread.IsUI())
	if startup == 0 || mainThread != startup || worker == startup {
		return fmt.Errorf("startup/main/worker identity precondition failed")
	}
	// A hidden top-level STATIC window guarantees an enumerable window on an interactive desktop.
	class, _ := syscall.UTF16PtrFromString("STATIC")
	title, _ := syscall.UTF16PtrFromString("Golphi native callback probe")
	hwnd, _, e := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 16, 16, 0, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", e)
	}
	fmt.Printf("native window created hwnd=0x%x thread=%d\n", hwnd, uithread.Current())
	defer func() {
		tid := uithread.Current()
		result, _, destroyErr := user32.NewProc("DestroyWindow").Call(hwnd)
		fmt.Printf("cleanup DestroyWindow=%d thread=%d\n", result, tid)
		if result == 0 {
			err = fmt.Errorf("DestroyWindow: %v (previous error: %v)", destroyErr, err)
		} else if tid != startup {
			err = fmt.Errorf("cleanup thread %d != startup %d", tid, startup)
		}
	}()
	var callbacks []uint64
	ownWindowSeen := false
	callback := syscall.NewCallback(func(window, param uintptr) uintptr {
		callbacks = append(callbacks, uithread.Current())
		if window == hwnd {
			ownWindowSeen = true
		}
		return 1
	})
	result, _, e := user32.NewProc("EnumWindows").Call(callback, 0)
	fmt.Printf("native EnumWindows result=%d callbacks=%d own-window-seen=%v callback-threads=%v\n", result, len(callbacks), ownWindowSeen, callbacks)
	if result == 0 {
		return fmt.Errorf("EnumWindows: %v", e)
	}
	if !ownWindowSeen {
		return fmt.Errorf("EnumWindows did not callback with probe's native window")
	}
	return validateThreads(startup, mainThread, worker, callbacks)
}
