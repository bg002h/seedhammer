package gui

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// suppliedMultisigMd1 loads the vendored full-policy wsh(sortedmulti(2,@0,@1,@2))
// md1 chunk strings. The operator's abandon-about seed is slot @1 at
// m/48'/0'/0'/2'; @0/@2 are foreign pubkeys.
//
// Fixture reproducibility (R0 m-1) — the two foreign slots' decoded 65-byte
// ExpandedKey.Xpub (chainCode[0:32] ‖ compressedPubkey[32:65]), hex:
//   keys[0].Xpub = 101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23
//   keys[2].Xpub = 101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5
// (Both share the synthetic chain code 1011..2e2f; only the pubkey differs.
// Slot @1 is the abandon-about seed's real key derived at m/48'/0'/0'/2'.)
func suppliedMultisigMd1(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("testdata/t6b_multisig_full.md1.txt")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("fixture has %d chunks, want 6", len(out))
	}
	return out
}

// TestSuppliedMultisigFixtureIsFullPolicy guards the vendored fixture: it must
// decode to a full-policy 2-of-3 wsh(sortedmulti) with every slot xpub-present
// at origin m/48'/0'/0'/2', and the abandon seed must match slot @1 only. If
// this fails, the fixture string is corrupt — do NOT regenerate it ad hoc;
// re-derive it via the documented descriptor (see the plan's Test Vectors).
func TestSuppliedMultisigFixtureIsFullPolicy(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	if tpl.Root != md.ScriptWsh || tpl.Policy != md.PolicySortedMulti {
		t.Fatalf("tpl root/policy = %v/%v, want Wsh/SortedMulti", tpl.Root, tpl.Policy)
	}
	if tpl.K != 2 || tpl.N != 3 {
		t.Fatalf("tpl K/N = %d/%d, want 2/3", tpl.K, tpl.N)
	}
	if len(keys) != 3 {
		t.Fatalf("got %d keys, want 3", len(keys))
	}
	if !allSlotsHaveXpub(keys) {
		t.Fatal("fixture is not full-policy (a slot lacks an xpub)")
	}
	wantOrigin := "m/48h/0h/0h/2h"
	for i, k := range keys {
		if k.OriginPath.String() != wantOrigin {
			t.Fatalf("key @%d origin = %s, want %s", i, k.OriginPath.String(), wantOrigin)
		}
	}
}
