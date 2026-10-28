package uithread

import "syscall"

// Current returns the caller's Linux OS thread ID.
func Current() uint64 { return uint64(syscall.Gettid()) }
