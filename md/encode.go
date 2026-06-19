package md

import (
	"errors"
	"math/bits"
)

// ─── Encoder (port of md-codec encode.rs / tree.rs / tlv.rs / varint.rs /
// origin_path.rs / use_site_path.rs / header.rs / tag.rs writers). The faithful
// inverse of the shipped decoder. ────────────────────────────────────────────

// Encode-side range guards (net-new vs the decoder; the three pre-emission
// validators — validatePlaceholderUsage / validateMultipathConsistency /
// validateTapScriptTree — and readUnknownPayload are reused from md.go, R0-M5/M6).
var (
	errThresholdRange = errors.New("md: threshold k out of range 1..32")
	errChildCount     = errors.New("md: child count out of range 1..32")
	// errKGreaterThanN is reused from md.go.
	errEmptyTLVEncode = errors.New("md: empty TLV entry (encode)")
	errVarintOverflow = errors.New("md: varint value exceeds single-extension range")
	errPathDepth      = errors.New("md: origin path depth exceeds 15")
	errAltCount       = errors.New("md: alt-count out of range 2..9")
	errKeyCountRange  = errors.New("md: key count n out of range 1..32")
	errDivergentCount = errors.New("md: divergent path count != n")
	// errOverrideOrder is reused from md.go.
)

// kiw = ⌈log₂(n)⌉ = 32 - leadingZeros(n-1), clamped to 0 at n ∈ {0,1}
// (encode.rs:37-41; mirrors the decoder's md.go:842 computation, using
// saturating subtraction so n=0 yields 0 rather than wrapping).
func kiw(n uint8) uint8 {
	if n <= 1 {
		return 0
	}
	return uint8(32 - bits.LeadingZeros32(uint32(n)-1))
}

// ─── tag (inverse of readTag md.go:81-101; tag.rs:140-146). ──────────────────

// writeTag writes the 6-bit primary tag. v0.30 allocates no extension subcodes,
// so every tag is a single 6-bit primary write (the 0x3F extension prefix is
// never emitted by the encoder).
func writeTag(w *bitWriter, t tag) {
	w.write(uint64(t), 6)
}

// ─── varint (inverse of readVarint md.go:141-166; varint.rs:15-42). ──────────

func writeVarint(w *bitWriter, value uint32) error {
	var bitsNeeded int
	if value == 0 {
		bitsNeeded = 0
	} else {
		bitsNeeded = 32 - bits.LeadingZeros32(value)
	}
	if bitsNeeded <= 14 {
		w.write(uint64(bitsNeeded), 4)
		w.write(uint64(value), bitsNeeded)
		return nil
	}
	lHigh := bitsNeeded - 14
	if lHigh > 15 {
		return errVarintOverflow
	}
	w.write(15, 4)
	w.write(uint64(lHigh), 4)
	const lowMask = (uint64(1) << 14) - 1
	w.write(uint64(value)&lowMask, 14)
	w.write(uint64(value)>>14, lHigh)
	return nil
}

// ─── header (inverse of readHeader md.go:308-319; header.rs:30-33). ──────────

func writeHeader(w *bitWriter, h header) {
	v := (uint64(b2u(h.divergentPaths)) << 4) | uint64(h.version&0b1111)
	w.write(v, 5)
}

// ─── origin path (inverse of readOriginPath/readPathDecl; origin_path.rs). ───

func writePathComponent(w *bitWriter, c pathComponent) error {
	w.write(uint64(b2u(c.hardened)), 1)
	return writeVarint(w, c.value)
}

func writeOriginPath(w *bitWriter, p originPath) error {
	if len(p.components) > maxPathComponents {
		return errPathDepth
	}
	w.write(uint64(len(p.components)), 4)
	for _, c := range p.components {
		if err := writePathComponent(w, c); err != nil {
			return err
		}
	}
	return nil
}

func writePathDecl(w *bitWriter, p pathDecl) error {
	if p.n < 1 || p.n > 32 {
		return errKeyCountRange
	}
	w.write(uint64(p.n-1), 5)
	if p.divergent != nil {
		if len(p.divergent) != int(p.n) {
			return errDivergentCount
		}
		for _, op := range p.divergent {
			if err := writeOriginPath(w, op); err != nil {
				return err
			}
		}
		return nil
	}
	// Shared mode. A nil shared pointer is treated as an empty path.
	var sp originPath
	if p.shared != nil {
		sp = *p.shared
	}
	return writeOriginPath(w, sp)
}

// ─── use-site path (inverse of readUseSitePath; use_site_path.rs:80-96). ─────

func writeAlternative(w *bitWriter, a alternative) error {
	w.write(uint64(b2u(a.hardened)), 1)
	return writeVarint(w, a.value)
}

