// Command buildpayloadcomposer emits the records file for cmd/emu's THIRD
// systemwide test payload — the one carrying the Wallet Policy COMPOSER's own
// record classes, which neither of the first two has.
//
// WHY A THIRD BLOB. The records blob (sysw_test_payload.bin) holds
// ClassMnemonic, ClassPassphrase and ClassFreeText; the cards blob
// (sysw_cards_payload.bin) holds mk1 cosigner cards and a ClassMnemonic. NEITHER
// holds a `key:`, a `hash:` or a `now:` record, so the composer's key sources,
// its hash-lock picker and the pack-time bound beside a time lock have nothing
// to draw from in the emulator, and the C8 journey — spec §12 item 2 — halts
// before the screens it exists to walk.
//
// WHY NOT EXTEND EITHER EXISTING BLOB. The records blob's digest is pinned at
// sysw_test_payload.go and PHOTOGRAPHED in the published Load Payload journey;
// the cards blob's is pinned beside its own embed and photographed in the
// cosigner-card walks. Editing either makes a published document wrong in the
// one way a reader cannot detect — it would still read as consistent. The same
// reasoning is written out at length in cmd/emu/sysw_cards_payload.go; it has
// not changed, only gained a third instance.
//
// It derives through the DEVICE'S OWN path (bip39.MnemonicSeed →
// hdkeychain.NewMaster → bip32.Derive → Neuter), as cmd/buildpayloadcards and
// cmd/journeykeys do, so the xpubs here are the xpubs the machine derives.
//
// THE CROSS-IMPLEMENTATION CHECK IS INSIDE THIS GENERATOR, NOT IN A COMMENT.
// Every xpub below is pinned to what `ms derive --template bip48-p2wsh
// --account <n>` produced on the host, and this program REFUSES TO EMIT if the
// device's own derivation disagrees with the pin. A comment saying "these match
// ms" is a claim; a refusal is a gate. Two implementations in two languages
// meeting on the same account xpub is the property the whole S4 fixture rests
// on — the keyed arm's Template-ID, Policy-ID and four addresses are all
// downstream of it.
//
// TEST MATERIAL, public by construction: both masters are BIP-39's own
// published vectors. Never put funds behind them.
//
// Scratch tool for building a cmd/emu artifact; not part of the firmware.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
)

// The two masters, BIP-39 published test vectors.
const (
	masterA = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	masterB = "legal winner thank year wave sausage worth useful legal winner thank yellow"
)

// The hashlock digest. NOT the hash of anything: a repeating byte is
// unmistakably a fixture, which is the point — an operator who sees
// `abababab..abababab` on a device screen cannot mistake it for a preimage
// somebody holds. The composer only ever carries the digest.
const hashlock = "abababababababababababababababababababababababababababababababab"

// The pack time: 2026-09-01 00:00:00 UTC at height 905000, the S3 fixture's
// values. Supplied EXPLICITLY rather than left to `me sysw pack` to append,
// because an auto-appended `now:` moves with the wall clock and the blob's
// digest is pinned — a fixture that rebuilds to a different digest every day is
// not a fixture.
const packTime = "1788220800,905000"

// slot is one `key:` record: a cosigner key the composer can seat.
type slot struct {
	label    string
	mnemonic string
	account  uint32
	// wantXpub is what `ms derive --template bip48-p2wsh --account <account>`
	// printed on the host. The refusal below is the cross-implementation check.
	wantXpub string
}

// The two `key:` records, in the order the composer's picker walks them.
//
// BOTH ARE MASTER A, AT TWO ACCOUNTS, AND THAT IS DELIBERATE. It is the
// composer's "one master, two accounts" case, and seating both into one 2-of-2
// path fires §8g's same-seed warning at the mapping review — a screen no other
// emulator fixture reaches. A reader who takes that warning for a defect has
// misread the fixture: it is the fixture's purpose.
var slots = []slot{
	{
		label:    "A@0 — composer slot @0",
		mnemonic: masterA,
		account:  0,
		wantXpub: "xpub6DkFAXWQ2dHxq2vatrt9qyA3bXYU4ToWQwCHbf5XB2mSTexcHZCeKS1VZYcPoBd5X8yVcbXFHJR9R8UCVpt82VX1VhR28mCyxUFL4r6KFrf",
	},
	{
		label:    "A@1 — composer slot @1, same master, second account",
		mnemonic: masterA,
		account:  1,
		wantXpub: "xpub6DzhyrnFFYQ1HimDiM388xHnDiRPNdZJFBmmxge3Y1WWcHLtMJLfRuhRHqnQCPbTj3fGKTuKFLHzzwpJkp5Dtc3UtLKZKaVZe1yqMBXd6Vk",
	},
}

