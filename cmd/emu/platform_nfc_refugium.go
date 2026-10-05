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
// with shNFC.forceReader(). That mode exists for one walk and one reason: with
// the reader withheld here, "nothing was read" would hold by this file's
// construction and say nothing about gui. Forced on, the page's tags reach a
// real reader that gui is handed, and only gui's own gate (startScanner under
// the profile) stands between them and a read (walk_refugium_nfc.js,
// "refugium-attached"). FeatureNFC stays unset either way.
func (p *platform) NFCReader() io.ReadCloser {
	if p.nfc.isForced() {
		return p.nfc.reader()
	}
	return nil
}
