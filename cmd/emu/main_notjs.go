//go:build !js

// This file exists so `go build ./...` and `go vet ./...` succeed on a host.
// Without it the package has no buildable files off GOOS=js, and every
// repo-wide command fails on cmd/emu rather than skipping it.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "emu is a browser build: run ./cmd/emu/build.sh (GOOS=js GOARCH=wasm)")
	os.Exit(2)
}
