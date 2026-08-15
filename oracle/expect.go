package oracle

// The DERIVED expectation: what a walk's recorded inputs REQUIRE to have been
// engraved, computed by invoking the primary toolchain.
//
// # Why this file exists (F-170, F-171)
//
// Two holes, and they are the same hole from opposite ends.
//
// F-170: the walk asserted a plate COUNT — `census.strings.length === plates`,
// with `plates` a parameter defaulting to 6. A walk that engraved six WRONG
// strings was green. The plan's §3 preamble has forbidden that since it was
// written: "A walk's expected artifact census MUST derive from the recorded
// input tuple, never from what the walk produced… The script computes how many
// md1 chunks, mk1s and ms1s the inputs REQUIRE and fails when fewer arrive."
// Nothing implemented it.
//
// F-171: nothing in the tree invoked the pinned md/mk/ms at all, so the
// byte-identity gates S2 and S5 rest on had no mechanism — unimplemented, not
// merely unrun.
//
// Deriving the expectation is what closes both: the count falls out of the
// derivation (len of the result), and the strings are produced by the primary
// rather than copied from the device.
//
// # What the oracle chain covers, exactly
//
// Every step is a primary-toolchain invocation. Nothing here re-implements a
// derivation, and nothing is taken from the device:
//
//	seed words  --ms derive--> master fingerprint + account xpub
//	account xpub --mk encode--> the mk1 chunk(s)
//
// The `ms derive --template bip48-*` half did not exist until 2026-08-14: ms
// offered single-sig templates only and accepted no literal path, so seed ->
// multisig account xpub had NO oracle and this file could only have started
// from an xpub somebody typed in. That gap is why the templates were added
// upstream first (ms-cli 0.15.0) rather than worked around here.
//
// # The origin path is CHECKED, not merely recorded
//
// The recorded tuple names a per-slot origin. This code does not trust it: it
// derives the template's own path and REFUSES when the two disagree. A record
// whose origins drifted from the key material would otherwise describe a
// different wallet than the one engraved, and nothing would notice.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ExpectKind names the shape of a walk's artifact set. It is a machine key, not
// prose: the tuple's Template field is human text and cannot dispatch.
//
// Deliberately a closed set with no default. An unknown kind REFUSES rather
// than deriving nothing and reporting a vacuous match — "zero expected strings,
// zero mismatches" is the exact false-PASS this stage exists to remove.
type ExpectKind string

const (
	// KindCosignerCards: the walk engraves gathered cosigner mk1 cards
	// verbatim, one card per recorded seed, at that seed's recorded origin.
	// This is Trace A's shape and S0's record.
	KindCosignerCards ExpectKind = "cosigner-cards"

	// KindBuiltPolicyFull: the walk BUILDS a k-of-n multisig policy and
	// engraves the whole backup for it — the ms1 secret share for every
	// distinct master the device holds, an mk1 key card for every held slot,
	// and the md1 policy descriptor. This is Trace B's shape and S5's record.
	KindBuiltPolicyFull ExpectKind = "built-policy-full"

	// KindBuiltPolicyWatch: the same build in WATCH-ONLY mode, which engraves
	// no ms1 at all — the operator hand-engraves the seed and bundleEngrave
	// shows the reminder instead.
	KindBuiltPolicyWatch ExpectKind = "built-policy-watch"
)

// WHY THE ENGRAVE MODE IS TWO KINDS AND NOT A FIELD ON Expect
//
// Full and watch-only differ in exactly one observable: whether ms1 plates are
// expected. Both shapes were available — a `mode` field beside Kind, or two
// kinds — and the field loses on three counts, all of them structural rather
// than stylistic:
//
//  1. EVERY TOOLCHAIN-FREE CHECK DISPATCHES ON THE KIND ALONE. ArtifactKindsFor
//     and CheckArtifactShape answer "what artifact kinds does this expectation
//     produce, in what order" as a function of the kind. With a separate mode
//     field they could only answer the UNION, and a full-mode expectation that
//     had lost every one of its ms1 artifacts would still be "consistent" —
//     which is precisely the vacuity this package exists to refuse.
//  2. A MISSING KIND REFUSES; A MISSING FIELD DEFAULTS. Kind is checked against
//     a closed set and an unknown one is a hard refusal. A `mode string` that
//     an inputs file forgot to set arrives as "" and takes whichever branch the
//     zero value happens to select — fail-open, silently, on the one input that
//     decides whether a seed reaches steel.
//  3. THE SET IS CLOSED AND SMALL. Two kinds cost one extra const and one extra
//     switch arm. A mode field costs a validity check at every site that reads
//     it, forever, and one forgotten site is a hole.
//
// The cost is that "built policy" is now two names rather than one; that is
// paid once, here, in ArtifactKindsFor and DeriveExpected's switch.

// KnownExpectKind reports whether DeriveExpected can actually derive this kind,
// and ArtifactKindsFor names the artifact kinds it produces.
//
// These exist so the UNTAGGED tests can ask the same question DeriveExpected
// asks, without a toolchain. Re-review finding I-1: a hand-authored expectation
// naming a kind nothing derives, carrying artifacts of a kind that kind never
// produces, passed the whole untagged suite green — because the only check that
// would have caught it lived behind the `oraclelive` build tag, which nothing
// routinely runs. A check that exists only where it is never run is the same
// defect as the skip this stage was built to remove.
//
// Keep this switch and DeriveExpected's in agreement: DeriveExpected now calls
// KnownExpectKind rather than repeating the comparison, so there is one source
// of truth and a new kind cannot be half-added.
func KnownExpectKind(k ExpectKind) bool {
	return len(ArtifactKindsFor(k)) > 0
}

