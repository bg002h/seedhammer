package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"seedhammer.com/bip39"
)

// A 24-word phrase, so `6 words`, `3 words` and even/odd are all proper subsets
// and distinguishable from one another. All-distinct words, because a phrase
// with repeats cannot tell "typed the right word" from "typed the word at some
// other position".
// It is bip39.New(sha256("\x00"*8)) — a REAL checksummed 24-word mnemonic,
// found by search rather than written out, because a hand-written phrase does
// not carry a valid checksum and ParseMnemonic refuses it.
const platePhrase = "quantum process dice drink awful runway taste nominee envelope " +
	"debate office bulk tent monkey april game bubble hundred hold govern " +
	"task inject wave more"

func platePhraseMnemonic(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic(platePhrase)
	if err != nil {
		t.Fatalf("the fixture phrase does not parse: %v", err)
	}
	if len(m) != 24 {
		t.Fatalf("the fixture is %d words, want 24", len(m))
	}
	seen := map[bip39.Word]bool{}
	for _, w := range m {
		if seen[w] {
			t.Fatalf("INCONCLUSIVE: the fixture repeats %q, so a right word at the "+
				"wrong position is indistinguishable from a right answer",
				bip39.LabelFor(w))
		}
		seen[w] = true
	}
	return m
}

var wordPrompt = regexp.MustCompile(`word(\d+)of(\d+)`)

// promptedPosition reads which plate position the keyboard is asking for, OFF
// THE TITLE THE OPERATOR IS LOOKING AT. Driving from the rendered prompt rather
// than from the flow's internals is what makes these tests able to fail on a
// prompt that names the wrong word -- which is the whole point of pre-filling
// the untested slots.
func promptedPosition(t *testing.T, content string) (int, bool) {
	t.Helper()
	m := wordPrompt.FindStringSubmatch(strings.ReplaceAll(strings.ToLower(content), " ", ""))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unparsable prompt %q", content)
	}
	return n, true
}

// typePromptedWords answers every prompt the keyboard raises, typing the plate's
// own word except at the 1-indexed positions in `wrong`, where it types a word
// the fixture does not contain. It returns the positions it was asked for, in
// order, so a test can assert WHICH words were checked.
func typePromptedWords(t *testing.T, ctx *Context, frame func() (string, bool),
	m bip39.Mnemonic, wrong map[int]bool, maxFrames int) []int {
	t.Helper()
	var asked []int
	for i := 0; i < maxFrames; i++ {
		content, ok := frame()
		if !ok {
			break
		}
		pos, ok := promptedPosition(t, content)
		if !ok {
			if len(asked) == 0 {
				// The menu is redrawn once while the queued clicks are
				// processed, so entry has not started yet. Breaking here would
				// report "0 words prompted" for a flow that works.
				continue
			}
			break // entry is over; the caller inspects what came next
		}
		if len(asked) > 0 && asked[len(asked)-1] == pos {
			continue // same prompt redrawn before the keystroke landed
		}
		asked = append(asked, pos)
		word := strings.ToLower(bip39.LabelFor(m[pos-1]))
		if wrong[pos] {
			word = "zoo" // in the wordlist, not in the fixture
		}
		runes(&ctx.Router, word)
		click(&ctx.Router, Button3)
	}
	return asked
}

type plateVerifyRun struct {
	ctx   *Context
	frame func() (string, bool)
	quit  func()
	menu  string
	got   *verifyProvenance
}

// startPlateVerify runs the flow and stops at the §7.2 depth menu.
func startPlateVerify(t *testing.T, m bip39.Mnemonic) *plateVerifyRun {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	got := new(verifyProvenance)
	frame, _, quit := runUITouch(ctx, func() {
		*got = plateVerifyFlow(ctx, &descriptorTheme, m)
	})
	content, ok := pumpUntil(frame, "every word", 32)
	if !ok {
		quit()
		t.Fatalf("the §7.2 menu never appeared; got %q", content)
	}
	return &plateVerifyRun{ctx: ctx, frame: frame, quit: quit, menu: content, got: got}
}

