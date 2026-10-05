//go:build tinygo && rp && !refugium

package main

import (
	"io"

	"seedhammer.com/driver/st25r3916"
	"seedhammer.com/gui"
	"seedhammer.com/nfc/poller"
	"seedhammer.com/nfc/type5"
)

// initNFC wires the tag reader. The ST25R3916 is soldered to every board, so
// the reader is unconditional here — unlike SyswReader/PayloadReader, which
// report a REGION that may be empty. gui reads FeatureNFC instead of calling
// NFCReader(), because calling NFCReader() to ask whether a reader exists
// consumes a tag on the emulator (§13 D9; gui.FeatureNFC says why).
//
// The Refugium build's twin is nfc_refugium.go.
func (p *Platform) initNFC(d *st25r3916.Device) {
	p.nfc = newNFCDevice(d)
	p.feats |= gui.FeatureNFC
}

func (p *Platform) NFCReader() io.ReadCloser {
	return poller.New(p.nfc)
}

// nfcDev adapts the ST25R3916 to poller.Device. It moved here from
// platform_sh2.go with its only user: the Refugium build constructs none.
type nfcDev struct {
	*st25r3916.Device
	trans    *type5.Transceiver
	iso15693 bool
}

func newNFCDevice(d *st25r3916.Device) *nfcDev {
	return &nfcDev{
		Device: d,
		trans:  type5.NewTransceiver(d, st25r3916.FIFOSize),
	}
}

func (d *nfcDev) SetProtocol(mode poller.Protocol) error {
	d.iso15693 = false
	var prot st25r3916.Protocol
	switch mode {
	case poller.ISO14443a:
		prot = st25r3916.ISO14443a
	case poller.ISO15693:
		d.iso15693 = true
		prot = st25r3916.ISO15693
	default:
		panic("unsupported mode")
	}
	return d.Device.SetProtocol(prot)
}

func (d *nfcDev) Write(buf []byte) (int, error) {
	if d.iso15693 {
		return d.trans.Write(buf)
	}
	return d.Device.Write(buf)
}

func (d *nfcDev) Read(buf []byte) (int, error) {
	if d.iso15693 {
		return d.trans.Read(buf)
	}
	return d.Device.Read(buf)
}

func (d nfcDev) ReadCapacity() int {
	if d.iso15693 {
		return d.trans.ReadCapacity()
	}
	return st25r3916.FIFOSize
}