// ArtifactKindsFor returns the Artifact.Kind values ("md1"/"mk1"/"ms1") the
// given ExpectKind produces, IN THE FLOW'S ENGRAVE ORDER, or nil if the kind is
// not derivable.
//
// This is the "richer answer" the single-kind version reserved for the first
// stage that actually had the shape. S5 has it: one record carries md1 + mk1 +
// ms1. The ORDER in the returned slice is load-bearing, not documentation —
// CheckArtifactShape requires a committed expectation's artifacts to arrive as
// consecutive non-empty runs in exactly this sequence, and CompareCensus is
// order-sensitive by design, so a set that arrives in another order is a
// different restore than the one the inputs describe.
//
// The order is READ OFF THE DEVICE, not chosen here: multisigEngraveCards
// (gui/multisig_engrave.go:11-35) appends the ms1 secret card first when full,
// then the mk1 key card, then the md1 descriptor. The plan RULED that plate
// order stands for now and DEFERRED the "public plates first, secret last"
// reordering to its own R0, so this mirrors today's tail rather than the tail
// somebody may later want.
func ArtifactKindsFor(k ExpectKind) []string {
	switch k {
	case KindCosignerCards:
		return []string{"mk1"}
	case KindBuiltPolicyFull:
		return []string{"ms1", "mk1", "md1"}
	case KindBuiltPolicyWatch:
		return []string{"mk1", "md1"}
	default:
		return nil
	}
}

// CheckArtifactShape requires an artifact set to have the shape its kind
// declares: every declared kind present as a non-empty consecutive run, the
// runs in the declared order, and nothing outside them.
//
// It needs no toolchain, so it runs where the verdict is decided. What it buys
// over the old "every artifact is kind X" check is the two failures a multi-kind
// census makes possible and a single-kind one could not: a MISSING class (a
// full-mode expectation holding no ms1 at all — the C2 defect the plan's
// TestFullModeEngravesMs1ForEveryMaster exists to catch, one layer up) and a
// REORDERED one (ms1 plates cut after the public plates, which is a different
// abort story and a different restore).
func CheckArtifactShape(k ExpectKind, arts []Artifact) error {
	want := ArtifactKindsFor(k)
	if len(want) == 0 {
		return fmt.Errorf("%w: expectation kind %q produces no artifact kind this package can "+
			"name, so nothing says what its artifacts ought to be", ErrBadRecord, k)
	}
	if len(arts) == 0 {
		return fmt.Errorf("%w: kind %q engraves %v and the artifact set is empty",
			ErrBadRecord, k, want)
	}
	at := 0
	for _, kind := range want {
		n := 0
		for at+n < len(arts) && arts[at+n].Kind == kind {
			n++
		}
		if n == 0 {
			if at >= len(arts) {
				return fmt.Errorf("%w: kind %q engraves %v in that order, but the set ends after "+
					"%d artifact(s) with no %s at all", ErrBadRecord, k, want, at, kind)
			}
			return fmt.Errorf("%w: artifact %d (%s, %s) is kind %q; kind %q engraves %v in that "+
				"order, so %q was expected here", ErrBadRecord, at, arts[at].Kind, arts[at].Label,
				arts[at].Kind, k, want, kind)
		}
		at += n
	}
	if at != len(arts) {
		return fmt.Errorf("%w: kind %q engraves %v in that order, which accounts for %d of %d "+
			"artifact(s); artifact %d (%s, %s) comes after the last run",
			ErrBadRecord, k, want, at, len(arts), at, arts[at].Kind, arts[at].Label)
	}
	return nil
}

// CheckFingerprintScope requires each artifact to carry exactly the fingerprint
// evidence its own kind can honestly supply — in BOTH directions.
//
// mk1 and ms1 MUST carry one. A key card and a seed card each belong to exactly
// one master, and the fingerprint is what binds the seed to the plate
// independently of the xpub; without it the census would prove only that the
// encoders are self-consistent, not that the right key was engraved.
//
// md1 MUST NOT. A policy descriptor spans every slot, so there is no single
// master it belongs to. Nothing in the derivation computes one, so a fingerprint
// on an md1 chunk is a value no oracle produced — and "demanded of nobody" is
// the weaker rule: it would let a hand-authored expectation decorate its md1
// chunks with a plausible fingerprint and stay green. Forbidding it is the
// direction that can actually fail.
//
// The >=2-DISTINCT rule stays scoped to cosigner-cards, per the S0b re-review's
// M-3. There the whole claim is "these cards come from distinct seeds", so one
// fingerprint across the census means the gate is weaker than it looks. A built
// policy legitimately has ONE held master — the ordinary single-self-slot
// build — so applying the same rule there would fail honest records.
func CheckFingerprintScope(k ExpectKind, arts []Artifact) error {
	if len(ArtifactKindsFor(k)) == 0 {
		return fmt.Errorf("%w: expectation kind %q names no artifact kinds, so there is no "+
			"fingerprint rule to apply", ErrBadRecord, k)
	}
	var problems []string
	seen := map[string]bool{}
	for i, a := range arts {
		if a.Kind == "md1" {
			if a.Fingerprint != "" {
				problems = append(problems, fmt.Sprintf("artifact %d (%s) is an md1 chunk carrying "+
					"fingerprint %q; a policy descriptor spans every slot and belongs to no single "+
					"master, so nothing derived that value", i, a.Label, a.Fingerprint))
			}
			continue
		}
		if a.Fingerprint == "" {
			problems = append(problems, fmt.Sprintf("artifact %d (%s, %s) carries no fingerprint, "+
				"so nothing binds this plate to the seed it claims", i, a.Kind, a.Label))
			continue
		}
		seen[a.Fingerprint] = true
	}
	if k == KindCosignerCards && len(seen) < 2 {
		problems = append(problems, fmt.Sprintf("only %d distinct fingerprint(s) across the whole "+
			"census — kind %q exists to engrave cards from DISTINCT seeds, so this gate is weaker "+
			"than it looks", len(seen), k))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w:\n%s", ErrBadRecord, strings.Join(problems, "\n"))
	}
	return nil
}

