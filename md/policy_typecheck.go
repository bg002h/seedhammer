package md

// THE TYPE RULE of the policy summary (md review r2 I-1; port of md-codec
// 0.49.0 `policy_shape::policy_type_checks`, descriptor-mnemonic ca48a05e).
//
// policy_shape's walk vouches for a thresh node's own children (the
// allowlist); it does not vouch for the CONTEXT around a node. Untyped trees
// decode -- the decoder does not type-check -- and some of them spend in ways
// no summary of their fragments describes: wsh(or_b(pk(@0),thresh(2,...)))
// spends with NO signature, because the thresh is B where or_b needs a W and
// its first CHECKSIG consumes the 0 that pk(@0) left. So a tree that is not
// typed miniscript in its script context has no summary at all.
//
// Rust answers this by running md's own descriptor conversion
// (`to_miniscript::tree_type_checks`, i.e. `node_to_descriptor` over synthetic
// xpub keys) and asking whether it succeeds. This port has no rust-miniscript,
// so it re-implements exactly the checks that conversion runs, quoted from
// rust-miniscript at the pin descriptor-mnemonic vendors (ff4732e):
//
//   - the correctness type system, miniscript/types/correctness.rs
//     (Base B/K/V/W, Input, dissatisfiable "d", unit "u"); the malleability
//     half of Type::type_check (types/malleability.rs) never errors, so it is
//     not ported;
//   - Miniscript::from_ast (miniscript/mod.rs:322-338) at EVERY node: the type,
//     `tree_height > MAX_RECURSION_DEPTH` (402), and
//     Ctx::check_global_validity (context.rs): multi only outside Tap, multi_a
//     only in Tap, and the script-size limits on ExtData::pk_cost
//     (types/extra_props.rs) -- Legacy 520, Segwitv0 10000 and 3600, Tap the
//     block weight;
//   - the descriptor constructors: wsh/sh/sh(wsh) require a B root
//     (ScriptContext::top_level_type_check), and TapTree::combine refuses a
//     leaf deeper than 128;
//   - every TAP LEAF's root must be B too. rust-miniscript's TapTree::leaf
//     takes any Miniscript<Tap>, so md-codec at the vendored pin (a7680cf2)
//     still calls tr(NUMS, v:pk(@0)) typed; md review r3 found that gap and
//     the primary is adding the per-leaf B check. This port applies it now,
//     so it leads the vendored file on exactly the non-B-leaf trees until the
//     re-vendor (no vendored case has one);
//   - md's own conversion rules (to_miniscript.rs node_to_descriptor /
//     node_to_miniscript): bare PkK/PkH and Check over a bare key both mean
//     c:pk_k / c:pk_h; sortedmulti only as the sole child of wsh/sh/sh(wsh)
//     (and only k-of-n with n <= 20, no size check, as
//     Descriptor::new_*_sortedmulti); sortedmulti_a only as a whole tap leaf;
//     raw_pkh, a TapTree or a top-level wrapper inside miniscript refused;
//     thresh/multi/multi_a need 1 <= k <= n (Threshold::new, multi n <= 20,
//     multi_a n <= 999); older/after need 1 <= v < 2^31 (AbsLockTime and
//     RelLockTime::from_consensus).
//
// ONE DIFFERENCE BY CONSTRUCTION: Rust's lookup_key also refuses a placeholder
// index >= n. This walk sees only the tree, so it cannot; every tree it sees
// in production comes from Reassemble, which already refuses such an index
// (validatePlaceholderUsage), so on a decoded card the two agree.
//
// Display and identity only: decode, ids, addresses and re-encoding never
// consult it.

type msBase uint8

const (
	msB msBase = iota
	msK
	msV
	msW
)

type msInput uint8

const (
	msZero msInput = iota
	msOne
	msAny
	msOneNonZero
	msAnyNonZero
)

// msCorr is rust-miniscript's Correctness.
type msCorr struct {
	base   msBase
	input  msInput
	dissat bool // "d"
	unit   bool // "u"
}