// drain runs the flow to completion so the return value is actually produced --
// quit() would cancel the coroutine mid-flow and an assertion on the outcome
// could then never fail.
func (r *plateVerifyRun) drain() {
	for i := 0; i < 32; i++ {
		if _, more := r.frame(); !more {
			return
		}
	}
}

// SPEC §7.2, all its rows, labels verbatim -- including `skip`, which is
// NORMATIVE (R1-I1): without it §7.1.1's `not verified` outcome is unreachable
// by any path an implementer transcribing the table would build.
func TestPlateVerifyMenuOffersEveryRowSpecNames(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()
	for _, row := range []string{
		"every word", "even words", "odd words", "6 words", "3 words",
		"read only", "skip",
	} {
		if !uiContains(r.menu, row) {
			t.Errorf("§7.2's %q row is missing from the menu; got %q", row, r.menu)
		}
	}
}

// §7.1.1, and spec test 17. The four provenances must be DISTINGUISHABLE, and
// none of the three that are not a bypass may render as the bare word
// "verified" -- an operator assertion and a device comparison are different
// facts, and collapsing them is F-123's over-claim, about the plate this time.
func TestVerifyProvenanceIsNeverRenderedAsVerified(t *testing.T) {
	lines := map[verifyProvenance]string{
		provDeviceComparedAll:    verifyProvenanceLine(provDeviceComparedAll, 24, 24),
		provDeviceComparedSubset: verifyProvenanceLine(provDeviceComparedSubset, 6, 24),
		provOperatorAsserted:     verifyProvenanceLine(provOperatorAsserted, 0, 24),
		provNotVerified:          verifyProvenanceLine(provNotVerified, 0, 24),
	}
	want := map[verifyProvenance]string{
		provDeviceComparedAll:    "device-compared (every word)",
		provDeviceComparedSubset: "device-compared (6 of 24)",
		provOperatorAsserted:     "operator-asserted",
		provNotVerified:          "not verified",
	}
	for p, got := range lines {
		if got != want[p] {
			t.Errorf("provenance %d renders %q, want §7.1.1's %q", p, got, want[p])
		}
	}
	seen := map[string]bool{}
	for p, s := range lines {
		if seen[s] {
			t.Errorf("provenance %d shares its rendering %q with another -- the four "+
				"must be distinguishable in what the flow displays", p, s)
		}
		seen[s] = true
	}
	// The forbidden collapse: anything that is not the bypass claiming the bare
	// word "verified" for itself.
	for _, p := range []verifyProvenance{provDeviceComparedAll, provDeviceComparedSubset,
		provOperatorAsserted} {
		if strings.Contains(lines[p], "verified") {
			t.Errorf("provenance %d renders %q, which claims the bare word "+
				"\"verified\"", p, lines[p])
		}
	}
	// `not verified` is the one that must contain it, because that IS §7.1.1's
	// string. Asserted so a rewrite to e.g. "unchecked" fails here rather than
	// drifting off the normative table.
	if lines[provNotVerified] != "not verified" {
		t.Errorf("the bypass outcome renders %q, not §7.1.1's \"not verified\"",
			lines[provNotVerified])
	}
}

// §7.2.1: even/odd are 1-INDEXED over the engraved word list. An implementer
// picking the other indexing produces a verify that silently checks the
// COMPLEMENT of what its label says -- and that passes every "does it compare
// words" test there is.
func TestVerifyEvenAndOddAreOneIndexed(t *testing.T) {
	const total = 24
	even := verifyPositions(depthEvenWords, total)
	odd := verifyPositions(depthOddWords, total)
	if len(even) == 0 || even[0] != 1 {
		t.Errorf("`even words` starts at index %v, want 1 (the 1-indexed word 2)", even)
	}
	if len(odd) == 0 || odd[0] != 0 {
		t.Errorf("`odd words` starts at index %v, want 0 (the 1-indexed word 1)", odd)
	}
	if len(even) != total/2 || len(odd) != total/2 {
		t.Errorf("even/odd are %d/%d of %d words, want half each", len(even), len(odd), total)
	}
	all := map[int]bool{}
	for _, i := range append(append([]int{}, even...), odd...) {
		if all[i] {
			t.Errorf("position %d is in both halves", i)
		}
		all[i] = true
	}
	if len(all) != total {
		t.Errorf("even and odd together cover %d of %d positions", len(all), total)
	}
	if got := len(verifyPositions(depthEvery, total)); got != total {
		t.Errorf("`every word` covers %d of %d", got, total)
	}
	if got := len(verifyPositions(depthSix, total)); got != 6 {
		t.Errorf("`6 words` draws %d", got)
	}
	if got := len(verifyPositions(depthThree, total)); got != 3 {
		t.Errorf("`3 words` draws %d", got)
	}
	// The two rows that check nothing must draw nothing, or a `skip` that
	// prompted for a word would be a bypass that is not one.
	for _, d := range []int{depthReadOnly, depthSkip} {
		if got := verifyPositions(d, total); got != nil {
			t.Errorf("depth %d selects positions %v; it checks no words", d, got)
		}
	}
}

