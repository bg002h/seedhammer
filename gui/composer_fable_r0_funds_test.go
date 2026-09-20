package gui

// Lens-1 (funds safety and policy correctness) tests written for the composer
// fable review r0, adopted from the reviewer's preserved harness
// (`/scratch/code/shibboleth/.tmp/fable-funds-work/harness/`, three
// `zz_fable_funds_*_test.go` files, package gui).
//
// The three RED-at-tip assertions are kept verbatim in substance -- each one
// was the counterexample for a finding -- and the reviewer's FABLE_OUT-gated
// evidence drivers (JSON dumps for the host and Bitcoin Core legs) are not
// adopted: they assert nothing and skip unless an env var is set. The helpers
// they shared (`fableSeeds`, the two preimages, `fableHash`) are kept because
// the assertions use them.
//
// C-1 additionally asserts AGAINST THE HOST ORACLE: `md compose --wrapper wsh
// --experimental` is the Rust primary, and the Go port must admit exactly the
// lists it admits. The oracle leg skips when `md` is not on PATH; the literal
// table beside it does not, so the rule is pinned with or without the CLI.

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"seedhammer.com/md"
)

var fableFundsSeeds = []string{
	"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
	"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
}

var fableFundsPreimage = sha256.Sum256([]byte("fable-funds-preimage-1"))
var fableFundsPreimage2 = sha256.Sum256([]byte("fable-funds-preimage-2"))

func fableFundsHash(t *testing.T, pre [32]byte) *md.HashLock {
	t.Helper()
	d := sha256.Sum256(pre[:])
	h, ok := md.NewHashLock(md.KindSha256, d[:])
	if !ok {
		t.Fatal("md.NewHashLock refused a 32-byte sha256 digest")
	}
	return h
}

// ─── C-1: at most ONE key-less path per policy ──────────────────────────────

// fableKeylessCase is one path list, named the way `md compose --path` names
// it, so the oracle leg can be built from the same row.
type fableKeylessCase struct {
	what  string
	args  []string // the --path arguments, in listed order
	paths []md.SpendPath
	admit bool // what `md compose --wrapper wsh --experimental` does
}

