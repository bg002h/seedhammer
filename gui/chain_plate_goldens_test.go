package gui

import (
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/bspline"
	"seedhammer.com/codex32"
	"seedhammer.com/font/constant"
	"seedhammer.com/internal/golden"
)

// ═══ THE OTHER END OF EVERY CLASS CHAIN ══════════════════════════════════════
//
// One golden per packable class, RECORDED FROM A GO LITERAL, so that
// gui/chain_class_walk_test.go can reach the same plate from bytes `me sysw
// pack` wrote and compare knot for knot.
//
// THE EQUALITY IS THE POINT, and it is the only reason these exist. A golden
// recorded from the chain itself would prove nothing but that the chain is
// deterministic: the chain would define the thing it is measured against, and a
// producer that started emitting a byte-different record would simply move both
// sides together. Recorded HERE, from a constant typed beside the assertion,
// the golden is an independent statement of what the plate is -- so a divergence
// between `me`'s record and the Go literal moves the spline and fails.
//
// It is the same arrangement gui/transaction_golden_test.go has with the four
// tx-*.bin goldens, and those were already what the Tx chain compares against.
// This file adds the five that did not exist.
//
// WHAT A MATCH DOES NOT PROVE. Both sides call the same plate builder, so this
// is an equality of INPUTS carried by two routes, not two independent renders.
// What it catches is a record that arrives at the device different from the one
// the constant describes -- a changed encoder, a changed classifier, a changed
// decode on the way out of the container. What it cannot catch is a plate
// builder that is wrong in the same way for both.
//
// THE CONTRACT IS gui/transaction_golden_test.go's: these exist in order to
// MOVE, and what they buy is that a movement is SEEN. When one fails, do not
// reach for -update first. Run
//
//	go test ./gui -run TestChainPlateGoldens -artifacts -outputdir=DIR
//
// and diff chain-*.bin.svg against chain-*.bin.orig.svg. Only once that diff is
// the edit you meant:
//
//	go test ./gui -run TestChainPlateGoldens -update
//
// SCOPE -update WITH -run: a bare `go test ./... -update` also rewrites
// backup's seventeen frozen goldens.

// The record CONTENTS every class chain packs, as constants. Each one is
// character-for-character the record in gui/testdata/chain/chain_payloads.json,
// and TestChainFixtureRecordsMatchTheGoldenConstants proves it -- so the two
// halves of the equality cannot drift apart silently.
const (
	// BIP-39's own all-zero-entropy vector; also cmd/emu/sysw_test_payload.bin's
	// seed and gui's fixtureMasterA.
	chainSeedWords = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	// 50 characters, which is the length `me`'s ms_codec accepts. See the note
	// in scripts/gen-chain-fixtures.sh: the fork's own 48- and 74-character ms1
	// fixtures are REFUSED by `me sysw pack`.
	chainCodex32 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
	// The decoded bodies. The records carry these hex-encoded (§5.3.1).
	chainFreeText   = "SEEDHAMMER II CHAIN WALK"
	chainPassphrase = "correct horse battery staple"
)

// chainMdMkMd1Chunk1 is the FIRST md1 chunk a 2-of-3 wsh Build cuts from
// cmd/emu/sysw_cards_payload.bin: master A's seed at slot 0 plus the payload's
// B@0 and C@0 cosigner cards, fingerprints omitted.
//
// MEASURED AND PASTED, not derived beside the assertion. Deriving it here would
// need the whole policy assembly restated in the test, which is the code under
// test; pinning the string instead means a changed card, a changed derivation
// or a changed encoder shows up as a diff of this line and a failing chain,
// with the two strings printed side by side. Re-derive it by running
// TestChainMdMkFromTheEmulatorsOwnPayloadToFourPlates -v, which logs the whole
// nine-plate census.
const chainMdMkMd1Chunk1 = "md1fxrvxzspqjtvyyy4qqxppcgsc27rchwsv0jskp2rsal4egz4ep5859p875x67p5s5tk09nzz08lv4"

// chainGoldenCompare is CompareBSpline over a chain golden, at the production
// stroke and the real plate bounds.
func chainGoldenCompare(t *testing.T, name string, plate Plate, update bool) {
	t.Helper()
	// A golden over an EMPTY engraving passes forever, and -update would bake
	// that in silently. Measure unions only ENGRAVED segments, so this asks
	// about ink and not about travel moves.
	if bspline.Measure(plate.Spline).Bounds.Empty() {
		t.Fatalf("%s cuts nothing", name)
	}
	P := engraverParams
	bounds := bspline.Bounds{Max: SquarePlate.Dims(P.Millimeter)}
	dir := t.TempDir()
	if err := golden.CompareBSpline(filepath.Join("testdata", name+".bin"),
		update, dir, P.StrokeWidth, bounds, plate.Spline); err != nil {
		t.Fatalf("%s moved: %v\n\n"+
			"Re-run with -artifacts -outputdir=DIR and diff\n"+
			"  %s   what the code draws NOW\n"+
			"  %s   what this golden holds\n"+
			"Only once that diff is the edit you meant:\n"+
			"  go test ./gui -run TestChainPlateGoldens -update",
			name, err,
			filepath.Join(dir, name+".bin.svg"),
			filepath.Join(dir, name+".bin.orig.svg"))
	}
}

