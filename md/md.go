package md

import (
	"errors"
	"math/bits"

	"seedhammer.com/codex32"
)

// ─── Error sentinels (port of md-codec error.rs variants used by the
// single-string decode path). Only ErrChunkedUnsupported is exported (the GUI
// matches it); all others are internal and surfaced to the GUI as a generic
// "can't decode" outcome. ───────────────────────────────────────────────────

// ErrChunkedUnsupported is returned by Decode when the string carries the
// chunked-payload flag (bit 0 of the first 5-bit data symbol set). Reassembly
// of multi-part md1 descriptors is out of scope for T2c (ledger #10).
var ErrChunkedUnsupported = errors.New("md: chunked md1 not supported")

var (
	errWireVersion      = errors.New("md: wire version mismatch")
	errTagOutOfRange    = errors.New("md: tag out of range")
	errKGreaterThanN    = errors.New("md: threshold k greater than n")
	errDepthExceeded    = errors.New("md: decode recursion depth exceeded")
	errOperatorContext  = errors.New("md: operator context violation")
	errTLVOrdering      = errors.New("md: TLV ordering violation")
	errTLVLength        = errors.New("md: TLV length exceeds remaining")
	errEmptyTLV         = errors.New("md: empty TLV entry")
	errPlaceholderRange = errors.New("md: placeholder index out of range")
	errOverrideOrder    = errors.New("md: override order violation")
)

// ─── Tag (port of tag.rs). 6-bit primary codes 0x00..=0x23; 0x24..=0x3E
// reserved; 0x3F extension prefix (consumes a 4-bit subcode and rejects). ─────

type tag uint8

const (
	tagWpkh         tag = 0x00
	tagTr           tag = 0x01
	tagWsh          tag = 0x02
	tagSh           tag = 0x03
	tagPkh          tag = 0x04
	tagTapTree      tag = 0x05
	tagMulti        tag = 0x06
	tagSortedMulti  tag = 0x07
	tagMultiA       tag = 0x08
	tagSortedMultiA tag = 0x09
	tagPkK          tag = 0x0A
	tagPkH          tag = 0x0B
	tagCheck        tag = 0x0C
	tagVerify       tag = 0x0D
	tagSwap         tag = 0x0E
	tagAlt          tag = 0x0F
	tagDupIf        tag = 0x10
	tagNonZero      tag = 0x11
	tagZeroNotEqual tag = 0x12
	tagAndV         tag = 0x13
	tagAndB         tag = 0x14
	tagAndOr        tag = 0x15
	tagOrB          tag = 0x16
	tagOrC          tag = 0x17
	tagOrD          tag = 0x18
	tagOrI          tag = 0x19
	tagThresh       tag = 0x1A
	tagAfter        tag = 0x1B
	tagOlder        tag = 0x1C
	tagSha256       tag = 0x1D
	tagHash160      tag = 0x1E
	tagHash256      tag = 0x1F
	tagRipemd160    tag = 0x20
	tagRawPkH       tag = 0x21
	tagFalse        tag = 0x22
	tagTrue         tag = 0x23
)

const extensionPrefix6Bit = 0x3F

// readTag decodes a tag from r, consuming 6 bits (or 10 for the extension
// prefix). Port of tag.rs:156-202.
func readTag(r *bitReader) (tag, error) {
	primary, err := r.read(6)
	if err != nil {
		return 0, err
	}
	if primary == extensionPrefix6Bit {
		// Consume the 4-bit subcode and reject — v0.30 allocates none.
		if _, err := r.read(4); err != nil {
			return 0, err
		}
		return 0, errTagOutOfRange
	}
	if primary <= uint64(tagTrue) {
		return tag(primary), nil
	}
	return 0, errTagOutOfRange
}

// ─── Body variants (port of tree.rs:18-73). Go interface with one struct per
// Rust Body variant; node = {tag, body}. ─────────────────────────────────────

type body interface{ isBody() }

