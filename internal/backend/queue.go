package backend

import (
	"errors"
	"sync"
)

var (
	ErrNotRunning = errors.New("backend queue is not running")
	ErrNilTask    = errors.New("backend queue task is nil")
)

// Queue owns all posted Go work. The native backend only wakes the UI loop.
// Post and Stop are safe across goroutines. Dispatch runs only on the UI thread,
// outside an application callback; nested Dispatch calls are ignored.
// A Queue must not be copied. Its zero value is not ready for use.
type Queue struct {
	mu                         sync.Mutex
	wake                       func() error
	tasks                      []func()
	open, stopped, dispatching bool
}

// NewQueue requires a nonblocking, thread-safe wake that never dispatches Go
// work inline and never calls back into this Queue. It must remain valid until
// Stop returns. A failed wake must not imply that a task has been accepted.
func NewQueue(wake func() error) *Queue { return &Queue{wake: wake} }

// Open is called once native preparation is complete; Stop is irreversible.
func (q *Queue) Open() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.stopped {
		q.open = true
	}
}

// Post reports admission, not completion. Holding the lock through wake makes
// rejection atomic and lets Stop fence all native wake calls before teardown.
func (q *Queue) Post(fn func()) error {
	if fn == nil {
		return ErrNilTask
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.open {
		return ErrNotRunning
	}
	if err := q.wake(); err != nil {
		return err
	}
	q.tasks = append(q.tasks, fn)
	return nil
}

// Stop closes admission and drops unstarted tasks. The current task may finish.
// After return there is no in-flight wake; only then may its native target die.
func (q *Queue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.open = false
	q.stopped = true
	q.tasks = nil
}

// Dispatch executes queued tasks sequentially. Dequeuing under the same lock
// as Stop is the start boundary: a selected task is already started even if
// Stop runs before its first instruction. Panic closes admission and is returned
// as data, never unwound through a native callback. The caller retains failure
// and requests loop exit before performing cleanup outside native callbacks.
func (q *Queue) Dispatch() *Panic {
	q.mu.Lock()
	if q.dispatching {
		q.mu.Unlock()
		return nil
	}
	q.dispatching = true
	q.mu.Unlock()
	defer func() { q.mu.Lock(); q.dispatching = false; q.mu.Unlock() }()
	for {
		q.mu.Lock()
		if !q.open || len(q.tasks) == 0 {
			q.mu.Unlock()
			return nil
		}
		fn := q.tasks[0]
		q.tasks[0] = nil
		q.tasks = q.tasks[1:]
		q.mu.Unlock()
		if failure := Capture(fn); failure != nil {
			q.Stop()
			return failure
		}
	}
}