// Spec test 3, half one: the draw is UNIFORM and WITHOUT REPLACEMENT. A
// predictable or skewed draw lets an attacker who controls the plate know which
// positions go unchecked.
func TestPlateVerifyDrawIsUniformAndWithoutReplacement(t *testing.T) {
	const (
		total  = 24
		k      = 3
		trials = 20000
	)
	counts := make([]int, total)
	for i := 0; i < trials; i++ {
		pos := drawPositions(total, k)
		if len(pos) != k {
			t.Fatalf("draw %d returned %d positions, want %d", i, len(pos), k)
		}
		seen := map[int]bool{}
		for _, p := range pos {
			if p < 0 || p >= total {
				t.Fatalf("draw %d produced out-of-range position %d", i, p)
			}
			if seen[p] {
				t.Fatalf("draw %d repeated position %d -- the draw is WITH replacement", i, p)
			}
			seen[p] = true
			counts[p]++
		}
	}
	expect := float64(trials*k) / float64(total)
	for p, c := range counts {
		if d := float64(c) / expect; d < 0.9 || d > 1.1 {
			t.Errorf("position %d drawn %d times, expected ~%.0f (%.2fx) -- the draw is "+
				"not uniform", p, c, expect, d)
		}
	}
	if got := len(drawPositions(total, total+5)); got != total {
		t.Errorf("a k larger than the word count drew %d positions, want %d", got, total)
	}
}

// Spec test 3, half two, AT THE FLOW: the draw is fresh for every verification
// ATTEMPT. A fixed set would let a second attempt pass by reciting the same
// positions, which is a memory test rather than a plate check.
//
// Driven through the real screens, because the property is about WHERE the draw
// sits: a correct drawPositions hoisted out of the retry loop passes the unit
// test above and fails this one.
//
// Four attempts, and it is enough for at least one to differ. Two independent
// 3-of-24 draws collide with probability 1/2024, so three retries all matching
// the first is about 1 in 10^10 -- rarer than the suite is ever run.
func TestPlateVerifyRedrawsOnEveryAttempt(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	for i := 0; i < 4; i++ {
		click(&r.ctx.Router, Down) // -> `3 words`
	}
	click(&r.ctx.Router, Button3)

	var attempts [][]int
	const rounds = 4
	for round := 0; round < rounds; round++ {
		// Every prompted word is answered WRONGLY, so the attempt fails and a
		// retry is offered without the test having to know which positions were
		// drawn before it types.
		asked := typePromptedWords(t, r.ctx, r.frame, m, allWrong(), 24)
		if len(asked) != 3 {
			t.Fatalf("attempt %d prompted %d words, want 3 (%v)", round, len(asked), asked)
		}
		attempts = append(attempts, asked)
		if content, ok := pumpUntil(r.frame, "did not match", 32); !ok {
			t.Fatalf("attempt %d was not reported as a mismatch; got %q", round, content)
		}
		if round == rounds-1 {
			break
		}
		click(&r.ctx.Router, Down)    // GIVE UP -> RETRY
		click(&r.ctx.Router, Button3) // retry
	}
	same := 0
	for _, a := range attempts[1:] {
		if equalInts(a, attempts[0]) {
			same++
		}
	}
	if same == len(attempts)-1 {
		t.Errorf("every retry drew the same positions %v -- the draw is hoisted out of "+
			"the attempt loop, so a second try is a recital rather than a check",
			attempts[0])
	}
	t.Logf("attempts: %v", attempts)
}

