package md

// THE TYPE RULE of the policy summary (md review r2 I-1 and r3; port of
// md-codec 0.49.0 `policy_shape::policy_type_checks`, descriptor-mnemonic
// f8b4a675 and 5ad5a9b2, whose doc lists the six rule groups this file
// implements).
//
// policy_shape's walk vouches for a thresh node's own children (the
// allowlist); it does not vouch for the CONTEXT around a node. Untyped trees
// decode -- the decoder does not type-check -- and some of them spend in ways
// no summary of their fragments describes: wsh(or_b(pk(@0),thresh(2,...)))
// spends with NO signature, because the thresh is B where or_b needs a W and
// its first CHECKSIG consumes the 0 that pk(@0) left. So a tree that is not
// valid miniscript in its script context has no summary at all.
//
// Rust answers this by running md's own descriptor conversion over synthetic
// xpub keys (`to_miniscript::tree_type_checks`: `node_to_descriptor`, then
// `top_level_type_check` and `within_resource_limits` on every script root).
// This port has no rust-miniscript, so it re-implements exactly those checks,
// quoted from rust-miniscript at the pin descriptor-mnemonic vendors
// (ff4732e). By the doc's six groups:
//
//  1. Types: miniscript/types/correctness.rs (Base B/K/V/W, Input, "d", "u").
//     The malleability half of Type::type_check (types/malleability.rs) never
//     errors, so it is not ported.
//  2. Script roots are B: wsh, sh(wsh), sh and EVERY tap leaf
//     (ScriptContext::top_level_type_check).
//  3. Timelock ranges: older/after need 1 <= v < 2^31 (AbsLockTime and
//     RelLockTime::from_consensus).
//  4. Fragment placement (to_miniscript.rs node_to_descriptor /
//     node_to_miniscript and context.rs check_global_consensus_validity):
//     multi only outside Tap, multi_a only in Tap; sortedmulti only as the
//     sole child of wsh/sh/sh(wsh), sortedmulti_a only as a whole tap leaf;
//     raw_pkh, a TapTree or a descriptor wrapper (pkh, wpkh, ...) inside
//     miniscript refused. Bare PkK/PkH and Check over a bare key both mean
//     c:pk_k / c:pk_h. thresh/multi/multi_a need 1 <= k <= n (multi n <= 20,
//     multi_a n <= 999); TapTree::combine refuses a leaf deeper than 128.
//  5. Script limits: Miniscript::from_ast at EVERY node runs the recursion
//     depth (402) and check_global_validity on ExtData::pk_cost -- Legacy 520,
//     Segwitv0 10000 and 3600, Tap the block weight. A sortedmulti root is
//     built unchecked, then checked at the root by group 6's call.
//  6. Satisfaction limits: Miniscript::within_resource_limits, i.e.
//     Ctx::check_local_validity at each script root, over ExtData's
//     static_ops / sat_data / dissat_data (types/extra_props.rs): some
//     satisfaction exists and at most 201 ops run (wsh/sh), at most 100
//     witness items (wsh) or a 1650-byte scriptSig (sh); a tap leaf needs a
//     satisfaction (md-codec's own per-leaf check, md review r4) within the
//     1000-element stack. ExtData::threshold's sort-and-fold is ported as
//     written: it satisfies min(k+1, n) children (`i <= k`), so a thresh with
//     a never-satisfiable child is never satisfiable when those include it.
//
// ONE DIFFERENCE BY CONSTRUCTION: Rust's lookup_key also refuses a placeholder
// index >= n. This walk sees only the tree, so it cannot; every tree it sees
// in production comes from Reassemble, which already refuses such an index
// (validatePlaceholderUsage), so on a decoded card the two agree.
//
// Display and identity only: decode, ids, addresses and re-encoding never
// consult it.

import "sort"

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

// msSat is rust-miniscript's SatData.
type msSat struct {
	wsSize    int // max_witness_stack_size
	wsCount   int // max_witness_stack_count
	sigSize   int // max_script_sig_size
	execStack int // max_exec_stack_count
	execOps   int // max_exec_op_count
}

// msOpt is Option<SatData>: ok=false is None (no such (dis)satisfaction).
type msOpt struct {
	ok bool
	d  msSat
}

func some(s msSat) msOpt { return msOpt{ok: true, d: s} }