type childrenBody struct{ children []node } // Class-1 fixed-arity (Sh/Wsh/wrappers/and/or/andor/TapTree)
type variableBody struct {                  // Thresh only
	k        uint8
	children []node
}
type multiKeysBody struct { // Multi / SortedMulti / MultiA / SortedMultiA
	k       uint8
	indices []uint8
}
type trBody struct { // Tr
	isNums   bool
	keyIndex uint8
	tree     *node
}
type keyArgBody struct{ index uint8 }
type hash256Body [32]byte
type hash160Body [20]byte
type timelockBody uint32
type emptyBody struct{}

func (childrenBody) isBody()  {}
func (variableBody) isBody()  {}
func (multiKeysBody) isBody() {}
func (trBody) isBody()        {}
func (keyArgBody) isBody()    {}
func (hash256Body) isBody()   {}
func (hash160Body) isBody()   {}
func (timelockBody) isBody()  {}
func (emptyBody) isBody()     {}

type node struct {
	tag  tag
	body body
}

// ─── Varint (port of varint.rs:44-56). LP4-ext. ──────────────────────────────

func readVarint(r *bitReader) (uint32, error) {
	l, err := r.read(4)
	if err != nil {
		return 0, err
	}
	if l < 15 {
		v, err := r.read(int(l))
		if err != nil {
			return 0, err
		}
		return uint32(v), nil
	}
	lHigh, err := r.read(4)
	if err != nil {
		return 0, err
	}
	payloadLow, err := r.read(14)
	if err != nil {
		return 0, err
	}
	payloadHigh, err := r.read(int(lHigh))
	if err != nil {
		return 0, err
	}
	return (uint32(payloadHigh) << 14) | uint32(payloadLow), nil
}

// ─── Origin paths (port of origin_path.rs). ──────────────────────────────────

const maxPathComponents = 15

type pathComponent struct {
	hardened bool
	value    uint32
}

func readPathComponent(r *bitReader) (pathComponent, error) {
	h, err := r.readBool()
	if err != nil {
		return pathComponent{}, err
	}
	v, err := readVarint(r)
	if err != nil {
		return pathComponent{}, err
	}
	return pathComponent{hardened: h, value: v}, nil
}

type originPath struct{ components []pathComponent }

// readOriginPath: 4-bit depth followed by that-many components.
func readOriginPath(r *bitReader) (originPath, error) {
	depth, err := r.read(4)
	if err != nil {
		return originPath{}, err
	}
	comps := make([]pathComponent, 0, depth)
	for i := uint64(0); i < depth; i++ {
		c, err := readPathComponent(r)
		if err != nil {
			return originPath{}, err
		}
		comps = append(comps, c)
	}
	return originPath{components: comps}, nil
}

type pathDecl struct {
	n         uint8
	shared    *originPath  // set iff !divergent
	divergent []originPath // set iff divergent
}

// readPathDecl: n = read(5)+1, then either one shared path or n divergent paths.
func readPathDecl(r *bitReader, divergent bool) (pathDecl, error) {
	raw, err := r.read(5)
	if err != nil {
		return pathDecl{}, err
	}
	n := uint8(raw) + 1
	if divergent {
		paths := make([]originPath, 0, n)
		for i := uint8(0); i < n; i++ {
			p, err := readOriginPath(r)
			if err != nil {
				return pathDecl{}, err
			}
			paths = append(paths, p)
		}
		return pathDecl{n: n, divergent: paths}, nil
	}
	p, err := readOriginPath(r)
	if err != nil {
		return pathDecl{}, err
	}
	return pathDecl{n: n, shared: &p}, nil
}

// ─── Use-site path (port of use_site_path.rs:98-116). ────────────────────────

const minAltCount = 2

type alternative struct {
	hardened bool
	value    uint32
}

func readAlternative(r *bitReader) (alternative, error) {
	h, err := r.readBool()
	if err != nil {
		return alternative{}, err
	}
	v, err := readVarint(r)
	if err != nil {
		return alternative{}, err
	}
	return alternative{hardened: h, value: v}, nil
}

// useSitePath models the Rust `multipath: Option<Vec<Alternative>>`: the
// hasMultipath flag distinguishes None (bare `*`) from Some([]) — a bare
// []alternative cannot. validateMultipathConsistency + useSiteString need
// the distinction (R0-M2).
type useSitePath struct {
	hasMultipath     bool
	multipath        []alternative
	wildcardHardened bool
}

