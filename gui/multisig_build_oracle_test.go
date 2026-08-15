package gui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// # Two layers, because one of them used to be all there was (C-3)
//
// This gate was a single test that CALLED t.Skipf when ~/.cargo/bin/md was
// absent. The workflow that decides whether a merge lands installs Go and
// nothing else, so it skipped on every push and every pull request — measured:
// CI run 31898063163 on 4b8488e reported `ok seedhammer.com/gui` with S2's
// headline deliverable never executed. S2's GREEN was real where it was
// measured and was never ENFORCED anywhere.
//
//	TestAssembledMd1MatchesTheCommittedGolden      the GATE. No toolchain, no
//	                                               skip path, runs everywhere.
//	TestAssembledMd1MatchesThePrimaryByteForByte   the FRESHNESS check. TIER 2
//	                                               (§4.6): shells out to the
//	                                               pinned primary toolchain.
//
// The golden is not fork-authored data pretending to be an oracle: every byte in
// it came out of the pinned `md`, it can only be written by a live run
// (`-update`, below), and its provenance is checked against oracle/pins.json on
// every run by the gate itself. See oracle/expectfile.go for the full argument,
// including why this is not what oracle.CheckDataSource refuses.

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
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 0}
	got, stub, _, err := assembleBuildPolicy(p, selfXpub, selfFP, cards)
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
			"  go test ./gui -run TestAssembledMd1MatchesThePrimaryByteForByte -update",
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

// ─── LAYER 2: the freshness check. Tier 2 — shells out. ─────────────────────

// s2OracleMD resolves the pinned md binary the way cmd/gaterecord does.
//
// ABSENCE FAILS (C-3). It used to t.Skipf — "a contributor without the Rust
// toolchain should not see a red suite they cannot fix", a real concern and the
// wrong remedy, because the machine that decides a merge is permanently such a
// contributor. That concern is now served properly: the gate above gives a
// toolchain-free contributor the real byte-identity comparison, and this arm
// tells them, in words, what it needs and how to decline it.
func s2OracleMD(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		if oracle.OraclesOptional() {
			t.Skipf("no home directory (%v) and %s=1", err, oracle.OraclesOptionalEnv)
		}
		t.Fatalf("no home directory: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	bin := filepath.Join(dir, "md")
	if _, err := os.Stat(bin); err != nil {
		if oracle.OraclesOptional() {
			t.Skipf("%s=1: the md oracle is not installed at %s; the committed golden "+
				"still compared", oracle.OraclesOptionalEnv, bin)
		}
		t.Fatalf("%s", oracle.MissingOracleMessage("md", dir))
	}
	var pin oracle.Pin
	for _, fp := range pf.Pins {
		if fp.Name == "md" {
			pin = fp.Pin
		}
	}
	if pin.Name == "" {
		t.Fatal("pins.json has no entry for md, so nothing identifies the oracle")
	}
	res, err := oracle.Resolve(oracle.Request{Pin: pin, Bin: bin})
	if err != nil {
		t.Fatalf("the pinned md oracle does not resolve, so nothing compared against it "+
			"can be trusted: %v", err)
	}
	t.Logf("md oracle resolved: commit %s by %s (reports %q, matches pin: %v)",
		res.Commit, res.Method, res.ReportedVersion, res.VersionMatchesPin)
	return bin
}

// TestAssembledMd1MatchesThePrimaryByteForByte re-derives Trace A's md1 from the
// live pinned oracle and requires the device AND the committed golden to equal
// it. This is what stops the golden from becoming a cached wrong answer: it runs
// on every machine that could MINT one.
//
// `-update` re-mints the golden. It requires the live oracle by construction —
// there is no code path that writes a golden from anything else, which is what
// makes the opt-out above safe.
func TestAssembledMd1MatchesThePrimaryByteForByte(t *testing.T) {
	bin := s2OracleMD(t)
	got, xpubs, stub := s2TraceAPolicy(t)

	args := s2MDArgs(xpubs)
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		msg := ""
		if ee, ok := err.(*exec.ExitError); ok {
			msg = string(ee.Stderr)
		}
		t.Fatalf("md encode failed: %v\n%s", err, msg)
	}

	// REFUSE any stdout line that is not an md1 string rather than collecting it
	// as one: md encode prints "chunk-set-id: 0x…" ahead of the strings, and a
	// line-splitter that trusts every line would adopt that header as a chunk and
	// then report a length mismatch instead of a content one.
	var want []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "md1") {
			want = append(want, line)
		}
	}
	if len(want) == 0 {
		t.Fatalf("md encode produced no md1 strings; stdout was:\n%s", out)
	}

	if len(got) != len(want) {
		t.Fatalf("the device assembled %d chunk(s); the primary produced %d for the "+
			"same inputs\ndevice:  %v\nprimary: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			// In full, both of them, always. A truncated string makes two
			// different values read as one.
			t.Fatalf("chunk %d differs:\n  primary %s\n  device  %s", i, want[i], got[i])
		}
	}

	arts := make([]oracle.Artifact, len(want))
	for i, s := range want {
		arts[i] = oracle.Artifact{
			Kind:   "md1",
			Label:  "Trace A 2-of-3 wsh policy, chunk " + string(rune('0'+i)),
			String: s,
		}
	}
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}

	if *update {
		if oracle.OraclesOptional() {
			t.Fatalf("refusing to mint a golden with %s=1 set: a golden may only ever be "+
				"the output of a live derivation", oracle.OraclesOptionalEnv)
		}
		ef, err := oracle.NewExpectFile("S2", "", "", "Trace A's 2-of-3 wsh policy as the "+
			"PINNED PRIMARY encodes it: self = masterA at the shared origin, cosigners B@0 and "+
			"C@0, self slot @0, fingerprints omitted. Minted only by "+
			"`go test ./gui -run TestAssembledMd1MatchesThePrimaryByteForByte -update`, which "+
			"cannot run without the pinned md binary. The device's own assembly is compared "+
			"against this on every machine, with no toolchain, by "+
			"TestAssembledMd1MatchesTheCommittedGolden.",
			pf, []string{"md"}, []string{"md " + strings.Join(args, " ")}, arts)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(s2GoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ef.Write(s2GoldenPath); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d chunk(s))", s2GoldenPath, len(arts))
		return
	}

	// THE FRESHNESS ASSERTION. The golden must be what the live oracle produces
	// today, not merely what the device produces today.
	golden, err := oracle.LoadExpect(s2GoldenPath)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no committed golden to check for freshness at %s: %v",
			s2GoldenPath, err)
	}
	if err := golden.CheckProvenance(pf); err != nil {
		t.Fatalf("%s: %v", s2GoldenPath, err)
	}
	if err := oracle.CompareCensus(golden.Artifacts, want); err != nil {
		t.Fatalf("the committed golden is not what the pinned primary produces today — "+
			"re-mint it with -update, do not edit it.\n%v", err)
	}
	t.Logf("%d md1 chunk(s) byte-identical to the primary and to the committed golden; "+
		"policy stub %x", len(got), stub)
}
