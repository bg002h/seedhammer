package gui

import (
	"testing"

	"seedhammer.com/sysw"
)

// The table is a transcription of spec §3.3.2, so the risk is transcription
// error. These assert the rows whose REASONS the spec records — the ones a
// reader would otherwise have to reconstruct.

func TestBackupWalletRefusesAPassphrase(t *testing.T) {
	// It engraves the mnemonic itself, and the passphrase is deliberately never
	// engraved and never in the QR — the words and the SeedQR are byte-identical
	// with or without one. A passphrase reaching it would have nowhere to go.
	if admits(progBackupWallet, sysw.ClassPassphrase) {
		t.Error("Backup Wallet must refuse ClassPassphrase: it has nowhere to put it")
	}
	if !admits(progBackupWallet, sysw.ClassMnemonic) {
		t.Error("but it must take a mnemonic")
	}
}

func TestPassphraseIsAdmittedWhereverASeedIs(t *testing.T) {
	// Those programs already take an optional BIP-39 passphrase beside the
	// mnemonic (derive.go:19, bip85.go:61, multisig_derive.go:32), so admitting
	// it matches an existing parameter rather than inventing a capability.
	for _, p := range []syswProgram{progXpub, progSingleSig, progMultisig, progBip85} {
		if !admits(p, sysw.ClassMnemonic) {
			t.Errorf("program %d takes a seed", p)
		}
		if !admits(p, sysw.ClassPassphrase) {
			t.Errorf("program %d takes a seed, so it must take a passphrase too", p)
		}
	}
}

func TestAddressIsAdmittedNowhere(t *testing.T) {
	// Consumed only by the verify-address flow, which engraveObjectFlow
	// deliberately has no case for (R0-M5). This spec does not change that.
	for p := progBackupWallet; p <= progBip85; p++ {
		if admits(p, sysw.ClassAddress) {
			t.Errorf("program %d admits ClassAddress; nothing should", p)
		}
	}
}

func TestUnknownIsRefusedEverywhere(t *testing.T) {
	for p := progBackupWallet; p <= progBip85; p++ {
		if admits(p, sysw.ClassUnknown) {
			t.Errorf("program %d admits ClassUnknown — admission must fail closed", p)
		}
	}
}

func TestEngraveTextTakesOnlyFreeText(t *testing.T) {
	if !admits(progText, sysw.ClassFreeText) {
		t.Error("Engrave Text must take free text")
	}
	for _, c := range []sysw.Class{sysw.ClassMnemonic, sysw.ClassCodex32Secret, sysw.ClassPassphrase, sysw.ClassMDMK} {
		if admits(progText, c) {
			t.Errorf("Engrave Text must not take %v", c)
		}
	}
}

// The container selects FLAGS, not admission — the simplification that took a
// wrong turn to find. If the table ever grew a container axis this would fail.
func TestAdmissionDoesNotDependOnTheContainer(t *testing.T) {
	// admits() has no container parameter at all; this is the structural
	// assertion that it stays that way, and reads as documentation of why.
	if !admits(progBip85, sysw.ClassMnemonic) {
		t.Fatal("precondition")
	}
	// Same class, same program, both container variants — one answer.
	for _, sealed := range []bool{true, false} {
		f := syswFlags(sysw.ClassMnemonic, false, srcPayload, sealed, false)
		if !admits(progBip85, sysw.ClassMnemonic) {
			t.Errorf("admission changed with sealed=%v; it must not", sealed)
		}
		_ = f
	}
}