// readUseSitePath: has_mp(1), [alt_count-2(3) + alts], wildcard(1).
func readUseSitePath(r *bitReader) (useSitePath, error) {
	hasMP, err := r.readBool()
	if err != nil {
		return useSitePath{}, err
	}
	var alts []alternative
	if hasMP {
		raw, err := r.read(3)
		if err != nil {
			return useSitePath{}, err
		}
		altCount := int(raw) + minAltCount
		alts = make([]alternative, 0, altCount)
		for i := 0; i < altCount; i++ {
			a, err := readAlternative(r)
			if err != nil {
				return useSitePath{}, err
			}
			alts = append(alts, a)
		}
	}
	wild, err := r.readBool()
	if err != nil {
		return useSitePath{}, err
	}
	return useSitePath{hasMultipath: hasMP, multipath: alts, wildcardHardened: wild}, nil
}

// ─── Header (port of header.rs:38-50). 5 bits; version must == 4. ────────────

const wfRedesignVersion = 4

type header struct {
	version        uint8
	divergentPaths bool
}

func readHeader(r *bitReader) (header, error) {
	raw, err := r.read(5)
	if err != nil {
		return header{}, err
	}
	divergent := (raw>>4)&1 != 0
	version := uint8(raw & 0b1111)
	if version != wfRedesignVersion {
		return header{}, errWireVersion
	}
	return header{version: version, divergentPaths: divergent}, nil
}

// ─── Tree (port of tree.rs:196-328). ─────────────────────────────────────────

const maxDecodeDepth = 128

func readNode(r *bitReader, kiw uint8) (node, error) {
	return readNodeDepth(r, kiw, 0)
}

func readNodeDepth(r *bitReader, kiw uint8, depth uint8) (node, error) {
	if depth >= maxDecodeDepth {
		return node{}, errDepthExceeded
	}
	t, err := readTag(r)
	if err != nil {
		return node{}, err
	}
	var b body
	switch t {
	case tagPkK, tagPkH, tagWpkh, tagPkh:
		idx, err := r.read(int(kiw))
		if err != nil {
			return node{}, err
		}
		b = keyArgBody{index: uint8(idx)}
	case tagSh, tagWsh, tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual:
		child, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{child}}
	case tagAndV, tagAndB, tagOrB, tagOrC, tagOrD, tagOrI:
		l, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		r2, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{l, r2}}
	case tagAndOr:
		a, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		bb, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		c, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{a, bb, c}}
	case tagTapTree:
		l, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		r2, err := readNodeDepth(r, kiw, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{l, r2}}
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		kRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		countRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		k := uint8(kRaw) + 1
		count := int(countRaw) + 1
		if int(k) > count {
			return node{}, errKGreaterThanN
		}
		indices := make([]uint8, 0, count)
		for i := 0; i < count; i++ {
			idx, err := r.read(int(kiw))
			if err != nil {
				return node{}, err
			}
			indices = append(indices, uint8(idx))
		}
		b = multiKeysBody{k: k, indices: indices}
	case tagThresh:
		kRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		countRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		k := uint8(kRaw) + 1
		count := int(countRaw) + 1
		if int(k) > count {
			return node{}, errKGreaterThanN
		}
		children := make([]node, 0, count)
		for i := 0; i < count; i++ {
			c, err := readNodeDepth(r, kiw, depth+1)
			if err != nil {
				return node{}, err
			}
			children = append(children, c)
		}
		b = variableBody{k: k, children: children}
	case tagTr:
		isNums, err := r.readBool()
		if err != nil {
			return node{}, err
		}
		var keyIndex uint8
		if !isNums {
			idx, err := r.read(int(kiw))
			if err != nil {
				return node{}, err
			}
			keyIndex = uint8(idx)
		}
		hasTree, err := r.readBool()
		if err != nil {
			return node{}, err
		}
		var sub *node
		if hasTree {
			child, err := readNodeDepth(r, kiw, depth+1)
			if err != nil {
				return node{}, err
			}
			sub = &child
		}
		b = trBody{isNums: isNums, keyIndex: keyIndex, tree: sub}
	case tagAfter, tagOlder:
		v, err := r.read(32)
		if err != nil {
			return node{}, err
		}
		b = timelockBody(uint32(v))
	case tagSha256, tagHash256:
		var h hash256Body
		for i := range h {
			v, err := r.read(8)
			if err != nil {
				return node{}, err
			}
			h[i] = uint8(v)
		}
		b = h
	case tagHash160, tagRipemd160, tagRawPkH:
		var h hash160Body
		for i := range h {
			v, err := r.read(8)
			if err != nil {
				return node{}, err
			}
			h[i] = uint8(v)
		}
		b = h
	case tagFalse, tagTrue:
		b = emptyBody{}
	default:
		return node{}, errTagOutOfRange
	}
	return node{tag: t, body: b}, nil
}

