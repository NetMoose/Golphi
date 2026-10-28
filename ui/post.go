package ui

import (
	"sync/atomic"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Published only after native preparation and withdrawn on stop. Workers never
// access application.running or any component state. Queue's lock serializes
// admission with Stop and fences native Wake calls before driver cleanup.
var postQueue atomic.Pointer[backend.Queue]

// Post ставит fn в очередь UI-потока из любой goroutine. nil означает только
// принятие: остановка может отменить ещё не начатую задачу. Вызов не выполняет
// fn непосредственно, даже с UI-потока; вложенные задачи идут в конец очереди.
// До готовности Run, после начала остановки и для fn == nil возвращается error.
func Post(fn func()) error {
	if fn == nil {
		return backend.ErrNilTask
	}
	q := postQueue.Load()
	if q == nil {
		return backend.ErrNotRunning
	}
	return q.Post(fn)
}