// Expect is the part of the derivation that the input TUPLE cannot supply.
//
// Kept as small as it can be. Everything derivable from the tuple — origins,
// seeds, order — is derived from the tuple, because a field restated here is a
// field that can disagree with the record.
type Expect struct {
	Kind ExpectKind `json:"kind"`
	// PolicyIDStub is the 4-byte stub every cosigner card carries, hex.
	//
	// REQUIRED for cosigner-cards: those cards are engraved verbatim from a
	// payload, the stub is an INPUT to mk1 encoding, and it is not recoverable
	// from a seed, so it must be stated.
	//
	// OPTIONAL and CROSS-CHECKED for a built policy, because there the stub is
	// not an input at all: md computes the policy id FROM the policy
	// (`--policy-id-fingerprint`), so a value stated here cannot be the source
	// of truth. It is still allowed, and when present it must equal what md
	// derived — a stated stub that disagrees is a refusal, never an override.
	// That makes the field a checkable claim rather than a second source that
	// can silently win.
	PolicyIDStub string `json:"policy_id_stub,omitempty"`
	// HeldSlots names the slot indices whose key material the DEVICE holds, and
	// therefore engraves a card for.
	//
	// REQUIRED for a built policy and refused for cosigner-cards. It is exactly
	// the kind of thing Expect exists to carry: the tuple says which key sits at
	// which path, and cannot say which of them this device is the custodian of.
	// One mk1 per held slot; one ms1 per DISTINCT master among them in full
	// mode (several slots may share one master — the plan's 3-of-4 across
	// masters A and B is that shape).
	//
	// Strictly ascending and distinct, because the mk1 engrave order follows it
	// and a repeated or unordered entry would make the expected census
	// ambiguous — and an ambiguous expectation is one a wrong census can match.
	HeldSlots []int `json:"held_slots,omitempty"`
}

// Bins are the resolved oracle binary paths.
//
// ABSOLUTE PATHS, always, and they come from pin resolution. Never invoke an
// oracle by bare name: on the maintainer's own machine `md` is a shell alias
// for `mkdir -p`, so a gate that shelled out through a shell would silently run
// coreutils and report whatever that did. exec.Command does not use a shell, so
// this is safe by construction — stated because the next person to write a
// convenience wrapper needs to know.
type Bins struct {
	MD, MK, MS string
}

