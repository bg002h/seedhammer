package gui

import (
	"encoding/hex"
	"fmt"

	"seedhammer.com/md"
)

// The composer's consent surface (SPEC §7e).
//
// IT IS A NEW SURFACE, and neither shipped one would do. walletPolicyConsentLines
// summarises through md1Summary, which prints "Complex policy - cannot display
// safely." for every shape the codec marks non-renderable -- measured for
// every multi-path or taproot shape this composer exists to author
// (md/md_test.go:337,416). And policySummaryLines, the one structural summary
// that exists, counts a multi-path wsh script as ONE branch
// (md/policy_shape.go:41-43). Neither describes what the operator built.
//
// EVERY LINE IS DERIVED FROM THE DECODED md1, never from composerState. That
// is not a preference: §8q's self-check compares the decoded shape against
// what the operator composed, and a surface that read the builder's INPUT
// would print a builder defect back as agreement.

// composerListedPaths maps each LEAF, in the order PolicyShape reports them,
// to the operator's own path number, and names the path §5 extracted as the
// taproot internal key (0 when there is none).
//
// IT EXISTS SO THE FIX REACHES PRODUCTION. composerConsentLinesFor takes this
// numbering and was correct in isolation, but the only call site the live UI
// reaches went through composerConsentLines, which hardcodes (nil, 0) -- so on
// a tr policy with an extracted internal key the consent printed "Path 1" for
// the operator's Path 2 while the seating prompt for the same slot said
// "Path 2". Two screens disagreeing about which path is which, on the surface
// whose whole job is proving which wallet is being consented to.
func composerListedPaths(list md.PathList) (listed []int, keyPathNo int) {
	internal := -1
	if list.Wrapper == md.ComposeTr {
		for i, p := range list.Paths {
			if p.Keys != nil && p.Keys.N == 1 && p.Lock == nil && p.Hash == nil {
				internal = i
				break
			}
		}
	}
	for i := range list.Paths {
		if i == internal {
			continue
		}
		listed = append(listed, i+1)
	}
	if internal >= 0 {
		keyPathNo = internal + 1
	}
	return listed, keyPathNo
}

// composerDigestShort renders a digest as §7e asks: first 8 and last 8 hex.
// A full 64-hex line is CUT rather than wrapped at the label budget, and a
// cut digest hides which end is missing.
//
// It takes a HashLock and slices from the END rather than at 56. A 20-byte
// kind is 40 hex and h[56:] PANICS on it -- that literal is one of
// SPEC_hashlock_kinds §10's six 64-hex assumptions. There is deliberately only
// one of these: a [32]byte twin kept beside it would be the shape this cycle
// is retiring, and a second copy of a width rule is how the two disagree.
func composerDigestShort(l *md.HashLock) string {
	h := hex.EncodeToString(l.Digest())
	return h[:8] + ".." + h[len(h)-8:]
}

// composerConsentHashLine is the consent screen's digest row.
//
// A NAMED FUNCTION so a gate can reach it. It was an inline concatenation, and
// cmd/emu/shots_composer.js asserted its pre-cycle text -- the walk-vs-copy
// anchors gate could not see the drift, because that gate checks
// composerCopyTable() and this is a ROW BUILDER, not a composerCopy* body.
// Anything a walk waits for has to be callable from a test, or the gate has a
// hole exactly where the screens are most likely to change.
func composerConsentHashLine(d *md.HashLock) string {
	// The consent screen NAMES THE KIND, like every other surface that shows a
	// digest (§6's both-or-neither rule): this is the screen an operator reads
	// before agreeing to a policy.
	return "  hash " + d.Kind().Token() + " " + composerDigestShort(d)
}

