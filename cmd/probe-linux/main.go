//go:build linux

// probe-linux is an explicit GTK ABI experiment, not the Golphi runtime.
package main

import (
	"fmt"
	"github.com/NetMoose/Golphi/internal/uithread"
	_ "github.com/NetMoose/Golphi/ui"
	"github.com/ebitengine/purego"
	"os"
	"sync"
)

var callbackThread uint64
var callbackCount int
var callbackPanic any
var trampolineOnce sync.Once
var trampoline uintptr

// One process-lifetime trampoline: never allocate a callback per native object.
func clicked(_, _ uintptr) {
	defer func() {
		if p := recover(); p != nil {
			callbackPanic = p
		}
	}()
	callbackThread = uithread.Current()
	callbackCount++
}

func run() error {
	if !uithread.IsUI() {
		return fmt.Errorf("native probe requires startup thread")
	}
	lib, err := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return err
	}
	// Keep GTK loaded until process exit: GTK can install global handlers/TLS.
	var initCheck func() int32
	var major, minor, micro func() uint32
	var buttonNew func(string) uintptr
	var refSink func(uintptr) uintptr
	var unref func(uintptr)
	var connect func(uintptr, string, uintptr, uintptr, uintptr, uint32) uint64
	var emit func(uintptr, string)
	var disconnect func(uintptr, uint64)
	var defaultDisplay func() uintptr
	var instanceIsA func(uintptr, uintptr) int32
	var x11Type func() uintptr
	purego.RegisterLibFunc(&initCheck, lib, "gtk_init_check")
	purego.RegisterLibFunc(&major, lib, "gtk_get_major_version")
	purego.RegisterLibFunc(&minor, lib, "gtk_get_minor_version")
	purego.RegisterLibFunc(&micro, lib, "gtk_get_micro_version")
	purego.RegisterLibFunc(&buttonNew, lib, "gtk_button_new_with_label")
	purego.RegisterLibFunc(&refSink, lib, "g_object_ref_sink")
	purego.RegisterLibFunc(&unref, lib, "g_object_unref")
	purego.RegisterLibFunc(&connect, lib, "g_signal_connect_data")
	purego.RegisterLibFunc(&emit, lib, "g_signal_emit_by_name")
	purego.RegisterLibFunc(&disconnect, lib, "g_signal_handler_disconnect")
	purego.RegisterLibFunc(&defaultDisplay, lib, "gdk_display_get_default")
	purego.RegisterLibFunc(&instanceIsA, lib, "g_type_check_instance_is_a")
	purego.RegisterLibFunc(&x11Type, lib, "gdk_x11_display_get_type")
	if initCheck() == 0 {
		return fmt.Errorf("gtk_init_check failed (DISPLAY=%q)", os.Getenv("DISPLAY"))
	}
	if instanceIsA(defaultDisplay(), x11Type()) == 0 {
		return fmt.Errorf("GTK display is not X11")
	}
	object := buttonNew("Golphi native probe")
	if object == 0 {
		return fmt.Errorf("gtk_button_new_with_label returned nil")
	}
	refSink(object)
	defer unref(object)
	trampolineOnce.Do(func() { trampoline = purego.NewCallback(clicked) })
	handler := connect(object, "clicked", trampoline, 0, 0, 0)
	if handler == 0 {
		return fmt.Errorf("failed to connect clicked signal")
	}
	defer disconnect(object, handler)
	emit(object, "clicked")
	if callbackPanic != nil {
		return fmt.Errorf("callback panic contained: %v", callbackPanic)
	}
	if callbackCount != 1 || callbackThread != uithread.Startup() || !uithread.IsUI() {
		return fmt.Errorf("thread mismatch: startup=%d current=%d callback=%d count=%d", uithread.Startup(), uithread.Current(), callbackThread, callbackCount)
	}
	fmt.Printf("GTK=%d.%d.%d DISPLAY=%s backend=X11 startup=%d native=%d callback=%d callbacks=%d callback_same_thread=true\n", major(), minor(), micro(), os.Getenv("DISPLAY"), uithread.Startup(), uithread.Current(), callbackThread, callbackCount)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
