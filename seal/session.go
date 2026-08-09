package seal

// §10.2.2's session lifecycle and §10.2.4's residency timer both need one
// question answered the same way: which records are SECRET, and is any of them
// still in memory. Answering it here rather than in gui is the same rule F-77
// enforces for card grouping — the UI must not re-derive what the classifier
// already decided.

// IsSecret reports whether a classification is seed material.
//
// md1 and mk1 are NOT secret wherever they travelled: §6.3's table is explicit
// that an xpub and a wallet policy leak privacy but do not spend coins, and
// §11.2 requires vector F's THREE ms1 records to be the ones offered first —
// its twelve mk1/md1 records are ordinary plates. Encrypting them is defence in
// depth, not protection of key material.
func IsSecret(c Classification) bool {
	return c == ClassCodex32Secret || c == ClassMnemonic
}

// SecretsResident reports whether any SEAL-OWNED secret record still holds
// non-zero bytes.
//
// READ THAT NARROWLY, and do not build a control on the wide reading (lens 3 M2
// and lens 8 §5 item 2; owned by F-89 and F-90 item 2). An earlier version of
// this comment said the predicate goes false "because the secret is ACTUALLY
// GONE, not because a button was pressed". Two things are wrong with that:
//
//   - It does not go false ON CANCEL. unlockEngraveCodex32 and
//     unlockEngraveMnemonic clear the record when the PLATE IS BUILT, before
//     Engrave is ever called — which is the right design and is where B2a-ii's
//     C1 fix moved it. So the predicate flips the instant the plate reaches the
//     screen, by every route, not on cancellation.
//   - At that instant the seed is NOT gone. It is live as plate.Spline for the
//     whole ~21-minute cut (F-83, accepted), plus codex32.String and
//     backup.SeedString on the ms1 arm and F-88's four rows on the mnemonic
//     arm. This function scans p.Secret only and reaches none of them.
//
// So what it measures is exactly "no seal-owned record buffer is non-zero",
// which is NOT "no seed material is resident". §10.2.4's third row currently
// reads the two as equivalent. Nothing consumes the predicate in B2a, so this
// is not a defect today — but B2b's timer is specified to key on it, and on the
// ms1 arm (six of the seven canonical vectors) it would report "nothing to
// protect" while four string copies of the share are still live. Fix the
// contract before building the timer on it.
//
// B2a has no timer (that is B2b), but the predicate ships here because it is
// the definition the RECORD wipe must satisfy, and a test can assert on it.
func (p *Payload) SecretsResident() bool {
	for _, r := range p.Secret {
		if !IsSecret(r.Class) {
			continue
		}
		for _, b := range r.Record {
			if b != 0 {
				return true
			}
		}
	}
	return false
}

// WipeSecretAt zeroes one record's bytes. §10.2.2 wipes per RECORD as each
// plate leaves the screen, not per session, so Payload.Wipe is too coarse for
// the offer loop — it is the right thing only on the way out.
//
// Out-of-range is a no-op rather than a panic: on a device a panic is a brick.
func (p *Payload) WipeSecretAt(i int) {
	if i < 0 || i >= len(p.Secret) {
		return
	}
	clear(p.Secret[i].Record)
}
