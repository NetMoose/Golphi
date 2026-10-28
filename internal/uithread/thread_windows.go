package uithread

import "syscall"

var getCurrentThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

// Current returns the caller's Windows OS thread ID.
func Current() uint64 { id, _, _ := getCurrentThreadID.Call(); return uint64(id) }
