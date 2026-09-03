package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/md"
)

// §7e's self-check, on the DECODED md1.
//
// WHAT IT PROVES. Before the consent screen is shown, the device asserts that
// the decoded shape, the slot assignment, every slot's origin and
// fingerprint, the fixed use-site and §4f's pairwise-distinguishability
// invariant ALL hold on the chunk set it is about to engrave. So a builder
// defect in the shape, the seating, the origins, the fingerprints or the
// use-site cannot reach steel as a REVIEWED wallet.
//
// WHAT STAYS OUTSIDE IT: the key bytes themselves, which the addresses on the
// consent screen cover.
//
// IT IS PROVOKED BY FAULT INJECTION, NOT BY AN INPUT (§12 item 4). No
// operator input can make the builder disagree with itself, which is exactly
// why a test that only fed inputs would report this gate as passing while
// never running it. A gate that has never executed is a hypothesis.
var composerSelfCheckFaultHook func(chunks []string) []string

// composerUseSiteIsFixed reports whether a slot carries §5's fixed use-site,
// `/<0;1>/*`. The composer emits nothing else, so anything else is a builder
// defect rather than an exotic wallet.
func composerUseSiteIsFixed(u md.UseSite) bool {
	if !u.HasMultipath || u.WildcardHardened || len(u.Multipath) != 2 {
		return false
	}
	return !u.Multipath[0].Hardened && u.Multipath[0].Value == 0 &&
		!u.Multipath[1].Hardened && u.Multipath[1].Value == 1
}

// composerLeafPaths is the operator's path list with an extracted taproot
// internal-key path removed, in listed order -- which is the order §5 puts
// the leaves on the spine, and therefore the order PolicyShape reports them.
func composerLeafPaths(list md.PathList) []md.SpendPath {
	internal := -1
	if list.Wrapper == md.ComposeTr {
		for i, p := range list.Paths {
			if p.Keys != nil && p.Keys.N == 1 && p.Lock == nil && p.Hash == nil {
				internal = i
				break
			}
		}
	}
	out := make([]md.SpendPath, 0, len(list.Paths))
	for i, p := range list.Paths {
		if i == internal {
			continue
		}
		out = append(out, p)
	}
	return out
}

