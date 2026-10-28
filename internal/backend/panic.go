package backend

import "runtime/debug"

// Panic preserves the original value and stack without choosing a public error
// representation. Native adapters retain this data until their loop returns.
type Panic struct {
	Value any
	Stack []byte
}

// Capture is the outer guard of every Go callback entered through FFI. The
// adapter must keep ABI return conversion and failure storage non-panicking.
// No user callback, including a failure handler, may run after Capture returns
// a failure. Fatal runtime faults and os.Exit cannot be recovered here.
func Capture(fn func()) (failure *Panic) {
	completed := false
	defer func() {
		if !completed {
			failure = &Panic{Value: recover(), Stack: debug.Stack()}
		}
	}()
	fn()
	completed = true
	return nil
}
