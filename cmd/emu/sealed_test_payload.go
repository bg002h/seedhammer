//go:build js

// This file exists ONLY in the browser build, and it MUST stay that way.
//
// It embeds a real §6.1 sealed payload blob and prints the passphrase that
// opens it, so the emulator can demonstrate the whole Sealed Payload flow
// (menu entry -> passphrase prompt -> KDF -> decrypt -> plate) without
// hardware. That is a rehearsal tool, not something any shipped device may
// carry: a real SeedHammer II must never boot with a pre-known passphrase
// baked in, so this data and the code that serves it are confined to
// cmd/emu, which builds ONLY under GOOS=js (main.go's "//go:build js";
// main_notjs.go's "//go:build !js" stub is what every other target compiles
// instead). cmd/controller — the real firmware's entrypoint
// (platform_sh2.go) — does not import this package, cannot import it (a
// second `func main` in the same program is a compile error), and nothing
// in gui or seal reaches back into cmd/emu. Verified by building
// cmd/controller with TinyGo and grepping the resulting .uf2 for a byte run
// unique to sealedTestPayload; see the dispatch report for the result.
//
// PROVENANCE. These are the exact bytes `me seal` produced, and the exact
// passphrase it printed, for the 2026-08-09 Plan B Phase B2b hardware
// rehearsal (mnemonic-engrave design/HARDWARE_RESULT_2026-08-09_phaseB2b.md):
// Vector F's 15 secret records (3 codex32 shares, 6 mk1 xpub cards, 6 md1
// wallet-policy cards — all published, throwaway test vectors from
// mnemonic-engrave crates/me-cli/testdata/seal_vectors.json and this repo's
// own seal/testdata/vectors.json, never anyone's funds), sealed at 300,000
// iterations. It is embedded here VERBATIM, not resealed — a fresh `me seal`
// run generates a fresh random passphrase, which would silently make the
// text below wrong.
//
// DO NOT regenerate, re-seal, or otherwise replace these bytes without also
// updating sealedTestPassphrase to match — the two are one output from one
// `me seal` invocation and must never drift apart.
package main

import (
	_ "embed"

	"seedhammer.com/seal"
)

//go:embed sealed_test_payload.bin
var sealedTestPayload string

// sealedTestPassphrase is §8.1-normalisable input, not yet normalised: Open
// normalises internally, and this is the same twelve words an operator would
// type on the device's own keyboard, letter by letter.
const sealedTestPassphrase = "mosquito neither reopen morning canoe find tiny brand resist satisfy gun ball"

// embeddedPayloadReader implements seal.Reader over the built-in test blob.
// It is the emulator's stand-in for seal.XIPReader (device) and
// seal.FileReader (host `go test`): the browser's GOOS=js/GOARCH=wasm target
// has neither XIP flash nor a real filesystem (there is no globalThis.fs in a
// browser, so os.Open in seal.FileReader would simply fail here), so this is
// the third, minimal implementation the environment actually allows.
type embeddedPayloadReader struct{ data []byte }

var _ seal.Reader = embeddedPayloadReader{}

// Probe matches seal.Magic against the blob's first 8 bytes — the same check
// XIPReader.Probe and FileReader.Probe make, just over an in-memory slice
// instead of flash or a file.
func (r embeddedPayloadReader) Probe() bool {
	return len(r.data) >= len(seal.Magic) && string(r.data[:len(seal.Magic)]) == seal.Magic
}

// Read returns the embedded blob's bytes, copied out so the caller's slice
// never aliases the package-level string (mirrors XIPReader.Read's XIP copy
// and FileReader.Read's file copy — both exist so a caller holding the
// result across other work is never surprised by it changing underneath).
func (r embeddedPayloadReader) Read() ([]byte, error) {
	if !r.Probe() {
		return nil, seal.ErrNoPayload
	}
	out := make([]byte, len(r.data))
	copy(out, r.data)
	return out, nil
}