// ─── TLV (port of tlv.rs:210-447). ───────────────────────────────────────────

const (
	tlvUseSitePathOverrides uint8 = 0x00
	tlvFingerprints         uint8 = 0x01
	tlvPubkeys              uint8 = 0x02
	tlvOriginPathOverrides  uint8 = 0x03
)

type idxUseSite struct {
	idx  uint8
	path useSitePath
}
type idxFP struct {
	idx uint8
	fp  [4]byte
}
type idxPub struct {
	idx  uint8
	xpub [65]byte
}
type idxOrigin struct {
	idx  uint8
	path originPath
}
type tlvUnknown struct {
	tag     uint8
	payload []byte
	bitLen  int
}

type tlvSection struct {
	useSiteOverrides []idxUseSite // nil = absent (Option None); non-nil non-empty when present
	useSitePresent   bool
	fingerprints     []idxFP
	fpPresent        bool
	pubkeys          []idxPub
	pubPresent       bool
	originOverrides  []idxOrigin
	originPresent    bool
	unknown          []tlvUnknown
}

// readTLV consumes all remaining bits, parsing TLV entries with ascending-tag
// ordering and ≤7-bit trailing-padding rollback tolerance.
func readTLV(r *bitReader, kiw uint8, n uint8) (tlvSection, error) {
	var section tlvSection
	haveLastTag := false
	var lastTag uint8
	for {
		entryStart := r.pos()
		if r.remaining() < 5 {
			break // not enough bits for even a tag — clean end-of-stream
		}
		ok, err := readTLVEntry(r, kiw, n, &section, &haveLastTag, &lastTag)
		if ok {
			continue
		}
		// Decide: rollback-as-padding or propagate error.
		r.restore(entryStart)
		if r.remaining() <= 7 {
			break
		}
		return tlvSection{}, err
	}
	return section, nil
}

// readTLVEntry parses one TLV entry. Returns (true, nil) on success;
// (false, err) on any failure (the caller decides rollback vs propagate).
func readTLVEntry(r *bitReader, kiw uint8, n uint8, section *tlvSection, haveLastTag *bool, lastTag *uint8) (bool, error) {
	tagRaw, err := r.read(5)
	if err != nil {
		return false, err
	}
	t := uint8(tagRaw)
	if *haveLastTag && t <= *lastTag {
		return false, errTLVOrdering
	}
	bitLenRaw, err := readVarint(r)
	if err != nil {
		return false, err
	}
	bitLen := int(bitLenRaw)
	if bitLen > r.remaining() {
		return false, errTLVLength
	}
	if bitLen == 0 {
		return false, errEmptyTLV
	}
	switch t {
	case tlvUseSitePathOverrides:
		entry, err := readUseSiteOverrides(r, bitLen, kiw, n, t)
		if err != nil {
			return false, err
		}
		section.useSiteOverrides = entry
		section.useSitePresent = true
	case tlvFingerprints:
		entry, err := readFingerprints(r, bitLen, kiw, n, t)
		if err != nil {
			return false, err
		}
		section.fingerprints = entry
		section.fpPresent = true
	case tlvPubkeys:
		entry, err := readPubkeys(r, bitLen, kiw, n, t)
		if err != nil {
			return false, err
		}
		section.pubkeys = entry
		section.pubPresent = true
	case tlvOriginPathOverrides:
		entry, err := readOriginPathOverrides(r, bitLen, kiw, n, t)
		if err != nil {
			return false, err
		}
		section.originOverrides = entry
		section.originPresent = true
	default:
		// Unknown — buffer and skip per D6 forward-compat.
		payload, err := readUnknownPayload(r, bitLen)
		if err != nil {
			return false, err
		}
		section.unknown = append(section.unknown, tlvUnknown{tag: t, payload: payload, bitLen: bitLen})
	}
	*haveLastTag = true
	*lastTag = t
	return true, nil
}