func fableKeylessCases(t *testing.T) []fableKeylessCase {
	t.Helper()
	H := fableFundsHash(t, fableFundsPreimage)
	H2 := fableFundsHash(t, fableFundsPreimage2)
	h1 := fmt.Sprintf("sha256=%x", H.Digest())
	h2 := fmt.Sprintf("sha256=%x", H2.Digest())
	keyed := func(k, n uint8) md.SpendPath {
		return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}}
	}
	older := func(n uint32) *md.Lock { return &md.Lock{Kind: md.LockOlderBlocks, Value: n} }
	after := func(h uint32) *md.Lock { return &md.Lock{Kind: md.LockAfterHeight, Value: h} }
	return []fableKeylessCase{
		// REFUSED: two or more key-less paths, whatever locks they carry and
		// wherever they sit. Measured against md 0.16.2.
		{"[keyed, K, K]", []string{"1of1", "keyless," + h1, "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, {Hash: H2}}, false},
		{"[keyed, K, K+older(5)]", []string{"1of1", "keyless," + h1, "keyless," + h2 + ",older=5"},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, {Hash: H2, Lock: older(5)}}, false},
		{"[K, K+older(5), keyed]", []string{"keyless," + h1, "keyless," + h2 + ",older=5", "1of1"},
			[]md.SpendPath{{Hash: H}, {Hash: H2, Lock: older(5)}, keyed(1, 1)}, false},
		{"[keyed, K+older(5), K+after(200)]", []string{"1of1", "keyless," + h1 + ",older=5", "keyless," + h2 + ",after=200"},
			[]md.SpendPath{keyed(1, 1), {Hash: H, Lock: older(5)}, {Hash: H2, Lock: after(200)}}, false},
		{"[K+older(5), keyed, K+after(200)]", []string{"keyless," + h1 + ",older=5", "1of1", "keyless," + h2 + ",after=200"},
			[]md.SpendPath{{Hash: H, Lock: older(5)}, keyed(1, 1), {Hash: H2, Lock: after(200)}}, false},
		{"[keyed, 2of2, K, K]", []string{"1of1", "2of2", "keyless," + h1, "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), keyed(2, 2), {Hash: H}, {Hash: H2}}, false},
		{"[keyed, K, 2of2, K]", []string{"1of1", "keyless," + h1, "2of2", "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, keyed(2, 2), {Hash: H2}}, false},
		// ADMITTED: at most one key-less path.
		{"[K+older(5), 2of3]", []string{"keyless," + h1 + ",older=5", "2of3"},
			[]md.SpendPath{{Hash: H, Lock: older(5)}, keyed(2, 3)}, true},
		{"[keyed, 2of2, K]", []string{"1of1", "2of2", "keyless," + h1},
			[]md.SpendPath{keyed(1, 1), keyed(2, 2), {Hash: H}}, true},
		{"[keyed, K]", []string{"1of1", "keyless," + h1},
			[]md.SpendPath{keyed(1, 1), {Hash: H}}, true},
		{"[K, keyed]", []string{"keyless," + h1, "1of1"},
			[]md.SpendPath{{Hash: H}, keyed(1, 1)}, true},
		{"[keyed, K+older(5)]", []string{"1of1", "keyless," + h1 + ",older=5"},
			[]md.SpendPath{keyed(1, 1), {Hash: H, Lock: older(5)}}, true},
	}
}

// TestFableRedTwoKeylessPathsAreRefused is lens-1 C-1.
//
// `or_i(l, r)` is non-malleable only if one arm is `safe` (needs a signature),
// and a key-less path is never safe -- so two of them anywhere in the list put
// two unsafe arms under one `or_i` in §5's right-leaning chain, whatever locks
// they carry. The primary catches it by re-parsing its own lowering through
// `md encode`; this port has no post-lowering parse (md/compose.go:1-22
// "It emits no text"), so the rule is stated in ValidatePathList instead.
func TestFableRedTwoKeylessPathsAreRefused(t *testing.T) {
	for _, c := range fableKeylessCases(t) {
		list := md.PathList{Wrapper: md.ComposeWsh, Paths: c.paths}
		_, err := md.ValidatePathList(list)
		switch {
		case c.admit && err != nil:
			t.Errorf("ValidatePathList refused %s (%v); md compose --experimental admits it", c.what, err)
		case !c.admit && err == nil:
			t.Errorf("ValidatePathList admitted %s; md compose --experimental refuses it as malleable", c.what)
		case !c.admit && !errors.Is(err, md.ErrComposeTwoKeylessPaths):
			t.Errorf("ValidatePathList refused %s with %v, want ErrComposeTwoKeylessPaths", c.what, err)
		}
	}
}

// TestFableTwoKeylessPathsAgreeWithTheHostOracle runs the SAME lists through
// `md compose` and requires the Go port's answer to match arm for arm.
//
// The Rust primary is normative (CLAUDE.md, Rust-primary rule), so the table
// above is only as good as its agreement with the CLI. Skipped where `md` is
// absent -- the table still runs.
func TestFableTwoKeylessPathsAgreeWithTheHostOracle(t *testing.T) {
	bin, err := exec.LookPath("md")
	if err != nil {
		t.Skip("md is not on PATH; the literal table in TestFableRedTwoKeylessPathsAreRefused still runs")
	}
	for _, c := range fableKeylessCases(t) {
		args := []string{"compose", "--wrapper", "wsh", "--experimental"}
		for _, p := range c.args {
			args = append(args, "--path", p)
		}
		out, cerr := exec.Command(bin, args...).CombinedOutput()
		oracleAdmits := cerr == nil
		if oracleAdmits != c.admit {
			t.Fatalf("the table says md compose admits=%v for %s, the CLI says %v:\n%s",
				c.admit, c.what, oracleAdmits, out)
		}
		_, gerr := md.ValidatePathList(md.PathList{Wrapper: md.ComposeWsh, Paths: c.paths})
		if (gerr == nil) != oracleAdmits {
			t.Errorf("%s: md compose admits=%v, md.ValidatePathList admits=%v (%v)",
				c.what, oracleAdmits, gerr == nil, gerr)
		}
	}
}