// composerBranchLines describes one spend path from its decoded Branch.
//
// `sole` is len(shape.Branches) == 1, which is what makes the UNSORTED mark
// honest: §5's key-set rule admits sortedmulti only for a SOLE unlocked,
// unhashed path, so an unsorted key set anywhere else is lowering-forced and
// the operator declined nothing (§5a). Marking those too would teach the
// operator to discount the mark that matters.
func composerBranchLines(b md.Branch, pathNo int, sole bool) []string {
	head := fmt.Sprintf("Path %d: ", pathNo)
	switch {
	case b.Keys == 0:
		head += "KEY-LESS (EXPERIMENTAL)"
	case b.N > 0:
		head += fmt.Sprintf("%d-of-%d", b.K, b.N)
	case b.Keys == 1:
		head += "1 key"
	default:
		head += fmt.Sprintf("%d key(s), custom", b.Keys)
	}
	out := []string{head}
	for _, l := range b.Locks {
		// §7e asks for "its lock kind and value in operator units (§6b echo
		// form)" -- "1000 blocks (about 6.9 days)", not the path-list ROW form
		// "1000 blocks". The row has one line to spend; the consent does not.
		for _, line := range composerLockEcho(l, composerBound{}) {
			out = append(out, "  "+line)
		}
	}
	for _, d := range b.Hashlocks {
		// The consent screen NAMES THE KIND, like every other surface that
		// shows a digest (§6's both-or-neither rule): this is the screen an
		// operator reads before agreeing to a policy.
		out = append(out, composerConsentHashLine(d))
	}
	// `len(b.Hashlocks)`, AND THE CHANGE OF MEANING IS DELIBERATE (§7.4). This
	// read `len(b.Sha256Digests)`, which was zero for a ripemd160 or hash160
	// path because the decoder recorded digests only for tagSha256 -- so a
	// 20-byte-locked sole unsorted path PRINTED `UNSORTED (EXPERIMENTAL)` and
	// now does not.
	//
	// The new reading is the correct one: the mark is honest only for a sole
	// path that is unlocked AND unhashed (§5 admits sortedmulti only there).
	//
	// BUT THE FLIP §7.4 ANNOUNCES IS NOT OBSERVABLE, measured: a keyed+hashed
	// path decodes with N=0, so `b.N >= 2` already excludes it and this clause
	// never gets to decide. §7.4 says such a path "today prints UNSORTED
	// (EXPERIMENTAL) and after this change does not" -- it did not print it
	// before either. F-542. The clause stays because it states the rule the
	// mark means, and TestUnsortedMarkIgnoresNoHashlockKind pins the outcome at
	// every kind.
	if sole && !b.Sorted && b.N >= 2 && len(b.Locks) == 0 && len(b.Hashlocks) == 0 {
		out = append(out, "  UNSORTED (EXPERIMENTAL)")
	}
	return out
}