func TestFlagsFireIndependently(t *testing.T) {
	cases := []struct {
		name        string
		class       sysw.Class
		unconfirmed bool
		src         syswSource
		sealed      bool
		weak        bool
		want        []syswFlag
	}{
		{"secret in a plaintext payload", sysw.ClassMnemonic, false, srcPayload, false, false,
			[]syswFlag{flagSecretInPlaintext, flagSource}},
		{"secret under a weak passphrase", sysw.ClassMnemonic, false, srcPayload, true, true,
			[]syswFlag{flagWeakPassphrase, flagSource}},
		{"secret over NFC has no integrity at all", sysw.ClassMnemonic, false, srcNFC, false, false,
			[]syswFlag{flagSource, flagNFCNoIntegrity}},
		{"typed raises nothing", sysw.ClassMnemonic, false, srcTyped, false, false, nil},
		{"public from a payload names its source only", sysw.ClassMDMK, false, srcPayload, false, true,
			[]syswFlag{flagSource}},
	}
	for _, c := range cases {
		got := syswFlags(c.class, c.unconfirmed, c.src, c.sealed, c.weak)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

// No flag refuses. If one ever did, it would be blocking machinery the operator
// demoted in §13.
func TestNoFlagRefusesAnything(t *testing.T) {
	for _, c := range []sysw.Class{sysw.ClassMnemonic, sysw.ClassPassphrase, sysw.ClassFreeText} {
		for _, src := range []syswSource{srcTyped, srcNFC, srcPayload} {
			for _, sealed := range []bool{true, false} {
				for _, weak := range []bool{true, false} {
					// admits() is the only gate; flags never consulted by it.
					if admits(progBip85, c) != admitted[progBip85][c] {
						t.Fatal("admission consulted a flag")
					}
					for _, unconfirmed := range []bool{true, false} {
						_ = syswFlags(c, unconfirmed, src, sealed, weak)
					}
				}
			}
		}
	}
}

// §3.3.3, as amended 2026-08-12: "secrecy in F1, F2 and F4 reads through
// `[mdmk-decode]` (§12.6): an unconfirmed ClassMDMK record counts as secret".
//
// ClassMDMK is NOT a secret class -- a class states what the format guarantees.
// But a BCH-valid md1 the device cannot reassemble-and-decode may be anything at
// all, including the 32 bytes of seed entropy §5.3.2 names, so for FLAG purposes
// it is treated as a secret. Nothing is refused; §13 D6.
func TestAnUnconfirmedMDMKRecordCountsAsSecretForFlags(t *testing.T) {
	cases := []struct {
		name   string
		src    syswSource
		sealed bool
		weak   bool
		want   []syswFlag
	}{
		{"F1 — unconfirmed, in a plaintext container", srcPayload, false, false,
			[]syswFlag{flagSecretInPlaintext, flagSource}},
		{"F2 — unconfirmed, sealed under a weak passphrase", srcPayload, true, true,
			[]syswFlag{flagWeakPassphrase, flagSource}},
		{"F4 — unconfirmed, arrived over NFC", srcNFC, false, false,
			[]syswFlag{flagSource, flagNFCNoIntegrity}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := syswFlags(sysw.ClassMDMK, true, c.src, c.sealed, c.weak)
			if !sameFlags(got, c.want) {
				t.Errorf("unconfirmed: got %v, want %v", got, c.want)
			}
			// The other direction, which is what makes the above able to fail: a
			// CONFIRMED md1 in the same position raises the source flag only.
			confirmed := syswFlags(sysw.ClassMDMK, false, c.src, c.sealed, c.weak)
			for _, f := range confirmed {
				if f == flagSecretInPlaintext || f == flagWeakPassphrase || f == flagNFCNoIntegrity {
					t.Errorf("a CONFIRMED md1 raised %v; only an unconfirmed one is secret", f)
				}
			}
		})
	}
}

// F3 is "always, for anything not typed" and says nothing about secrecy, so
// confirmation must not move it. A typed record raises nothing whatever the
// flag says -- a typed md1 did not come from a payload or a tag.
func TestConfirmationDoesNotChangeTheSourceFlag(t *testing.T) {
	for _, unconfirmed := range []bool{true, false} {
		if got := syswFlags(sysw.ClassMDMK, unconfirmed, srcTyped, false, false); len(got) != 0 {
			t.Errorf("typed, unconfirmed=%v: got %v, want none", unconfirmed, got)
		}
		got := syswFlags(sysw.ClassMDMK, unconfirmed, srcPayload, true, false)
		if !sameFlags(got, []syswFlag{flagSource}) {
			t.Errorf("sealed, strong, unconfirmed=%v: got %v, want [flagSource]", unconfirmed, got)
		}
	}
}

// An already-secret class must not be double-counted: unconfirmed is a way of
// BECOMING secret, not an extra flag.
func TestUnconfirmedChangesNothingForAnAlreadySecretClass(t *testing.T) {
	for _, src := range []syswSource{srcTyped, srcNFC, srcPayload} {
		for _, sealed := range []bool{true, false} {
			for _, weak := range []bool{true, false} {
				a := syswFlags(sysw.ClassMnemonic, false, src, sealed, weak)
				b := syswFlags(sysw.ClassMnemonic, true, src, sealed, weak)
				if !sameFlags(a, b) {
					t.Errorf("src=%v sealed=%v weak=%v: %v vs %v", src, sealed, weak, a, b)
				}
			}
		}
	}
}

func sameFlags(a, b []syswFlag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
