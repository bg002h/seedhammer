// Command buildpayloadcards emits the records file for cmd/emu's SECOND
// systemwide test payload — the one carrying cosigner key cards, which the
// first one does not.
//
// WHY A SECOND BLOB. cmd/emu/sysw_test_payload.bin holds ClassMnemonic,
// ClassPassphrase and ClassFreeText and zero ClassMDMK, so no emulator walk can
// reach the Build-policy cosigner gather — Trace A halts one screen before the
// defect the plan exists to fix. The first blob cannot simply be extended: its
// digest is pinned in sysw_test_payload.go AND photographed in the published
// Load Payload journey, so changing it makes that document wrong.
//
// It derives through the DEVICE'S OWN path (bip39.MnemonicSeed →
// hdkeychain.NewMaster → bip32.Derive → Neuter), as cmd/journeykeys does, so
// the xpubs here are the xpubs the machine derives.
//
// TEST MATERIAL, public by construction: all three masters are BIP-39's own
// published vectors. Never put funds behind them.
//
// Scratch tool for building a cmd/emu artifact; not part of the firmware.
package main

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/mk"
)

// The three masters, BIP-39 published test vectors.
const (
	masterA = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	masterB = "legal winner thank year wave sausage worth useful legal winner thank yellow"
	masterC = "letter advice cage absurd amount doctor acoustic avoid letter advice cage above"
)

// stub is a placeholder policy-id stub. mk1 requires one; buildCosignerCards
// decodes the card and does not check it, so a fixed value keeps these cards
// usable for any policy a walk builds. Not a real wallet's id.
var stub = [4]byte{0x5b, 0x48, 0xaf, 0x35}

type card struct {
	label    string
	mnemonic string
	account  uint32
}

// The coverage list, from the plan's S0 deliverable 2. Each entry exists for a
// named stage; deleting one silently strands that stage's walk.
var wanted = []card{
	{"A@0 — Trace B slot @0, and the HONEST `both` case for S4's gate", masterA, 0},
	{"A@1 — Trace B slot @1: same master, second account (multi-account)", masterA, 1},
	{"B@0 — Trace A cosigner, and Trace B slot @2 (second master)", masterB, 0},
	{"C@0 — Trace A cosigner, and Trace B slot @3", masterC, 0},
}

func main() {
	net := &chaincfg.MainNetParams
	fmt.Fprintln(os.Stderr, "# records for cmd/emu's cosigner-card payload")
	for _, w := range wanted {
		m, err := bip39.ParseMnemonic(w.mnemonic)
		if err != nil {
			fatal("parsing %s: %v", w.label, err)
		}
		pathStr := fmt.Sprintf("m/48'/0'/%d'/2'", w.account)
		path, err := bip32.ParsePath(pathStr)
		if err != nil {
			fatal("path %s: %v", pathStr, err)
		}
		seed := bip39.MnemonicSeed(m, "")
		master, err := hdkeychain.NewMaster(seed, net)
		if err != nil {
			fatal("master for %s: %v", w.label, err)
		}
		pk, err := master.ECPubKey()
		if err != nil {
			fatal("pubkey for %s: %v", w.label, err)
		}
		mfp := fmt.Sprintf("%08x", bip32.Fingerprint(pk))
		acct, err := bip32.Derive(master, path)
		if err != nil {
			fatal("derive %s: %v", pathStr, err)
		}
		pub, err := acct.Neuter()
		if err != nil {
			fatal("neuter %s: %v", w.label, err)
		}
		strs, err := mk.Encode(mk.Card{
			Network:     "mainnet",
			Path:        pathStr,
			Fingerprint: mfp,
			Stubs:       [][4]byte{stub},
			Xpub:        pub.String(),
		})
		if err != nil {
			fatal("mk.Encode %s: %v", w.label, err)
		}
		fmt.Fprintf(os.Stderr, "# %s -> %s, %d chunk(s)\n", w.label, pathStr, len(strs))
		for _, s := range strs {
			fmt.Println(s)
		}
	}
	// The seed the `both` slot derives from. Last so the cards read first.
	fmt.Println(masterA)
	fmt.Fprintln(os.Stderr, "# + ClassMnemonic (master A), for derived/both slots")
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "buildpayloadcards: "+f+"\n", a...)
	os.Exit(1)
}