// msExt is the part of rust-miniscript's ExtData the checks read.
type msExt struct {
	pkCost     int
	freeVerify bool // has_free_verify: a v: over it costs no extra opcode
	height     int  // tree_height
	staticOps  int
	sat        msOpt
	dissat     msOpt
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
	msMaxOpsPerScript         = 201  // limits.rs MAX_OPS_PER_SCRIPT
	msMaxStandardP2WSHItems   = 100  // limits.rs MAX_STANDARD_P2WSH_STACK_ITEMS
	msMaxScriptSigSize        = 1650 // limits.rs MAX_SCRIPTSIG_SIZE
	msMaxStackSize            = 1000 // limits.rs MAX_STACK_SIZE
)

// treeTypeChecks reports whether tree converts to valid miniscript in its own
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
				return msSortedMultiRootOK(b, ctxLegacy)
			}
		}
		// sh_inner_to_descriptor's `_` arm: everything else (including a
		// malformed wsh/wpkh/sortedmulti body) is a Legacy miniscript.
		t, ok := msNode(inner, ctxLegacy)
		return ok && msScriptOK(t, ctxLegacy)
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

// msSortedMultiRootOK is Descriptor::new_*_sortedmulti (threshold only) and
// then tree_type_checks' script_ok on the unchecked Miniscript::sortedmulti,
// whose check_local_validity also runs the global checks.
func msSortedMultiRootOK(b multiKeysBody, ctx msCtx) bool {
	k, n := int(b.k), len(b.indices)
	if !msThresholdOK(k, n, msMaxPubkeysPerMultisig) {
		return false
	}
	t := msMultiType(k, n)
	return msGlobalOK(t, tagSortedMulti, ctx) && msScriptOK(t, ctx)
}

// wshInnerTypeChecks is wsh_inner_to_descriptor plus script_ok: a sole
// sortedmulti, or a Segwitv0 miniscript.
func wshInnerTypeChecks(inner node) bool {
	if inner.tag == tagSortedMulti {
		if b, ok := inner.body.(multiKeysBody); ok {
			return msSortedMultiRootOK(b, ctxSegwitv0)
		}
	}
	t, ok := msNode(inner, ctxSegwitv0)
	return ok && msScriptOK(t, ctxSegwitv0)
}

// msScriptOK is tree_type_checks' script_ok on a script root that from_ast
// already admitted: top_level_type_check (B) and within_resource_limits
// (Ctx::check_local_validity; its global half already held at from_ast, or
// is run by the caller for an unchecked sortedmulti).
func msScriptOK(t msType, ctx msCtx) bool {
	if t.corr.base != msB {
		return false
	}
	sat := t.ext.sat
	switch ctx {
	case ctxLegacy, ctxSegwitv0:
		// check_local_consensus_validity: sat_op_count.
		if !sat.ok || t.ext.staticOps+sat.d.execOps > msMaxOpsPerScript {
			return false
		}
		if ctx == ctxLegacy {
			// check_local_policy_validity: max_satisfaction_size.
			return sat.d.sigSize <= msMaxScriptSigSize
		}
		// max_satisfaction_witness_elements = count + 1 (the script).
		return sat.d.wsCount+1 <= msMaxStandardP2WSHItems
	case ctxTap:
		// Tap's local checks read sat_data only when it is Some; md-codec
		// adds `Tap::max_satisfaction_size(ms).is_some()` per leaf (md review
		// r4), so a never-satisfiable leaf refuses the whole tr.
		return sat.ok && sat.d.wsCount+sat.d.execStack <= msMaxStackSize
	}
	return false
}

// tapTreeTypeChecks is tree_to_taptree plus script_ok per leaf. depth counts
// the TapTree nodes above n.
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
		t := msLeafMultiA(int(b.k), len(b.indices))
		return msGlobalOK(t, tagSortedMultiA, ctxTap) && msScriptOK(t, ctxTap)
	}
	t, ok := msNode(n, ctxTap)
	return ok && msScriptOK(t, ctxTap)
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

// msMultiType is multi / sortedmulti: Correctness::multi and ExtData::multi
// (compressed keys: md's keys are xpubs).
func msMultiType(k, n int) msType {
	return msType{
		corr: msCorr{base: msB, input: msAnyNonZero, dissat: true, unit: true},
		ext: msExt{
			pkCost: msMultiNumCost(k, n) + 34*n + 1, freeVerify: true, staticOps: 1,
			sat:    some(msSat{wsSize: 1 + 73*k, wsCount: k + 1, sigSize: 1 + 73*k, execStack: n, execOps: n}),
			dissat: some(msSat{wsSize: 1 + k, wsCount: k + 1, sigSize: 1 + k, execStack: n, execOps: n}),
		},
	}
}

