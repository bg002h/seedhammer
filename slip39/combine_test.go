package slip39

import (
	"encoding/hex"
	"errors"
	"testing"
)

func hexEq(b []byte) string { return hex.EncodeToString(b) }

func TestCombineBasic2of3(t *testing.T) {
	shares := vectorShares(t, 3) // all mnemonics of official vector idx 3
	parsed := make([]Share, len(shares))
	for i, m := range shares {
		s, err := ParseShare(m)
		if err != nil {
			t.Fatalf("share %d: %v", i, err)
		}
		parsed[i] = s
	}
	got, err := Combine(parsed[:2], []byte("TREZOR")) // any 2 of 3
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if hexEq(got) != "b43ceb7e57a0ea8766221624d01b0864" {
		t.Errorf("recovered %x want b43c…0864", got)
	}
}

// copyShare / copyShares deep-copy a share (including its mutable Value backing
// array) so a test can perturb a value byte without disturbing the source.
func copyShare(s Share) Share { s.Value = append([]byte(nil), s.Value...); return s }
func copyShares(ss []Share) []Share {
	out := make([]Share, len(ss))
	for i, s := range ss {
		out[i] = copyShare(s)
	}
	return out
}

// TestCombineErrorPathSentinels is the M2 regression+convention guard. It does
// NOT assert the leaked group-share buffers are zeroed (they are function-local
// to Combine and unobservable seam-free — spec R0 Q1/Minor-2). Instead it proves
// the additive scrub `defer` did NOT change control flow or error classification:
// each of the three error returns that previously skipped the scrub
// (combine.go:103 / :108 / :116) still returns its correct sentinel. The
// success-path equivalence is covered by the existing official-vector tests; the
// helper-zeroes-its-buffer invariant by TestWipeZeroes.
//
// Vector idx 17 has GroupThreshold=2, GroupCount=4, with group 1 = one
// MemberThreshold=1 share (parsed[1], carries its share directly, no member
// digest) and group 3 = two MemberThreshold=2 shares (parsed[0], parsed[2]).
func TestCombineErrorPathSentinels(t *testing.T) {
	parsed := parseAll(t, vectorShares(t, 17))

	// Sanity: the clean set recovers (so the perturbations below are the only
	// reason any path errors).
	if _, err := Combine(copyShares(parsed), []byte("TREZOR")); err != nil {
		t.Fatalf("clean idx17 Combine: %v", err)
	}

	// Path (a) combine.go:103 — a member of the 2-member group (group 3) is
	// perturbed so its member-layer digest fails AFTER group 1 has already
	// recovered and appended its gv (groups iterate sorted: 1 then 3).
	pa := copyShares(parsed)
	pa[0].Value[0] ^= 0xff
	if _, err := Combine(pa, []byte("TREZOR")); !errors.Is(err, errDigestVerificationFailed) {
		t.Fatalf("path(a): err = %v, want errDigestVerificationFailed", err)
	}

	// Path (b) combine.go:108 — supply only group 1's single share, so the
	// recovered-group count (1) != GroupThreshold (2).
	pb := []Share{copyShare(parsed[1])}
	if _, err := Combine(pb, []byte("TREZOR")); !errors.Is(err, errInsufficientShares) {
		t.Fatalf("path(b): err = %v, want errInsufficientShares", err)
	}

	// Path (c) combine.go:116 — perturb group 1's threshold-1 share so it
	// "recovers" a corrupt gv (no member digest to catch it), group 3 recovers
	// cleanly, then the GROUP-layer recoverSecret digest fails.
	pc := copyShares(parsed)
	pc[1].Value[0] ^= 0xff
	if _, err := Combine(pc, []byte("TREZOR")); !errors.Is(err, errDigestVerificationFailed) {
		t.Fatalf("path(c): err = %v, want errDigestVerificationFailed", err)
	}
}