func writeUseSitePath(w *bitWriter, us useSitePath) error {
	if us.hasMultipath {
		if len(us.multipath) < minAltCount || len(us.multipath) > maxAltCount {
			return errAltCount
		}
		w.write(1, 1)
		w.write(uint64(len(us.multipath)-minAltCount), 3)
		for _, a := range us.multipath {
			if err := writeAlternative(w, a); err != nil {
				return err
			}
		}
	} else {
		w.write(0, 1)
	}
	w.write(uint64(b2u(us.wildcardHardened)), 1)
	return nil
}

const maxAltCount = 9

// ─── tree (inverse of readNodeDepth md.go:329-489; tree.rs:79-176). ──────────

// writeNode encodes a node: the 6-bit TAG is written FIRST on EVERY arm
// (R0-I1), then the body. keyIndexWidth is the per-@N index field width (kiw).
func writeNode(w *bitWriter, n node, keyIndexWidth uint8) error {
	writeTag(w, n.tag)
	switch b := n.body.(type) {
	case keyArgBody:
		w.write(uint64(b.index), int(keyIndexWidth))
	case childrenBody:
		for _, c := range b.children {
			if err := writeNode(w, c, keyIndexWidth); err != nil {
				return err
			}
		}
	case variableBody:
		// Thresh: (k-1) 5b, (len-1) 5b, children. Enforce k,n∈1..32 & k≤n.
		if b.k < 1 || b.k > 32 {
			return errThresholdRange
		}
		if len(b.children) < 1 || len(b.children) > 32 {
			return errChildCount
		}
		if int(b.k) > len(b.children) {
			return errKGreaterThanN
		}
		w.write(uint64(b.k-1), 5)
		w.write(uint64(len(b.children)-1), 5)
		for _, c := range b.children {
			if err := writeNode(w, c, keyIndexWidth); err != nil {
				return err
			}
		}
	case multiKeysBody:
		// Multi-family: (k-1) 5b, (n-1) 5b, then RAW kiw-width indices.
		if b.k < 1 || b.k > 32 {
			return errThresholdRange
		}
		if len(b.indices) < 1 || len(b.indices) > 32 {
			return errChildCount
		}
		if int(b.k) > len(b.indices) {
			return errKGreaterThanN
		}
		w.write(uint64(b.k-1), 5)
		w.write(uint64(len(b.indices)-1), 5)
		for _, idx := range b.indices {
			w.write(uint64(idx), int(keyIndexWidth))
		}
	case trBody:
		// is_nums 1b; if !is_nums key_index@kiw; has_tree 1b; optional subtree.
		w.write(uint64(b2u(b.isNums)), 1)
		if !b.isNums {
			w.write(uint64(b.keyIndex), int(keyIndexWidth))
		}
		w.write(uint64(b2u(b.tree != nil)), 1)
		if b.tree != nil {
			if err := writeNode(w, *b.tree, keyIndexWidth); err != nil {
				return err
			}
		}
	case timelockBody:
		w.write(uint64(uint32(b)), 32)
	case hash256Body:
		for _, by := range b {
			w.write(uint64(by), 8)
		}
	case hash160Body:
		for _, by := range b {
			w.write(uint64(by), 8)
		}
	case emptyBody:
		// nothing
	default:
		return errOperatorContext
	}
	return nil
}

// ─── TLV (inverse of readTLV md.go:536-557; tlv.rs:86-208). ──────────────────

// tlvEntry is a (tag, sub-bitstream-bytes, bitLen) tuple collected before the
// sorted emit, mirroring Rust's entries Vec.
type tlvEntry struct {
	tag     uint8
	payload []byte
	bitLen  int
}

