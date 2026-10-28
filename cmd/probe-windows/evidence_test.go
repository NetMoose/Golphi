package main

import "testing"

func TestThreadEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		startup, main, worker uint64
		callbacks             []uint64
		ok                    bool
	}{
		{"valid", 7, 7, 8, []uint64{7, 7}, true},
		{"missing startup", 0, 0, 8, []uint64{0}, false},
		{"wrong main", 7, 8, 9, []uint64{7}, false},
		{"worker on UI", 7, 7, 7, []uint64{7}, false},
		{"no callback", 7, 7, 8, nil, false},
		{"wrong callback", 7, 7, 8, []uint64{7, 9}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateThreads(tc.startup, tc.main, tc.worker, tc.callbacks); (got == nil) != tc.ok {
				t.Fatalf("error=%v want ok=%v", got, tc.ok)
			}
		})
	}
}
