package md

// ─── StripToTemplate (port of synthesize_template_descriptor, synthesize.rs:
// 1180-1212) ─────────────────────────────────────────────────────────────────
//
// StripToTemplate converts a DEVICE-BUILT full keyed wallet-policy md1 into the
// keyless template md1 by applying the toolkit's mutations on a decoded clone
// and re-emitting via split(). It is golden-locked byte-for-byte to
// `toolkit bundle --md1-form=template`.
//
// Mutations (synthesize.rs:1182-1198):
//  1. tlv.pubkeys     = None  → pubkeys = nil, pubPresent = false
//  2. tlv.fingerprints = None → fingerprints = nil, fpPresent = false (C1: BOTH
//     the slice and the present-flag must clear, else writeTLVSection trips
//     errEmptyTLVEncode on a present-but-empty section, encode.go:271-273)
//  3. CONDITIONAL origin elision: only when canonicalOrigin(tree) is present do
//     we elide the path-decl to a shared-empty path (the canonical wrapper
//     re-derives it on decode); otherwise KEEP the source origins verbatim — a
//     non-canonical wrapper (general policy) needs them on the wire or decode
//     rejects with errMissingExplicitOrigin (the C1 regression). The header
//     divergent bit is recomputed on re-emit from the path-decl shape.
//
// The supply path engraves a supplied template VERBATIM and does NOT call this
// — StripToTemplate is the DEVICE-BUILT leg only.
func StripToTemplate(md1Chunks []string) ([]string, error) {
	d, err := Reassemble(md1Chunks)
	if err != nil {
		return nil, err
	}

	// Mutation 1 — null the pubkeys (keyless).
	d.tlv.pubkeys = nil
	d.tlv.pubPresent = false

	// Mutation 2 — null the fingerprints (C1: clear the present-flag too).
	d.tlv.fingerprints = nil
	d.tlv.fpPresent = false

	// Mutation 3 — C1-conditional origin elision. The verdict is a whole-tree
	// property (the wrapper shape), so it governs every @N at once.
	if _, ok := canonicalOrigin(d.tree); ok {
		empty := originPath{}
		d.pathDecl.shared = &empty
		d.pathDecl.divergent = nil // header divergent bit recomputes on re-emit
	}
	// else: keep d.pathDecl as the source origins (non-canonical wrapper).

	// Re-emit the keyless template via the shape-general split() (encodePayload-
	// backed) — the same chunker the encoders use.
	return split(d)
}

// TapTreeDepthChunks reports the taproot script-tree branch DEPTH of a (chunked)
// md1, counting nested TapTree branch nodes along the deepest path:
//   - 0 = not taproot, a key-path-only tr (no tree), OR a single-leaf tree
//   - 1 = a single {A,B} branch (one branch level)
//   - ≥2 = a NESTED taptree (e.g. {{A,B},C}) — the DD6 EXPERIMENTAL gate
//
// The fork's wire codec encodes any depth (no taptree-depth refusal); the
// experimental framing is about off-device recovery via shipped rust-miniscript
// (PR #953), not the on-device encode.
func TapTreeDepthChunks(strs []string) (int, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return 0, err
	}
	if d.tree.tag != tagTr {
		return 0, nil
	}
	b, ok := d.tree.body.(trBody)
	if !ok || b.tree == nil {
		return 0, nil // key-path-only taproot
	}
	return tapBranchDepth(*b.tree), nil
}

// tapBranchDepth returns the nesting depth of tagTapTree BRANCH nodes: a script
// leaf = 0, a single {A,B} branch = 1, a nested {{A,B},C} = 2. The tr root that
// has any script tree is at least depth 1 (a bare-leaf tree).
func tapBranchDepth(n node) int {
	if n.tag != tagTapTree {
		return 0 // a script leaf contributes no branch level
	}
	cb, ok := n.body.(childrenBody)
	if !ok {
		return 0
	}
	deepest := 0
	for _, c := range cb.children {
		if dch := tapBranchDepth(c); dch > deepest {
			deepest = dch
		}
	}
	return deepest + 1
}
