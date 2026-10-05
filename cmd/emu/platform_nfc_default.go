//go:build js && !refugium

package main

import (
	"io"

	"seedhammer.com/gui"
)

// The default build's NFC half of the emulator platform. Its twin,
// platform_nfc_refugium.go, models the Refugium build's machine: no reader.

// Features reports no secure boot, so the version line reads "(UNLOCKED)".
// That is true here and worth saying: nothing about a browser build is signed.
//
// It DOES report FeatureNFC, and that is not a fib: nfcSource above is a real
// tag source, fed from `window.shNFC`, and §8.2 makes walking the NFC journeys
// in a browser part of this work. Reporting no reader here would take the SCAN
// row off every seed entry in the emulator and leave J-C unwalkable by the very
// tool that exists to walk it. The bit reports the READER, not a pending tag —
// which is why it is a constant rather than a peek at p.nfc, a peek that would
// consume the tag it looked at.
func (p *platform) Features() gui.Features { return gui.FeatureNFC }

// NFCReader hands out the page's tag source, which outlives any one tag.
//
// The comment here used to read "returns nil: this emulator has no tag source",
// which stopped being true when nfc.go was added and was never updated. It
// matters now: stage 10 made the consuming half of NFC real, so this is the
// source every J-C walk in a browser goes through.
//
// nil is still a SUPPORTED value, not a stub -- gui checks it (verify_address.go,
// mk1_inspect.go, md1_gather.go, derive_xpub.go) and offers Back-only where a
// scan would go.
//
// CALLING THIS NO LONGER CONSUMES A TAG, and the paragraph that used to say so
// here was true of the one-shot source it described. It is now a queue behind a
// persistent reader (nfc.go), because a screen fetches this ONCE at entry and
// the old shape therefore capped every flow at one card -- no bundle gather was
// walkable. The caution it carried still stands in weaker form: this is the tag
// SOURCE, so a flow that called it merely to ask whether a reader exists would
// take the reader away from the screen that needs it. Ask Features() instead,
// as derive_xpub.go:156 says at its own site.
func (p *platform) NFCReader() io.ReadCloser {
	p.nfc.noteAsk()
	return p.nfc.reader()
}
