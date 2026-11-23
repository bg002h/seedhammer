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
// Grouped by RUNS of (hrp, chunk_set_id) -- see mdmkGroups -- NEVER by HRP alone: a 2-of-3 wsh-sortedmulti
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
	var out []int
	for _, g := range mdmkGroups(records) {
		if !g.confirmed {
			out = append(out, g.idxs...)
		}
	}
	sort.Ints(out)
	return out
}

// mdmkGroup is one `[mdmk-decode]` group: the ClassMDMK records of one card,
// as indices into the caller's slice, in payload order.
type mdmkGroup struct {
	// kind is "md1" or "mk1".
	kind      string
	idxs      []int
	confirmed bool
}

// mdmkGroups groups the ClassMDMK records of a payload BY RUNS, in every
// payload, on host and device alike (SPEC_systemwide_payloads.md section 14d):
// a record joins the latest group of its (hrp, chunk-set id) while that group
// holds fewer strings than its first string's count; otherwise it opens a new
// group. So two complete copies of one mk1 card are two groups, and each
// confirms. A non-chunked record is its own group. A record whose card
// identity cannot be read is its own unconfirmed group (fail CLOSED).
//
// Groups are returned in the order of their first record. The real decoders are
// the arbiter of `confirmed`, per section 12.6.
func mdmkGroups(records []string) []mdmkGroup {
	type runKey struct {
		hrp  byte
		csid uint32
	}
	type run struct {
		g     int
		count int
	}
	var groups []mdmkGroup
	latest := make(map[runKey]run) // the latest chunked group of each key

	for i, r := range records {
		if Classify(r) != ClassMDMK {
			continue
		}
		kind := "md1"
		if mdmkHRP(r) == 'k' {
			kind = "mk1"
		}
		k, count, ok := cardKeyOf(r)
		if !ok {
			groups = append(groups, mdmkGroup{kind: kind, idxs: []int{i}})
			continue
		}
		if !k.chunked {
			groups = append(groups, mdmkGroup{kind: kind, idxs: []int{i}})
			continue
		}
		key := runKey{hrp: k.hrp, csid: k.csid}
		if l, seen := latest[key]; seen && len(groups[l.g].idxs) < l.count {
			groups[l.g].idxs = append(groups[l.g].idxs, i)
			continue
		}
		groups = append(groups, mdmkGroup{kind: kind, idxs: []int{i}})
		latest[key] = run{g: len(groups) - 1, count: count}
	}

	for gi := range groups {
		g := &groups[gi]
		if g.confirmed || len(g.idxs) == 0 {
			continue
		}
		first := records[g.idxs[0]]
		if _, _, ok := cardKeyOf(first); !ok {
			continue
		}
		set := make([]string, len(g.idxs))
		for j, i := range g.idxs {
			set[j] = records[i]
		}
		k, _, _ := cardKeyOf(first)
		switch {
		case k.hrp == 'd' && k.chunked:
			_, err := md.Reassemble(set)
			g.confirmed = err == nil
		case k.hrp == 'd':
			_, err := md.Decode(set[0])
			g.confirmed = err == nil
		default: // 'k'
			_, err := mk.Decode(set)
			g.confirmed = err == nil
		}
	}
	return groups
}

// mdmkHRP is the discriminant of a ClassMDMK record: 'd' for md1, 'k' for mk1.
func mdmkHRP(record string) byte {
	if codex32.ValidMD(record) {
		return 'd'
	}
	return 'k'
}

// mdmkChunkIndex is the chunk index of a confirmed md1/mk1 string (0 for a
// non-chunked one).
func mdmkChunkIndex(record string) int {
	switch {
	case codex32.ValidMD(record):
		if h, err := md.ParseChunkHeader(record); err == nil && h.Chunked {
			return h.ChunkIndex
		}
	case codex32.ValidMK(record):
		if h, err := mk.ParseHeader(record); err == nil && h.Chunked {
			return h.ChunkIndex
		}
	}
	return 0
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
func cardKeyOf(record string) (card, int, bool) {
	switch {
	case codex32.ValidMD(record):
		h, err := md.ParseChunkHeader(record)
		if err != nil {
			return card{}, 0, false
		}
		return card{hrp: 'd', chunked: h.Chunked, csid: h.ChunkSetID}, h.TotalChunks, true
	case codex32.ValidMK(record):
		h, err := mk.ParseHeader(record)
		if err != nil {
			return card{}, 0, false
		}
		return card{hrp: 'k', chunked: h.Chunked, csid: h.ChunkSetID}, h.TotalChunks, true
	}
	return card{}, 0, false
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
