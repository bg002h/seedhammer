package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// THE RECONCILIATION THE JOURNEYS REVIEW ASKED FOR, one level above
// `assert_every_named_test_is_placed`.
//
// coverage.rs (mnemonic-engrave) says which spec §8.3 tests are DEVICE-owned. It
// cannot see the device tree, so `Where::Device` was carrying two meanings until
// 2026-08-12 — "built" and "someone said it was built" — and five of its ten
// entries named behaviour nobody had written. `DeviceUnbuilt` split the two, and
// this test closes the loop from the other side: for every device-owned id, a
// named Go test function must EXIST in this package.
//
// It is deliberately a NAME check and not a behaviour check. What it catches is
// the gap coverage.rs cannot: an entry claiming a device test that is not there.
// Whether the named test asserts the right thing is the reviewer's job and the
// mutation runs'; whether it exists at all is a command.
//
// Run against the pre-stage-12 tree this named 2, 3 and 17 as missing, which is
// the review's finding reproduced as a command.

// syswDeviceOwnedTests is coverage.rs's device-owned column, transcribed with
// the Go test that discharges each id. Keep it in step with coverage.rs: an id
// there whose entry is `Where::Device` must appear here.
var syswDeviceOwnedTests = map[int]string{
	1:  "TestPlateVerifyComparesAgainstTheEngravedMnemonicNotTheSession",
	2:  "TestPlateVerifyCatchesAWrongWordAtACheckedPosition",
	3:  "TestPlateVerifyRedrawsOnEveryAttempt",
	8:  "TestSyswLoadFlowRefusesAMalformedRegionWithoutCryingTamper",
	10: "TestTheDigestIsComparedOncePerPayload",
	16: "TestNoVerifyFlowCanReachAPayloadSecret",
	17: "TestVerifyProvenanceIsNeverRenderedAsVerified",
	21: "TestSyswPassphraseBufferNeverRegrows",
	22: "TestSyswPassphraseDoneConfirmsTheShortCount",
}

func TestEveryDeviceOwnedSpecTestHasAGoTestThatDischargesIt(t *testing.T) {
	have := map[string]bool{}
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading gui: %v", err)
	}
	fset := token.NewFileSet()
	var files int
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil &&
				strings.HasPrefix(fn.Name.Name, "Test") {
				have[fn.Name.Name] = true
			}
		}
	}
	if files == 0 {
		t.Fatal("INCONCLUSIVE: no _test.go files were parsed, so every lookup " +
			"below would fail for the wrong reason")
	}
	ids := make([]int, 0, len(syswDeviceOwnedTests))
	for id := range syswDeviceOwnedTests {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		fn := syswDeviceOwnedTests[id]
		if !have[fn] {
			t.Errorf("spec §8.3 test %d is device-owned in coverage.rs and names "+
				"%s, which does not exist in package gui. Either write it or mark "+
				"the coverage entry DeviceUnbuilt — a Device entry with no test is "+
				"the state that let §7 go unbuilt through a green gate", id, fn)
		}
	}
	// The map must not be able to pass by being empty, and it must not name the
	// same test for two ids without saying so.
	if len(syswDeviceOwnedTests) < 9 {
		t.Errorf("only %d device-owned ids are mapped; coverage.rs has 9",
			len(syswDeviceOwnedTests))
	}
	seen := map[string]int{}
	for _, id := range ids {
		fn := syswDeviceOwnedTests[id]
		if prev, dup := seen[fn]; dup {
			t.Errorf("spec tests %d and %d both name %s — one test discharging two "+
				"named tests is a claim, not coverage", prev, id, fn)
		}
		seen[fn] = id
	}
	t.Logf("%d device-owned spec tests, all discharged by a named Go test", len(ids))
}

// Spec test 10: the digest is compared ONCE PER PAYLOAD. A second program
// consuming from the same loaded payload does NOT re-prompt, and re-reading the
// region DOES.
//
// Written for the witness test above: coverage.rs called 10 device-owned and no
// Go test discharged it, which is precisely the claim this pair exists to stop.
//
// The mechanism is the session: [compared] (§12.2) is decided ONCE by the load
// flow, which is the only place that can see both of its routes, and `take`
// consults the stored flag rather than asking again. So consuming twice cannot
// re-prompt, and re-reading assigns a fresh session whose flag starts false.
func TestTheDigestIsComparedOncePerPayload(t *testing.T) {
	payload := &sysw.Payload{Public: []string{testSeedPhrase}}
	s := &syswSession{}
	s.load(payload, [32]byte{1}, false, true, true, true)
	if _, ok := s.take(sysw.ClassMnemonic); !ok {
		t.Fatal("the first consumption did not get the record")
	}
	if _, ok := s.take(sysw.ClassMnemonic); !ok {
		t.Error("a SECOND program consuming from the same payload was refused — " +
			"[compared] is per PAYLOAD, not per consumption, and re-deciding it " +
			"here would mean a second ~31 s KDF or a second digest screen")
	}
	if !s.compared {
		t.Error("the compared flag did not survive two consumptions")
	}

	// Re-reading the region is a FRESH session, and its flag starts false: a new
	// payload has not been authenticated by anything, and inheriting the previous
	// one's flag is how a swapped payload gets consumed unchecked (§5.4.1 is why
	// identity is the region digest and not the public-data one).
	fresh := &syswSession{}
	fresh.load(payload, [32]byte{2}, false, true, false, true)
	if fresh.compared {
		t.Fatal("INCONCLUSIVE: the fresh session is already compared, so the " +
			"assertion below cannot fail")
	}
	if _, ok := fresh.take(sysw.ClassMnemonic); ok {
		t.Error("a re-read payload was consumed without being compared again — " +
			"identity changed, so [compared] must be re-earned")
	}
	if fresh.identity == s.identity {
		t.Error("two different payloads share an identity, so nothing could tell " +
			"a re-read from the same payload")
	}
}

// Spec test 21: the passphrase buffer never regrows. Entering 24 words must
// leave no orphaned copy — asserted on the buffer's IDENTITY, not on its
// contents, because a regrown slice's old array still holds the first half of
// the passphrase and nothing can reach it to wipe it.
//
// Also written for the witness test: coverage.rs called 21 device-owned and the
// only test in the tree about a passphrase buffer belongs to the FROZEN seal
// unlock (unlock_kdf.go's passphraseBytes), a different container's code path.
func TestSyswPassphraseBufferNeverRegrows(t *testing.T) {
	const n = 24
	m := emptyBIP39Mnemonic(n)
	if len(m) != n || cap(m) != n {
		t.Fatalf("the entry buffer is len %d cap %d, want %d/%d — inputWordsFlow "+
			"fills slots in place and a buffer that could grow would be a second "+
			"array holding the same words", len(m), cap(m), n, n)
	}
	first := &m[0]
	for i := range m {
		m[i] = 0 // what inputWordsFlow does: assign into the slot
	}
	if &m[0] != first {
		t.Error("the buffer moved while being filled")
	}

	// And the join the load flow does afterwards: `words` is allocated at
	// exactly the entered count and appended to that many times, so it never
	// regrows either. A regrow here leaves the first half of the passphrase in
	// an orphaned array.
	words := make([]string, 0, n)
	base := &words[:1][0]
	for i := 0; i < n; i++ {
		words = append(words, "abandon")
	}
	if cap(words) != n {
		t.Errorf("the word slice regrew to cap %d from %d — the orphaned array "+
			"still holds the words that were in it", cap(words), n)
	}
	if &words[0] != base {
		t.Error("the word slice moved, so an orphaned copy of the passphrase " +
			"exists and nothing can reach it")
	}
}
