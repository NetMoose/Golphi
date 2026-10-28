//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "probe-tree-linux requires Linux/X11 GTK4"); os.Exit(1) }
