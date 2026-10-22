//go:build !darwin

package main

import (
	"fmt"
	"os"
)

// The graphical UI is macOS-only for now (see gui.go); elsewhere v builds
// without WebView and cgo.
func runUI([]string) {
	fmt.Fprintln(os.Stderr, "v: the graphical UI is only available on macOS for now")
	os.Exit(2)
}