// Artifact is one expected engraved string and the invocation that produced it.
//
// JSON-tagged because these are what a committed expectation holds
// (expectfile.go); the tags are the on-disk shape, so renaming a field is a
// schema change.
type Artifact struct {
	// Kind is "md1", "mk1" or "ms1".
	Kind string `json:"kind"`
	// Label names which input produced it, e.g. "payload:masterA (card A@0)".
	Label string `json:"label"`
	// String is the expected engraved string, byte for byte.
	String string `json:"string"`
	// Origin is the derivation path the oracle CONFIRMED for this artifact.
	Origin string `json:"origin,omitempty"`
	// Fingerprint is the master fingerprint the oracle derived from the seed.
	// Recorded because it binds the seed to the card independently of the xpub.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// DerivedSet is what one live derivation produced, plus enough about HOW to
// mint a provenance block that is measured rather than declared.
//
// Oracles and Args are collected by DeriveExpected as it invokes each binary —
// never from a table listing which oracles a kind "uses", because such a table
// is a claim that decays the first time a kind grows a step.
type DerivedSet struct {
	// Artifacts is the expected engraved set, in order.
	Artifacts []Artifact
	// Oracles names the binaries actually invoked, in first-use order.
	Oracles []string
	// Args records each distinct invocation FORM, with key material redacted.
	// A committed expectation carries these so a human can redo the derivation;
	// nothing parses them.
	//
	// Redacted deliberately: seed words on a recorded argv would put key
	// material in a committed file and in CI logs, which is the same refusal
	// SeedRef exists for.
	Args []string
}

// note records an oracle invocation on a DerivedSet, deduplicating both lists.
func (d *DerivedSet) note(oracle, argForm string) {
	if !slices.Contains(d.Oracles, oracle) {
		d.Oracles = append(d.Oracles, oracle)
	}
	if !slices.Contains(d.Args, argForm) {
		d.Args = append(d.Args, argForm)
	}
}

// Seed is one recorded seed's words, supplied alongside the tuple.
//
// Words are NOT in InputTuple by design — a gate record carrying seed words
// would be key material in a repo and in CI logs (see SeedRef). They live in
// the operator-authored inputs file, which is committed only because these
// particular masters are BIP-39's own published vectors.
type Seed struct {
	Label string
	Words string
}

// DeriveExpected computes the artifacts a walk's inputs require, in engrave
// order, by invoking the primary toolchain. It also reports which oracles it
// invoked, so a provenance block records what ran rather than what a table says
// ought to have run.
func DeriveExpected(e Expect, in InputTuple, seeds []Seed, bins Bins) (DerivedSet, error) {
	if !KnownExpectKind(e.Kind) {
		return DerivedSet{}, fmt.Errorf("%w: unknown expectation kind %q; refusing to derive nothing "+
			"and report it as a match. Every gate record must carry an expectation this "+
			"package can DERIVE — a stage engraving a shape no ExpectKind names needs a new "+
			"kind here first, not a record minted without one", ErrBadRecord, e.Kind)
	}
	if len(seeds) == 0 {
		return DerivedSet{}, fmt.Errorf("%w: no seeds supplied, so nothing can be derived", ErrIncompleteInputs)
	}
	if len(in.Origins) != len(seeds) {
		return DerivedSet{}, fmt.Errorf("%w: %d origin(s) for %d seed(s) — the tuple cannot say which "+
			"key sits at which path", ErrIncompleteInputs, len(in.Origins), len(seeds))
	}
	switch e.Kind {
	case KindCosignerCards:
		if err := checkStub(e.PolicyIDStub); err != nil {
			return DerivedSet{}, err
		}
		if len(e.HeldSlots) != 0 {
			return DerivedSet{}, fmt.Errorf("%w: kind %q engraves GATHERED cards verbatim and holds "+
				"no slot of its own, so held_slots %v names something this derivation has no use "+
				"for; a tuple that means to say the device holds a slot wants a built-policy kind",
				ErrIncompleteInputs, e.Kind, e.HeldSlots)
		}
		return deriveCosignerCards(e, in, seeds, bins)
	case KindBuiltPolicyFull, KindBuiltPolicyWatch:
		return deriveBuiltPolicy(e, in, seeds, bins, e.Kind == KindBuiltPolicyFull)
	default:
		// Unreachable while KnownExpectKind and this switch agree, which is the
		// point of routing the first check through KnownExpectKind. If a kind is
		// ever added to ArtifactKindsFor and not here, it lands on a REFUSAL
		// rather than falling out of the function with an empty DerivedSet that
		// would then "match" an empty census.
		return DerivedSet{}, fmt.Errorf("%w: kind %q is derivable according to ArtifactKindsFor but "+
			"has no arm in DeriveExpected — the two have drifted, and half a kind derives nothing",
			ErrBadRecord, e.Kind)
	}
}

// deriveCosignerCards derives Trace A's shape: one mk1 card per recorded seed,
// at that seed's recorded origin, carrying the stated policy-id stub.
func deriveCosignerCards(e Expect, in InputTuple, seeds []Seed, bins Bins) (DerivedSet, error) {
	var d DerivedSet
	for i, s := range seeds {
		origin := in.Origins[i]
		acct, tmpl, network, err := templateForOrigin(origin)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		fp, xpub, gotPath, err := msDerive(bins.MS, s.Words, tmpl, acct, network)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		d.note("ms", fmt.Sprintf("ms derive --phrase <seed words: see the inputs file> "+
			"--template %s --account %d --network %s --json", tmpl, acct, network))
		// The recorded origin is a CLAIM. The oracle just derived the path its
		// own template implies; if they differ, the record describes a
		// different key than the one engraved.
		if !samePath(gotPath, origin) {
			return DerivedSet{}, fmt.Errorf("seed %q: the tuple records origin %q but %s derives %q "+
				"for that template — the record and the key material disagree",
				s.Label, origin, tmpl, gotPath)
		}
		chunks, err := mkEncode(bins.MK, xpub, gotPath, fp, e.PolicyIDStub)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		d.note("mk", fmt.Sprintf("mk encode --xpub <account xpub from ms derive> "+
			"--origin-path %s --origin-fingerprint <master fp from ms derive> "+
			"--policy-id-stub %s --group-size 0", gotPath, e.PolicyIDStub))
		for _, c := range chunks {
			d.Artifacts = append(d.Artifacts, Artifact{
				Kind: "mk1", Label: s.Label, String: c, Origin: gotPath, Fingerprint: fp,
			})
		}
	}
	return d, nil
}

// slotKey is one policy slot after the ms oracle has spoken for it.
type slotKey struct {
	fp, xpub, path, template, network string
}

// deriveBuiltPolicy derives Trace B's shape: the ms1/mk1/md1 backup a device
// engraves for a k-of-n multisig policy it BUILT, rather than cards it gathered.
//
// The chain, every step a primary invocation and nothing reimplemented here:
//
//	seed words   --ms derive--> master fingerprint + account xpub, per slot
//	all n xpubs  --md encode--> the md1 chunks AND the policy id
//	seed words   --ms encode--> the ms1 secret share, per held master (full)
//	xpub + stub  --mk encode--> the mk1 chunk(s), per held slot
//
// The policy id is DERIVED here rather than stated: md computes it from the
// policy, so asking the operator for it would install a second source of truth
// that can disagree with the wallet actually encoded.
func deriveBuiltPolicy(e Expect, in InputTuple, seeds []Seed, bins Bins, full bool) (DerivedSet, error) {
	n := len(seeds)
	if in.N != n {
		return DerivedSet{}, fmt.Errorf("%w: the tuple says n=%d but supplies %d slot(s); a built "+
			"policy's key count is not a free-text field, and a descriptor built over a different "+
			"number of keys is a different wallet", ErrIncompleteInputs, in.N, n)
	}
	if in.K < 1 || in.K > n {
		return DerivedSet{}, fmt.Errorf("%w: k=%d over %d slot(s) is not a threshold this gate can "+
			"name; a built policy must state one (cosigner-cards may leave k unset because it "+
			"builds no policy — this kind does)", ErrIncompleteInputs, in.K, n)
	}
	held, err := checkHeldSlots(e.HeldSlots, n)
	if err != nil {
		return DerivedSet{}, err
	}

	// PURE PASS FIRST, before a single oracle is invoked. Everything below is a
	// property of the recorded origins alone, so it refuses in a couple of
	// microseconds on a machine with no toolchain at all — and, more usefully,
	// its refusals are reachable by an untagged test.
	tmpl, network, sharedPath, err := uniformOrigins(in.Origins)
	if err != nil {
		return DerivedSet{}, err
	}
	policy, err := policyTemplate(tmpl, in.K, n)
	if err != nil {
		return DerivedSet{}, err
	}

	var d DerivedSet
	slots := make([]slotKey, n)
	xpubs := make([]string, n)
	for i, s := range seeds {
		acct, _, _, err := templateForOrigin(in.Origins[i])
		if err != nil {
			return DerivedSet{}, fmt.Errorf("slot %d (%s): %w", i, s.Label, err)
		}
		fp, xpub, gotPath, err := msDerive(bins.MS, s.Words, tmpl, acct, network)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("slot %d (%s): %w", i, s.Label, err)
		}
		d.note("ms", fmt.Sprintf("ms derive --phrase <seed words: see the inputs file> "+
			"--template %s --account %d --network %s --json", tmpl, acct, network))
		// The recorded origin is a CLAIM, checked the same way cosigner cards
		// check it: a record whose origins drifted from its key material
		// describes a different wallet than the one engraved.
		if !samePath(gotPath, in.Origins[i]) {
			return DerivedSet{}, fmt.Errorf("slot %d (%s): the tuple records origin %q but %s "+
				"derives %q for that template — the record and the key material disagree",
				i, s.Label, in.Origins[i], tmpl, gotPath)
		}
		slots[i] = slotKey{fp: fp, xpub: xpub, path: gotPath, template: tmpl, network: network}
		xpubs[i] = xpub
	}

	// THE POLICY, and with it the policy id every mk1 card must carry.
	chunks, stub, err := mdEncode(bins.MD, policy, xpubs, sharedPath, network)
	if err != nil {
		return DerivedSet{}, err
	}
	d.note("md", fmt.Sprintf("md encode %q --key @i=<account xpub i from ms derive, i=0..%d> "+
		"--path %s --network %s --group-size 0 --force-chunked --policy-id-fingerprint",
		policy, n-1, sharedPath, network))
	if e.PolicyIDStub != "" {
		if err := checkStub(e.PolicyIDStub); err != nil {
			return DerivedSet{}, err
		}
		if e.PolicyIDStub != stub {
			return DerivedSet{}, fmt.Errorf("%w: the expect block states policy_id_stub %s but md "+
				"derives %s from this policy. The stub is computed FROM the descriptor, so a stated "+
				"one is a claim about which wallet these inputs describe — and it is wrong. Fix the "+
				"inputs, or drop the field and let md answer",
				ErrRecordMismatch, e.PolicyIDStub, stub)
		}
	}

	// THE ORDER IS THE FLOW'S, and CompareCensus is order-sensitive by design.
	// gui/multisig_engrave.go:11-35 — full mode appends the ms1 secret card
	// first, then the mk1 key card, then the md1 descriptor; watch-only omits
	// the ms1 entirely. ArtifactKindsFor states the same order, and
	// CheckArtifactShape holds a committed expectation to it with no toolchain.
	if full {
		// ONE ms1 PER DISTINCT MASTER, not per held slot: several slots may be
		// legs of the same master (the plan's 3-of-4 across masters A and B),
		// and engraving that master's seed twice is one plate of pure risk.
		// Distinctness is keyed on the WORDS, which are the master's identity;
		// keying on the 4-byte fingerprint would let a collision silently drop
		// a master out of a backup labelled "Full".
		seenMaster := map[string]bool{}
		for _, i := range held {
			if seenMaster[seeds[i].Words] {
				continue
			}
			seenMaster[seeds[i].Words] = true
			ms1, err := msEncode(bins.MS, seeds[i].Words)
			if err != nil {
				return DerivedSet{}, fmt.Errorf("slot %d (%s): %w", i, seeds[i].Label, err)
			}
			d.note("ms", "ms encode --phrase <seed words: see the inputs file> "+
				"--group-size 0 --no-engraving-card")
			d.Artifacts = append(d.Artifacts, Artifact{
				Kind:        "ms1",
				Label:       seeds[i].Label + " (ms1 secret share)",
				String:      ms1,
				Fingerprint: slots[i].fp,
			})
		}
	}
	for _, i := range held {
		cards, err := mkEncode(bins.MK, slots[i].xpub, slots[i].path, slots[i].fp, stub)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("slot %d (%s): %w", i, seeds[i].Label, err)
		}
		d.note("mk", fmt.Sprintf("mk encode --xpub <account xpub from ms derive> "+
			"--origin-path %s --origin-fingerprint <master fp from ms derive> "+
			"--policy-id-stub <policy id from md encode --policy-id-fingerprint> "+
			"--group-size 0", slots[i].path))
		for _, c := range cards {
			d.Artifacts = append(d.Artifacts, Artifact{
				Kind:        "mk1",
				Label:       fmt.Sprintf("%s (mk1 key, slot %d)", seeds[i].Label, i),
				String:      c,
				Origin:      slots[i].path,
				Fingerprint: slots[i].fp,
			})
		}
	}
	for j, c := range chunks {
		d.Artifacts = append(d.Artifacts, Artifact{
			Kind: "md1",
			// The template goes in the label deliberately: it is the one thing a
			// human reading the committed expectation needs in order to say WHICH
			// wallet these chunks describe, and it is measured rather than
			// restated — policyTemplate built it and md accepted it.
			Label:  fmt.Sprintf("%s (md1 policy, chunk %d)", policy, j),
			String: c,
			Origin: sharedPath,
			// NO FINGERPRINT, and CheckFingerprintScope refuses one. A descriptor
			// spans every slot and belongs to no single master.
		})
	}
	return d, nil
}

