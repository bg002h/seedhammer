package sysw

import (
	"sort"
	"strings"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/mt"
)

// MDMKUnconfirmed returns the indices of the ClassMDMK records that are NOT
// decode-confirmed -- SPEC_systemwide_payloads §12.6, `[mdmk-decode]`.
//
// PORT, NOT PRIMARY. The normative behaviour lands in
// crates/me-cli/src/sysw/record.rs (mdmk_unconfirmed) with vector S-J, and this
// converges on it; it may never lead. What is bound is SEMANTICS -- the same
// answer for the same records -- not line-for-line code, because the two sides
// reach it through different decoder APIs. Where they differ is spelled out at
// cardKeyOf below.
//
// Grouped by (hrp, chunk_set_id), NEVER by HRP alone: a 2-of-3 wsh-sortedmulti
// wallet has three separate mk1 cards and three md1 cards, each chunked
// independently, so HRP grouping sees six chunks against a declared total of two
// and reports EVERY multisig wallet as unconfirmed.
//
// The returned indices are into the CALLER'S slice, whatever else it holds --
// R1-I2: filter the iteration, never the indices. A caller that pre-filtered to
// the ClassMDMK records and reported those positions would name the wrong record.
//
// Nothing here refuses anything. §13 D6 demoted the refusal this replaced: an
// unconfirmed record loads, and counts as SECRET for flag evaluation (§3.3.3).
func MDMKUnconfirmed(records []string) []int {
	type cardKey struct {
		hrp     byte
		chunked bool
		csid    uint32
		// uniq separates non-chunked records, which are each their OWN card.
		// One key for all of them would let the first one to decode vouch for
		// the rest, which is exactly how smuggled entropy would pass.
		uniq int
	}

	groups := make(map[cardKey][]int)
	var order []cardKey // map order is randomised; the walk must not be
	var out []int

	for i, r := range records {
		if Classify(r) != ClassMDMK {
			continue
		}
		k, ok := cardKeyOf(r)
		if !ok {
			// Fail CLOSED. A record whose card identity cannot be read cannot be
			// grouped, so nothing could ever confirm it. REACHABLE: Classify
			// trims (classify.go) and the decoders do not, so " md1..." arrives
			// here.
			out = append(out, i)
			continue
		}
		key := cardKey{hrp: k.hrp, chunked: k.chunked, csid: k.csid}
		if !k.chunked {
			key.uniq = i + 1
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	for _, key := range order {
		idxs := groups[key]
		set := make([]string, len(idxs))
		for j, i := range idxs {
			set[j] = records[i]
		}
		// The real decoders are the arbiter, per §12.6. A BCH verifier is not
		// one, and that is the whole point of the rule.
		var confirmed bool
		switch {
		case key.hrp == 'd' && key.chunked:
			_, err := md.Reassemble(set)
			confirmed = err == nil
		case key.hrp == 'd':
			_, err := md.Decode(set[0])
			confirmed = err == nil
		default: // 'k'
			_, err := mk.Decode(set)
			confirmed = err == nil
		}
		if !confirmed {
			out = append(out, idxs...)
		}
	}

	sort.Ints(out)
	return out
}

// card is one record's card identity: the HRP discriminant plus, when the record
// is chunked, its 20-bit chunk-set id.
type card struct {
	hrp     byte
	chunked bool
	csid    uint32
}

// cardKeyOf mirrors the primary's chunk_key. ok=false means "this is not a card
// at all", which is MDMKUnconfirmed's fail-closed arm; chunked=false means "a
// card that is not chunked", which is a normal, confirmable state.
//
// WHERE THE TWO IMPLEMENTATIONS TAKE DIFFERENT ROUTES TO THE SAME ANSWER. Rust
// asks ChunkHeader::read and treats ANY failure as "not chunked", so a record
// whose wire version is not 4 becomes a non-chunked card that then fails to
// decode. md.ParseChunkHeader consults the syms[0]&1 discriminator first and
// then errors on a bad version, so the same record lands in the arm above
// instead. Both end at UNCONFIRMED, which is what §12.6 binds; only the route
// differs, and vector S-J pins that the agreement is real for every card form.
func cardKeyOf(record string) (card, bool) {
	switch {
	case codex32.ValidMD(record):
		h, err := md.ParseChunkHeader(record)
		if err != nil {
			return card{}, false
		}
		return card{hrp: 'd', chunked: h.Chunked, csid: h.ChunkSetID}, true
	case codex32.ValidMK(record):
		h, err := mk.ParseHeader(record)
		if err != nil {
			return card{}, false
		}
		return card{hrp: 'k', chunked: h.Chunked, csid: h.ChunkSetID}, true
	}
	return card{}, false
}

// MTUnconfirmed returns the indices of the ClassMt records that are NOT
// decode-confirmed -- `[mt-decode]`, the mt sibling of MDMKUnconfirmed above,
// converging on me-cli's sysw::mt::mt_unconfirmed (Rust primary).
//
// Grouped by chunk_set_id (every mt1 is chunked; there is no non-chunked form
// and therefore no uniq arm). A group confirms only when it is COMPLETE, the
// bytes PARSE as one serialized Bitcoin transaction, and the set id equals
// the top 20 bits of the display txid -- mt.Decode is all three. Reassembly
// alone confirms nothing for mt: any complete set of BCH-valid strings
// reassembles, which is exactly the smuggling channel §5.3.2 names.
//
// Indices are into the CALLER'S slice; nothing here refuses anything -- an
// unconfirmed record loads and counts as SECRET for flag evaluation.
func MTUnconfirmed(records []string) []int {
	groups := make(map[uint32][]int)
	var order []uint32 // map order is randomised; the walk must not be
	var out []int

	for i, r := range records {
		if Classify(r) != ClassMt {
			continue
		}
		h, err := mt.ParseHeader(strings.TrimSpace(r))
		if err != nil {
			// Fail CLOSED: a record whose set identity cannot be read cannot
			// be grouped, so nothing could ever confirm it. (Classify trims,
			// so a padded record classifies and then lands here.)
			out = append(out, i)
			continue
		}
		if _, seen := groups[h.ChunkSetID]; !seen {
			order = append(order, h.ChunkSetID)
		}
		groups[h.ChunkSetID] = append(groups[h.ChunkSetID], i)
	}

	for _, csid := range order {
		idxs := groups[csid]
		set := make([]string, len(idxs))
		for j, i := range idxs {
			set[j] = strings.TrimSpace(records[i])
		}
		if _, err := mt.Decode(set); err != nil {
			out = append(out, idxs...)
		}
	}
	sort.Ints(out)
	return out
}