// composerConsentLines is the whole surface, in §7e's order: paths, the
// key-path line, the id NAMED by kind with both stubs, then addresses or the
// D4 line saying there are none.
// THERE IS NO PARAMETERLESS WRAPPER, and its absence is the fix.
//
// composerConsentLines(chunks) used to sit here hardcoding
// composerConsentLinesFor(chunks, nil, 0), and it was the ONLY form the live
// UI reached -- so the path-numbering fix below was correct in isolation and
// dead from production's point of view. A wrapper that supplies the very
// argument a fix is about is a wrapper that un-fixes it; callers pass the
// numbering they have, and a caller with none passes nil explicitly.
//
// `listed[i]` is the operator's path number for branch i; `keyPathNo` names
// the path §5 extracted as the taproot internal key, or 0.
func composerConsentLinesFor(chunks []string, listed []int, keyPathNo int) ([]string, error) {
	shape, err := md.PolicyShapeChunks(chunks)
	if err != nil {
		return nil, err
	}
	if !shape.Complete {
		// THE HONESTY CONTRACT (md/policy_shape.go:60-63): an incomplete walk
		// means the summariser met a node it could not classify, and a partial
		// description is worse than none -- the operator would believe they had
		// seen the whole policy. The composer only builds shapes §5 lowers, so
		// this is a builder defect, and it says so rather than showing a
		// half-policy.
		return nil, fmt.Errorf("md: this device cannot describe the policy it just built")
	}
	tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		return nil, err
	}

	var lines []string
	// THE SCRIPT, FIRST (journey I-7). Every other line on the Review describes
	// the policy WITHIN a wrapper, and the path list is byte-identical under
	// all four, so without this line there is nowhere before steel that the
	// operator's script choice can be confirmed. That silence is what made
	// journey C-1 -- a picker that committed Taproot over a Segwit policy --
	// invisible all the way to the plate.
	lines = append(lines, composerScriptLine(tpl))
	sole := len(shape.Branches) == 1
	// THE OPERATOR'S PATH NUMBER, NOT THE BRANCH ORDINAL. PolicyShape.Branches
	// are LEAVES, and a taproot internal key is reported through KeyPath
	// rather than as a Branch (md/policy_shape.go:42-45) -- so for
	// tr[P1: 1-of-1, P2: 2-of-3] §5 extracts P1 and the branch list holds one
	// entry, which printed as "Path 1: 2-of-3" while the seating prompt for
	// the same slot said "Path 2". Two screens disagreeing about which path is
	// which, on the surface that consents to steel.
	//
	// `listed` maps branch ordinal -> the operator's path number; it is nil
	// when the caller has no path list (a consent rendered from chunks alone),
	// and then the branch ordinal is the honest answer because there is no
	// other numbering to be wrong about.
	for i, b := range shape.Branches {
		pathNo := i + 1
		if i < len(listed) {
			pathNo = listed[i]
		}
		lines = append(lines, composerBranchLines(b, pathNo, sole)...)
	}
	lines = append(lines, "")
	switch shape.KeyPath {
	case md.KeyPathSpendable:
		line := "Key-path: A KEY CAN SPEND ALONE"
		if keyPathNo > 0 {
			line = fmt.Sprintf("Key-path (Path %d): A KEY CAN SPEND ALONE", keyPathNo)
		}
		lines = append(lines, line)
	case md.KeyPathNUMS:
		lines = append(lines, composerCopyNUMS())
	}

	id, kind, err := md.FormAwareIdChunks(chunks)
	if err != nil {
		return nil, err
	}
	stub, err := md.FormAwareStubChunks(chunks)
	if err != nil {
		return nil, err
	}
	label := "mk1 stub (policy): %x"
	if kind == md.WalletIdTemplate {
		label = "mk1 stub (template): %x"
	}
	lines = append(lines, "", fmt.Sprintf("%s: %x", kind, id), fmt.Sprintf(label, stub))

	// ADDRESSES, or a line saying plainly why there are none (D4). Never
	// silence: an absent address block is indistinguishable from a screen that
	// simply has none, and "I did not see any addresses" is exactly the
	// observation that should stop an operator (gui/wallet_policy.go:245-249).
	// §8i, RESTATED AT CONSENT. §6c and §8i's own heading say "at entry AND at
	// consent": the rule whose whole purpose is to prevent an unspendable
	// wallet was stated once, several screens earlier, on a policy that may
	// have gained its hashlock afterwards.
	//
	// NAMING THE KINDS, not just firing on their presence. Before this cycle
	// the body was constant, so the loop only had to find ONE hashlock and
	// stop. It now reports which kinds the policy actually holds, which means
	// the loop has to see every branch -- a `break` on the first would name
	// sha256 on a wallet whose second path is ripemd160.

	// MIXED LOCK BASES under a non-taproot wrapper (fable review r0 lens 2
	// I-2). Nunchuk refuses the whole wsh script when any two locks disagree
	// about TIME vs HEIGHT; Core imports it. Under tr each leaf is checked on
	// its own and a composer path carries at most one lock, so no leaf can
	// mix and the notice must not fire -- shape.KeyPath is what separates the
	// two, since a tr policy always reports one.
	if shape.KeyPath == md.KeyPathNone && composerMixesLockBases(shape.Branches) {
		lines = append(lines, "", composerCopyMixedLockBases())
	}
	// OUTSIDE LIANA'S MODEL (fable review r0 lens 5 I-1/I-2). Liana imports
	// 17 of 56 composable shapes; the other 39 fall into nine classes, of
	// which §8f (NUMS, above) and §8g (same-seed, below) already name two.
	// This names the rest -- tpl.Root is needed for the wrapper class (sh /
	// sh(wsh)), which PolicyShape does not carry: policyShape walks tagWsh
	// and tagSh identically (md/policy_shape.go), so a legacy wrapper is
	// indistinguishable from wsh on shape.Branches alone.
	if class := composerLianaOutsideModelClass(tpl.Root, shape); class != "" {
		lines = append(lines, "", composerCopyOutsideLianaModel(class))
	}
	// §8a, RESTATED AT CONSENT (fable review r0, lens 1 I-1 = lens 3 I-1).
	//
	// §8a fires at path CREATION, several screens and possibly several edits
	// earlier, and the row here says only `KEY-LESS (EXPERIMENTAL)` -- three
	// words for "no coordinator will import this wallet". The consent is the
	// screen §7e calls the promise and the last one before steel, so the
	// import consequence is restated here for the same reason §8i is (§6c:
	// "at entry AND at consent"). Whole body, verbatim, not a summary: a
	// shortened restatement is a second copy of a rule that can drift from
	// the first.
	//
	// ON THE DECODED SHAPE, not on st.list: this function is handed the
	// CHUNKS that will be cut, so a path that became key-less after the §8a
	// confirm -- or one that never saw it -- is still named here.
	for _, b := range shape.Branches {
		if b.Keys == 0 {
			lines = append(lines, "", composerCopyKeylessPath())
			break
		}
	}
	var kinds []md.HashKind
	seen := make(map[md.HashKind]bool)
	for _, b := range shape.Branches {
		for _, l := range b.Hashlocks {
			if k := l.Kind(); !seen[k] {
				seen[k] = true
				kinds = append(kinds, k)
			}
		}
	}
	if len(kinds) > 0 {
		lines = append(lines, "", composerCopyHashRuleForKinds(kinds))
	}
	// §8d, on the surface §7g's divergence table puts it on: the row
	// "consent | compares the shown id with a coordinator's" answers
	// "DOCUMENTATION: §8d line". It was printed only on the stub screen,
	// several screens and possibly several edits earlier.
	lines = append(lines, "", composerCopyOwnWallet())

	at, ok := policyAddressAt(chunks, tpl, keys)
	if !ok {
		// THROUGH noAddressLines, NOT ITS OWN COPY. This branch said "Keyless
		// template - no addresses." unconditionally, which is false of every
		// refusal except one, and F-531 made a second of them reachable here.
		return append(lines, noAddressLines(chunks, keys)...), nil
	}
	// NO DUPLICATE-KEY WARNING HERE (review I-1): F-531's gates make this
	// branch unreachable for a duplicate, so the F-514 block that used to sit
	// on it could not fire. It lives in noAddressLines now, on the branch
	// above. See the longer note at gui/wallet_policy.go's equivalent.
	lines = append(lines, "")
	for _, chain := range []struct {
		label  string
		change bool
	}{{"Receive", false}, {"Change", true}} {
		for i := 0; i < addrProofPerChain; i++ {
			a, err := at(uint32(i), chain.change)
			if err != nil {
				return nil, fmt.Errorf("md: address derivation failed for %s %d", chain.label, i)
			}
			lines = append(lines, fmt.Sprintf("%s %d:", chain.label, i), a)
		}
	}
	return lines, nil
}