// checkHeldSlots validates the held-slot set against the slot count.
func checkHeldSlots(held []int, n int) ([]int, error) {
	if len(held) == 0 {
		return nil, fmt.Errorf("%w: a built policy engraves an mk1 for every slot the DEVICE holds "+
			"and held_slots names none, so this expectation would require a backup carrying no key "+
			"card at all — which is not a backup of anything", ErrIncompleteInputs)
	}
	for i, s := range held {
		if s < 0 || s >= n {
			return nil, fmt.Errorf("%w: held_slots names slot %d, but the tuple has %d slot(s) "+
				"(0..%d)", ErrIncompleteInputs, s, n, n-1)
		}
		if i > 0 && s <= held[i-1] {
			return nil, fmt.Errorf("%w: held_slots %v must be strictly ascending and distinct — the "+
				"mk1 engrave order follows it, and a repeated or unordered entry makes the expected "+
				"census ambiguous", ErrIncompleteInputs, held)
		}
	}
	return held, nil
}

// uniformOrigins reduces the recorded origins to the ONE template, network and
// path a single md1 can encode, or refuses.
//
// DIVERGENT ORIGINS ARE REFUSED, AND THE REFUSAL WAS MEASURED RATHER THAN
// ASSUMED. md's `--path` is documented as flattening divergent mode to shared,
// so it cannot express a per-slot origin; and the bracketed form the descriptor
// itself uses is rejected by the pinned md — verified 2026-08-15 against
// md 0.13.0:
//
//	md encode 'wsh(sortedmulti(2,[73c5da0a/48h/0h/0h/2h]@0/<0;1>/*,...))' --key @0=xpub…
//	md: template parse error: internal: synthetic key [73c5da0a not found in key map
//
// So there is no invocation this gate could make that would derive the RIGHT
// md1 for a divergent-origin policy. Deriving a shared-origin one anyway and
// comparing it against a divergent-origin engraving would fail at the bytes with
// no explanation of why; refusing names the reason. Divergent support means
// finding md's own form for it and proving that form against a real walk —
// which is work, not a flag, and it does not belong in a gate written before the
// stage it judges.
func uniformOrigins(origins []string) (template, network, path string, err error) {
	for i, o := range origins {
		_, tmpl, net, err := templateForOrigin(o)
		if err != nil {
			return "", "", "", fmt.Errorf("slot %d: %w", i, err)
		}
		if i == 0 {
			template, network, path = tmpl, net, strings.TrimSpace(o)
			continue
		}
		if tmpl != template {
			return "", "", "", fmt.Errorf("%w: slot 0's origin implies template %s and slot %d's "+
				"implies %s; one md1 encodes ONE script type, so a policy mixing them is not a "+
				"policy this gate can derive", ErrIncompleteInputs, template, i, tmpl)
		}
		if net != network {
			return "", "", "", fmt.Errorf("%w: slot 0 is %s and slot %d is %s; one policy is on one "+
				"network", ErrIncompleteInputs, network, i, net)
		}
		if !samePath(o, path) {
			return "", "", "", fmt.Errorf("%w: the origins DIVERGE — slot 0 at %q, slot %d at %q. "+
				"The pinned md offers no invocation that encodes a per-slot origin (`--path` "+
				"flattens divergent mode to shared, and the bracketed [fp/path]@i form is a "+
				"template parse error), so this gate cannot derive the md1 a divergent-origin "+
				"policy requires and will not derive a DIFFERENT wallet's md1 in its place",
				ErrIncompleteInputs, path, i, o)
		}
	}
	if template == "" {
		return "", "", "", fmt.Errorf("%w: no origins", ErrIncompleteInputs)
	}
	return template, network, path, nil
}