// writeTLVSection encodes the TLV section: each present sparse TLV is built as a
// sub-bitstream (idx@kiw + value), then all entries (including buffered unknowns)
// are sorted by tag ascending and emitted as [tag:5][varint(bitLen)][reEmit].
func writeTLVSection(w *bitWriter, s tlvSection, keyIndexWidth uint8) error {
	var entries []tlvEntry

	if s.useSitePresent {
		if len(s.useSiteOverrides) == 0 {
			return errEmptyTLVEncode
		}
		var sub bitWriter
		var last uint8
		haveLast := false
		for _, e := range s.useSiteOverrides {
			if haveLast && e.idx <= last {
				return errOverrideOrder
			}
			haveLast = true
			last = e.idx
			sub.write(uint64(e.idx), int(keyIndexWidth))
			if err := writeUseSitePath(&sub, e.path); err != nil {
				return err
			}
		}
		entries = append(entries, tlvEntry{tlvUseSitePathOverrides, sub.intoBytes(), sub.bitLen()})
	}

	if s.fpPresent {
		if len(s.fingerprints) == 0 {
			return errEmptyTLVEncode
		}
		var sub bitWriter
		var last uint8
		haveLast := false
		for _, e := range s.fingerprints {
			if haveLast && e.idx <= last {
				return errOverrideOrder
			}
			haveLast = true
			last = e.idx
			sub.write(uint64(e.idx), int(keyIndexWidth))
			for _, by := range e.fp {
				sub.write(uint64(by), 8)
			}
		}
		entries = append(entries, tlvEntry{tlvFingerprints, sub.intoBytes(), sub.bitLen()})
	}

	if s.pubPresent {
		if len(s.pubkeys) == 0 {
			return errEmptyTLVEncode
		}
		var sub bitWriter
		var last uint8
		haveLast := false
		for _, e := range s.pubkeys {
			if haveLast && e.idx <= last {
				return errOverrideOrder
			}
			haveLast = true
			last = e.idx
			sub.write(uint64(e.idx), int(keyIndexWidth))
			for _, by := range e.xpub {
				sub.write(uint64(by), 8)
			}
		}
		entries = append(entries, tlvEntry{tlvPubkeys, sub.intoBytes(), sub.bitLen()})
	}

	if s.originPresent {
		if len(s.originOverrides) == 0 {
			return errEmptyTLVEncode
		}
		var sub bitWriter
		var last uint8
		haveLast := false
		for _, e := range s.originOverrides {
			if haveLast && e.idx <= last {
				return errOverrideOrder
			}
			haveLast = true
			last = e.idx
			sub.write(uint64(e.idx), int(keyIndexWidth))
			if err := writeOriginPath(&sub, e.path); err != nil {
				return err
			}
		}
		entries = append(entries, tlvEntry{tlvOriginPathOverrides, sub.intoBytes(), sub.bitLen()})
	}

	for _, u := range s.unknown {
		entries = append(entries, tlvEntry{u.tag, u.payload, u.bitLen})
	}

	// Sort by tag ascending (stable; the four sparse tags are distinct, but a
	// buffered unknown could collide with a sparse one in pathological input —
	// stable sort keeps the Rust ordering, sparse-then-unknown by build order).
	sortTLVEntriesByTag(entries)

	for _, e := range entries {
		w.write(uint64(e.tag), 5)
		if err := writeVarint(w, uint32(e.bitLen)); err != nil {
			return err
		}
		if err := reEmitBits(w, e.payload, e.bitLen); err != nil {
			return err
		}
	}
	return nil
}

// sortTLVEntriesByTag stable-sorts entries by tag ascending (a small insertion
// sort; the entry count is ≤4 sparse + a handful of unknowns, and avoiding
// sort.Slice keeps the production path reflect-free / TinyGo-light).
func sortTLVEntriesByTag(entries []tlvEntry) {
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && entries[j-1].tag > entries[j].tag {
			entries[j-1], entries[j] = entries[j], entries[j-1]
			j--
		}
	}
}

// ─── encodePayload (inverse of decodePayload md.go:826-863; encode.rs:65-92). ─

// encodePayload canonicalizes a clone of d, validates it, then emits the
// canonical payload bit stream: Header(5b) → PathDecl → UseSitePath → writeNode
// → TLV. Returns (bytes, exact-bit-count, error). The bytes are low-bit
// zero-padded in the final byte; the bit count is the exact unpadded length.
func encodePayload(d *descriptor) ([]byte, int, error) {
	dc, err := canonicalize(d)
	if err != nil {
		return nil, 0, err
	}
	// Reuse the decoder-side validators (R0-M5/M6).
	if err := validatePlaceholderUsage(dc.tree, dc.n); err != nil {
		return nil, 0, err
	}
	if dc.tlv.useSitePresent {
		if err := validateMultipathConsistency(dc.useSite, dc.tlv.useSiteOverrides); err != nil {
			return nil, 0, err
		}
	}
	if dc.tree.tag == tagTr {
		if b, ok := dc.tree.body.(trBody); ok && b.tree != nil {
			if err := validateTapScriptTree(*b.tree); err != nil {
				return nil, 0, err
			}
		}
	}

	var w bitWriter
	writeHeader(&w, header{
		version:        wfRedesignVersion,
		divergentPaths: dc.pathDecl.divergent != nil,
	})
	if err := writePathDecl(&w, dc.pathDecl); err != nil {
		return nil, 0, err
	}
	if err := writeUseSitePath(&w, dc.useSite); err != nil {
		return nil, 0, err
	}
	width := kiw(dc.pathDecl.n)
	if err := writeNode(&w, dc.tree, width); err != nil {
		return nil, 0, err
	}
	if err := writeTLVSection(&w, dc.tlv, width); err != nil {
		return nil, 0, err
	}
	return w.intoBytes(), w.bitLen(), nil
}

// b2u converts a bool to a uint8 (1/0) for bit writing.
func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
