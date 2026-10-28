// Package uithread identifies the startup OS thread reserved for native UI.
package uithread

import "runtime"

var startup uint64

func init() {
	runtime.LockOSThread()
	startup = Current()
}

// Startup returns the OS thread ID captured during package initialization.
func Startup() uint64 { return startup }

// IsUI reports whether the caller is executing on the reserved startup thread.
func IsUI() bool { return Current() == startup }