func readUnknownPayload(r *bitReader, bitLen int) ([]byte, error) {
	out := make([]byte, 0, (bitLen+7)/8)
	remaining := bitLen
	var cur byte
	var curBits int
	for remaining > 0 {
		chunk := remaining
		if chunk > 8 {
			chunk = 8
		}
		v, err := r.read(chunk)
		if err != nil {
			return nil, err
		}
		// Pack MSB-first into output bytes (mirrors BitWriter).
		for i := chunk - 1; i >= 0; i-- {
			bit := byte((v >> uint(i)) & 1)
			cur = (cur << 1) | bit
			curBits++
			if curBits == 8 {
				out = append(out, cur)
				cur = 0
				curBits = 0
			}
		}
		remaining -= chunk
	}
	if curBits > 0 {
		out = append(out, cur<<uint(8-curBits))
	}
	return out, nil
}

// readSparseTLVIdx reads one key_index_width-bit idx, range-checks it against n,
// and (if last is set) verifies it is strictly greater than the previous idx.
func readSparseTLVIdx(r *bitReader, kiw uint8, n uint8, haveLast bool, last uint8) (uint8, error) {
	raw, err := r.read(int(kiw))
	if err != nil {
		return 0, err
	}
	idx := uint8(raw)
	if idx >= n {
		return 0, errPlaceholderRange
	}
	if haveLast && idx <= last {
		return 0, errOverrideOrder
	}
	return idx, nil
}

func readUseSiteOverrides(r *bitReader, bitLen int, kiw uint8, n uint8, t uint8) ([]idxUseSite, error) {
	start := r.pos()
	savedLimit := r.limit()
	r.setLimit(start + bitLen)
	var entries []idxUseSite
	haveLast := false
	var last uint8
	loopErr := func() error {
		for r.pos()-start < bitLen {
			idx, err := readSparseTLVIdx(r, kiw, n, haveLast, last)
			if err != nil {
				return err
			}
			p, err := readUseSitePath(r)
			if err != nil {
				return err
			}
			haveLast = true
			last = idx
			entries = append(entries, idxUseSite{idx: idx, path: p})
		}
		return nil
	}()
	r.setLimit(savedLimit)
	if loopErr != nil {
		return nil, loopErr
	}
	if len(entries) == 0 {
		return nil, errEmptyTLV
	}
	return entries, nil
}

func readFingerprints(r *bitReader, bitLen int, kiw uint8, n uint8, t uint8) ([]idxFP, error) {
	start := r.pos()
	savedLimit := r.limit()
	r.setLimit(start + bitLen)
	var entries []idxFP
	haveLast := false
	var last uint8
	loopErr := func() error {
		for r.pos()-start < bitLen {
			idx, err := readSparseTLVIdx(r, kiw, n, haveLast, last)
			if err != nil {
				return err
			}
			var fp [4]byte
			for i := range fp {
				v, err := r.read(8)
				if err != nil {
					return err
				}
				fp[i] = uint8(v)
			}
			haveLast = true
			last = idx
			entries = append(entries, idxFP{idx: idx, fp: fp})
		}
		return nil
	}()
	r.setLimit(savedLimit)
	if loopErr != nil {
		return nil, loopErr
	}
	if len(entries) == 0 {
		return nil, errEmptyTLV
	}
	return entries, nil
}

