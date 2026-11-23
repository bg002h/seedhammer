package sysw

import (
	"crypto/sha256"
	"strings"

	"seedhammer.com/codex32"
)

// mr1 records, the chunk-set rule and the kit rule: SPEC_systemwide_payloads.md
// section 14b-14d (amendment S1, 2026-11-22).
//
// PORT, NOT PRIMARY. The Rust primary is mnemonic-engrave
// crates/me-cli/src/sysw/mr.rs (build phase ME-1); this converges on it and may
// never lead. What is bound is SEMANTICS: the vendored
// testdata/sysw_mr_v1_vectors.json, written by an independent Python
// implementation, is the contract (mr_vectors_test.go).
//
// NOT CHECKED, ON PURPOSE (STOP-1, RULED (a)): whether the reassembled chunk
// bytes are a card. These are mr's FRAMING rules; decoding the card is
// mr-codec's, not the device's.

// The reasons are the spec's own identifiers; the device sentences (section
// 14g) are the gui's.
const (
	MrCase           = "MR_CASE"
	MrCharacter      = "MR_CHARACTER"
	MrLength         = "MR_LENGTH"
	MrChecksum       = "MR_CHECKSUM"
	MrReservedSymbol = "MR_RESERVED_SYMBOL"
	MrChunkShort     = "MR_CHUNK_SHORT"
	MrChunkPadding   = "MR_CHUNK_PADDING"

	SetCount     = "SET_COUNT"
	SetIndex     = "SET_INDEX"
	SetDuplicate = "SET_DUPLICATE"
	SetMissing   = "SET_MISSING"
	SetSlices    = "SET_SLICES"
	SetID        = "SET_ID"
	CardSplit    = "CARD_SPLIT"

	KitOtherRecord     = "KIT_OTHER_RECORD"
	KitUnconfirmedMDMK = "KIT_UNCONFIRMED_MDMK"
)

const (
	mrAlphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	// mr1's checksum is 13 symbols; L (the symbols after "mr1") is 14..93.
	mrChecksumSyms = 13
	mrMinL         = 14
	mrMaxL         = 93
	// The 37-bit chunk header (section 14b) and the largest slice (S1 14c
	// SET_SLICES: count = max(2, ceil(len/45))).
	mrHeaderBits  = 37
	mrChunkBytes  = 45
	mrChunkDomain = "mr1/chunks"
)

// MrDetail carries the facts a reason's sentence names (section 14g). Record
// and Records are 0-BASED positions in the list given; the device adds one.
// Only the fields a reason names are set.
type MrDetail struct {
	// Begins is {B}: the first 8 characters of the set's or card's first
	// record in payload order.
	Begins string
	// Record is the offending record (or {r}).
	Record int
	// Index is {i}, Count is {c}, FirstCount is {c0}.
	Index, Count, FirstCount int
	// Records is {rr}: the first two records that carry a repeated index.
	Records [2]int
	// Kind is {k}: "md1", "mk1" or "mr1" (CARD_SPLIT).
	Kind string
	// Class is {class}: the class of a KIT_OTHER_RECORD record.
	Class Class
}

// MrRefusal is a reason and its detail.
type MrRefusal struct {
	Reason string
	Detail MrDetail
}

// IsMrCandidate reports whether the record's trimmed form begins with mr1 in
// any case (section 14b).
func IsMrCandidate(record string) bool {
	r := strings.TrimSpace(record)
	return len(r) >= 3 && strings.EqualFold(r[:3], "mr1")
}

// MrStringReason runs section 14b's seven checks, in order, over a candidate
// and returns the first one it fails, or "" when it is ClassMr.
func MrStringReason(record string) string {
	rec := strings.TrimSpace(record)
	for i := 0; i < len(rec); i++ {
		if rec[i] >= 'A' && rec[i] <= 'Z' {
			return MrCase
		}
	}
	body := rec
	if len(body) >= 3 {
		body = body[3:]
	}
	for i := 0; i < len(body); i++ {
		if strings.IndexByte(mrAlphabet, body[i]) < 0 {
			return MrCharacter
		}
	}
	if len(body) < mrMinL || len(body) > mrMaxL {
		return MrLength
	}
	if !codex32.ValidMR(rec) {
		return MrChecksum
	}
	switch body[0] {
	case 'u', 'a', '7':
		return MrReservedSymbol
	}
	if body[0] == 'l' {
		d := len(body) - mrChecksumSyms
		if d < 8 {
			return MrChunkShort
		}
		// Whole bytes after the 37-bit header, then 0 to 4 zero bits.
		rest := 5*d - mrHeaderBits
		pad := rest % 8
		if pad > 4 {
			return MrChunkPadding
		}
		if pad > 0 && symbolBits(body, d, 5*d-pad, pad) != 0 {
			return MrChunkPadding
		}
	}
	return ""
}

