package gui

import (
	"bytes"
	"errors"
	"io"
	"log"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/nonstandard"
	"seedhammer.com/sysw"
)

type scanner struct {
	buf      []byte
	n        int
	overflow bool
}

var (
	errScanInProgress    = errors.New("scan: in progress")
	errScanOverflow      = errors.New("scan: buffer overflow")
	errScanUnknownFormat = errors.New("scan: unknown format")
)

func (s *scanner) Scan(r io.Reader) (any, error) {
	if cap(s.buf) == 0 {
		s.buf = make([]byte, 8*1024)
	}
	nn, err := r.Read(s.buf[s.n:])
	s.n += nn
	s.overflow = s.overflow || s.n == len(s.buf)
	if s.overflow {
		// Discard the rest of the content.
		s.n = 0
		return nil, errScanOverflow
	}
	s.overflow = false
	switch err {
	case io.EOF:
	case nil:
		// Report progress.
		return nil, errScanInProgress
	default:
		log.Printf("nfc poller: %v", err)
		s.n = 0
		return nil, err
	}

	buf := s.buf[:s.n]
	s.n = 0
	if len(buf) == 0 {
		return nil, nil
	}
	const cmdPrefix = "command: "
	if bytes.HasPrefix(buf, []byte(cmdPrefix)) {
		cmd := debugCommand{string(buf[len(cmdPrefix):])}
		return cmd, nil
	} else if isSyswEncoded(buf) {
		// §5.3.1: THE TWO PREFIXES ARE MATCHED BEFORE THE SNIFFERS, and they
		// are RESERVED. A text:/pass: record whose body is not valid lowercase
		// hex is refused here rather than falling through -- a sniffer below
		// could otherwise claim it, and treating it as free text would turn a
		// malformed record into an engraved plate.
		//
		// Decoded through sysw's OWN codec, never a local hex call: the encoding
		// is normative (§5.3.1) and a second implementation of it is the shape
		// this module has already been burnt by.
		body, err := sysw.DecodeBody(string(buf))
		if err != nil {
			return nil, errScanUnknownFormat
		}
		if bytes.HasPrefix(buf, []byte(sysw.PassPrefix)) {
			return passScan(body), nil
		}
		return freeTextScan(body), nil
	} else if m, err := bip39.Parse(buf); err == nil {
		return m, nil
		// TODO: re-enable SLIP39 support. Note that
		// github.com/gavincarr/go-slip39 adds ~55kb of RAM use in the unicode
		// package.
		// } else if m, err := slip39.ParseShare(sbuf); err == nil {
		// 	res.Content = m
	} else if d, err := nonstandard.OutputDescriptor(buf); err == nil {
		return d, nil
	} else if s, err := codex32.New(string(buf)); err == nil {
		return s, nil
	} else if codex32.ValidMD(string(buf)) || codex32.ValidMK(string(buf)) {
		return mdmkText(buf), nil
	} else if _, aerr := btcaddr.DecodeAddress(string(buf), &chaincfg.MainNetParams); aerr == nil {
		return addressText(buf), nil
	} else if _, aerr := btcaddr.DecodeAddress(string(buf), &chaincfg.TestNet3Params); aerr == nil {
		return addressText(buf), nil
	} else {
		return nil, errScanUnknownFormat
	}
}

func isSyswEncoded(buf []byte) bool {
	return bytes.HasPrefix(buf, []byte(sysw.TextPrefix)) ||
		bytes.HasPrefix(buf, []byte(sysw.PassPrefix))
}

// freeTextScan is a ClassFreeText record read off a tag, ALREADY DECODED: the
// wire form is hex (§5.3.1) and nothing downstream should have to know that.
type freeTextScan string

// passScan is a ClassPassphrase record read off a tag, already decoded. It is a
// SECRET, and it is []byte rather than a string for the reason
// engravePassphraseFlow gives at length: the passphrase program owns a buffer it
// can scrub, and a Go string is a copy nothing can reach. §6.2.2a accepts the
// residue this path already carries; it does not invite another copy.
type passScan []byte

// addressText is a candidate Bitcoin address recognized by the scanner, consumed
// only by the verify-address flow. engraveObjectFlow has no addressText case, so
// a top-level address scan falls through to "unknown format" (R0-M5).
type addressText string

// mdmkText is a BCH-validated md1/mk1 string, engraved verbatim as text/QR.
type mdmkText string

type debugCommand struct {
	Command string
}
