package gui

import (
	"path/filepath"
	"testing"

	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/oracle"
)

// ─── S2's gate: the md1 is compared by PRODUCTION, not acceptance ────────────
//
// "The host decodes it" is the weaker relation and does not satisfy this gate: a
// decoder that accepts the device's bytes proves the bytes are well-formed, not
// that they are the bytes this wallet should have. So the primary BUILDS an md1
// from the same inputs and the strings must be equal.
//
// The oracle is resolved by SOURCE COMMIT / binary hash, never by --version:
// a version string is self-reported, so pinning by it would let a substituted
// binary launder a device defect through every byte-identity gate in the plan.
//
// # This gate used to skip, and skipping is what made it not a gate (C-3)
//
// It was a single test that CALLED t.Skipf when ~/.cargo/bin/md was absent. The
// workflow that decides whether a merge lands installs Go and nothing else, so
// it skipped on every push and every pull request — measured: CI run
// 31898063163 on 4b8488e reported `ok seedhammer.com/gui` with S2's headline
// deliverable never executed. S2's GREEN was real where it was measured and was
// never ENFORCED anywhere.
//
// So the comparison the device must pass is against a COMMITTED golden holding
// the primary's own output. It needs no toolchain, it has no skip path, and it
// runs everywhere. The live re-derivation that keeps that golden honest is not a
// test at all any more — it lives behind the `oraclelive` build tag
// (multisig_build_oracle_live_test.go), so in a normal build it does not exist
// rather than deciding for itself to skip. Operator directive, 2026-08-15:
// "Don't skip jobs unless I ask."
//
// The golden is not fork-authored data pretending to be an oracle: every byte in
// it came out of the pinned `md`, it can only be written by a live run
// (`./scripts/oracle-live.sh -update`), and its provenance is checked against
// oracle/pins.json on every run by the gate below. See oracle/expectfile.go for
// the full argument, including why this is not what oracle.CheckDataSource
// refuses.

// s2GoldenPath is the committed primary output for Trace A's policy.
var s2GoldenPath = filepath.Join("testdata", "s2_md1_golden.expect.json")

// s2TraceAPolicy assembles Trace A's md1 on the DEVICE's own code path and
// returns the chunks, so both layers below compare the same thing.
//
// Trace A's inputs: self = masterA at the shared origin, cosigners B@0 and C@0,
// 2-of-3 wsh, self slot @0, fingerprints omitted.
func s2TraceAPolicy(t *testing.T) (chunks []string, xpubs []string, stub [4]byte) {
	t.Helper()
	selfXpub, selfFP := dupTestSelf(t, fixtureMasterA)
	cards := []mk.Card{dupTestCard(t, 1), dupTestCard(t, 2)} // B@0, C@0
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{0}}
	got, stub, _, err := assembleBuildPolicy(p, selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), cards)
	if err != nil {
		t.Fatalf("the device could not assemble Trace A: %v", err)
	}

	// THE TEMPLATE IS ASSERTED, NOT ASSUMED. The oracle is invoked with a
	// template written here; if the device encodes a DIFFERENT one the two sides
	// would be compared as if they were the same wallet. So the device's own
	// bytes are decoded and the template read back off them first.
	tpl, keys, err := md.ExpandWalletPolicyChunks(got)
	if err != nil {
		t.Fatalf("the device's own md1 does not decode: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("the assembled policy has %d slots, want 3", len(keys))
	}
	if tpl.Root != md.ScriptWsh || tpl.Policy != md.PolicySortedMulti ||
		tpl.K != 2 || tpl.M != 3 || tpl.N != 3 {
		t.Fatalf("the device encoded root=%v policy=%v %d-of-%d over %d key(s), which is "+
			"not what %q says; the two sides would be compared as different wallets",
			tpl.Root, tpl.Policy, tpl.K, tpl.M, tpl.N, s2WantTemplate)
	}
	// The three account xpubs, in SLOT order, as base58 — which is what the
	// oracle takes. Slot @0 is the self key; @1 and @2 are the cards.
	return got, []string{selfXpub, cards[0].Xpub, cards[1].Xpub}, stub
}

const s2WantTemplate = "wsh(sortedmulti(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*))"

// s2MDArgs is the exact argv the primary is invoked with. Built in one place so
// the committed golden's recorded invocation cannot drift from the one the
// freshness check runs.
func s2MDArgs(xpubs []string) []string {
	args := []string{"encode", s2WantTemplate}
	for i, x := range xpubs {
		args = append(args, "--key", "@"+string(rune('0'+i))+"="+x)
	}
	return append(args,
		"--path", multisigSharedOrigin().String(),
		"--network", "mainnet",
		// REQUIRED: md's default inserts a separator every 5 characters for
		// display, and a byte comparison against an engraved string must see the
		// unbroken form.
		"--group-size", "0",
		// REQUIRED: a 3-key policy is 335 data symbols and the regular code caps
		// a single string at 80, so without this md exits with a codec error
		// rather than chunking. Measured, not guessed.
		"--force-chunked",
	)
}

// ─── LAYER 1: the gate. No toolchain, no skip path. ─────────────────────────

// TestAssembledMd1MatchesTheCommittedGolden is S2's closing gate as it is
// actually ENFORCED: the device assembles Trace A's policy and every chunk must
// equal the primary's own output, committed in testdata.
//
// Every failure mode is fatal. A missing, empty or unparseable golden is
// INCONCLUSIVE, never a skip — that distinction is the whole finding.
func TestAssembledMd1MatchesTheCommittedGolden(t *testing.T) {
	golden, err := oracle.LoadExpect(s2GoldenPath)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: S2's byte-identity gate has no usable committed golden at %s: %v\n"+
			"Re-mint it on a machine with the pinned oracles:\n"+
			"  ./scripts/oracle-live.sh -update",
			s2GoldenPath, err)
	}

	// THE STALENESS BINDING. Bump md's pin without re-minting the golden and
	// this goes red, with no Rust toolchain anywhere in sight.
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	if err := golden.CheckProvenance(pf); err != nil {
		t.Fatalf("%s: %v", s2GoldenPath, err)
	}

	got, _, stub := s2TraceAPolicy(t)
	want := make([]oracle.Artifact, len(golden.Artifacts))
	copy(want, golden.Artifacts)
	if err := oracle.CompareCensus(want, got); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d md1 chunk(s) byte-identical to the committed primary output "+
		"(md @ %s); policy stub %x", len(got), golden.Derivation.Oracles[0].Commit[:8], stub)
}
