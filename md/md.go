package md

import (
	"errors"
	"math/bits"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
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

// ─── Validators (port of validate.rs:17-226) ────────────────────────────────

var (
	errMissingExplicitOrigin    = errors.New("md: missing explicit origin")
	errPlaceholderNotReferenced = errors.New("md: placeholder not referenced")
	errPlaceholderOrder         = errors.New("md: placeholder first-occurrence out of order")
	errMultipathAltMismatch     = errors.New("md: multipath alt-count mismatch")
	errForbiddenTapLeaf         = errors.New("md: forbidden tap-tree leaf")
	errNUMSConflict             = errors.New("md: NUMS sentinel conflict")
	errInvalidXpubBytes         = errors.New("md: invalid xpub bytes")
)

// validatePlaceholderUsage enforces (1) every @i for 0≤i<n appears at least
// once, and (2) first occurrences (pre-order) are in canonical ascending order
// (port of validate.rs:17-110).
func validatePlaceholderUsage(root node, n uint8) error {
	seen := make([]bool, n)
	var firstOccurrences []uint8
	if err := walkForPlaceholders(root, seen, &firstOccurrences); err != nil {
		return err
	}
	for _, wasSeen := range seen {
		if !wasSeen {
			return errPlaceholderNotReferenced
		}
	}
	for pos, idx := range firstOccurrences {
		if int(idx) != pos {
			return errPlaceholderOrder
		}
	}
	return nil
}

func walkForPlaceholders(n node, seen []bool, firstOccurrences *[]uint8) error {
	switch b := n.body.(type) {
	case keyArgBody:
		if int(b.index) >= len(seen) {
			return errPlaceholderRange
		}
		if !seen[b.index] {
			seen[b.index] = true
			*firstOccurrences = append(*firstOccurrences, b.index)
		}
	case childrenBody:
		for _, c := range b.children {
			if err := walkForPlaceholders(c, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case variableBody:
		for _, c := range b.children {
			if err := walkForPlaceholders(c, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case multiKeysBody:
		for _, index := range b.indices {
			if int(index) >= len(seen) {
				return errPlaceholderRange
			}
			if !seen[index] {
				seen[index] = true
				*firstOccurrences = append(*firstOccurrences, index)
			}
		}
	case trBody:
		if !b.isNums {
			if int(b.keyIndex) >= len(seen) {
				return errNUMSConflict
			}
			if !seen[b.keyIndex] {
				seen[b.keyIndex] = true
				*firstOccurrences = append(*firstOccurrences, b.keyIndex)
			}
		}
		if b.tree != nil {
			if err := walkForPlaceholders(*b.tree, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case hash256Body, hash160Body, timelockBody, emptyBody:
	}
	return nil
}

// validateMultipathConsistency: all multipath groups (shared default + per-@N
// overrides) MUST carry the same alt-count (port of validate.rs:117-138).
func validateMultipathConsistency(shared useSitePath, overrides []idxUseSite) error {
	haveSeen := false
	var seenAltCount int
	check := func(p useSitePath) error {
		if p.hasMultipath {
			if !haveSeen {
				haveSeen = true
				seenAltCount = len(p.multipath)
			} else if seenAltCount != len(p.multipath) {
				return errMultipathAltMismatch
			}
		}
		return nil
	}
	if err := check(shared); err != nil {
		return err
	}
	for _, o := range overrides {
		if err := check(o.path); err != nil {
			return err
		}
	}
	return nil
}

// validateTapScriptTree: all leaves in a tap-script-tree must be permitted-leaf
// tags per §6.3.1 (port of validate.rs:141-169).
func validateTapScriptTree(n node) error {
	if n.tag == tagTapTree {
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := validateTapScriptTree(c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if isForbiddenLeafTag(n.tag) {
		return errForbiddenTapLeaf
	}
	return nil
}

func isForbiddenLeafTag(t tag) bool {
	switch t {
	case tagWpkh, tagTr, tagWsh, tagSh, tagPkh, tagMulti, tagSortedMulti:
		return true
	}
	return false
}

// validateExplicitOriginRequired: when canonicalOrigin(tree) is None, every @N
// must have an explicit (non-empty) origin path on the wire — via an
// OriginPathOverrides entry or via path_decl (port of validate.rs:182-207).
func validateExplicitOriginRequired(d *descriptor) error {
	if _, ok := canonicalOrigin(d.tree); ok {
		return nil
	}
	for idx := uint8(0); idx < d.n; idx++ {
		// Override path takes precedence — if present and non-empty, OK.
		if d.tlv.originPresent {
			overridden := false
			for _, o := range d.tlv.originOverrides {
				if o.idx == idx && len(o.path.components) != 0 {
					overridden = true
					break
				}
			}
			if overridden {
				continue
			}
		}
		// Otherwise consult the path_decl for this idx.
		declEmpty := true
		if d.pathDecl.divergent != nil {
			if int(idx) < len(d.pathDecl.divergent) {
				declEmpty = len(d.pathDecl.divergent[idx].components) == 0
			}
		} else if d.pathDecl.shared != nil {
			declEmpty = len(d.pathDecl.shared.components) == 0
		}
		if declEmpty {
			return errMissingExplicitOrigin
		}
	}
	return nil
}

// validateXpubBytes checks that every Pubkeys TLV entry's 33-byte compressed
// pubkey field (bytes 32..65 of the 65-byte payload) parses as a valid
// secp256k1 point (D4; faithful port of validate.rs:216-226). When the Pubkeys
// TLV is absent (template-only mode) this is a no-op. The 32-byte chain-code
// prefix (bytes 0..32) is intentionally unvalidated — any 32 bytes are a
// structurally valid BIP-32 chain code (validate.rs:209-212).
func validateXpubBytes(d *descriptor) error {
	if !d.tlv.pubPresent {
		return nil
	}
	for _, p := range d.tlv.pubkeys {
		if _, err := secp256k1.ParsePubKey(p.xpub[32:65]); err != nil {
			return errInvalidXpubBytes
		}
	}
	return nil
}

// ─── Canonical-origin table (port of canonical_origin.rs:38-79) ──────────────

func mkOrigin(comps ...pathComponent) originPath { return originPath{components: comps} }

func isWshInnerMulti(t tag) bool { return t == tagMulti || t == tagSortedMulti }

// canonicalOrigin returns the canonical path-from-master for the top-level
// wrapper `tree`, or (zero, false) for shapes that require explicit overrides.
// Used by validateExplicitOriginRequired AND, per the R0-I1 precedence, as the
// final fallback in resolveOriginPath (expand.go) to substitute a renderable
// key's OriginPath when an elided shared path-decl would otherwise display the
// wrong depth/childnum.
func canonicalOrigin(tree node) (originPath, bool) {
	switch tree.tag {
	case tagPkh:
		if _, ok := tree.body.(keyArgBody); ok {
			return mkOrigin(pathComponent{true, 44}, pathComponent{true, 0}, pathComponent{true, 0}), true
		}
	case tagWpkh:
		if _, ok := tree.body.(keyArgBody); ok {
			return mkOrigin(pathComponent{true, 84}, pathComponent{true, 0}, pathComponent{true, 0}), true
		}
	case tagTr:
		if b, ok := tree.body.(trBody); ok {
			if b.tree == nil {
				return mkOrigin(pathComponent{true, 86}, pathComponent{true, 0}, pathComponent{true, 0}), true
			}
			return originPath{}, false // tr(@N, TapTree) → forced explicit
		}
	case tagWsh:
		if b, ok := tree.body.(childrenBody); ok && len(b.children) == 1 && isWshInnerMulti(b.children[0].tag) {
			return mkOrigin(pathComponent{true, 48}, pathComponent{true, 0}, pathComponent{true, 0}, pathComponent{true, 2}), true
		}
	case tagSh:
		if b, ok := tree.body.(childrenBody); ok && len(b.children) == 1 {
			inner := b.children[0]
			if inner.tag == tagWsh {
				if gb, ok := inner.body.(childrenBody); ok && len(gb.children) == 1 && isWshInnerMulti(gb.children[0].tag) {
					return mkOrigin(pathComponent{true, 48}, pathComponent{true, 0}, pathComponent{true, 0}, pathComponent{true, 1}), true
				}
			}
		}
	}
	return originPath{}, false
}

// ─── decodePayloadValidated (decode.rs:56-69 ordering) ───────────────────────

func decodePayloadValidated(b []byte, totalBits int) (*descriptor, error) {
	d, err := decodePayload(b, totalBits)
	if err != nil {
		return nil, err
	}
	if err := validatePlaceholderUsage(d.tree, d.n); err != nil {
		return nil, err
	}
	if d.tlv.useSitePresent {
		if err := validateMultipathConsistency(d.useSite, d.tlv.useSiteOverrides); err != nil {
			return nil, err
		}
	}
	if d.tree.tag == tagTr {
		if b, ok := d.tree.body.(trBody); ok && b.tree != nil {
			if err := validateTapScriptTree(*b.tree); err != nil {
				return nil, err
			}
		}
	}
	if err := validateExplicitOriginRequired(d); err != nil {
		return nil, err
	}
	if err := validateXpubBytes(d); err != nil {
		return nil, err
	}
	return d, nil
}

// ─── Public template types ───────────────────────────────────────────────────

// ScriptKind is the top-level descriptor wrapper.
type ScriptKind int

const (
	ScriptWpkh ScriptKind = iota
	ScriptPkh
	ScriptSh
	ScriptWsh
	ScriptTr
	// ScriptShWpkh is APPENDED after ScriptTr (R0-M2): a BIP-49 nested-segwit
	// sh(wpkh) single-sig wrapper. It is an EncodeSingleSig input discriminant
	// only — the decoder summarizes an sh(wpkh) wire to Root==ScriptSh (the
	// on-wire root tag is Sh). Appending (not inserting) preserves the existing
	// values so rootScriptKind/#10b consumers are unaffected.
	ScriptShWpkh
)

// PolicyKind is the spending-policy shape.
type PolicyKind int

const (
	PolicySingle PolicyKind = iota
	PolicyMulti
	PolicySortedMulti
	PolicyMultiA
	PolicySortedMultiA
	PolicyComplex
)

// KeyOrigin is the per-@N renderable key info: the key's placeholder index, its
// xpub fingerprint (8-hex lowercase or ""), its DECODED origin path ("m" for an
// elided origin), and its use-site path (e.g. "<0;1>/*").
type KeyOrigin struct {
	Index       int
	Fingerprint string
	OriginPath  string
	UseSite     string
}

// Template is the decoded, renderable BIP-388 descriptor summary.
type Template struct {
	N          int
	Root       ScriptKind
	Policy     PolicyKind
	K, M       int
	Keys       []KeyOrigin
	Renderable bool
	// InnerWsh is the sh-nesting discriminant (R0-C2): true iff Root==ScriptSh
	// AND the immediate sh child is a wsh wrapper (sh(wsh(multi/sortedmulti))).
	// It distinguishes a nested-segwit P2SH-P2WSH from a bare legacy P2SH
	// multisig — both summarize to ScriptSh+PolicySortedMulti, but they hash to
	// DIFFERENT addresses, so a consumer building a *bip380.Descriptor MUST use
	// this to pick P2SH_P2WSH vs P2SH and never verify one against the other.
	// Meaningful only when Root==ScriptSh; false for every other root.
	InnerWsh bool
	// InnerWpkh is the sh(wpkh) single-sig discriminant: true iff Root==ScriptSh
	// AND the immediate sh child is a wpkh key (sh(wpkh) — BIP-49 P2SH-P2WPKH).
	// A consumer building a *bip380.Descriptor uses it to pick P2SH_P2WPKH for the
	// single-sig sh root, symmetric with InnerWsh for the sorted-multi sh root.
	// Meaningful only when Root==ScriptSh && Policy==PolicySingle.
	InnerWpkh bool
}

// Decode decodes a single-string md1 descriptor into a Template. It refuses
// chunked md1 (ErrChunkedUnsupported) and returns an error for any malformed or
// out-of-spec wire payload.
func Decode(s string) (Template, error) {
	syms, err := codex32.MDDataSymbols(s)
	if err != nil {
		return Template{}, err
	}
	if len(syms) == 0 || syms[0]&1 == 1 { // chunked-flag (bit 0 of symbol 0)
		return Template{}, ErrChunkedUnsupported
	}
	b := symbolsToBytes(syms)
	d, err := decodePayloadValidated(b, 5*len(syms))
	if err != nil {
		return Template{}, err
	}
	return summarize(d), nil
}

// ─── summarize (spec §4.2 renderable-shape classifier) ───────────────────────

func rootScriptKind(t tag) ScriptKind {
	switch t {
	case tagPkh:
		return ScriptPkh
	case tagSh:
		return ScriptSh
	case tagWsh:
		return ScriptWsh
	case tagTr:
		return ScriptTr
	default:
		return ScriptWpkh
	}
}

// classifyPolicy walks the renderable shapes and returns (policy, k, m). A
// non-renderable shape returns PolicyComplex.
func classifyPolicy(tree node) (PolicyKind, int, int) {
	switch tree.tag {
	case tagWpkh, tagPkh:
		if _, ok := tree.body.(keyArgBody); ok {
			return PolicySingle, 0, 0
		}
	case tagTr:
		if b, ok := tree.body.(trBody); ok {
			// A tr is renderable ONLY as a single-key taproot: no NUMS internal
			// key AND no script tree. ANY tr with a script tree is refused
			// (spec §4.2): summarizing a tapscript leaf would omit the key-path
			// and other-leaf spend conditions and mislead the operator
			// (invariant 2.4). The !b.isNums guard makes the keyspend
			// classification locally robust — a NUMS-keypath-only tr (no @i
			// referenced) is already rejected by validatePlaceholderUsage before
			// summarize, but we never claim a single-key policy for is_nums here.
			if !b.isNums && b.tree == nil {
				return PolicySingle, 0, 0 // tr(@N) key-path only
			}
		}
	case tagWsh:
		if b, ok := tree.body.(childrenBody); ok && len(b.children) == 1 {
			if pol, k, m, ok := multiPolicy(b.children[0]); ok {
				return pol, k, m
			}
		}
	case tagSh:
		if b, ok := tree.body.(childrenBody); ok && len(b.children) == 1 {
			inner := b.children[0]
			// sh(wpkh) — BIP-49 P2SH-P2WPKH single-sig nested-segwit.
			if inner.tag == tagWpkh {
				if _, ok := inner.body.(keyArgBody); ok {
					return PolicySingle, 0, 0
				}
			}
			// sh(wsh(multi/sortedmulti))
			if inner.tag == tagWsh {
				if gb, ok := inner.body.(childrenBody); ok && len(gb.children) == 1 {
					if pol, k, m, ok := multiPolicy(gb.children[0]); ok {
						return pol, k, m
					}
				}
			}
			// sh(multi/sortedmulti) legacy P2SH multisig
			if pol, k, m, ok := multiPolicy(inner); ok {
				return pol, k, m
			}
		}
	}
	return PolicyComplex, 0, 0
}

func multiPolicy(n node) (PolicyKind, int, int, bool) {
	if b, ok := n.body.(multiKeysBody); ok {
		switch n.tag {
		case tagMulti:
			return PolicyMulti, int(b.k), len(b.indices), true
		case tagSortedMulti:
			return PolicySortedMulti, int(b.k), len(b.indices), true
		}
	}
	return PolicyComplex, 0, 0, false
}

// innerWshNesting reports whether tree is an sh(wsh(...)) wrapper — the
// nesting discriminant for the ScriptSh + PolicySortedMulti collapse (R0-C2).
// It mirrors the sh→wsh test in canonicalOrigin (md.go:1110-1118): an sh with a
// single wsh child. Returns false for a bare sh(sortedmulti) and for any
// non-sh root.
func innerWshNesting(tree node) bool {
	if tree.tag != tagSh {
		return false
	}
	b, ok := tree.body.(childrenBody)
	if !ok || len(b.children) != 1 {
		return false
	}
	return b.children[0].tag == tagWsh
}

// innerWpkhNesting reports whether tree is an sh(wpkh) wrapper — the single-sig
// nested-segwit discriminant (BIP-49 P2SH-P2WPKH). It mirrors innerWshNesting:
// an sh with a single wpkh-key child. Returns false for a bare wpkh, for any
// sh(wsh(...))/sh(multi), and for any non-sh root.
func innerWpkhNesting(tree node) bool {
	if tree.tag != tagSh {
		return false
	}
	b, ok := tree.body.(childrenBody)
	if !ok || len(b.children) != 1 {
		return false
	}
	if b.children[0].tag != tagWpkh {
		return false
	}
	_, ok = b.children[0].body.(keyArgBody)
	return ok
}

func summarize(d *descriptor) Template {
	root := rootScriptKind(d.tree.tag)
	policy, k, m := classifyPolicy(d.tree)
	renderable := policy != PolicyComplex

	keys := make([]KeyOrigin, 0, d.n)
	for idx := uint8(0); idx < d.n; idx++ {
		keys = append(keys, KeyOrigin{
			Index:       int(idx),
			Fingerprint: fingerprintFor(d, idx),
			OriginPath:  originPathStringFor(d, idx),
			UseSite:     useSiteStringFor(d, idx),
		})
	}
	return Template{
		N:          int(d.n),
		Root:       root,
		Policy:     policy,
		K:          k,
		M:          m,
		Keys:       keys,
		Renderable: renderable,
		InnerWsh:   innerWshNesting(d.tree),
		InnerWpkh:  innerWpkhNesting(d.tree),
	}
}

// fingerprintFor returns the 8-hex lowercase fingerprint for @idx, or "".
func fingerprintFor(d *descriptor, idx uint8) string {
	if !d.tlv.fpPresent {
		return ""
	}
	for _, fp := range d.tlv.fingerprints {
		if fp.idx == idx {
			return hexLower(fp.fp[:])
		}
	}
	return ""
}

// originPathStringFor returns the DECODED origin path for @idx ("m" if elided).
// Override (TLV) takes precedence over path_decl. Never substitutes the
// canonical implied path.
func originPathStringFor(d *descriptor, idx uint8) string {
	if d.tlv.originPresent {
		for _, o := range d.tlv.originOverrides {
			if o.idx == idx {
				return pathString(o.path.components)
			}
		}
	}
	if d.pathDecl.divergent != nil {
		if int(idx) < len(d.pathDecl.divergent) {
			return pathString(d.pathDecl.divergent[idx].components)
		}
		return "m"
	}
	if d.pathDecl.shared != nil {
		return pathString(d.pathDecl.shared.components)
	}
	return "m"
}

// useSiteStringFor returns the use-site path for @idx (override or shared).
func useSiteStringFor(d *descriptor, idx uint8) string {
	if d.tlv.useSitePresent {
		for _, o := range d.tlv.useSiteOverrides {
			if o.idx == idx {
				return useSiteString(o.path)
			}
		}
	}
	return useSiteString(d.useSite)
}

// pathString renders BIP-32 path components from master (e.g. "m/84h/0h/0h").
// Empty components → "m". The hardened marker is the "h" form, matching the GUI's
// dominant notation (bip32.Path.String / bip380 descriptor Encode) so the inspect
// screen agrees with the derive/verify/restore screens (t4-M1). Display-only —
// the path string is never serialized; only components go on-card.
func pathString(comps []pathComponent) string {
	s := "m"
	for _, c := range comps {
		s += "/" + u32str(c.value)
		if c.hardened {
			s += "h"
		}
	}
	return s
}

// useSiteString renders a use-site path: "<0;1>/*" for the standard multipath,
// "*" for a bare wildcard, with a trailing "h" if the wildcard is hardened. The
// hardened marker is the "h" form, matching pathString and the GUI's dominant
// notation (t4-M1). Display-only.
func useSiteString(us useSitePath) string {
	var s string
	if us.hasMultipath {
		s = "<"
		for i, a := range us.multipath {
			if i > 0 {
				s += ";"
			}
			s += u32str(a.value)
			if a.hardened {
				s += "h"
			}
		}
		s += ">/*"
	} else {
		s = "*"
	}
	if us.wildcardHardened {
		s += "h"
	}
	return s
}

func u32str(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func hexLower(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}