// msExt is the part of rust-miniscript's ExtData the global checks read.
type msExt struct {
	pkCost     int
	freeVerify bool // has_free_verify: a v: over it costs no extra opcode
	height     int  // tree_height
}

type msType struct {
	corr msCorr
	ext  msExt
}

type msCtx uint8

const (
	ctxLegacy msCtx = iota
	ctxSegwitv0
	ctxTap
)

const (
	msMaxRecursionDepth       = 402       // lib.rs MAX_RECURSION_DEPTH
	msMaxScriptElementSize    = 520       // limits.rs, Legacy consensus
	msMaxScriptSize           = 10_000    // limits.rs, Segwitv0 consensus
	msMaxStandardP2WSHScript  = 3600      // limits.rs, Segwitv0 policy
	msMaxBlockWeight          = 4_000_000 // Tap: Weight::MAX_BLOCK
	msMaxPubkeysPerMultisig   = 20
	msMaxPubkeysInChecksigadd = 999
	msTaprootControlMaxNodes  = 128
)

// treeTypeChecks reports whether tree converts to typed miniscript in its own
// context (Rust policy_type_checks). See the file comment for exactly what it
// checks and where each rule is quoted from.
func treeTypeChecks(tree node) bool {
	switch tree.tag {
	case tagPkh, tagWpkh:
		_, ok := tree.body.(keyArgBody)
		return ok
	case tagWsh:
		inner, ok := soleChild(tree)
		if !ok {
			return false
		}
		return wshInnerTypeChecks(inner)
	case tagSh:
		inner, ok := soleChild(tree)
		if !ok {
			return false
		}
		switch inner.tag {
		case tagWsh:
			if g, ok := soleChild(inner); ok {
				return wshInnerTypeChecks(g)
			}
		case tagWpkh:
			if _, ok := inner.body.(keyArgBody); ok {
				return true
			}
		case tagSortedMulti:
			if b, ok := inner.body.(multiKeysBody); ok {
				return msThresholdOK(int(b.k), len(b.indices), msMaxPubkeysPerMultisig)
			}
		}
		// sh_inner_to_descriptor's `_` arm: everything else (including a
		// malformed wsh/wpkh/sortedmulti body) is a Legacy miniscript.
		t, ok := msNode(inner, ctxLegacy)
		return ok && t.corr.base == msB
	case tagTr:
		b, ok := tree.body.(trBody)
		if !ok {
			return false
		}
		if b.tree == nil {
			return true
		}
		return tapTreeTypeChecks(*b.tree, 0)
	}
	return false
}

// wshInnerTypeChecks is wsh_inner_to_descriptor: a sole sortedmulti, or a
// Segwitv0 miniscript whose root is B.
func wshInnerTypeChecks(inner node) bool {
	if inner.tag == tagSortedMulti {
		if b, ok := inner.body.(multiKeysBody); ok {
			return msThresholdOK(int(b.k), len(b.indices), msMaxPubkeysPerMultisig)
		}
	}
	t, ok := msNode(inner, ctxSegwitv0)
	return ok && t.corr.base == msB
}

// tapTreeTypeChecks is tree_to_taptree. depth counts the TapTree nodes above n.
func tapTreeTypeChecks(n node, depth int) bool {
	if n.tag == tagTapTree {
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return false
		}
		return tapTreeTypeChecks(b.children[0], depth+1) && tapTreeTypeChecks(b.children[1], depth+1)
	}
	// TapTree::combine refuses a child leaf already at depth > 127, so a leaf
	// may sit at most 128 TapTree nodes deep.
	if depth > msTaprootControlMaxNodes {
		return false
	}
	if n.tag == tagSortedMultiA {
		b, ok := n.body.(multiKeysBody)
		if !ok || !msThresholdOK(int(b.k), len(b.indices), msMaxPubkeysInChecksigadd) {
			return false
		}
		// Miniscript::<_, Tap>::from_ast(SortedMultiA): typed B, size checked.
		return msGlobalOK(msLeafMultiA(int(b.k), len(b.indices)), tagSortedMultiA, ctxTap)
	}
	// A tap leaf's root must be B, like wsh's (md review r3, ahead of the
	// Rust pin: see the file comment).
	t, ok := msNode(n, ctxTap)
	return ok && t.corr.base == msB
}

