package seal

// F-77 — §6.3's card grouping, published for the ENCRYPTED section.
//
// AdmittedRecord's label fields were populated for SectionPublic only, because
// pass 3 (decodePublicSet) is the sole place a grouping is computed and it runs
// only there (record.go:214). But §6.3 admits md1/mk1 into the encrypted
// section explicitly, and the vectors carry them: vector C's secret set is
// ms1 x1 / mk1 x2 / md1 x3, vector F's is ms1 x3 / mk1 x6 / md1 x6. Without
// this, §10.2.2's secret-session plate labels are unimplementable for every
// multisig payload.
//
// LABEL-ONLY, and that is normative here rather than a shortcut:
//
//   - It runs over the ClassMDMK SUBSET. cardKey fails closed for anything that
//     is not an md1/mk1 card, and the encrypted section legitimately carries ms1
//     and bare mnemonics, so grouping the whole section would reject every real
//     payload.
//   - A grouping failure is DISCARDED, not returned. §10.2.1 requires the decode
//     for the public section only; turning a label failure into a rejection
//     would change ADMISSION, and admission changes land in Rust first with test
//     vectors (the Rust-primary rule). Publishing a partition that is already
//     computed changes no behaviour at all.
//
// A record whose card cannot be read therefore keeps its zero label fields, and
// gui's plateLabel already renders that as "record N" rather than mislabelling
// it as an md1 (gui/unlock_platelist.go:50-55).
func labelEncryptedCards(out []AdmittedRecord) {
	// Stringifying an md1/mk1 copies PUBLIC data by §6.3 — an xpub or a wallet
	// policy, not key material — which is why the same conversion is already
	// done unremarked for the public section (record.go:217-220). ms1 and
	// mnemonic records are never converted here.
	at := make([]int, 0, len(out))
	strs := make([]string, 0, len(out))
	for i, r := range out {
		if r.Class != ClassMDMK {
			continue
		}
		at = append(at, i)
		strs = append(strs, string(r.Record))
	}
	if len(strs) == 0 {
		return
	}
	g, err := groupRecords(strs)
	if err != nil {
		return
	}
	// labelCards indexes by position within the slice it is handed, so the
	// subset is labelled in its own coordinates and scattered back. Reusing it
	// rather than reimplementing the card/plate arithmetic is the point: two
	// implementations of "which card is this" is exactly what F-77 exists to
	// avoid.
	sub := make([]AdmittedRecord, len(strs))
	labelCards(sub, g)
	for j, i := range at {
		out[i].HRP = sub[j].HRP
		out[i].CardIndex = sub[j].CardIndex
		out[i].CardTotal = sub[j].CardTotal
		out[i].PlateIndex = sub[j].PlateIndex
		out[i].PlateTotal = sub[j].PlateTotal
	}
}
