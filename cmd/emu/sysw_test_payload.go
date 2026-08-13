//go:build js

// This file exists ONLY in the browser build, and it MUST stay that way, for
// the same reason sealed_test_payload.go must: a real SeedHammer II may not
// boot carrying a payload somebody else packed. The confinement argument is
// that file's, unchanged — cmd/emu builds only under GOOS=js, cmd/controller
// does not and cannot import it. TestSyswTestPayloadIsConfinedToJSOnlyFiles
// is this blob's half of that argument.
//
// WHY IT EXISTS. Load Payload is the newest program (position 8 of ten) and
// was the ONLY one the simulator could not walk: SyswReader returned nil, so
// §10.1 detection found nothing and every payload source stayed invisible.
// That is a faithful emulation of a machine with an empty region, and it is
// useless for rehearsing the flow that region exists for — the boot offer, the
// digest comparison, and the FROM PAYLOAD default the pickers acquire once one
// is loaded. Discovered while trying to write the Load Payload operator
// journey, which is built from real emulator framebuffers and therefore could
// not be written at all.
//
// PROVENANCE — these are the exact bytes `me sysw pack` emitted, 2026-08-13,
// from `me 0.6.0`:
//
//	me sysw pack --no-passphrase --in records.txt --out sysw_test_payload.bin
//
// where records.txt was, verbatim, these three lines:
//
//	abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about
//	pass:636f727265637420686f727365206261747465727920737461706c65
//	text:5345454448414d4d45522049492044454d4f205041594c4f4144
//
// decoding to a mnemonic, the passphrase "correct horse battery staple", and
// the free text "SEEDHAMMER II DEMO PAYLOAD". The hex bodies are REQUIRED, not
// stylistic: spec §5.3.1 makes `text:`/`pass:` bodies lowercase hex and refuses
// a non-hex body as ClassUnknown rather than treating it as free text.
//
// It is deliberately a THREE-CLASS payload — ClassMnemonic, ClassPassphrase,
// ClassFreeText — because a single-record payload cannot show the thing worth
// showing: that one loaded payload feeds several programs, each defaulting to
// FROM PAYLOAD for the class it wants.
//
// TEST MATERIAL, public by construction. The mnemonic is BIP-39's own
// all-zero-entropy vector and the passphrase is the xkcd string; both are
// published, and the payload is UNSEALED so there is no secret here to leak.
// Never put funds behind them.
//
// DO NOT regenerate these bytes without updating syswTestDigest below. The
// digest is what the device shows the operator to compare, so it is the one
// value a journey document quotes on both sides of the air gap; a silent
// regeneration makes the document wrong rather than making it fail.
package main

import (
	_ "embed"

	"seedhammer.com/sysw"
)

//go:embed sysw_test_payload.bin
var syswTestPayload string

// syswTestDigest is what `me sysw show` prints for that blob, and therefore
// what the device's comparison screen must display. Pinned by
// TestSyswTestPayloadMatchesItsDigest so the two cannot drift.
const syswTestDigest = "55ad b800 6ec6 a066 94f3 6a0e 900a c8d5"

// embeddedSyswReader implements sysw.Reader over the built-in blob. It is the
// emulator's stand-in for the device's XIP reader and the host's FileReader,
// for the reason embeddedPayloadReader is seal's: a browser has neither XIP
// flash nor a filesystem.
//
// It serves the CONTAINER, not a 64 KiB region image. Both are things `me sysw
// pack` emits (--region adds the padding), the padding does not change the
// digest, and Read's contract is the container's bytes — FileReader over a
// bare blob behaves identically. Embedding the padded form would add 64 KiB to
// the wasm to represent bytes the reader would immediately bound away.
type embeddedSyswReader struct{ data []byte }

var _ sysw.Reader = embeddedSyswReader{}

// Probe matches sysw.MAGIC against the first eight bytes — the same check the
// real readers make, over an in-memory slice. It is spelled out here rather
// than calling the package's own hasMagic, which is unexported.
func (r embeddedSyswReader) Probe() bool {
	return len(r.data) >= len(sysw.MAGIC) && [8]byte(r.data[:8]) == sysw.MAGIC
}

// Read returns a COPY, so a caller holding the result across other work is
// never surprised by it changing underneath — the same reason the XIP and file
// readers copy.
func (r embeddedSyswReader) Read() ([]byte, error) {
	if !r.Probe() {
		return nil, sysw.ErrBadMagic
	}
	out := make([]byte, len(r.data))
	copy(out, r.data)
	return out, nil
}