// msThresholdOK is Threshold::new's validate_k_n: 1 <= k <= n, and n <= max
// when max > 0.
func msThresholdOK(k, n, max int) bool {
	return k >= 1 && k <= n && (max == 0 || n <= max)
}

// msTimelockOK is AbsLockTime/RelLockTime::from_consensus: after needs
// 1 <= v <= 0x7FFF_FFFF; older needs the disable flag (bit 31) clear and
// v != 0. Both come to 1 <= v < 2^31.
func msTimelockOK(v uint32) bool { return v >= 1 && v < 1<<31 }

// msScriptNumSize is lib.rs script_num_size.
func msScriptNumSize(n int) int {
	switch {
	case n <= 0x10:
		return 1
	case n < 0x80:
		return 2
	case n < 0x8000:
		return 3
	case n < 0x800000:
		return 4
	case n < 0x80000000:
		return 5
	}
	return 6
}

// msMultiNumCost is the k/n push cost shared by multi and multi_a.
func msMultiNumCost(k, n int) int {
	switch {
	case k > 16 && n > 16:
		return 4
	case k > 16 || n > 16:
		return 3
	}
	return 2
}

func msLeafMultiA(k, n int) msType {
	return msType{
		corr: msCorr{base: msB, input: msAny, dissat: true, unit: true},
		ext:  msExt{pkCost: msMultiNumCost(k, n) + 33*n + (n - 1) + 1, freeVerify: true},
	}
}

// msGlobalOK is the rest of Miniscript::from_ast after the type: the recursion
// depth and Ctx::check_global_validity (consensus + policy).
func msGlobalOK(t msType, tg tag, ctx msCtx) bool {
	if t.ext.height > msMaxRecursionDepth {
		return false
	}
	switch ctx {
	case ctxLegacy:
		if tg == tagMultiA || tg == tagSortedMultiA {
			return false
		}
		return t.ext.pkCost <= msMaxScriptElementSize
	case ctxSegwitv0:
		if tg == tagMultiA || tg == tagSortedMultiA {
			return false
		}
		return t.ext.pkCost <= msMaxScriptSize && t.ext.pkCost <= msMaxStandardP2WSHScript
	case ctxTap:
		if tg == tagMulti || tg == tagSortedMulti {
			return false
		}
		return t.ext.pkCost <= msMaxBlockWeight
	}
	return false
}

