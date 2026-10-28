package main

import "fmt"

func validateThreads(startup, main, worker uint64, callbacks []uint64) error {
	if startup == 0 || main != startup {
		return fmt.Errorf("main thread %d != nonzero startup thread %d", main, startup)
	}
	if worker == 0 || worker == startup {
		return fmt.Errorf("worker thread %d must differ from startup %d", worker, startup)
	}
	if len(callbacks) == 0 {
		return fmt.Errorf("no native callbacks observed")
	}
	for i, tid := range callbacks {
		if tid != startup {
			return fmt.Errorf("callback %d thread %d != startup %d", i, tid, startup)
		}
	}
	return nil
}
