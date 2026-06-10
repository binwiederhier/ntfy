//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ntfy-tray is only supported on Windows")
	os.Exit(1)
}
