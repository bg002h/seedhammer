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
// TIER 2 (§4.6): this shells out to the pinned primary toolchain. It is the
// gate, not a unit test.
//
// "The host decodes it" is the weaker relation and does not satisfy this gate: a
// decoder that accepts the device's bytes proves the bytes are well-formed, not
// that they are the bytes this wallet should have. So the primary BUILDS an md1
// from the same inputs and the strings must be equal.
//
// The oracle is resolved by SOURCE COMMIT / binary hash, never by --version:
// a version string is self-reported, so pinning by it would let a substituted
// binary launder a device defect through every byte-identity gate in the plan.

// s2OracleMD resolves the pinned md binary the way cmd/gaterecord does, and
// SKIPS when it is not installed — a contributor without the Rust toolchain
// should not see a red suite they cannot fix. It FAILS, loudly, when the binary
// is present but is not the pinned one.
func s2OracleMD(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	bin := filepath.Join(dir, "md")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("the md oracle is not installed at %s; skipping the byte-identity gate", bin)
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

// TestAssembledMd1MatchesThePrimaryByteForByte is S2's closing gate.
func TestAssembledMd1MatchesThePrimaryByteForByte(t *testing.T) {
	bin := s2OracleMD(t)

	// Trace A's inputs: self = masterA at the shared origin, cosigners B@0 and
	// C@0, 2-of-3 wsh, self slot @0, fingerprints omitted.
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
	const wantTemplate = "wsh(sortedmulti(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*))"
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
			tpl.Root, tpl.Policy, tpl.K, tpl.M, tpl.N, wantTemplate)
	}

	// The three account xpubs, in SLOT order, as base58 — which is what the
	// oracle takes. Slot @0 is the self key; @1 and @2 are the cards.
	xpubs := []string{selfXpub, cards[0].Xpub, cards[1].Xpub}

	args := []string{"encode", wantTemplate}
	for i, x := range xpubs {
		args = append(args, "--key", "@"+string(rune('0'+i))+"="+x)
	}
	args = append(args,
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
	t.Logf("%d md1 chunk(s) byte-identical to the primary; policy stub %x", len(got), stub)
}