// msNode is node_to_miniscript followed by Miniscript::from_ast: the type of
// n in ctx, or ok=false where Rust's conversion errors.
func msNode(n node, ctx msCtx) (msType, bool) {
	var t msType
	switch n.tag {
	case tagPkK, tagPkH:
		// md's wire normalises c:pk_k / c:pk_h to the bare key; Rust re-applies
		// the c:, so a bare key is the B fragment pk(K) / pkh(K).
		if _, ok := n.body.(keyArgBody); !ok {
			return msType{}, false
		}
		k := msKey(n.tag, ctx)
		if !msGlobalOK(k, n.tag, ctx) {
			return msType{}, false
		}
		t = msCheck(k)
		return t, msGlobalOK(t, tagCheck, ctx)
	case tagCheck:
		c, ok := soleChild(n)
		if !ok {
			return msType{}, false
		}
		// Check-idempotence: Check over a BARE key denotes the same fragment.
		if c.tag == tagPkK || c.tag == tagPkH {
			if _, ok := c.body.(keyArgBody); ok {
				return msNode(c, ctx)
			}
		}
		sub, ok := msNode(c, ctx)
		if !ok || sub.corr.base != msK {
			return msType{}, false
		}
		t = msCheck(sub)
	case tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual:
		c, ok := soleChild(n)
		if !ok {
			return msType{}, false
		}
		sub, ok := msNode(c, ctx)
		if !ok {
			return msType{}, false
		}
		if t, ok = msWrap(n.tag, sub); !ok {
			return msType{}, false
		}
	case tagAndV, tagAndB, tagOrB, tagOrC, tagOrD, tagOrI:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return msType{}, false
		}
		l, ok := msNode(b.children[0], ctx)
		if !ok {
			return msType{}, false
		}
		r, ok := msNode(b.children[1], ctx)
		if !ok {
			return msType{}, false
		}
		if t, ok = msBinary(n.tag, l, r); !ok {
			return msType{}, false
		}
	case tagAndOr:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 3 {
			return msType{}, false
		}
		var s [3]msType
		for i := range s {
			if s[i], ok = msNode(b.children[i], ctx); !ok {
				return msType{}, false
			}
		}
		if t, ok = msAndOr(s[0], s[1], s[2]); !ok {
			return msType{}, false
		}
	case tagThresh:
		b, ok := n.body.(variableBody)
		if !ok {
			return msType{}, false
		}
		subs := make([]msType, len(b.children))
		for i, c := range b.children {
			if subs[i], ok = msNode(c, ctx); !ok {
				return msType{}, false
			}
		}
		if !msThresholdOK(int(b.k), len(subs), 0) {
			return msType{}, false
		}
		if t, ok = msThresh(int(b.k), subs); !ok {
			return msType{}, false
		}
	case tagMulti, tagMultiA:
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return msType{}, false
		}
		k, nk := int(b.k), len(b.indices)
		if n.tag == tagMulti {
			if !msThresholdOK(k, nk, msMaxPubkeysPerMultisig) {
				return msType{}, false
			}
			t = msType{
				corr: msCorr{base: msB, input: msAnyNonZero, dissat: true, unit: true},
				ext:  msExt{pkCost: msMultiNumCost(k, nk) + 34*nk + 1, freeVerify: true},
			}
		} else {
			if !msThresholdOK(k, nk, msMaxPubkeysInChecksigadd) {
				return msType{}, false
			}
			t = msLeafMultiA(k, nk)
		}
	case tagAfter, tagOlder:
		v, ok := n.body.(timelockBody)
		if !ok || !msTimelockOK(uint32(v)) {
			return msType{}, false
		}
		t = msType{
			corr: msCorr{base: msB, input: msZero, dissat: false, unit: false},
			ext:  msExt{pkCost: msScriptNumSize(int(v)) + 1},
		}
	case tagSha256, tagHash256:
		if _, ok := n.body.(hash256Body); !ok {
			return msType{}, false
		}
		t = msHash(33 + 6)
	case tagRipemd160, tagHash160:
		if _, ok := n.body.(hash160Body); !ok {
			return msType{}, false
		}
		t = msHash(21 + 6)
	case tagFalse, tagTrue:
		if _, ok := n.body.(emptyBody); !ok {
			return msType{}, false
		}
		// TRUE is not d; FALSE is. Both B, Zero, u, pk_cost 1.
		t = msType{
			corr: msCorr{base: msB, input: msZero, dissat: n.tag == tagFalse, unit: true},
			ext:  msExt{pkCost: 1},
		}
	default:
		// sortedmulti/sortedmulti_a below their one legal position, raw_pkh,
		// a TapTree node, a top-level wrapper, or an unknown tag.
		return msType{}, false
	}
	return t, msGlobalOK(t, n.tag, ctx)
}

// msKey is pk_k / pk_h (type K) in ctx.
func msKey(tg tag, ctx msCtx) msType {
	if tg == tagPkH {
		return msType{
			corr: msCorr{base: msK, input: msAnyNonZero, dissat: true, unit: true},
			ext:  msExt{pkCost: 24},
		}
	}
	cost := 34 // Ecdsa, compressed (md's keys are xpubs)
	if ctx == ctxTap {
		cost = 33 // Schnorr
	}
	return msType{
		corr: msCorr{base: msK, input: msOneNonZero, dissat: true, unit: true},
		ext:  msExt{pkCost: cost},
	}
}

