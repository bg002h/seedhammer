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
func composerDigestShort(d [32]byte) string {
	h := hex.EncodeToString(d[:])
	return h[:8] + ".." + h[56:]
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
	for _, d := range b.Sha256Digests {
		out = append(out, "  hash "+composerDigestShort(d))
	}
	if sole && !b.Sorted && b.N >= 2 && len(b.Locks) == 0 && len(b.Sha256Digests) == 0 {
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
	for _, b := range shape.Branches {
		if len(b.Sha256Digests) > 0 {
			lines = append(lines, "", composerCopyHashRule())
			break
		}
	}
	// §8d, on the surface §7g's divergence table puts it on: the row
	// "consent | compares the shown id with a coordinator's" answers
	// "DOCUMENTATION: §8d line". It was printed only on the stub screen,
	// several screens and possibly several edits earlier.
	lines = append(lines, "", composerCopyOwnWallet())

	at, ok := policyAddressAt(chunks, tpl, keys)
	if !ok {
		return append(lines, "", "Keyless template - no addresses.", "Verify off-device."), nil
	}
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