// policyTemplate builds the BIP-388 template md encodes, from the script type
// the origins imply and the recorded threshold.
//
// It is generated rather than stated so that k, n and the script type cannot
// disagree with the tuple they came from. The generated form is pinned against
// the DEVICE's own S2 template by TestPolicyTemplateMatchesTheDevicesOwnS2Template.
func policyTemplate(msTemplate string, k, n int) (string, error) {
	var open, close string
	switch msTemplate {
	case "bip48-p2wsh":
		open, close = "wsh(", ")"
	case "bip48-p2sh-p2wsh":
		open, close = "sh(wsh(", "))"
	default:
		return "", fmt.Errorf("%w: no md template is registered for ms template %q",
			ErrIncompleteInputs, msTemplate)
	}
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("@%d/<0;1>/*", i)
	}
	return fmt.Sprintf("%ssortedmulti(%d,%s)%s", open, k, strings.Join(keys, ","), close), nil
}

// CompareCensus checks a census against derived artifacts, byte for byte and in
// order.
//
// Order is part of the answer, not incidental: a bundle is cut card by card and
// plate by plate, and a set that arrives in a different order is a different
// restore than the one the inputs describe.
func CompareCensus(want []Artifact, got []string) error {
	var problems []string
	if len(want) != len(got) {
		problems = append(problems, fmt.Sprintf(
			"the inputs require %d engraved string(s); the census holds %d",
			len(want), len(got)))
	}
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i].String != got[i] {
			// In full, both of them, always. A digest or a string rendered
			// truncated makes two different values read as one.
			problems = append(problems, fmt.Sprintf(
				"plate %d (%s, %s) differs:\n  expected %s\n  engraved %s",
				i, want[i].Kind, want[i].Label, want[i].String, got[i]))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w:\n%s", ErrRecordMismatch, strings.Join(problems, "\n"))
	}
	if n == 0 {
		return fmt.Errorf("%w: nothing was compared, so this check passed by checking nothing",
			ErrRecordMismatch)
	}
	return nil
}

// pathRe matches an account path down to the BIP-48 script_type level, in
// either notation: m/48'/0'/0'/2' or m/48h/0h/0h/2h.
var pathRe = regexp.MustCompile(`^m/(\d+)['h]/(\d+)['h]/(\d+)['h](?:/(\d+)['h])?$`)

// templateForOrigin maps a recorded origin to the ms template that produces it.
//
// It refuses anything it cannot name, rather than guessing a template and
// deriving a key at a path nobody asked for.
func templateForOrigin(origin string) (account int, template string, network string, err error) {
	m := pathRe.FindStringSubmatch(strings.TrimSpace(origin))
	if m == nil {
		return 0, "", "", fmt.Errorf("origin %q is not an account path this gate can name", origin)
	}
	purpose, _ := strconv.Atoi(m[1])
	coin, _ := strconv.Atoi(m[2])
	account, _ = strconv.Atoi(m[3])
	switch coin {
	case 0:
		network = "mainnet"
	case 1:
		network = "testnet"
	default:
		return 0, "", "", fmt.Errorf("origin %q has coin_type %d, which this gate has no oracle for", origin, coin)
	}
	if purpose != 48 {
		// Single-sig purposes are derivable too, but nothing in this plan
		// engraves one as a cosigner card. Refuse rather than silently widen.
		return 0, "", "", fmt.Errorf("origin %q has purpose %d'; this gate derives BIP-48 cosigner cards only", origin, purpose)
	}
	if m[4] == "" {
		return 0, "", "", fmt.Errorf("origin %q is BIP-48 but names no script_type level", origin)
	}
	switch m[4] {
	case "2":
		template = "bip48-p2wsh"
	case "1":
		template = "bip48-p2sh-p2wsh"
	default:
		return 0, "", "", fmt.Errorf("origin %q has script_type %s', which BIP-48 does not register", origin, m[4])
	}
	return account, template, network, nil
}