func msHash(cost int) msType {
	return msType{
		corr: msCorr{base: msB, input: msOneNonZero, dissat: true, unit: true},
		ext:  msExt{pkCost: cost, freeVerify: true},
	}
}

// msCheck is c: (cast_check); the caller has checked the child is K.
func msCheck(s msType) msType {
	return msType{
		corr: msCorr{base: msB, input: s.corr.input, dissat: s.corr.dissat, unit: true},
		ext:  msExt{pkCost: s.ext.pkCost + 1, freeVerify: true, height: s.ext.height + 1},
	}
}

// msWrap is cast_verify, cast_swap, cast_alt, cast_dupif, cast_nonzero and
// cast_zeronotequal (correctness.rs and extra_props.rs).
func msWrap(tg tag, s msType) (msType, bool) {
	c := s.corr
	h := s.ext.height + 1
	switch tg {
	case tagAlt:
		if c.base != msB {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msW, input: msAny, dissat: c.dissat, unit: c.unit},
			ext:  msExt{pkCost: s.ext.pkCost + 2, height: h},
		}, true
	case tagSwap:
		if c.base != msB || (c.input != msOne && c.input != msOneNonZero) {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msW, input: msAny, dissat: c.dissat, unit: c.unit},
			ext:  msExt{pkCost: s.ext.pkCost + 1, freeVerify: s.ext.freeVerify, height: h},
		}, true
	case tagDupIf:
		if c.base != msV || c.input != msZero {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: msOneNonZero, dissat: true, unit: false},
			ext:  msExt{pkCost: s.ext.pkCost + 3, height: h},
		}, true
	case tagVerify:
		if c.base != msB {
			return msType{}, false
		}
		cost := s.ext.pkCost
		if !s.ext.freeVerify {
			cost++
		}
		return msType{
			corr: msCorr{base: msV, input: c.input, dissat: false, unit: false},
			ext:  msExt{pkCost: cost, height: h},
		}, true
	case tagNonZero:
		if c.input != msOneNonZero && c.input != msAnyNonZero {
			return msType{}, false
		}
		if c.base != msB {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: c.input, dissat: true, unit: c.unit},
			ext:  msExt{pkCost: s.ext.pkCost + 4, height: h},
		}, true
	case tagZeroNotEqual:
		if c.base != msB {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: c.input, dissat: c.dissat, unit: true},
			ext:  msExt{pkCost: s.ext.pkCost + 1, height: h},
		}, true
	}
	return msType{}, false
}