// symbolValue is the 5-bit value of an mr1 data character.
func symbolValue(c byte) uint32 { return uint32(strings.IndexByte(mrAlphabet, c)) }

// symbolBits reads n (at most 32) bits starting at bit position start of the
// first d symbols of body, MSB first.
func symbolBits(body string, d, start, n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		pos := start + i
		sym := symbolValue(body[pos/5])
		v = v<<1 | (sym>>(4-uint(pos%5)))&1
	}
	return v
}

// mrChunk is a parsed ClassMr chunk (its first symbol is `l`).
type mrChunk struct {
	record int
	csid   uint32
	count  int
	index  int
	slice  []byte
}

func isMrChunk(rec string) bool { return len(rec) > 3 && rec[3] == 'l' }

// parseMrChunk reads the 37-bit header and the slice of a ClassMr chunk.
func parseMrChunk(i int, rec string) mrChunk {
	body := rec[3:]
	d := len(body) - mrChecksumSyms
	csid := symbolBits(body, d, 5, 20)
	count := int(symbolBits(body, d, 25, 6)) + 1
	index := int(symbolBits(body, d, 31, 6))
	n := (5*d - mrHeaderBits) / 8
	sl := make([]byte, n)
	for j := range sl {
		sl[j] = byte(symbolBits(body, d, mrHeaderBits+8*j, 8))
	}
	return mrChunk{record: i, csid: csid, count: count, index: index, slice: sl}
}

// mrSet is one chunk set: a RUN of chunks (section 14c).
type mrSet struct {
	csid   uint32
	chunks []mrChunk
}

// trimmed returns the trimmed records and, for each, whether it is a ClassMr
// record.
func mrRecords(records []string) (trimmed []string, isMr []bool) {
	trimmed = make([]string, len(records))
	isMr = make([]bool, len(records))
	for i, r := range records {
		trimmed[i] = strings.TrimSpace(r)
		isMr[i] = IsMrCandidate(r) && MrStringReason(r) == ""
	}
	return
}

// mrSets groups the ClassMr chunks of a payload into sets in payload order: a
// chunk joins the latest set with its chunk-set id when that set holds fewer
// chunks than its first chunk's count; otherwise it opens a new set. Two sets
// with one id are two copies of a card.
func mrSets(trimmed []string, isMr []bool) []mrSet {
	var sets []mrSet
	for i, r := range trimmed {
		if !isMr[i] || !isMrChunk(r) {
			continue
		}
		c := parseMrChunk(i, r)
		latest := -1
		for k := len(sets) - 1; k >= 0; k-- {
			if sets[k].csid == c.csid {
				latest = k
				break
			}
		}
		if latest >= 0 && len(sets[latest].chunks) < sets[latest].chunks[0].count {
			sets[latest].chunks = append(sets[latest].chunks, c)
			continue
		}
		sets = append(sets, mrSet{csid: c.csid, chunks: []mrChunk{c}})
	}
	return sets
}

func prefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func suffix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}