// samePath compares two derivation paths across the ' and h notations.
func samePath(a, b string) bool {
	norm := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "h", "'")
	}
	return norm(a) != "" && norm(a) == norm(b)
}

func checkStub(s string) error {
	if len(s) != 8 {
		return fmt.Errorf("%w: policy_id_stub %q must be 8 hex chars (4 bytes)", ErrIncompleteInputs, s)
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return fmt.Errorf("%w: policy_id_stub %q must be lowercase hex", ErrIncompleteInputs, s)
		}
	}
	return nil
}

// msDeriveJSON is the shape `ms derive --json` emits.
type msDeriveJSON struct {
	MasterFingerprint string `json:"master_fingerprint"`
	AccountPath       string `json:"account_path"`
	AccountXpub       string `json:"account_xpub"`
}

// msDerive runs the ms oracle for one seed.
//
// The words go on argv, which ms warns about on stderr. These are BIP-39's
// published vectors and the warning is correct for real material; a gate that
// derived a real seed would pipe them instead.
func msDerive(bin, words, template string, account int, network string) (fp, xpub, path string, err error) {
	if bin == "" {
		return "", "", "", fmt.Errorf("no ms binary resolved")
	}
	out, err := exec.Command(bin, "derive",
		"--phrase", words,
		"--template", template,
		"--account", strconv.Itoa(account),
		"--network", network,
		"--json",
	).Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ms derive --template %s: %w", template, runErr(err))
	}
	var d msDeriveJSON
	if err := json.Unmarshal(out, &d); err != nil {
		return "", "", "", fmt.Errorf("ms derive emitted unparseable JSON: %w", err)
	}
	if d.AccountXpub == "" || d.MasterFingerprint == "" || d.AccountPath == "" {
		return "", "", "", fmt.Errorf("ms derive returned an incomplete account: %+v", d)
	}
	return d.MasterFingerprint, d.AccountXpub, d.AccountPath, nil
}

// mkEncode runs the mk oracle for one card, returning its chunks in order.
//
// --group-size 0 is REQUIRED: mk's default inserts a separator every 5
// characters for display, and a byte comparison against an engraved string must
// see the unbroken form.
func mkEncode(bin, xpub, path, fp, stub string) ([]string, error) {
	if bin == "" {
		return nil, fmt.Errorf("no mk binary resolved")
	}
	out, err := exec.Command(bin, "encode",
		"--xpub", xpub,
		"--origin-path", path,
		"--origin-fingerprint", fp,
		"--policy-id-stub", stub,
		"--group-size", "0",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("mk encode: %w", runErr(err))
	}
	// REFUSE any stdout line that is not an mk1 string, rather than collecting
	// it as one. `mk encode` emits only the strings today — but its sibling
	// `md encode` prints `chunk-set-id: 0x…` on STDOUT ahead of them, so a
	// line-splitter that trusts every line would silently adopt that header as
	// an expected artifact the moment this is extended to md1. An expectation
	// built from a header line compares garbage and reports a mismatch nobody
	// can read; worse, a count that happens to match would pass.
	var chunks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "mk1") {
			return nil, fmt.Errorf("mk encode emitted a non-mk1 line on stdout, which this "+
				"gate will not treat as an artifact: %q (use --json if the CLI has grown a "+
				"header)", line)
		}
		chunks = append(chunks, line)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("mk encode produced no strings")
	}
	return chunks, nil
}

// The two NON-ARTIFACT lines the pinned md prints on STDOUT, measured against
// md 0.13.0 on 2026-08-15 rather than read off a doc comment:
//
//	chunk-set-id: 0x30d86
//	md1fxrvxzspqjtvyyy4qqxppcgsc27rchwsv0jskp2rsal4egz4ep5859p875x67p5s5tk09nzz08lv4
//	… four more chunks …
//	policy-id-fingerprint: 0x06215ac0
//
// The header is printed WHETHER OR NOT --policy-id-fingerprint is passed; both
// were checked. mkEncode's comment predicted this trap verbatim before any md
// step existed, and it is the reason parseMdStdout classifies every line instead
// of collecting the ones that look plausible.
const (
	mdChunkSetIDPrefix = "chunk-set-id: "
	mdPolicyIDPrefix   = "policy-id-fingerprint: "
)

// mdEncode runs the md oracle for the whole policy, returning its chunks in
// order AND the policy id the mk1 cards must carry.
//
// --group-size 0 is REQUIRED for the reason it is required everywhere in this
// file: md's default inserts a separator every 5 characters for display, and a
// byte comparison against an engraved string must see the unbroken form.
//
// --force-chunked is REQUIRED and was measured, not guessed: a 3-key policy is
// 335 data symbols and the regular code caps a single string at 80, so without
// it md exits with a codec error rather than chunking
// (gui/multisig_build_oracle_test.go:104-107).
//
// --policy-id-fingerprint is what makes the policy id DERIVED rather than
// declared. Without it the caller would have to be told the stub, and a told
// stub is a second source of truth about which wallet this is.
func mdEncode(bin, policy string, xpubs []string, path, network string) (chunks []string, stub string, err error) {
	if bin == "" {
		return nil, "", fmt.Errorf("no md binary resolved")
	}
	args := []string{"encode", policy}
	for i, x := range xpubs {
		args = append(args, "--key", fmt.Sprintf("@%d=%s", i, x))
	}
	args = append(args,
		"--path", path,
		"--network", network,
		"--group-size", "0",
		"--force-chunked",
		"--policy-id-fingerprint",
	)
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return nil, "", fmt.Errorf("md encode: %w", runErr(err))
	}
	return parseMdStdout(out)
}