// allWrong marks every 1-indexed position of a 24-word plate as one to answer
// wrongly, so a test can fail an attempt without first knowing which positions
// the draw picked.
func allWrong() map[int]bool {
	w := make(map[int]bool, 24)
	for i := 1; i <= 24; i++ {
		w[i] = true
	}
	return w
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Spec test 2: A WRONG WORD AT ANY POSITION INCLUDED IN A CHECK IS CAUGHT -- and
// the failure names the position, so the operator knows which plate word to look
// at.
func TestPlateVerifyCatchesAWrongWordAtACheckedPosition(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	click(&r.ctx.Router, Button3) // `every word`, row 0
	asked := typePromptedWords(t, r.ctx, r.frame, m, map[int]bool{8: true}, 80)
	if len(asked) != len(m) {
		t.Fatalf("`every word` prompted %d of %d words: %v", len(asked), len(m), asked)
	}
	content, ok := pumpUntil(r.frame, "did not match", 32)
	if !ok {
		t.Fatalf("a wrong word at a CHECKED position was not caught; got %q", content)
	}
	if !uiContains(content, "Word 8") {
		t.Errorf("the failure does not name position 8; got %q", content)
	}
	click(&r.ctx.Router, Button1) // Back out of the mismatch screen -> the menu
	if content, ok = pumpUntil(r.frame, "every word", 32); !ok {
		t.Fatalf("the mismatch screen did not return to the menu; got %q", content)
	}
	for i := 0; i < 6; i++ {
		click(&r.ctx.Router, Down) // -> `skip`
	}
	click(&r.ctx.Router, Button3)
	if content, ok = pumpUntil(r.frame, "not verified", 32); !ok {
		t.Fatalf("`skip` did not render the bypass outcome; got %q", content)
	}
	click(&r.ctx.Router, Button3) // dismiss
	r.drain()
	if *r.got != provNotVerified {
		t.Errorf("outcome = %v, want provNotVerified", *r.got)
	}
}

// The other side of test 2, which the failing case alone cannot prove: a
// correctly-read plate PASSES, with the right provenance. Without this, a flow
// that reported every plate wrong would satisfy the test above.
func TestPlateVerifyPassesAnUnalteredPlateAndNamesTheProvenance(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	click(&r.ctx.Router, Button3) // `every word`
	asked := typePromptedWords(t, r.ctx, r.frame, m, map[int]bool{}, 80)
	if len(asked) != len(m) {
		t.Fatalf("prompted %d of %d words: %v", len(asked), len(m), asked)
	}
	// And it asked for EVERY position, in order -- a flow that prompted 24 times
	// for the same word would satisfy the count alone.
	for i, p := range asked {
		if p != i+1 {
			t.Fatalf("`every word` prompted %v, want 1..%d", asked, len(m))
		}
	}
	content, ok := pumpUntil(r.frame, "device-compared", 32)
	if !ok {
		t.Fatalf("a correct plate was not reported as device-compared; got %q", content)
	}
	if !uiContains(content, "every word") {
		t.Errorf("a full comparison is not named as such; got %q", content)
	}
	if uiContains(content, "did not match") {
		t.Errorf("a correct plate reported a mismatch; got %q", content)
	}
	click(&r.ctx.Router, Button3)
	r.drain()
	if *r.got != provDeviceComparedAll {
		t.Errorf("outcome = %v, want provDeviceComparedAll", *r.got)
	}
}

// A SUBSET reports itself as a subset, with its size. `device-compared (6 of
// 24)` and `device-compared (every word)` are different facts, and §7.3 gives
// them very different detection rates -- 25% versus 100% for one wrong word on a
// 24-word seed.
func TestPlateVerifySubsetNamesItsSize(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	for i := 0; i < 3; i++ {
		click(&r.ctx.Router, Down) // -> `6 words`
	}
	click(&r.ctx.Router, Button3)
	asked := typePromptedWords(t, r.ctx, r.frame, m, map[int]bool{}, 32)
	if len(asked) != 6 {
		t.Fatalf("`6 words` prompted %d words: %v", len(asked), asked)
	}
	content, ok := pumpUntil(r.frame, "device-compared", 32)
	if !ok {
		t.Fatalf("no outcome after a passing subset; got %q", content)
	}
	if !uiContains(content, "6 of 24") {
		t.Errorf("the subset outcome does not name its size; got %q", content)
	}
	if uiContains(content, "every word") {
		t.Errorf("a 6-word check reported itself as a full comparison; got %q", content)
	}
	click(&r.ctx.Router, Button3)
	r.drain()
	if *r.got != provDeviceComparedSubset {
		t.Errorf("outcome = %v, want provDeviceComparedSubset", *r.got)
	}
}

// `read only` is an operator ASSERTION (§7.1): the operator declares they
// checked by eye and it passed, and the device records the declaration WITHOUT
// pretending to have confirmed it.
func TestPlateVerifyReadOnlyIsRecordedAsAnAssertion(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	for i := 0; i < 5; i++ {
		click(&r.ctx.Router, Down) // -> `read only`
	}
	click(&r.ctx.Router, Button3)
	// The words are shown, because a by-eye comparison needs something to
	// compare the steel against.
	content, ok := pumpUntil(r.frame, bip39.LabelFor(m[0]), 32)
	if !ok {
		t.Fatalf("`read only` never showed the engraved words; got %q", content)
	}
	click(&r.ctx.Router, Button3) // confirm the word list
	if content, ok = pumpUntil(r.frame, "by eye", 32); !ok {
		t.Fatalf("`read only` never asked the operator to assert; got %q", content)
	}
	if !uiContains(content, "did not check") {
		t.Errorf("the assertion screen does not say the DEVICE did not check; got %q", content)
	}
	click(&r.ctx.Router, Down)    // NO -> IT MATCHED
	click(&r.ctx.Router, Button3) // assert
	if content, ok = pumpUntil(r.frame, "operator-asserted", 32); !ok {
		t.Fatalf("the assertion was not recorded as such; got %q", content)
	}
	if uiContains(content, "device-compared") {
		t.Errorf("an operator assertion was rendered as a device comparison; got %q", content)
	}
	click(&r.ctx.Router, Button3)
	r.drain()
	if *r.got != provOperatorAsserted {
		t.Errorf("outcome = %v, want provOperatorAsserted", *r.got)
	}
}

// BACK AT THE MENU IS `not verified`, and §7.1.1 has no fifth provenance for
// "the operator left". Found by mutation: replacing the Back arm with a device
// comparison SURVIVED the suite, because nothing drove Back at the depth menu --
// the highest-value outcome to get wrong, since it silently certifies a plate
// nobody checked.
func TestPlateVerifyBackAtTheMenuIsNotVerified(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	click(&r.ctx.Router, Button1) // Back / cancel at the ChoiceScreen
	content, ok := pumpUntil(r.frame, "not verified", 32)
	if !ok {
		t.Fatalf("Back at the depth menu did not record a bypass; got %q", content)
	}
	if uiContains(content, "device-compared") {
		t.Errorf("Back at the menu was recorded as a DEVICE COMPARISON -- a plate "+
			"nobody checked, certified; got %q", content)
	}
	click(&r.ctx.Router, Button3) // dismiss
	r.drain()
	if *r.got != provNotVerified {
		t.Errorf("outcome = %v, want provNotVerified", *r.got)
	}
}

// DECLINING the by-eye assertion is not an assertion. Found by mutation:
// returning true unconditionally from the assertion screen SURVIVED, because
// every test took the accepting branch -- and the whole point of §7.1.1 is that
// the device records what the operator DECLARED, not what it hoped they meant.
func TestPlateVerifyDecliningTheByEyeCheckAssertsNothing(t *testing.T) {
	m := platePhraseMnemonic(t)
	r := startPlateVerify(t, m)
	defer r.quit()

	for i := 0; i < 5; i++ {
		click(&r.ctx.Router, Down) // -> `read only`
	}
	click(&r.ctx.Router, Button3)
	content, ok := pumpUntil(r.frame, bip39.LabelFor(m[0]), 32)
	if !ok {
		t.Fatalf("`read only` never showed the engraved words; got %q", content)
	}
	click(&r.ctx.Router, Button3) // confirm the word list
	if content, ok = pumpUntil(r.frame, "by eye", 32); !ok {
		t.Fatalf("no assertion screen; got %q", content)
	}
	click(&r.ctx.Router, Button3) // choice 0 == NO, the resting position
	// Declining returns to the DEPTH MENU rather than recording anything.
	if content, ok = pumpUntil(r.frame, "every word", 32); !ok {
		t.Fatalf("declining the by-eye check did not return to the menu; got %q", content)
	}
	if uiContains(content, "operator-asserted") {
		t.Errorf("a declined by-eye check was recorded as an assertion; got %q", content)
	}
	for i := 0; i < 6; i++ {
		click(&r.ctx.Router, Down) // -> `skip`
	}
	click(&r.ctx.Router, Button3)
	if content, ok = pumpUntil(r.frame, "not verified", 32); !ok {
		t.Fatalf("`skip` after a declined assertion did not bypass; got %q", content)
	}
	click(&r.ctx.Router, Button3)
	r.drain()
	if *r.got != provNotVerified {
		t.Errorf("outcome = %v, want provNotVerified", *r.got)
	}
}

// Spec test 1 (§7.4): the session cache must NEVER answer a verification prompt
// on the operator's behalf. Asserted STRUCTURALLY, over the file, because what
// is wanted is "there is no way to reach it" rather than "this particular run
// did not" -- R0-C1 showed a behavioural test can be satisfied at the session
// layer while the UI still offers the option.
func TestPlateVerifyComparesAgainstTheEngravedMnemonicNotTheSession(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "plate_verify.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing plate_verify.go: %v", err)
	}
	forbidden := map[string]string{
		"syswOffer":   "would offer the payload as a source at a verify prompt",
		"take":        "would take a cached record as the operator's answer",
		"sysw":        "would give the verify a way to name the session at all",
		"syswSession": "would give the verify a way to name the session at all",
	}
	var names int
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		names++
		if why, bad := forbidden[id.Name]; bad {
			t.Errorf("plate_verify.go names %q, which %s (§7.4)", id.Name, why)
		}
		return true
	})
	if names == 0 {
		t.Fatal("INCONCLUSIVE: no identifiers were walked, so this test guards nothing")
	}
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Name.Name == "plateVerifyFlow" {
			found = true
		}
		return true
	})
	if !found {
		t.Error("plate_verify.go no longer declares plateVerifyFlow, so this file is " +
			"not the one under test")
	}
}

