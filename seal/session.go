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

// SecretsResident reports whether any secret record still holds non-zero bytes.
//
// This is §10.2.4's timer condition, and it is keyed on RESIDENCY rather than
// on which button was last pressed — which is what makes an aborted engrave
// safe: cancel a secret plate mid-cut, §10.2.2 wipes the record, and this goes
// false because the secret is ACTUALLY GONE, not because a button was pressed.
//
// B2a has no timer (that is B2b), but the predicate ships here because it is
// the definition the wipe must satisfy, and a test can assert on it.
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
