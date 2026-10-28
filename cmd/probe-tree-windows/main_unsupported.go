//go:build !windows || !amd64

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "probe-tree-windows requires Windows on amd64")
	os.Exit(1)
}