func readPubkeys(r *bitReader, bitLen int, kiw uint8, n uint8, t uint8) ([]idxPub, error) {
	start := r.pos()
	savedLimit := r.limit()
	r.setLimit(start + bitLen)
	var entries []idxPub
	haveLast := false
	var last uint8
	loopErr := func() error {
		for r.pos()-start < bitLen {
			idx, err := readSparseTLVIdx(r, kiw, n, haveLast, last)
			if err != nil {
				return err
			}
			var xpub [65]byte
			for i := range xpub {
				v, err := r.read(8)
				if err != nil {
					return err
				}
				xpub[i] = uint8(v)
			}
			haveLast = true
			last = idx
			entries = append(entries, idxPub{idx: idx, xpub: xpub})
		}
		return nil
	}()
	r.setLimit(savedLimit)
	if loopErr != nil {
		return nil, loopErr
	}
	if len(entries) == 0 {
		return nil, errEmptyTLV
	}
	return entries, nil
}

func readOriginPathOverrides(r *bitReader, bitLen int, kiw uint8, n uint8, t uint8) ([]idxOrigin, error) {
	start := r.pos()
	savedLimit := r.limit()
	r.setLimit(start + bitLen)
	var entries []idxOrigin
	haveLast := false
	var last uint8
	loopErr := func() error {
		for r.pos()-start < bitLen {
			idx, err := readSparseTLVIdx(r, kiw, n, haveLast, last)
			if err != nil {
				return err
			}
			p, err := readOriginPath(r)
			if err != nil {
				return err
			}
			haveLast = true
			last = idx
			entries = append(entries, idxOrigin{idx: idx, path: p})
		}
		return nil
	}()
	r.setLimit(savedLimit)
	if loopErr != nil {
		return nil, loopErr
	}
	if len(entries) == 0 {
		return nil, errEmptyTLV
	}
	return entries, nil
}

// ─── Descriptor + decodePayload (port of decode.rs:15-54). ────────────────────

type descriptor struct {
	n        uint8
	pathDecl pathDecl
	useSite  useSitePath
	tree     node
	tlv      tlvSection
}

// decodePayload decodes a Descriptor from the canonical payload bit stream
// (Header → PathDecl → UseSitePath → kiw → readNode → root-tag allow-list →
// TLV). The 5 post-decode validators are applied by decodePayloadValidated.
func decodePayload(b []byte, totalBits int) (*descriptor, error) {
	r := newBitReader(b, totalBits)

	h, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	pd, err := readPathDecl(r, h.divergentPaths)
	if err != nil {
		return nil, err
	}
	us, err := readUseSitePath(r)
	if err != nil {
		return nil, err
	}
	// kiw = ⌈log₂(n)⌉ = 32 - leadingZeros(n-1).
	kiw := uint8(32 - bits.LeadingZeros32(uint32(pd.n)-1))
	tree, err := readNode(r, kiw)
	if err != nil {
		return nil, err
	}
	switch tree.tag {
	case tagSh, tagWsh, tagWpkh, tagPkh, tagTr:
	default:
		return nil, errOperatorContext
	}
	tlv, err := readTLV(r, kiw, pd.n)
	if err != nil {
		return nil, err
	}
	return &descriptor{
		n:        pd.n,
		pathDecl: pd,
		useSite:  us,
		tree:     tree,
		tlv:      tlv,
	}, nil
}

// symbolsToBytes repacks 5-bit symbols into MSB-first byte-padded bytes (port
// of codex32.rs symbols_to_bytes).
func symbolsToBytes(syms []byte) []byte {
	out := make([]byte, 0, (len(syms)*5+7)/8)
	var cur byte
	var curBits int
	for _, s := range syms {
		for i := 4; i >= 0; i-- {
			bit := (s >> uint(i)) & 1
			cur = (cur << 1) | bit
			curBits++
			if curBits == 8 {
				out = append(out, cur)
				cur = 0
				curBits = 0
			}
		}
	}
	if curBits > 0 {
		out = append(out, cur<<uint(8-curBits))
	}
	return out
}

// _ keeps codex32 referenced until Decode (Task 4) uses it.
var _ = codex32.MDDataSymbols