// THE FLOW IS WIRED, not merely built. A verify nothing calls is this feature's
// recurring defect -- F-144 at feature level, and §8c's `done` button at widget
// level, which was constructed and handled and never drawn.
//
// Asserted over backupWalletFlow's AST: a grep for the identifier would pass on
// a call sitting in a function nobody reaches.
func TestBackupWalletOffersThePlateVerifyAfterTheCut(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "gui.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing gui.go: %v", err)
	}
	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		d, ok := n.(*ast.FuncDecl)
		if ok && d.Name.Name == "backupWalletFlow" {
			fn = d
		}
		return true
	})
	if fn == nil {
		t.Fatal("gui.go no longer declares backupWalletFlow")
	}
	// The call must sit INSIDE the branch taken when Engrave reported the plate
	// completed -- offering a verify for a cut that did not happen is a prompt
	// about nothing.
	var insideEngraveBranch bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		var engrave bool
		ast.Inspect(is.Cond, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "Engrave" {
				engrave = true
			}
			return true
		})
		if !engrave {
			return true
		}
		ast.Inspect(is.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "plateVerifyFlow" {
				insideEngraveBranch = true
			}
			return true
		})
		return true
	})
	if !insideEngraveBranch {
		t.Error("backupWalletFlow does not offer plateVerifyFlow after a completed " +
			"engrave -- the flow exists and nothing reaches it, which is F-144's shape")
	}
}