// msAndInput is the input rule shared by and_b and and_v.
func msAndInput(l, r msInput) msInput {
	switch {
	case l == msZero && r == msZero:
		return msZero
	case (l == msZero && r == msOne) || (l == msOne && r == msZero):
		return msOne
	case (l == msZero && r == msOneNonZero) || (l == msOneNonZero && r == msZero):
		return msOneNonZero
	case l == msOneNonZero || l == msAnyNonZero || (l == msZero && r == msAnyNonZero):
		return msAnyNonZero
	}
	return msAny
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// msBinary is and_v, and_b, or_b, or_c, or_d and or_i.
func msBinary(tg tag, l, r msType) (msType, bool) {
	lc, rc := l.corr, r.corr
	h := 1 + maxInt(l.ext.height, r.ext.height)
	cost := l.ext.pkCost + r.ext.pkCost
	switch tg {
	case tagAndB:
		if lc.base != msB || rc.base != msW {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: msAndInput(lc.input, rc.input), dissat: lc.dissat && rc.dissat, unit: true},
			ext:  msExt{pkCost: cost + 1, height: h},
		}, true
	case tagAndV:
		var base msBase
		switch {
		case lc.base == msV && rc.base == msB:
			base = msB
		case lc.base == msV && rc.base == msK:
			base = msK
		case lc.base == msV && rc.base == msV:
			base = msV
		default:
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: base, input: msAndInput(lc.input, rc.input), dissat: false, unit: rc.unit},
			ext:  msExt{pkCost: cost, freeVerify: r.ext.freeVerify, height: h},
		}, true
	case tagOrB:
		if !lc.dissat || !rc.dissat {
			return msType{}, false
		}
		if lc.base != msB || rc.base != msW {
			return msType{}, false
		}
		in := msAny
		switch {
		case lc.input == msZero && rc.input == msZero:
			in = msZero
		case (lc.input == msZero && rc.input == msOne) || (lc.input == msOne && rc.input == msZero) ||
			(lc.input == msZero && rc.input == msOneNonZero) || (lc.input == msOneNonZero && rc.input == msZero):
			in = msOne
		}
		return msType{
			corr: msCorr{base: msB, input: in, dissat: true, unit: true},
			ext:  msExt{pkCost: cost + 1, height: h},
		}, true
	case tagOrD, tagOrC:
		if !lc.dissat || !lc.unit {
			return msType{}, false
		}
		in := msAny
		switch {
		case lc.input == msZero && rc.input == msZero:
			in = msZero
		case (lc.input == msOne || lc.input == msOneNonZero) && rc.input == msZero:
			in = msOne
		}
		if tg == tagOrD {
			if lc.base != msB || rc.base != msB {
				return msType{}, false
			}
			return msType{
				corr: msCorr{base: msB, input: in, dissat: rc.dissat, unit: rc.unit},
				ext:  msExt{pkCost: cost + 3, height: h},
			}, true
		}
		if lc.base != msB || rc.base != msV {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msV, input: in, dissat: false, unit: false},
			ext:  msExt{pkCost: cost + 2, height: h},
		}, true
	case tagOrI:
		if lc.base != rc.base || lc.base == msW {
			return msType{}, false
		}
		in := msAny
		if lc.input == msZero && rc.input == msZero {
			in = msOne
		}
		return msType{
			corr: msCorr{base: lc.base, input: in, dissat: lc.dissat || rc.dissat, unit: lc.unit && rc.unit},
			ext:  msExt{pkCost: cost + 3, height: h},
		}, true
	}
	return msType{}, false
}

// msAndOr is and_or (andor(X,Y,Z)).
func msAndOr(a, b, c msType) (msType, bool) {
	ac, bc, cc := a.corr, b.corr, c.corr
	if !ac.dissat || !ac.unit {
		return msType{}, false
	}
	if ac.base != msB || bc.base != cc.base || bc.base == msW {
		return msType{}, false
	}
	in := msAny
	oneish := func(i msInput) bool { return i == msOne || i == msOneNonZero }
	switch {
	case ac.input == msZero && bc.input == msZero && cc.input == msZero:
		in = msZero
	case ac.input == msZero && oneish(bc.input) && oneish(cc.input),
		oneish(ac.input) && bc.input == msZero && cc.input == msZero:
		in = msOne
	}
	return msType{
		corr: msCorr{base: bc.base, input: in, dissat: cc.dissat, unit: bc.unit && cc.unit},
		ext: msExt{
			pkCost: a.ext.pkCost + b.ext.pkCost + c.ext.pkCost + 3,
			height: 1 + maxInt(a.ext.height, maxInt(b.ext.height, c.ext.height)),
		},
	}, true
}

// msThresh is Correctness::threshold and ExtData::threshold.
func msThresh(k int, subs []msType) (msType, bool) {
	args := 0
	cost := 1 + msScriptNumSize(k)
	h := 0
	for i, s := range subs {
		switch s.corr.input {
		case msOne, msOneNonZero:
			args++
		case msAny, msAnyNonZero:
			args += 2
		}
		if i == 0 && s.corr.base != msB {
			return msType{}, false
		}
		if i != 0 && s.corr.base != msW {
			return msType{}, false
		}
		if !s.corr.unit || !s.corr.dissat {
			return msType{}, false
		}
		cost += s.ext.pkCost
		h = maxInt(h, s.ext.height)
	}
	in := msAny
	switch args {
	case 0:
		in = msZero
	case 1:
		in = msOne
	}
	return msType{
		corr: msCorr{base: msB, input: in, dissat: true, unit: true},
		ext:  msExt{pkCost: cost + len(subs) - 1, freeVerify: true, height: h + 1},
	}, true
}
