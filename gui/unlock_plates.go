package gui

import "seedhammer.com/seal"

// The post-session plate list's model.
//
// After §10.2.2's secret session, RAM holds the public records AND whatever
// md1/mk1 travelled in the encrypted section -- five of vector C's six records,
// twelve of vector F's fifteen. §6.3 is explicit that md1/mk1 are not secret
// wherever they travelled, so they are ordinary plates and leaving them out of
// the list would be §6.4's incomplete-backup-believed-complete with the
// operator's own payload.

// unlockPlate is one entry: a record safe to leave resident, plus what the
// operator needs to see about it.
type unlockPlate struct {
	rec seal.AdmittedRecord
	// idx is the record's position within its OWN section, which is what
	// plateLabel's fallback branch numbers when a record somehow carries no
	// card labels.
	idx int
	// sealed marks a record that came from the encrypted section, and is set
	// only when BOTH sections carry cards -- see unlockPlateLabel.
	sealed bool
	// cut records that this plate has already been engraved THIS SESSION. A
	// convenience, not a guarantee: it does not survive a power cut, and
	// §10.2.2 requires the UI not imply that it does.
	cut bool
}

// unlockPlateLabel is plateLabel plus the two suffixes B2a adds.
//
// It WRAPS plateLabel rather than replacing it: the card/plate arithmetic and
// the md1/mk1 naming are §6.3's grouping rendered, and having two functions
// that both decide what a card is called is the divergence F-77 exists to
// prevent.
func unlockPlateLabel(r seal.AdmittedRecord, idx int, sealed, cut bool) string {
	s := plateLabel(r, idx)
	if sealed {
		// Card indices are computed PER SECTION, so a public mk1 1/2 and an
		// encrypted mk1 1/2 can both exist on one payload. Rendered without
		// this the list shows the same label twice and the operator cannot tell
		// which plate they are about to cut. Added only when both sections
		// actually carry cards, so the ordinary payload gains no noise.
		s += " (sealed)"
	}
	if cut {
		// Deliberately a WORD and not a glyph: F-78 measured that "·"
		// contributes zero pixels in this font, and a mark the operator cannot
		// see is worse than no mark at all.
		s += " (cut)"
	}
	return s
}

// unlockPlates builds the list. Secrets are absent by construction -- they were
// offered and wiped by §10.2.2's session before this is called, and
// seal.IsSecret is the single definition of which those are.
func unlockPlates(p *seal.Payload) []unlockPlate {
	var pub, enc bool
	for _, r := range p.Public {
		if r.Class == seal.ClassMDMK {
			pub = true
		}
	}
	for _, r := range p.Secret {
		if r.Class == seal.ClassMDMK {
			enc = true
		}
	}
	mixed := pub && enc
	out := make([]unlockPlate, 0, len(p.Public)+len(p.Secret))
	for i, r := range p.Public {
		out = append(out, unlockPlate{rec: r, idx: i})
	}
	for i, r := range p.Secret {
		if r.Class != seal.ClassMDMK {
			continue
		}
		out = append(out, unlockPlate{rec: r, idx: i, sealed: mixed})
	}
	return out
}
