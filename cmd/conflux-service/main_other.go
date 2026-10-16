//go:build !windows

package main

import (
	"flag"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/version"
	"os"
)

func main() {
	v := flag.Bool("version", false, "print version")
	flag.Parse()
	if *v {
		fmt.Println(version.String())
		return
	}
	fmt.Fprintln(os.Stderr, "Conflux service requires Windows 11 x64")
	os.Exit(1)
}
