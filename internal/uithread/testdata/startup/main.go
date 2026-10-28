package main

import (
	"fmt"
	"github.com/NetMoose/Golphi/internal/uithread"
	_ "github.com/NetMoose/Golphi/ui"
	"runtime"
	"time"
)

func main() {
	start := uithread.Startup()
	if start == 0 || !uithread.IsUI() || uithread.Current() != start {
		panic("main is not startup UI thread")
	}
	other := make(chan uint64)
	go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); other <- uithread.Current() }()
	if <-other == start {
		panic("worker shares pinned startup thread")
	}
	for range 100 {
		runtime.Gosched()
		time.Sleep(time.Microsecond)
		if uithread.Current() != start {
			panic("startup goroutine migrated")
		}
	}
	if err := checkMaps(); err != nil {
		panic(err)
	}
	fmt.Printf("startup=%d current=%d pinned=true gtk_loaded=false\n", start, uithread.Current())
}