// msLeafMultiA is multi_a / sortedmulti_a.
func msLeafMultiA(k, n int) msType {
	return msType{
		corr: msCorr{base: msB, input: msAny, dissat: true, unit: true},
		ext: msExt{
			pkCost: msMultiNumCost(k, n) + 33*n + (n - 1) + 1, freeVerify: true,
			sat:    some(msSat{wsSize: (n - k) + 66*k, wsCount: n, execStack: 2}),
			dissat: some(msSat{wsSize: n, wsCount: n, execStack: 2}),
		},
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
			t = msMultiType(k, nk)
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
			ext: msExt{
				pkCost: msScriptNumSize(int(v)) + 1, staticOps: 1,
				sat: some(msSat{execStack: 1}),
			},
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
		// TRUE is not d; FALSE is. Both B, Zero, u, pk_cost 1. TRUE has only a
		// satisfaction, FALSE only a dissatisfaction.
		e := some(msSat{execStack: 1})
		t = msType{
			corr: msCorr{base: msB, input: msZero, dissat: n.tag == tagFalse, unit: true},
			ext:  msExt{pkCost: 1},
		}
		if n.tag == tagTrue {
			t.ext.sat = e
		} else {
			t.ext.dissat = e
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
	keyBytes, sigBytes := 34, 73 // Ecdsa, compressed (md's keys are xpubs)
	if ctx == ctxTap {
		keyBytes, sigBytes = 33, 66 // Schnorr
	}
	if tg == tagPkH {
		return msType{
			corr: msCorr{base: msK, input: msAnyNonZero, dissat: true, unit: true},
			ext: msExt{
				pkCost: 24, staticOps: 3,
				sat:    some(msSat{wsSize: keyBytes + sigBytes, wsCount: 2, sigSize: keyBytes + sigBytes, execStack: 2}),
				dissat: some(msSat{wsSize: keyBytes + 1, wsCount: 2, sigSize: keyBytes + 1, execStack: 2}),
			},
		}
	}
	return msType{
		corr: msCorr{base: msK, input: msOneNonZero, dissat: true, unit: true},
		ext: msExt{
			pkCost: keyBytes,
			sat:    some(msSat{wsSize: sigBytes, wsCount: 1, sigSize: sigBytes, execStack: 1}),
			dissat: some(msSat{wsSize: 1, wsCount: 1, sigSize: 1, execStack: 1}),
		},
	}
}

// msHash is sha256/hash256/ripemd160/hash160 (their SatData are identical).
func msHash(cost int) msType {
	return msType{
		corr: msCorr{base: msB, input: msOneNonZero, dissat: true, unit: true},
		ext: msExt{
			pkCost: cost, freeVerify: true, staticOps: 4,
			sat:    some(msSat{wsSize: 33, wsCount: 1, sigSize: 33, execStack: 2}),
			dissat: some(msSat{wsSize: 33, wsCount: 2, sigSize: 33, execStack: 2}),
		},
	}
}

// msCheck is c: (cast_check); the caller has checked the child is K.
func msCheck(s msType) msType {
	return msType{
		corr: msCorr{base: msB, input: s.corr.input, dissat: s.corr.dissat, unit: true},
		ext: msExt{
			pkCost: s.ext.pkCost + 1, freeVerify: true, height: s.ext.height + 1,
			staticOps: s.ext.staticOps + 1, sat: s.ext.sat, dissat: s.ext.dissat,
		},
	}
}

// msWrap is cast_verify, cast_swap, cast_alt, cast_dupif, cast_nonzero and
// cast_zeronotequal (correctness.rs and extra_props.rs).
func msWrap(tg tag, s msType) (msType, bool) {
	c := s.corr
	e := s.ext
	h := e.height + 1
	switch tg {
	case tagAlt:
		if c.base != msB {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msW, input: msAny, dissat: c.dissat, unit: c.unit},
			ext:  msExt{pkCost: e.pkCost + 2, height: h, staticOps: e.staticOps + 2, sat: e.sat, dissat: e.dissat},
		}, true
	case tagSwap:
		if c.base != msB || (c.input != msOne && c.input != msOneNonZero) {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msW, input: msAny, dissat: c.dissat, unit: c.unit},
			ext:  msExt{pkCost: e.pkCost + 1, freeVerify: e.freeVerify, height: h, staticOps: e.staticOps + 1, sat: e.sat, dissat: e.dissat},
		}, true
	case tagDupIf:
		if c.base != msV || c.input != msZero {
			return msType{}, false
		}
		sat := e.sat
		if sat.ok {
			sat.d.wsSize++
			sat.d.wsCount += 2
			sat.d.sigSize++
			sat.d.execStack = max(1, sat.d.execStack)
		}
		return msType{
			corr: msCorr{base: msB, input: msOneNonZero, dissat: true, unit: false},
			ext: msExt{
				pkCost: e.pkCost + 3, height: h, staticOps: e.staticOps + 3, sat: sat,
				dissat: some(msSat{wsSize: 1, wsCount: 1, sigSize: 1, execStack: 1}),
			},
		}, true
	case tagVerify:
		if c.base != msB {
			return msType{}, false
		}
		cost := 0
		if !e.freeVerify {
			cost = 1
		}
		return msType{
			corr: msCorr{base: msV, input: c.input, dissat: false, unit: false},
			ext:  msExt{pkCost: e.pkCost + cost, height: h, staticOps: e.staticOps + cost, sat: e.sat},
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
			ext: msExt{
				pkCost: e.pkCost + 4, height: h, staticOps: e.staticOps + 4, sat: e.sat,
				dissat: some(msSat{wsSize: 1, wsCount: 1, sigSize: 1, execStack: 1}),
			},
		}, true
	case tagZeroNotEqual:
		if c.base != msB {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: c.input, dissat: c.dissat, unit: true},
			ext:  msExt{pkCost: e.pkCost + 1, height: h, staticOps: e.staticOps + 1, sat: e.sat, dissat: e.dissat},
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

// msConcat is the sat_concat closures of extra_props.rs: both parts run in
// sequence. rightOnTop adds 1 to the right part's exec stack (and_b, or_b:
// the left result sits on the stack while the right runs).
func msConcat(l, r msOpt, rightOnTop bool) msOpt {
	if !l.ok || !r.ok {
		return msOpt{}
	}
	rs := r.d.execStack
	if rightOnTop {
		rs++
	}
	return some(msSat{
		wsSize:    l.d.wsSize + r.d.wsSize,
		wsCount:   l.d.wsCount + r.d.wsCount,
		sigSize:   l.d.sigSize + r.d.sigSize,
		execStack: max(l.d.execStack, rs),
		execOps:   l.d.execOps + r.d.execOps,
	})
}

// msFieldwiseMax is SatData::fieldwise_max_opt.
func msFieldwiseMax(a, b msOpt) msOpt {
	switch {
	case !a.ok:
		return b
	case !b.ok:
		return a
	}
	return some(msSat{
		wsSize:    max(a.d.wsSize, b.d.wsSize),
		wsCount:   max(a.d.wsCount, b.d.wsCount),
		sigSize:   max(a.d.sigSize, b.d.sigSize),
		execStack: max(a.d.execStack, b.d.execStack),
		execOps:   max(a.d.execOps, b.d.execOps),
	})
}

// msOrIArm is or_i's with_1 (left arm, wsSize +2) and with_0 (right arm, +1).
func msOrIArm(o msOpt, left bool) msOpt {
	if !o.ok {
		return o
	}
	o.d.wsCount++
	o.d.sigSize++
	if left {
		o.d.wsSize += 2
	} else {
		o.d.wsSize++
	}
	return o
}

// msBinary is and_v, and_b, or_b, or_c, or_d and or_i.
func msBinary(tg tag, l, r msType) (msType, bool) {
	lc, rc := l.corr, r.corr
	le, re := l.ext, r.ext
	h := 1 + max(le.height, re.height)
	cost := le.pkCost + re.pkCost
	ops := le.staticOps + re.staticOps
	switch tg {
	case tagAndB:
		if lc.base != msB || rc.base != msW {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msB, input: msAndInput(lc.input, rc.input), dissat: lc.dissat && rc.dissat, unit: true},
			ext: msExt{
				pkCost: cost + 1, height: h, staticOps: ops + 1,
				sat:    msConcat(le.sat, re.sat, true),
				dissat: msConcat(le.dissat, re.dissat, true),
			},
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
			ext: msExt{
				pkCost: cost, freeVerify: re.freeVerify, height: h, staticOps: ops,
				sat: msConcat(le.sat, re.sat, false),
			},
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
			ext: msExt{
				pkCost: cost + 1, height: h, staticOps: ops + 1,
				sat:    msFieldwiseMax(msConcat(le.sat, re.dissat, true), msConcat(le.dissat, re.sat, true)),
				dissat: msConcat(le.dissat, re.dissat, true),
			},
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
		sat := msFieldwiseMax(le.sat, msConcat(le.dissat, re.sat, false))
		if tg == tagOrD {
			if lc.base != msB || rc.base != msB {
				return msType{}, false
			}
			return msType{
				corr: msCorr{base: msB, input: in, dissat: rc.dissat, unit: rc.unit},
				ext: msExt{
					pkCost: cost + 3, height: h, staticOps: ops + 3,
					sat: sat, dissat: msConcat(le.dissat, re.dissat, false),
				},
			}, true
		}
		if lc.base != msB || rc.base != msV {
			return msType{}, false
		}
		return msType{
			corr: msCorr{base: msV, input: in, dissat: false, unit: false},
			ext:  msExt{pkCost: cost + 2, height: h, staticOps: ops + 2, sat: sat},
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
			ext: msExt{
				pkCost: cost + 3, height: h, staticOps: ops + 3,
				sat:    msFieldwiseMax(msOrIArm(le.sat, true), msOrIArm(re.sat, false)),
				dissat: msFieldwiseMax(msOrIArm(le.dissat, true), msOrIArm(re.dissat, false)),
			},
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
	ae, be, ce := a.ext, b.ext, c.ext
	return msType{
		corr: msCorr{base: bc.base, input: in, dissat: cc.dissat, unit: bc.unit && cc.unit},
		ext: msExt{
			pkCost:    ae.pkCost + be.pkCost + ce.pkCost + 3,
			height:    1 + max(ae.height, be.height, ce.height),
			staticOps: 3 + ae.staticOps + be.staticOps + ce.staticOps,
			sat:       msFieldwiseMax(msConcat(ae.sat, be.sat, false), msConcat(ae.dissat, ce.sat, false)),
			dissat:    msConcat(ae.dissat, ce.dissat, false),
		},
	}, true
}

// msThresh is Correctness::threshold and ExtData::threshold.
func msThresh(k int, subs []msType) (msType, bool) {
	args := 0
	cost := 1 + msScriptNumSize(k)
	h := 0
	ops := 0
	dissat := some(msSat{})
	type pair struct{ sat, dissat msOpt }
	pairs := make([]pair, len(subs))
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
		h = max(h, s.ext.height)
		ops += s.ext.staticOps
		// The thresh dissatisfies by dissatisfying every child in order.
		dissat = msConcat(dissat, s.ext.dissat, false)
		pairs[i] = pair{s.ext.sat, s.ext.dissat}
	}
	in := msAny
	switch args {
	case 0:
		in = msZero
	case 1:
		in = msOne
	}
	// ExtData::threshold's satisfaction, one field at a time, AS WRITTEN:
	// stable-sort the (sat, dissat) pairs by sat - dissat (None, when either
	// is missing, sorts first), then walk them from the costliest end and take
	// the satisfaction for the first k+1 (`i <= k`), the dissatisfaction for
	// the rest; any missing one makes the whole field None. The vector stays
	// in its last order between fields, as Rust's does.
	fields := []struct {
		proj func(msSat) int
		acc  func(acc, x int) int
		set  func(*msSat, int)
	}{
		{func(d msSat) int { return d.wsCount }, func(a, x int) int { return a + x }, func(d *msSat, v int) { d.wsCount = v }},
		{func(d msSat) int { return d.wsSize }, func(a, x int) int { return a + x }, func(d *msSat, v int) { d.wsSize = v }},
		{func(d msSat) int { return d.sigSize }, func(a, x int) int { return a + x }, func(d *msSat, v int) { d.sigSize = v }},
		{func(d msSat) int { return d.execStack }, func(a, x int) int {
			if a > 0 {
				x++
			}
			return max(a, x)
		}, func(d *msSat, v int) { d.execStack = v }},
		{func(d msSat) int { return d.execOps }, func(a, x int) int { return a + x }, func(d *msSat, v int) { d.execOps = v }},
	}
	sat := some(msSat{})
	for _, f := range fields {
		less := func(i, j int) bool {
			a, b := pairs[i], pairs[j]
			aok, bok := a.sat.ok && a.dissat.ok, b.sat.ok && b.dissat.ok
			if !aok || !bok {
				return !aok && bok // None < Some
			}
			return f.proj(a.sat.d)-f.proj(a.dissat.d) < f.proj(b.sat.d)-f.proj(b.dissat.d)
		}
		sort.SliceStable(pairs, less)
		acc, ok := 0, true
		for i := 0; i < len(pairs) && ok; i++ {
			p := pairs[len(pairs)-1-i]
			o := p.dissat
			if i <= k {
				o = p.sat
			}
			if !o.ok {
				ok = false
				break
			}
			acc = f.acc(acc, f.proj(o.d))
		}
		if !ok {
			sat = msOpt{}
		} else if sat.ok {
			f.set(&sat.d, acc)
		}
	}
	return msType{
		corr: msCorr{base: msB, input: in, dissat: true, unit: true},
		ext: msExt{
			pkCost: cost + len(subs) - 1, freeVerify: true, height: h + 1,
			staticOps: ops + 1 + (len(subs) - 1), sat: sat, dissat: dissat,
		},
	}, true
}
