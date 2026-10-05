//go:build tinygo && rp && refugium

package main

import (
	"io"

	"seedhammer.com/driver/st25r3916"
)

// initNFC turns the reader OFF and keeps it off (Refugium plan F7 §4.2, D3).
//
// The platform reports no gui.FeatureNFC and NFCReader hands out nothing, so no
// screen offers a scan and no poll loop ever runs; gui's own gate
// (startScanner under the profile) is the second layer.
//
// Close writes regOpCtrl (0x02) = 0, clearing en, rx_en, tx_en and wu
// (driver/st25r3916). New touches no register, so this write is the firmware's
// ONLY contact with the chip in this build. It is made anyway because a warm
// reboot (machine.CPUReset) does not reset the ST25R3916: a field a previous
// image left on would otherwise stay on.
//
// If the write fails the field state is unknown, and the platform records a
// fault that gui turns into a power-off screen before anything else is drawn
// (NFCFault; gui/nfc_fault.go).
func (p *Platform) initNFC(d *st25r3916.Device) {
	if err := d.Close(); err != nil {
		p.nfcFault = err
	}
}

// NFCReader returns an UNTYPED nil: gui's startScanner compares the interface
// with nil, and a typed nil would start a poll loop on a nil device.
func (p *Platform) NFCReader() io.ReadCloser {
	return nil
}

// NFCFault reports a failure to turn the reader's field off at boot. gui
// asserts this optional interface at the top of every session; the other
// platforms do not implement it.
func (p *Platform) NFCFault() error {
	return p.nfcFault
}

// nfcDev is EMPTY in this build: Platform keeps its nfc field, but nothing
// constructs the poller adapter (nfc_default.go), so the driver's read path
// is not linked.
type nfcDev struct{}
