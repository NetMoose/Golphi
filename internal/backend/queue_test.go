package backend

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeWake models only an asynchronous native notification, never Go dispatch.
type fakeWake struct {
	ready chan struct{}
	err   error
}

func (f *fakeWake) Wake() error {
	if f.err != nil {
		return f.err
	}
	select {
	case f.ready <- struct{}{}:
	default:
	}
	return nil
}
func newTestQueue() (*Queue, *fakeWake) {
	f := &fakeWake{ready: make(chan struct{}, 1)}
	return NewQueue(f.Wake), f
}

func TestQueueAsyncFIFOAndNestedTail(t *testing.T) {
	q, f := newTestQueue()
	q.Open()
	var got []string
	q.Post(func() {
		got = append(got, "A start")
		q.Post(func() { got = append(got, "C") })
		q.Dispatch()
		got = append(got, "A end")
	})
	q.Post(func() { got = append(got, "B") })
	if len(got) != 0 {
		t.Fatal("Post dispatched inline")
	}
	<-f.ready
	if failure := q.Dispatch(); failure != nil {
		t.Fatal(failure)
	}
	if !reflect.DeepEqual(got, []string{"A start", "A end", "B", "C"}) {
		t.Fatal(got)
	}
}

func TestQueueAdmissionAndStop(t *testing.T) {
	q, _ := newTestQueue()
	ran := false
	if q.Post(func() { ran = true }) == nil {
		t.Fatal("accepted before readiness")
	}
	q.Open()
	if q.Post(nil) == nil {
		t.Fatal("accepted nil")
	}
	q.Post(func() { q.Stop() })
	q.Post(func() { ran = true })
	q.Dispatch()
	q.Open() // A stopped queue must never reopen.
	if q.Post(func() { ran = true }) == nil {
		t.Fatal("accepted after stop")
	}
	q.Dispatch()
	if ran {
		t.Fatal("cancelled task ran")
	}
}

func TestQueuePanicStopsAndRetainsValue(t *testing.T) {
	q, _ := newTestQueue()
	q.Open()
	value := &struct{}{}
	ran := false
	q.Post(func() { panic(value) })
	q.Post(func() { ran = true })
	failure := q.Dispatch()
	if failure == nil || failure.Value != value || len(failure.Stack) == 0 {
		t.Fatalf("lost panic: %#v", failure)
	}
	if ran || q.Post(func() {}) == nil {
		t.Fatal("panic did not close admission")
	}
}

func TestQueueWakeFailureRejectsTask(t *testing.T) {
	q, f := newTestQueue()
	q.Open()
	sentinel := errors.New("wake failed")
	f.err = sentinel
	ran := false
	if err := q.Post(func() { ran = true }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	f.err = nil
	q.Dispatch()
	if ran {
		t.Fatal("rejected task executed")
	}
}

func TestQueueConcurrentPostAndStop(t *testing.T) {
	q, _ := newTestQueue()
	q.Open()
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() { q.Post(func() { t.Error("cancelled work executed") }) })
	}
	q.Stop()
	wg.Wait()
	q.Dispatch()
	if q.Post(func() {}) == nil {
		t.Fatal("accepted after stop")
	}
}

func TestQueueConcurrentWakeAndDispatch(t *testing.T) {
	q, f := newTestQueue()
	q.Open()
	const n = 500
	var wg sync.WaitGroup
	count := 0
	for range n {
		wg.Go(func() {
			if err := q.Post(func() { count++ }); err != nil {
				t.Error(err)
			}
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for {
		select {
		case <-f.ready:
			q.Dispatch()
		case <-done:
			q.Dispatch()
			if count != n {
				t.Fatalf("lost tasks: %d", count)
			}
			return
		}
	}
}

func TestCaptureNativeCallbackPanic(t *testing.T) {
	if p := Capture(func() {}); p != nil {
		t.Fatal(p)
	}
	value := errors.New("callback failure")
	p := Capture(func() { panic(value) })
	if p == nil || p.Value != value || len(p.Stack) == 0 {
		t.Fatalf("lost callback panic: %#v", p)
	}
}

// Removing the wake/stop serialization would allow Close to free a live wake
// target. The blocked wake makes that lifetime race deterministic.
func TestStopFencesInFlightWake(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	q := NewQueue(func() error { close(entered); <-release; return nil })
	q.Open()
	posted := make(chan error, 1)
	go func() { posted <- q.Post(func() { t.Error("cancelled task ran") }) }()
	<-entered
	stopped := make(chan struct{})
	go func() { q.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned during native wake")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	<-stopped
	q.Dispatch()
	if q.Post(func() {}) == nil {
		t.Fatal("native wake used after teardown fence")
	}
}
