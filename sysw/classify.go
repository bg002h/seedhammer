package sysw

import (
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
)

// classifyConstellation places the record kinds the constellation already
// knows, mirroring the Rust primary's delegation to seal::record.
//
// The ORDER matters and mirrors the primary: the reserved prefixes are matched
// by Classify BEFORE this is reached, because free text is the universal
// fallback -- a sniffer running first would claim `text:...` records whose hex
// body happened to parse as something else.
func classifyConstellation(record string) Class {
	if _, err := bip39.Parse([]byte(record)); err == nil {
		return ClassMnemonic
	}
	if _, err := codex32.New(record); err == nil {
		return ClassCodex32Secret
	}
	if codex32.ValidMD(record) || codex32.ValidMK(record) {
		return ClassMDMK
	}
	return ClassUnknown
}
