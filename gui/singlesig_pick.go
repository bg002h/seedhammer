package gui

import (
	"seedhammer.com/bip32"
	"seedhammer.com/md"
)

// ─── T6a-2: single-sig wallet-type picker (BIP-84 default + Advanced) ─────────
//
// The single-sig flagship offers ONLY the 4 single-sig script types, mainnet
// only (no network axis). BIP-84 native-segwit (wpkh) is the one-tap DEFAULT on
// the first screen; the other three sit behind an "Advanced…" submenu so the
// common operator never has to understand script types.
//
// R0-m3: this is a NEW LOCAL table — it deliberately does NOT mutate or
// index-couple to the shared package-level scriptTypePurpose (derive_xpub.go),
// whose order is load-bearing for the 6-entry path picker.

// singleSigType is one selectable single-sig wallet type.
type singleSigType struct {
	label   string
	purpose int
	script  md.ScriptKind
}

// singleSigDefault is the one-tap default (BIP-84 native segwit). It is the
// first (index 0) entry on the picker's first screen.
var singleSigDefault = singleSigType{label: "BIP-84 native segwit", purpose: 84, script: md.ScriptWpkh}

// singleSigAdvanced are the three non-default single-sig types, shown in this
// order in the "Advanced…" submenu.
var singleSigAdvanced = []singleSigType{
	{label: "BIP-44 legacy", purpose: 44, script: md.ScriptPkh},
	{label: "BIP-49 nested segwit", purpose: 49, script: md.ScriptShWpkh},
	{label: "BIP-86 taproot", purpose: 86, script: md.ScriptTr},
}

// advancedEntryIndex is the index of the "Advanced…" entry on the first screen
// (it follows the single default entry).
func advancedEntryIndex() int {
	return 1
}

// singleSigPickFlow asks the operator for the single-sig wallet type. The first
// screen offers the BIP-84 default (one tap) plus an "Advanced…" entry; picking
// Advanced opens a second ChoiceScreen of the other three types. It returns the
// BIP-43 purpose, the md.ScriptKind, and ok==false on Back from the first
// screen. Mainnet-only (no network axis).
func singleSigPickFlow(ctx *Context, th *Colors) (purpose int, script md.ScriptKind, ok bool) {
	for {
		first := &ChoiceScreen{
			Title:   "Wallet Type",
			Lead:    "Choose address type",
			Choices: []string{singleSigDefault.label, "Advanced…"},
		}
		sel, ok := first.Choose(ctx, th)
		if !ok {
			return 0, 0, false
		}
		if sel == 0 {
			return singleSigDefault.purpose, singleSigDefault.script, true
		}
		// Advanced submenu.
		labels := make([]string, len(singleSigAdvanced))
		for i, t := range singleSigAdvanced {
			labels[i] = t.label
		}
		adv := &ChoiceScreen{Title: "Advanced", Lead: "Choose address type", Choices: labels}
		aIdx, aok := adv.Choose(ctx, th)
		if !aok {
			// Back from Advanced → re-show the first (default) screen.
			continue
		}
		t := singleSigAdvanced[aIdx]
		return t.purpose, t.script, true
	}
}

// singleSigPath builds the mainnet single-sig account path m/<purpose>'/0'/0'
// (coin-type 0', account 0'). The single-sig flagship is mainnet-only.
func singleSigPath(purpose int) bip32.Path {
	const hardened = 0x80000000
	return bip32.Path{uint32(purpose) | hardened, 0 | hardened, 0 | hardened}
}
