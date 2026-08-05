//go:build js

// Command emu runs the SeedHammer II firmware GUI in a browser.
//
// It is a GOOS=js build of seedhammer.com/gui -- the shipped screens, the
// shipped flows, the shipped engraving planner -- against a canvas instead of
// an LCD. Nothing above cmd/emu is emulated or reimplemented, so what it draws
// is what the machine draws.
//
// Build and serve:
//
//	./cmd/emu/build.sh && (cd cmd/emu && python3 -m http.server 8080)
//
// What it is NOT: an engraver. cmd/emu/engraver.go accepts the step stream and
// discards it, so the emulator qualifies the interface around engraving and
// says nothing about what steel comes out. For that, cut a plate -- or render
// one headlessly with cmd/plateview.
package main

import (
	"seedhammer.com/gui"
)

// Version is set by the Go linker with -ldflags='-X main.Version=...', as the
// firmware's own is. Left empty it reads "emu" on the info screen.
var Version string

func main() {
	ver := Version
	if ver == "" {
		ver = "emu"
	}
	for range gui.Run(newPlatform(), ver) {
	}
	// gui.Run returning means the GUI asked to stop. In a browser there is
	// nothing to exit to, so park forever rather than letting the Go runtime
	// report "all goroutines are asleep" as a crash.
	select {}
}
