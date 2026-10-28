package main

import (
	"bytes"
	"fmt"
	"os"
)

func checkMaps() error {
	b, e := os.ReadFile("/proc/self/maps")
	if e != nil {
		return e
	}
	if bytes.Contains(b, []byte("libgtk")) {
		return fmt.Errorf("GTK loaded by import")
	}
	return nil
}