// setProblem is section 14c's seven checks over one set, in order.
func setProblem(set mrSet, trimmed []string) (MrRefusal, bool) {
	first := set.chunks[0]
	begins := prefix(trimmed[first.record], 8)
	c1 := first.count
	for _, c := range set.chunks {
		if c.count != c1 {
			return MrRefusal{SetCount, MrDetail{Begins: begins, Record: c.record, Count: c.count, FirstCount: c1}}, true
		}
	}
	for _, c := range set.chunks {
		if c.index >= c1 {
			return MrRefusal{SetIndex, MrDetail{Begins: begins, Record: c.record, Index: c.index, Count: c1}}, true
		}
	}
	byIndex := make([]*mrChunk, c1)
	for k := range set.chunks {
		c := &set.chunks[k]
		if prev := byIndex[c.index]; prev != nil {
			return MrRefusal{SetDuplicate, MrDetail{Begins: begins, Index: c.index, Records: [2]int{prev.record, c.record}}}, true
		}
		byIndex[c.index] = c
	}
	for k := 0; k < c1; k++ {
		if byIndex[k] == nil {
			return MrRefusal{SetMissing, MrDetail{Begins: begins, Index: k, Count: c1}}, true
		}
	}
	var content []byte
	for _, c := range byIndex {
		content = append(content, c.slice...)
	}
	total := len(content)
	wantCount := (total + mrChunkBytes - 1) / mrChunkBytes
	if wantCount < 2 {
		wantCount = 2
	}
	each := (total + wantCount - 1) / wantCount
	ok := total >= 2 && c1 == wantCount
	for k := 0; ok && k < c1; k++ {
		want := each
		if k == c1-1 {
			want = total - each*(c1-1)
		}
		if len(byIndex[k].slice) != want {
			ok = false
		}
	}
	if !ok {
		return MrRefusal{SetSlices, MrDetail{Begins: begins}}, true
	}
	h := sha256.New()
	h.Write([]byte(mrChunkDomain))
	h.Write(content)
	sum := h.Sum(nil)
	if (uint32(sum[0])<<12 | uint32(sum[1])<<4 | uint32(sum[2])>>4) != set.csid {
		return MrRefusal{SetID, MrDetail{Begins: begins}}, true
	}
	lo, hi := set.chunks[0].record, set.chunks[len(set.chunks)-1].record
	if j := firstForeign(lo, hi, func(j int) bool {
		for _, c := range set.chunks {
			if c.record == j {
				return true
			}
		}
		return false
	}); j >= 0 {
		return MrRefusal{CardSplit, MrDetail{Kind: "mr1", Begins: begins, Record: j}}, true
	}
	return MrRefusal{}, false
}

// firstForeign returns the first record index in [lo, hi] that member rejects,
// or -1.
func firstForeign(lo, hi int, member func(int) bool) int {
	for j := lo; j <= hi; j++ {
		if !member(j) {
			return j
		}
	}
	return -1
}

// MrSetProblem is section 14c over the ClassMr chunks of a payload (every
// candidate having passed 14b): the first set, in order of its first record,
// that breaks a rule is refused with that rule. Positions are 0-based in the
// list given. A single-string card has no set and passes.
func MrSetProblem(records []string) (MrRefusal, bool) {
	trimmed, isMr := mrRecords(records)
	for _, set := range mrSets(trimmed, isMr) {
		if ref, bad := setProblem(set, trimmed); bad {
			return ref, true
		}
	}
	return MrRefusal{}, false
}

// KitCard is one card of an eligible kit (section 14d).
type KitCard struct {
	// Kind is "md1", "mk1" or "mr1".
	Kind string
	// Strings are the card's record positions (0-based, in the list given), in
	// index order; a single string is index 0.
	Strings []int
	// Begins is the first 8 characters of the index-0 string; Ends is the last
	// 4 characters of the highest-index string.
	Begins, Ends string
}

// Kit is the kit verdict of a payload (section 14d).
type Kit struct {
	// Offers: the payload holds at least one mr1 candidate. A payload that
	// does not offer a kit behaves exactly as before the amendment.
	Offers bool
	// Eligible: every check passed; Cards are then in the order of their first
	// record. N is len(Cards).
	Eligible bool
	// Refusal is the first failed check when Offers and not Eligible.
	Refusal MrRefusal
	Cards   []KitCard
}