// The seed record's own account xpub. NOT emitted — the payload carries master
// B as a mnemonic and the DEVICE derives this when the operator seats the seed
// into slot @2. It is checked anyway, because the keyed arm's whole host
// oracle (Policy-ID 4dd749a8…, the four addresses) is minted against this exact
// xpub at this exact origin; if the device's derivation of it ever diverged,
// every comparison downstream would fail with a message about an address rather
// than about a key.
const (
	seedAccount  = 0
	seedWantXpub = "xpub6FQya7zGhR92kacYsNnjreouvnHJMpXYsUXnW6NJJAJRCKsa26TzDy4LdnGhEurr3d6y1J8PJ7EEMKQp74XTqYvmGJNogYXSKDszYHtF8mX"
)

func main() {
	fmt.Fprintln(os.Stderr, "# records for cmd/emu's composer payload")

	for _, s := range slots {
		mfp, xpub, path := derive(s.label, s.mnemonic, s.account)
		if xpub != s.wantXpub {
			fatal("%s: the device's own derivation at %s gives\n  %s\nbut this file pins ms derive's\n  %s\n"+
				"One of the two implementations is wrong. Do NOT re-pin without finding out which.",
				s.label, path, xpub, s.wantXpub)
		}
		// `key:` carries the hex of the BRACKETED text — the origin without the
		// leading `m/`, then the account xpub — which is the form `me sysw pack`
		// admits and the form the composer's key source parses back.
		body := fmt.Sprintf("[%s/%s]%s", mfp, path[len("m/"):], xpub)
		fmt.Printf("key:%s\n", hex.EncodeToString([]byte(body)))
		fmt.Fprintf(os.Stderr, "# %s -> %s\n", s.label, body)
	}

	fmt.Printf("hash:%s\n", hashlock)
	fmt.Fprintf(os.Stderr, "# hash: sha256 hashlock digest (a fixture, not a hash of anything held)\n")

	fmt.Printf("now:%s\n", hex.EncodeToString([]byte(packTime)))
	fmt.Fprintf(os.Stderr, "# now: %s (seconds, height)\n", packTime)

	// The seed the composer's @2 slot is typed in from. Checked, then emitted as
	// the mnemonic itself: the device derives the account xpub, this program
	// only proves it will derive the pinned one.
	_, seedXpub, seedPath := derive("B@0 — the seed for slot @2", masterB, seedAccount)
	if seedXpub != seedWantXpub {
		fatal("the seed's own derivation at %s gives\n  %s\nbut this file pins ms derive's\n  %s\n"+
			"The keyed arm's Policy-ID and addresses are minted against the pin; do NOT re-pin without finding out which is wrong.",
			seedPath, seedXpub, seedWantXpub)
	}
	fmt.Println(masterB)
	fmt.Fprintf(os.Stderr, "# + ClassMnemonic (master B), the seed slot @2 is seated from; at %s it derives %s\n",
		seedPath, seedXpub)
}

// derive walks the DEVICE's path and returns the master fingerprint, the
// account xpub and the path string it used.
func derive(label, mnemonic string, account uint32) (mfp, xpub, path string) {
	net := &chaincfg.MainNetParams
	m, err := bip39.ParseMnemonic(mnemonic)
	if err != nil {
		fatal("parsing %s: %v", label, err)
	}
	path = fmt.Sprintf("m/48'/0'/%d'/2'", account)
	p, err := bip32.ParsePath(path)
	if err != nil {
		fatal("path %s: %v", path, err)
	}
	seed := bip39.MnemonicSeed(m, "")
	master, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		fatal("master for %s: %v", label, err)
	}
	pk, err := master.ECPubKey()
	if err != nil {
		fatal("pubkey for %s: %v", label, err)
	}
	mfp = fmt.Sprintf("%08x", bip32.Fingerprint(pk))
	acct, err := bip32.Derive(master, p)
	if err != nil {
		fatal("derive %s: %v", path, err)
	}
	pub, err := acct.Neuter()
	if err != nil {
		fatal("neuter %s: %v", label, err)
	}
	return mfp, pub.String(), path
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "buildpayloadcomposer: "+f+"\n", a...)
	os.Exit(1)
}
