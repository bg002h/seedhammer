package slip39

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"sort"
)

var (
	errEmptyShares               = errors.New("slip39: no shares")
	errInvalidShareValueLength   = errors.New("slip39: invalid share value length")
	errIdentifierMismatch        = errors.New("slip39: identifier mismatch")
	errExtendableMismatch        = errors.New("slip39: extendable mismatch")
	errIterationExponentMismatch = errors.New("slip39: iteration exponent mismatch")
	errGroupThresholdMismatch    = errors.New("slip39: group threshold mismatch")
	errGroupCountMismatch        = errors.New("slip39: group count mismatch")
	errShareValueLengthMismatch  = errors.New("slip39: share value length mismatch")
	errMemberThresholdMismatch   = errors.New("slip39: member threshold mismatch")
	errDuplicateMemberIndex      = errors.New("slip39: duplicate member index")
	errInsufficientShares        = errors.New("slip39: not enough shares")
	errDigestVerificationFailed  = errors.New("slip39: bad share set")
)

func validSecretLen(n int) bool {
	switch n {
	case 16, 20, 24, 28, 32:
		return true
	}
	return false
}

// Combine reconstructs the SLIP-39 master secret from a set of shares.
// passphrase is the SLIP-39 EMS-decryption passphrase ("" = none). Returns
// the master-secret bytes (BIP-39 entropy sizes) or a classifiable error.
// Port of mnemonic_toolkit::slip39::slip39_combine. Panic-free on any input
// (all preconditions checked before interpolation — SPEC §4.4).
func Combine(shares []Share, passphrase []byte) ([]byte, error) {
	if len(shares) == 0 {
		return nil, errEmptyShares
	}
	for i := range shares {
		if !validSecretLen(len(shares[i].Value)) {
			return nil, errInvalidShareValueLength
		}
	}
	first := shares[0]
	for _, s := range shares[1:] {
		switch {
		case s.Identifier != first.Identifier:
			return nil, errIdentifierMismatch
		case s.Extendable != first.Extendable:
			return nil, errExtendableMismatch
		case s.IterationExp != first.IterationExp:
			return nil, errIterationExponentMismatch
		case s.GroupThreshold != first.GroupThreshold:
			return nil, errGroupThresholdMismatch
		case s.GroupCount != first.GroupCount:
			return nil, errGroupCountMismatch
		case len(s.Value) != len(first.Value):
			return nil, errShareValueLengthMismatch
		}
	}
	// Group by GroupIndex (sorted keys for determinism).
	byGroup := map[int][]Share{}
	for _, s := range shares {
		byGroup[s.GroupIndex] = append(byGroup[s.GroupIndex], s)
	}
	gids := make([]int, 0, len(byGroup))
	for g := range byGroup {
		gids = append(gids, g)
	}
	sort.Ints(gids)

	type gshare struct {
		x byte
		v []byte
	}
	groupShares := make([]gshare, 0, len(gids))
	for _, g := range gids {
		gs := byGroup[g]
		mt := gs[0].MemberThreshold
		seen := map[int]bool{}
		for _, s := range gs {
			if s.MemberThreshold != mt {
				return nil, errMemberThresholdMismatch
			}
			if seen[s.MemberIndex] {
				return nil, errDuplicateMemberIndex
			}
			seen[s.MemberIndex] = true
		}
		if len(gs) != mt {
			return nil, errInsufficientShares
		}
		pts := make([]bytePoint, len(gs))
		for i, s := range gs {
			pts[i] = bytePoint{byte(s.MemberIndex), s.Value}
		}
		gv, err := recoverSecret(mt, pts)
		if err != nil {
			return nil, err
		}
		groupShares = append(groupShares, gshare{byte(g), gv})
	}
	if len(groupShares) != first.GroupThreshold {
		return nil, errInsufficientShares
	}
	gpts := make([]bytePoint, len(groupShares))
	for i, gs := range groupShares {
		gpts[i] = bytePoint{gs.x, gs.v}
	}
	ems, err := recoverSecret(first.GroupThreshold, gpts)
	if err != nil {
		return nil, err
	}
	master := feistelDecrypt(ems, passphrase, first.IterationExp, first.Identifier, first.Extendable)
	for _, gs := range groupShares {
		wipe(gs.v)
	}
	wipe(ems)
	return master, nil
}

// recoverSecret recovers one Shamir layer. threshold==1 → the single value
// (no digest). Else interpolate at 255/254 and verify the HMAC-SHA256 digest.
func recoverSecret(threshold int, shares []bytePoint) ([]byte, error) {
	if threshold == 1 {
		return append([]byte(nil), shares[0].y...), nil
	}
	s := interpolateSecretAt(shares, secretIndex)
	d := interpolateSecretAt(shares, digestIndex)
	digest, random := d[:digestLen], d[digestLen:]
	mac := hmac.New(sha256.New, random)
	mac.Write(s)
	sum := mac.Sum(nil)
	if subtle.ConstantTimeCompare(digest, sum[:digestLen]) != 1 {
		wipe(s)
		return nil, errDigestVerificationFailed
	}
	wipe(d)
	return s, nil
}

// ConsistentShares reports whether a partial share set is mutually
// consistent (for eager GUI validation; count-agnostic). Two-level.
func ConsistentShares(shares []Share) error {
	if len(shares) == 0 {
		return nil
	}
	first := shares[0]
	type gm struct{ g, m int }
	seen := map[gm]bool{}
	for _, s := range shares {
		switch {
		case s.Identifier != first.Identifier:
			return errIdentifierMismatch
		case s.Extendable != first.Extendable:
			return errExtendableMismatch
		case s.IterationExp != first.IterationExp:
			return errIterationExponentMismatch
		case s.GroupThreshold != first.GroupThreshold:
			return errGroupThresholdMismatch
		case s.GroupCount != first.GroupCount:
			return errGroupCountMismatch
		case len(s.Value) != len(first.Value):
			return errShareValueLengthMismatch
		}
		k := gm{s.GroupIndex, s.MemberIndex}
		if seen[k] {
			return errDuplicateMemberIndex
		}
		seen[k] = true
	}
	return nil
}