// composerMixesLockBases reports whether the decoded branches carry locks of
// BOTH bases -- TIME (older in 512-second units, after a Unix time) and
// HEIGHT (older in blocks, after a block height).
//
// THE SPLIT IS THE BASE, NOT relative-vs-absolute, and that is the whole
// precision of this predicate: decaying-multisig, a shipped preset, mixes
// older(blocks) with after(height) -- both HEIGHT -- and Nunchuk imports it.
// Reading the axis as relative-vs-absolute would put the notice on a preset
// the device offers by name.
//
// Switched with no default so a fifth LockKind is a missing case here rather
// than a lock silently counted as neither base.
func composerMixesLockBases(branches []md.Branch) bool {
	var time, height bool
	for _, b := range branches {
		for _, l := range b.Locks {
			switch l.Kind {
			case md.LockOlderUnits, md.LockAfterTime:
				time = true
			case md.LockOlderBlocks, md.LockAfterHeight:
				height = true
			}
		}
	}
	return time && height
}

// composerLianaOutsideModelClass names the FIRST way, in Liana's own order of
// refusal, that this policy sits outside Liana 8.0's spending-policy model --
// exactly one unlocked path, at least one path locked by `older` in blocks,
// and no hash anywhere -- or "" when it fits. `root` is the DECODED wrapper
// (md.ExpandWalletPolicyChunks' Template.Root), because PolicyShape does not
// carry it: policyShape walks tagWsh and tagSh identically, so a legacy sh
// wrapper is indistinguishable from wsh on shape.Branches alone.
//
// THE ORDER IS THE WHOLE PRECISION OF THIS FUNCTION, and it is Liana's own,
// not the review report's table order: the wrapper and the taproot internal
// key are checked before Liana ever walks the policy tree, so those two are
// named ahead of a policy-content reason even when a shape is outside the
// model in more than one way at once. decaying-multisig-wsh has BOTH an
// after(...) lock and no unlocked path; this reports the lock it hit
// (`analysis.rs:212-257` before `:633`), not every reason it is refused.
//
// A REAL SPENDABLE TAPROOT KEY PATH COUNTS AS AN UNLOCKED PATH (lens 5 I-2):
// md.KeyPathSpendable means "a real key can spend directly, WITHOUT
// satisfying any leaf" (md/policy_shape.go), which is exactly what an
// unlocked path IS. Every shipped tr preset puts its primary there rather
// than in a leaf, so without this a tr wallet with one real key path and one
// timelocked leaf -- Liana's OWN accepted shape -- would read as "no
// unlocked path" (its leaf list holds one entry and that entry is locked).
// KeyPathNUMS does not count: a NUMS key spends no path at all.
func composerLianaOutsideModelClass(root md.ScriptKind, shape md.PolicyShape) string {
	// 1. Liana takes wsh or tr only (analysis.rs:586-587). ScriptSh covers
	// BOTH bare sh and sh(wsh) (composerScriptLine's grouping): the composer
	// only ever composes a legacy wrapper as a single sortedmulti path
	// (ErrComposeLegacyWrapperShape), so this class and "no locked path"
	// below are always simultaneously true of a legacy-wrapper shape, and
	// this class is named first because Liana's wrapper check runs before
	// it ever looks for a recovery path.
	if root == md.ScriptSh {
		return "legacy wrapper"
	}
	// 2. NUMS internal key under tr (analysis.rs:568-569). §8f
	// (composerCopyNUMS) already names this by itself; this is a SEPARATE
	// class in Liana's own list, not a duplicate of it -- the two bodies say
	// different things (§8f is general to every coordinator; this one is
	// Liana's own order-of-refusal notice).
	if shape.KeyPath == md.KeyPathNUMS {
		return "NUMS key path"
	}
	var (
		anyLock    bool
		hash       bool
		after      bool
		olderUnits bool
		unlocked   int
	)
	if shape.KeyPath == md.KeyPathSpendable {
		unlocked++
	}
	olderBlocks := map[uint32]int{}
	for _, b := range shape.Branches {
		if len(b.Locks) == 0 {
			unlocked++
		}
		for _, l := range b.Locks {
			anyLock = true
			switch l.Kind {
			case md.LockAfterHeight, md.LockAfterTime:
				after = true
			case md.LockOlderUnits:
				olderUnits = true
			case md.LockOlderBlocks:
				olderBlocks[l.Value]++
			}
		}
		if len(b.Hashlocks) > 0 {
			hash = true
		}
	}
	switch {
	// 3. No lock ANYWHERE -- a plain multisig or single key has no recovery
	// path at all (analysis.rs:554-558, :472-474, :583). The demo payload's
	// own plain 2-of-3 is this class.
	case !anyLock:
		return "no locked path"
	// 4. A hash, keyed or key-less, any kind (analysis.rs:186-199, :212-257
	// only match Key/Threshold/Older/After). Checked before the lock-shaped
	// classes below because it is a PARSE-time refusal, not a policy-shape
	// one -- Liana never gets far enough to ask whether this path is a
	// locked recovery path.
	case hash:
		return "a hash lock"
	// 5. after(...), any path (analysis.rs:212-257).
	case after:
		return "an absolute lock"
	// 6. older in 512-second units, any path (csv_check :139-145).
	case olderUnits:
		return "a lock in time units"
	// 7. No unlocked path -- every path timelocked (analysis.rs:633).
	case unlocked == 0:
		return "no unlocked path"
	}
	// 8. Two locked paths with the same older value (analysis.rs:624-626).
	// Only LockOlderBlocks values reach here: an older-units or after value
	// would already have returned above.
	for _, n := range olderBlocks {
		if n >= 2 {
			return "two paths with one lock"
		}
	}
	// 9. A second unlocked path (analysis.rs:611-616). Liana refuses a
	// second unlocked MULTI-key path outright; a second unlocked path that
	// is a SINGLE key is silently folded into the first path as an extra
	// key, without changing its threshold (lens 5 I-2, X24) -- so this
	// class covers both: naming BOTH as "Liana will not show this wallet as
	// built" is the whole point of not distinguishing them here.
	if unlocked >= 2 {
		return "a second unlocked path"
	}
	return ""
}