// EvaluateKit is section 14d over records in payload order (public section,
// then secret section).
func EvaluateKit(records []string) Kit {
	var k Kit
	for _, r := range records {
		if IsMrCandidate(r) {
			k.Offers = true
			break
		}
	}
	if !k.Offers {
		return k
	}
	trimmed, isMr := mrRecords(records)
	refuse := func(reason string, d MrDetail) Kit {
		k.Refusal = MrRefusal{reason, d}
		return k
	}
	// 1. the section 14b reasons.
	for i, r := range records {
		if IsMrCandidate(r) {
			if reason := MrStringReason(r); reason != "" {
				return refuse(reason, MrDetail{Record: i})
			}
		}
	}
	// 2. a record that is neither ClassMDMK nor ClassMr.
	classes := make([]Class, len(records))
	for i, r := range records {
		classes[i] = Classify(r)
		if classes[i] != ClassMDMK && classes[i] != ClassMr {
			return refuse(KitOtherRecord, MrDetail{Record: i, Class: classes[i]})
		}
	}
	// 3. the section 14c reasons.
	sets := mrSets(trimmed, isMr)
	for _, set := range sets {
		if ref, bad := setProblem(set, trimmed); bad {
			k.Refusal = ref
			return k
		}
	}
	// 4. an md1/mk1 record unconfirmed under [mdmk-decode].
	groups := mdmkGroups(records)
	for i := range records {
		if classes[i] != ClassMDMK {
			continue
		}
		for _, g := range groups {
			if !g.confirmed && containsInt(g.idxs, i) {
				return refuse(KitUnconfirmedMDMK, MrDetail{Record: i})
			}
		}
	}
	// 5. an md1 or mk1 card with a record of another card between its first
	// and last record. Cards are listed in the order of their first record.
	type span struct {
		first int
		card  KitCard
		idxs  []int
	}
	var spans []span
	for _, g := range groups {
		spans = append(spans, span{first: g.idxs[0], card: mdmkCard(g, trimmed), idxs: g.idxs})
	}
	for _, set := range sets {
		var idxs []int
		for _, c := range set.chunks {
			idxs = append(idxs, c.record)
		}
		spans = append(spans, span{first: set.chunks[0].record, card: mrSetCard(set, trimmed), idxs: idxs})
	}
	for i, r := range trimmed {
		if isMr[i] && !isMrChunk(r) {
			spans = append(spans, span{first: i, card: KitCard{Kind: "mr1", Strings: []int{i},
				Begins: prefix(r, 8), Ends: suffix(r, 4)}, idxs: []int{i}})
		}
	}
	sortSpans := func() {
		for a := 1; a < len(spans); a++ {
			for b := a; b > 0 && spans[b].first < spans[b-1].first; b-- {
				spans[b], spans[b-1] = spans[b-1], spans[b]
			}
		}
	}
	sortSpans()
	for _, s := range spans {
		if s.card.Kind == "mr1" {
			continue // an mr1 split is a section 14c refusal, already reported
		}
		lo, hi := s.idxs[0], s.idxs[0]
		for _, i := range s.idxs {
			if i < lo {
				lo = i
			}
			if i > hi {
				hi = i
			}
		}
		if j := firstForeign(lo, hi, func(j int) bool { return containsInt(s.idxs, j) }); j >= 0 {
			return refuse(CardSplit, MrDetail{Kind: s.card.Kind, Begins: prefix(trimmed[s.first], 8), Record: j})
		}
	}
	k.Eligible = true
	for _, s := range spans {
		k.Cards = append(k.Cards, s.card)
	}
	return k
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// mrSetCard is the card of a chunk set: strings in index order.
func mrSetCard(set mrSet, trimmed []string) KitCard {
	byIndex := make([]int, set.chunks[0].count)
	for _, c := range set.chunks {
		byIndex[c.index] = c.record
	}
	return KitCard{Kind: "mr1", Strings: byIndex,
		Begins: prefix(trimmed[byIndex[0]], 8), Ends: suffix(trimmed[byIndex[len(byIndex)-1]], 4)}
}

// mdmkCard is the card of one confirmed [mdmk-decode] group: strings in chunk
// index order.
func mdmkCard(g mdmkGroup, trimmed []string) KitCard {
	type ent struct{ idx, pos int }
	ents := make([]ent, len(g.idxs))
	for n, i := range g.idxs {
		ents[n] = ent{idx: mdmkChunkIndex(trimmed[i]), pos: i}
	}
	for a := 1; a < len(ents); a++ {
		for b := a; b > 0 && ents[b].idx < ents[b-1].idx; b-- {
			ents[b], ents[b-1] = ents[b-1], ents[b]
		}
	}
	card := KitCard{Kind: g.kind}
	for _, e := range ents {
		card.Strings = append(card.Strings, e.pos)
	}
	card.Begins = prefix(trimmed[card.Strings[0]], 8)
	card.Ends = suffix(trimmed[card.Strings[len(card.Strings)-1]], 4)
	return card
}
