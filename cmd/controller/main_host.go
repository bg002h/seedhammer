//go:build !(tinygo && rp)

// This file exists so `go build ./...` and `go vet ./...` succeed on a host.
// The seam in lockboot_values.go gives the package host-buildable files, and a
// main package with files must declare main.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "controller is a device build: build it with tinygo -target pico-plus2")
	os.Exit(2)
}
