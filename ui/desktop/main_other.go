//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "Conflux desktop requires Windows 11"); os.Exit(1) }