// chainSeedPlate builds the Backup Wallet plate for a mnemonic, exactly as
// backupWalletFlow does after the operator skips the passphrase: the bare
// master fingerprint, then engraveSeed.
func chainSeedPlate(t *testing.T, words string) Plate {
	t.Helper()
	m, err := bip39.ParseMnemonic(words)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	mfp, err := masterFingerprintFor(m, &chaincfg.MainNetParams, "")
	if err != nil {
		t.Fatalf("masterFingerprintFor: %v", err)
	}
	p, err := engraveSeed(engraverParams, m, mfp)
	if err != nil {
		t.Fatalf("engraveSeed: %v", err)
	}
	return p
}

// chainCodex32Plate builds the plate engraveCodex32 hands to
// backupSeedStringFlow: the string verbatim, titled with its own identifier and
// carrying NO master fingerprint (the gui path does not derive one).
func chainCodex32Plate(t *testing.T, s string) Plate {
	t.Helper()
	cx, err := codex32.New(s)
	if err != nil {
		t.Fatalf("codex32.New: %v", err)
	}
	id, _, _ := cx.Split()
	e, err := backup.EngraveSeedString(engraverParams,
		backup.SeedString{Title: id, Seed: cx.String(), Font: constant.Font})
	if err != nil {
		t.Fatalf("EngraveSeedString: %v", err)
	}
	p, err := toPlate(e, engraverParams)
	if err != nil {
		t.Fatalf("toPlate: %v", err)
	}
	return p
}

// chainTextPlate builds the free-text plate engraveTextFlow builds after the
// operator declines the QR, takes font/sh and auto-fit, and leaves the title
// and footer blank -- which is the walk gui/chain_class_walk_test.go drives.
func chainTextPlate(t *testing.T, body string) Plate {
	t.Helper()
	p, err := ftBuildPlate(ftParamsAtSpeed(engraverParams, 0), &ftPlanSH, body, "", "", false, 0, 0)
	if err != nil {
		t.Fatalf("ftBuildPlate: %v", err)
	}
	return p
}

// chainPassPlate builds the BIP-39 Password plate with both fingerprints blank
// and no QR: the fields the walk leaves alone.
func chainPassPlate(t *testing.T, body string) Plate {
	t.Helper()
	p, err := ppBuildPlate(engraverParams, []byte(body), "", "", false, "", false)
	if err != nil {
		t.Fatalf("ppBuildPlate: %v", err)
	}
	return p
}

func TestChainPlateGoldens(t *testing.T) {
	t.Run("chain-seed", func(t *testing.T) {
		chainGoldenCompare(t, "chain-seed", chainSeedPlate(t, chainSeedWords), *update)
	})
	t.Run("chain-codex32", func(t *testing.T) {
		chainGoldenCompare(t, "chain-codex32", chainCodex32Plate(t, chainCodex32), *update)
	})
	t.Run("chain-text", func(t *testing.T) {
		chainGoldenCompare(t, "chain-text", chainTextPlate(t, chainFreeText), *update)
	})
	t.Run("chain-pass", func(t *testing.T) {
		chainGoldenCompare(t, "chain-pass", chainPassPlate(t, chainPassphrase), *update)
	})
	t.Run("chain-mdmk-md1-1", func(t *testing.T) {
		chainGoldenCompare(t, "chain-mdmk-md1-1", chainMdMkPlateFor(t, chainMdMkMd1Chunk1), *update)
	})
}

// chainMdMkPlateFor builds the TEXT + QR variant of one md1/mk1 string, which
// is variant 0 -- the one bundleEngrave's picker opens on and the one the
// ClassMDMK walk chooses.
func chainMdMkPlateFor(t *testing.T, s string) Plate {
	t.Helper()
	pl := newPlatform()
	labels, plates, err := validateMdmk(pl, s, "", "")
	if err != nil {
		t.Fatalf("validateMdmk: %v", err)
	}
	if len(plates) == 0 {
		t.Fatal("validateMdmk produced no plate")
	}
	if labels[0] != "TEXT + QR" {
		t.Fatalf("variant 0 is %q, not TEXT + QR -- the walk chooses index 0 and "+
			"this golden must be the same variant", labels[0])
	}
	return plates[0]
}
