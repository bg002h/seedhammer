// Package bundle composes the headless single-sig constellation pieces (md1
// policy card, mk1 key card, ms1 secret card) for the T6a verify-bundle flow.
// It is the only place md + mk + codex32 are composed (md cannot import mk).
// No GUI here (that is T6a-2); this is the deterministic comparator core.
package bundle

import (
	"bytes"
	"errors"
	"fmt"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// Bundle is one single-sig constellation trio: the ms1 secret string, the mk1
// key-card chunk strings, and the md1 wallet-policy chunk strings.
type Bundle struct {
	MS1 string
	MK1 []string
	MD1 []string
}

// Verify is the deterministic verify-bundle comparator (R0-I6): it compares a
// freshly-derived bundle against a read-back bundle on the master fingerprint,
// account xpub, origin path, the md1 exact string, the ms1 RECOVERED ENTROPY
// (not the string), and — within each bundle — the mk1↔md1 stub binding
// (mk1.policy_id_stub == md.WalletPolicyIDStub(md1); the cards belong together).
// It returns nil on full agreement, or an error naming the FIRST diverging
// field. Any entropy buffers it copies are scrubbed before return.
func Verify(derived, readback Bundle) error {
	// Stub binding FIRST, on BOTH bundles — a card set whose mk1 does not bind to
	// its md1 is malformed regardless of cross-bundle agreement (the key card
	// does not belong to this policy).
	if err := checkStubBinding("derived", derived); err != nil {
		return err
	}
	if err := checkStubBinding("readback", readback); err != nil {
		return err
	}

	// mk1: master fingerprint, account xpub, origin path.
	dCard, err := mk.Decode(derived.MK1)
	if err != nil {
		return fmt.Errorf("verify: derived mk1 decode: %w", err)
	}
	rCard, err := mk.Decode(readback.MK1)
	if err != nil {
		return fmt.Errorf("verify: readback mk1 decode: %w", err)
	}
	if dCard.Fingerprint != rCard.Fingerprint {
		return fmt.Errorf("verify: fingerprint mismatch (derived %s, readback %s)", dCard.Fingerprint, rCard.Fingerprint)
	}
	if dCard.Xpub != rCard.Xpub {
		return errors.New("verify: xpub mismatch")
	}
	if dCard.Path != rCard.Path {
		return fmt.Errorf("verify: origin path mismatch (derived %s, readback %s)", dCard.Path, rCard.Path)
	}

	// md1: deterministic exact-string match (subsumes the embedded
	// xpub/fp/origin/script — the encoder is deterministic).
	if !equalStrings(derived.MD1, readback.MD1) {
		return errors.New("verify: md1 string mismatch")
	}

	// ms1: compare RECOVERED ENTROPY bytes (so a re-typed ms1 with the same
	// entropy but any incidental string difference still matches).
	dEnt, err := ms1Entropy(derived.MS1)
	if err != nil {
		return fmt.Errorf("verify: derived ms1: %w", err)
	}
	rEnt, err := ms1Entropy(readback.MS1)
	if err != nil {
		wipe(dEnt)
		return fmt.Errorf("verify: readback ms1: %w", err)
	}
	match := bytes.Equal(dEnt, rEnt)
	wipe(dEnt)
	wipe(rEnt)
	if !match {
		return errors.New("verify: ms1 entropy mismatch")
	}
	return nil
}

// checkStubBinding asserts the bundle's mk1 carries a policy_id_stub equal to
// md.WalletPolicyIDStub(md1) — the cards belong to one policy.
func checkStubBinding(which string, b Bundle) error {
	card, err := mk.Decode(b.MK1)
	if err != nil {
		return fmt.Errorf("verify: %s mk1 decode: %w", which, err)
	}
	stub, err := md.WalletPolicyIDStubChunks(b.MD1)
	if err != nil {
		return fmt.Errorf("verify: %s md1 stub: %w", which, err)
	}
	for _, s := range card.Stubs {
		if s == stub {
			return nil
		}
	}
	return fmt.Errorf("verify: %s mk1/md1 stub mismatch (key card does not bind to this policy)", which)
}

// ms1Entropy decodes an ms1 secret string to its recovered BIP-39 entropy. The
// returned slice is SECRET; Verify scrubs it after the compare.
func ms1Entropy(s string) ([]byte, error) {
	str, err := codex32.New(s)
	if err != nil {
		return nil, err
	}
	_, _, entropy, err := codex32.DecodeMS1(str)
	if err != nil {
		return nil, err
	}
	// Copy so the caller owns a scrubbable buffer independent of the decoder.
	out := make([]byte, len(entropy))
	copy(out, entropy)
	wipe(entropy)
	return out, nil
}

func equalStrings(a, b []string) bool {
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

// wipe zeroes a byte slice (best-effort scrub of a copied entropy buffer).
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
