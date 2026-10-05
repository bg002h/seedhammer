//go:build js && refugium

package main

import (
	"io"

	"seedhammer.com/gui"
)

// The Refugium build's NFC half of the emulator platform (Refugium plan F7
// §4.2): the machine it models reads no tag for the whole power cycle, so it
// reports no FeatureNFC and hands out no reader. cmd/emu cannot see gui's
// unexported refugiumProfile, so it carries its own twin of the device's
// cmd/controller/nfc_refugium.go.

// Features reports neither secure boot nor a reader.
func (p *platform) Features() gui.Features { return 0 }

// NFCReader returns an untyped nil -- UNLESS a walk has forced the reader on
// with shNFC.forceReader(), when it hands out the page's real reader.
//
// WHAT THE FORCED MODE PROVES (corrected, review M-5). It does NOT put a reader
// in front of startScanner: gui's ctx.nfcReader() answers "no reader" under the
// profile WITHOUT calling this method, so the forced reader is never handed
// over. What the "refugium-attached" walk proves is that gui never asks -- every
// call is counted (noteAsk) and the walk asserts shNFC.readerAsks() == 0 with a
// reader on offer. startScanner's own nil-ing, the second gui layer, is pinned
// by gui's TestRefugiumStartScannerReadsNothing. FeatureNFC stays unset either
// way.
func (p *platform) NFCReader() io.ReadCloser {
	p.nfc.noteAsk()
	if p.nfc.isForced() {
		return p.nfc.reader()
	}
	return nil
}