// composerSelfCheck reports the FIRST mismatch by name. First, not all:
// §8q's body is fixed copy, and the name is for the implementation report and
// the test, where a list of consequences of one root cause is noise.
func composerSelfCheck(st *composerState, chunks []string) error {
	shape, err := md.PolicyShapeChunks(chunks)
	if err != nil {
		return err
	}
	if !shape.Complete {
		return errors.New("self-check: the decoded policy cannot be described")
	}
	leaves := composerLeafPaths(st.list)
	if len(shape.Branches) != len(leaves) {
		return fmt.Errorf("self-check: the decoded policy has %d spend paths, the shape has %d",
			len(shape.Branches), len(leaves))
	}
	for i, p := range leaves {
		b := shape.Branches[i]
		switch {
		case p.Keys == nil:
			if b.Keys != 0 {
				return fmt.Errorf("self-check: path %d is key-less in the shape and has %d keys decoded", i+1, b.Keys)
			}
		case p.Keys.N >= 2:
			// K AND N ARE ONLY MEANINGFUL FOR A PLAIN k-of-N HEAD, and reading
			// them outside that domain made four of the twelve offered presets
			// unbuildable (review r0 fold: tiered-recovery and
			// decaying-multisig, under both wrappers). md.Branch documents it
			// at md/policy_shape.go:45-48 -- "set ONLY when the branch is
			// exactly a threshold over KEYS … Zero means 'not a plain k-of-N'
			// — NOT '1-of-1'" -- and §5 lowers a multi behind a timelock to
			// and_v(v:multi(k,…),older(n)), which is not one. The self-check
			// was therefore comparing 2-of-2 against 0-of-0 on an HONEST build
			// and drawing §8q at an operator whose composition was correct.
			//
			// So the threshold is compared where the codec reports one, and
			// the key COUNT -- which Branch.Keys always carries -- where it
			// does not. Falling back to Keys is not a weakening: it is the
			// strongest fact the decoded tree offers for that branch, and the
			// lock and digest that make the branch not-a-plain-threshold are
			// themselves compared by value just below.
			switch {
			case b.K != 0 || b.N != 0:
				if b.K != int(p.Keys.K) || b.N != int(p.Keys.N) {
					return fmt.Errorf("self-check: path %d is %d-of-%d in the shape and %d-of-%d decoded",
						i+1, p.Keys.K, p.Keys.N, b.K, b.N)
				}
			case b.Keys != int(p.Keys.N):
				return fmt.Errorf("self-check: path %d has %d keys in the shape and %d decoded",
					i+1, p.Keys.N, b.Keys)
			}
		default:
			if b.Keys != 1 {
				return fmt.Errorf("self-check: path %d is one key in the shape and %d decoded", i+1, b.Keys)
			}
		}
		wantLocks := 0
		if p.Lock != nil {
			wantLocks = 1
		}
		if len(b.Locks) != wantLocks {
			return fmt.Errorf("self-check: path %d has %d locks in the shape and %d decoded",
				i+1, wantLocks, len(b.Locks))
		}
		if p.Lock != nil && (b.Locks[0].Kind != p.Lock.Kind || b.Locks[0].Value != p.Lock.Value) {
			return fmt.Errorf("self-check: path %d's lock is %v/%d in the shape and %v/%d decoded",
				i+1, p.Lock.Kind, p.Lock.Value, b.Locks[0].Kind, b.Locks[0].Value)
		}
		wantHash := 0
		if p.Hash != nil {
			wantHash = 1
		}
		if len(b.Sha256Digests) != wantHash {
			return fmt.Errorf("self-check: path %d has %d hash locks in the shape and %d decoded",
				i+1, wantHash, len(b.Sha256Digests))
		}
		if p.Hash != nil && b.Sha256Digests[0] != *p.Hash {
			return fmt.Errorf("self-check: path %d's digest differs from the shape's", i+1)
		}
	}

	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		return err
	}
	if len(keys) != len(st.assigned) {
		return fmt.Errorf("self-check: the decoded policy has %d slots, the seating has %d",
			len(keys), len(st.assigned))
	}
	for i, k := range keys {
		a := st.assigned[i]
		if !composerUseSiteIsFixed(k.UseSite) {
			return fmt.Errorf("self-check: slot @%d does not carry the fixed <0;1>/* use-site", i)
		}
		if a.src < 0 {
			// An unseated slot declares §4f's default origin and no
			// fingerprint. Both are checked, because a fingerprint that
			// appeared on an unseated slot would be a key nobody chose.
			if k.FingerprintPresent {
				return fmt.Errorf("self-check: unseated slot @%d declares a fingerprint", i)
			}
			continue
		}
		if composerOriginKey(originComponents(k.OriginPath)) != composerOriginKey(a.origin) {
			return fmt.Errorf("self-check: slot @%d declares %s, the mapping review showed %s",
				i, k.OriginPath, composerOriginText(a.origin))
		}
		if k.FingerprintPresent != a.fpPresent {
			return fmt.Errorf("self-check: slot @%d's fingerprint presence differs from the mapping review", i)
		}
		if a.fpPresent && k.Fingerprint != a.fingerprint {
			return fmt.Errorf("self-check: slot @%d declares %x, the mapping review showed %x",
				i, k.Fingerprint, a.fingerprint)
		}
	}
	// §4f's INVARIANT, ON THE DECODED md1 -- which is where §4f puts it ("no
	// two slots of a PRODUCED template declare the same origin unless BOTH
	// declare a fingerprint and those fingerprints differ") and where §7e says
	// it is re-checked. composerInvariantViolation reads composer UI state,
	// in which an unseated slot has no origin at all; the origins that matter
	// are the lowest-free accounts the codec assigned when it emitted this
	// chunk set.
	type seat struct {
		fp        [4]byte
		fpPresent bool
	}
	byOrigin := map[string][]seat{}
	for _, k := range keys {
		o := composerOriginKey(originComponents(k.OriginPath))
		byOrigin[o] = append(byOrigin[o], seat{k.Fingerprint, k.FingerprintPresent})
	}
	for o, seats := range byOrigin {
		for i := 0; i < len(seats); i++ {
			for j := i + 1; j < len(seats); j++ {
				if !seats[i].fpPresent || !seats[j].fpPresent || seats[i].fp == seats[j].fp {
					return fmt.Errorf("self-check: two slots declare %s without two "+
						"distinct fingerprints", o)
				}
			}
		}
	}
	return nil
}

// composerConsentFlow is §7e end to end: the check, then the paged surface,
// then §8l.
//
// THE SURFACE IS confirmReviewScreen's PAGED FORM (gui/multisig_build.go
// :1877-1939, its pager gated on a second page existing at :1926-1931):
// eight paths plus four addresses do not fit one frame.
func composerConsentFlow(ctx *Context, th *Colors, st *composerState, chunks []string) bool {
	checked := chunks
	if composerSelfCheckFaultHook != nil {
		checked = composerSelfCheckFaultHook(chunks)
	}
	if err := composerSelfCheck(st, checked); err != nil {
		// The BODY is §8q's fixed copy; the error's name reaches the
		// implementation report and the tests, not the operator, who cannot
		// act on "path 2's digest differs".
		showError(ctx, th, "Review", composerCopySelfCheckFailed())
		return false
	}
	listed, keyPathNo := composerListedPaths(st.list)
	lines, err := composerConsentLinesFor(checked, listed, keyPathNo)
	if err != nil {
		showError(ctx, th, "Review", composerCopySelfCheckFailed())
		return false
	}
	// ONE CONSENT SURFACE, and it is composerReadScreen.
	//
	// §7e and §9 item 9 name confirmReviewScreen's PAGED FORM, and
	// composerReadScreen is that form copied (gui/multisig_build.go:1877-1939)
	// plus the one thing §7e needs and it lacks: the checkmark is withheld
	// until the last page has been laid out once, so the addresses -- the only
	// proof of WHICH wallet this is -- cannot be consented to unseen. Part A
	// used composerReadScreen and Part B used confirmReviewScreen, which was
	// one decision described twice with different Back semantics.
	if !composerReadScreen(ctx, th, "Review", lines) {
		return false
	}
	return composerConfirmScreen(ctx, th, "Before you fund it",
		composerConfirmBody(composerCopyNothingChecked()))
}