// parseMdStdout classifies every line md printed, and REFUSES anything it does
// not recognise rather than collecting it as an artifact.
//
// This is the header trap, closed. A line-splitter that trusted every line would
// adopt `chunk-set-id: 0x30d86` as an expected md1 string; the comparison would
// then be against garbage, and a count that happened to match would PASS. Note
// what is deliberately NOT done: the header is not skipped by position, or by
// "lines before the first md1", or by trimming a fixed number of lines. It is
// matched by its own prefix, and a line matching no prefix is a refusal — so md
// growing a THIRD header line in some later version fails loudly here instead of
// being adopted silently.
func parseMdStdout(out []byte) (chunks []string, stub string, err error) {
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "md1"):
			if err := refuseSeparated("md1", line); err != nil {
				return nil, "", err
			}
			chunks = append(chunks, line)
		case strings.HasPrefix(line, mdChunkSetIDPrefix):
			// CONSUMED DELIBERATELY. It identifies the chunk set and is not an
			// engraved string; nothing downstream wants it, and the one thing it
			// must never be is an artifact.
			continue
		case strings.HasPrefix(line, mdPolicyIDPrefix):
			v, perr := parseHexStub(strings.TrimPrefix(line, mdPolicyIDPrefix))
			if perr != nil {
				return nil, "", fmt.Errorf("md encode printed %q, which this gate cannot read as a "+
					"policy id: %w", line, perr)
			}
			if stub != "" {
				return nil, "", fmt.Errorf("md encode printed two %q lines (%s and %s); the policy "+
					"id must be unambiguous, because it is engraved into every mk1 card",
					mdPolicyIDPrefix, stub, v)
			}
			stub = v
		default:
			return nil, "", fmt.Errorf("md encode emitted a line on stdout that this gate neither "+
				"recognises nor will treat as an artifact: %q (use --json if the CLI has grown a "+
				"header; do NOT widen this to accept unknown lines)", line)
		}
	}
	if len(chunks) == 0 {
		return nil, "", fmt.Errorf("md encode produced no md1 strings")
	}
	if stub == "" {
		return nil, "", fmt.Errorf("md encode printed no %q line, so the policy id every mk1 card "+
			"must carry was never derived — --policy-id-fingerprint is not optional here",
			mdPolicyIDPrefix)
	}
	return chunks, stub, nil
}

// msEncode runs the ms oracle for one master's seed, returning its ms1 string.
//
// --group-size 0 is MANDATORY and the default is 5: measured on the pinned
// binary, `ms encode --help` says "[default: 5]", and the default output is
//
//	ms10e ntrsq qqqqq qqqqq qqqqq qqqqq qqqqq qqcj9 sxraq 34v7f
//
// which can never equal an engraved unbroken string. The same trap mkEncode
// documents for mk. --no-engraving-card suppresses the human-facing stderr card
// this is not a human.
func msEncode(bin, words string) (string, error) {
	if bin == "" {
		return "", fmt.Errorf("no ms binary resolved")
	}
	out, err := exec.Command(bin, "encode",
		"--phrase", words,
		"--group-size", "0",
		"--no-engraving-card",
	).Output()
	if err != nil {
		return "", fmt.Errorf("ms encode: %w", runErr(err))
	}
	return parseMsStdout(out)
}

// parseMsStdout refuses anything that is not exactly ONE unbroken ms1 string.
func parseMsStdout(out []byte) (string, error) {
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "ms1") {
			return "", fmt.Errorf("ms encode emitted a non-ms1 line on stdout, which this gate will "+
				"not treat as an artifact: %q (use --json if the CLI has grown a header)", line)
		}
		if err := refuseSeparated("ms1", line); err != nil {
			return "", err
		}
		got = append(got, line)
	}
	switch len(got) {
	case 0:
		return "", fmt.Errorf("ms encode produced no ms1 string")
	case 1:
		return got[0], nil
	default:
		return "", fmt.Errorf("ms encode produced %d ms1 strings for one master; an ms1 secret share "+
			"is a single-string, single-plate card, so this gate cannot say which one was engraved",
			len(got))
	}
}

// refuseSeparated catches the --group-size trap AT THE BYTES rather than
// trusting that the flag was passed.
//
// The engraved forms are bech32-style: prefix plus the charset
// qpzry9x8gf2tvdw0s3jn54khce6mua7l, which contains no space, hyphen or comma. So
// any of those in a string the oracle just produced means a display separator
// survived into a value about to be compared byte for byte. Passing the flag is
// a discipline; this is the check.
func refuseSeparated(kind, line string) error {
	if i := strings.IndexAny(line, " -,"); i >= 0 {
		return fmt.Errorf("the oracle emitted a SEPARATED %s (%q, separator at byte %d). "+
			"--group-size 0 is required: the default inserts a display separator every 5 "+
			"characters, and a separated string can never equal an engraved one", kind, line, i)
	}
	return nil
}

// parseHexStub reads md's `0x…` policy-id form into the lowercase 8-hex-char
// shape mk encode takes for --policy-id-stub.
func parseHexStub(s string) (string, error) {
	v := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	if err := checkStub(v); err != nil {
		return "", err
	}
	return v, nil
}

// runErr surfaces a failed command's stderr, which is where these CLIs put the
// reason. Without it every failure reads "exit status 1".
func runErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}
