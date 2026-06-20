package gui

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// canonicalBip85Master is the standard BIP-85 spec test-vector master seed.
func canonicalBip85Master(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic("install scatter logic circle pencil average fall shoe quantum disease suspect usage")
	if err != nil {
		t.Fatalf("ParseMnemonic(canonical master): %v", err)
	}
	return m
}

// TestDeriveBip85Child_AbandonGoldens pins the BIP-85 BIP-39 children of the
// canonical abandon-about master at index 0 for each word count. A trailing-bytes
// truncation bug, a wrong path element, or an unhardened element all yield a
// different child and fail here.
func TestDeriveBip85Child_AbandonGoldens(t *testing.T) {
	tests := []struct {
		words int
		want  string
	}{
		{12, "prosper short ramp prepare exchange stove life snack client enough purpose fold"},
		{18, "winter brother stamp provide uniform useful doctor prevent venue upper peasant auto view club next clerk tone fox"},
		{24, "stick exact spice sock filter ginger museum horse kit multiply manual wear grief demand derive alert quiz fault december lava picture immune decade jaguar"},
	}
	for _, tc := range tests {
		child, err := deriveBip85Child(abandonAboutMnemonic(), "", tc.words, 0)
		if err != nil {
			t.Fatalf("words=%d: %v", tc.words, err)
		}
		if got := child.String(); got != tc.want {
			t.Fatalf("words=%d child mismatch:\n got %q\nwant %q", tc.words, got, tc.want)
		}
		if len(child) != tc.words {
			t.Fatalf("words=%d: child has %d words", tc.words, len(child))
		}
		if !child.Valid() {
			t.Fatalf("words=%d: child fails BIP-39 checksum", tc.words)
		}
	}
}

// TestDeriveBip85Child_CanonicalVector cross-checks the helper against the
// canonical BIP-85 spec vector (master -> m/83696968'/39'/0'/12'/0').
func TestDeriveBip85Child_CanonicalVector(t *testing.T) {
	child, err := deriveBip85Child(canonicalBip85Master(t), "", 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	const want = "girl mad pet galaxy egg matter matrix prison refuse sense ordinary nose"
	if got := child.String(); got != want {
		t.Fatalf("canonical vector mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestDeriveBip85Child_IndexVaries confirms distinct indices yield distinct
// children (the index participates in the hardened path).
func TestDeriveBip85Child_IndexVaries(t *testing.T) {
	c0, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c0.String() == c1.String() {
		t.Fatal("index 0 and index 1 produced the same child")
	}
	const wantIdx1 = "sing slogan bar group gauge sphere rescue fossil loyal vital model desert"
	if got := c1.String(); got != wantIdx1 {
		t.Fatalf("idx1 child mismatch:\n got %q\nwant %q", got, wantIdx1)
	}
}

// TestDeriveBip85Child_RejectsBadWords: out-of-spec word counts error (never panic).
func TestDeriveBip85Child_RejectsBadWords(t *testing.T) {
	for _, w := range []int{0, 11, 13, 15, 21, 25, 27, -3} {
		if _, err := deriveBip85Child(abandonAboutMnemonic(), "", w, 0); err == nil {
			t.Fatalf("words=%d: expected an error, got nil", w)
		}
	}
}

// TestDeriveBip85Child_RejectsNegativeIndex: a negative index errors.
func TestDeriveBip85Child_RejectsNegativeIndex(t *testing.T) {
	if _, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, -1); err == nil {
		t.Fatal("index=-1: expected an error, got nil")
	}
}

// TestEngraveBip85Child_UsesChildFP asserts the engrave glue stamps the CHILD's
// OWN bare-seed fingerprint (R0-I-A: wrong-identifier-on-permanent-backup) — not
// the master's — and that it engraves the child mnemonic (not the master).
func TestEngraveBip85Child_UsesChildFP(t *testing.T) {
	params := newPlatform().EngraverParams()
	master := abandonAboutMnemonic()
	masterFP, err := masterFingerprintFor(master, &chaincfg.MainNetParams, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := deriveBip85Child(master, "", 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantChildFP, err := masterFingerprintFor(child, &chaincfg.MainNetParams, "")
	if err != nil {
		t.Fatal(err)
	}
	_, gotFP, err := engraveBip85Child(params, child)
	if err != nil {
		t.Fatalf("engraveBip85Child: %v", err)
	}
	if gotFP != wantChildFP {
		t.Fatalf("engraved fp = %08x, want the CHILD's own fp %08x", gotFP, wantChildFP)
	}
	if gotFP == masterFP {
		t.Fatalf("engraved the MASTER's fp %08x — must be the child's own", masterFP)
	}
	// Pin the concrete child fp golden (abandon master, 12 words, idx 0).
	if gotFP != 0x02e8bff2 {
		t.Fatalf("child fp = %08x, want 02e8bff2", gotFP)
	}
}

// TestBip85ParamBounds asserts the picker's choice sets are exactly the in-spec
// bounds (validated-by-construction): word count {12,18,24}, index {0..9}. Any
// drift here (e.g. a 15 or a free-form index) would mint an out-of-spec child.
func TestBip85ParamBounds(t *testing.T) {
	if len(bip85WordChoices) != 3 ||
		bip85WordChoices[0] != 12 || bip85WordChoices[1] != 18 || bip85WordChoices[2] != 24 {
		t.Fatalf("bip85WordChoices = %v, want [12 18 24]", bip85WordChoices)
	}
	if len(bip85IndexChoices) != 10 {
		t.Fatalf("bip85IndexChoices len = %d, want 10 (0..9)", len(bip85IndexChoices))
	}
	for i, v := range bip85IndexChoices {
		if v != i {
			t.Fatalf("bip85IndexChoices[%d] = %d, want %d", i, v, i)
		}
	}
	// Every advertised (words,index) pair derives a valid child (no panic, no error).
	for _, w := range bip85WordChoices {
		for _, idx := range bip85IndexChoices {
			child, err := deriveBip85Child(abandonAboutMnemonic(), "", w, idx)
			if err != nil {
				t.Fatalf("words=%d idx=%d: %v", w, idx, err)
			}
			if len(child) != w || !child.Valid() {
				t.Fatalf("words=%d idx=%d: bad child (%d words, valid=%v)", w, idx, len(child), child.Valid())
			}
		}
	}
}

// TestChildSeedWarningAbort: pressing Back (Button1) at the child-seed warning
// drives ConfirmWarningScreen.Layout -> ConfirmNo, so childSeedWarning returns
// false (abort) and no engrave proceeds. The flow goroutine must actually reach
// and dismiss the warning (NON-vacuous): we keep the frame handle, render the
// warning, click Back, pump frames until the goroutine returns, then assert it
// returned false and that it ran to completion. Mirrors TestDescriptorAddressFlowBackExits.
func TestChildSeedWarningAbort(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got bool
		done := false
		frame, quit := runUI(ctx, func() {
			got = childSeedWarning(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		// Render the warning before driving it (the goroutine blocks on its first
		// ctx.Frame yield until pumped).
		if c, ok := pumpUntil(frame, "Child Seed", 16); !ok {
			t.Fatalf("child-seed warning not shown; got %q", c)
		}
		click(&ctx.Router, Button1) // Back -> ConfirmNo
		// Pump until the warning goroutine returns (the iterator ends).
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("childSeedWarning did not return after Back")
		}
		if got {
			t.Fatal("childSeedWarning returned true after Back; want false (abort)")
		}
	})
}

// TestBip85DeriveFlow_ScrubsBothMnemonics drives the FULL flow: type the abandon
// master, pick the child params (12 words, index 0), confirm the child-seed
// warning, and let the engrave complete; then it asserts BOTH the master and the
// derived child mnemonic []Word slices are zeroed on exit (I-3: two secrets to
// scrub). Mirrors TestEngraveSingleSigFlowSeedScrubbed (the seed-hook + zeroed-
// slice pattern) plus TestEngraveScreen (the connect/hold-confirm/complete dance).
func TestBip85DeriveFlow_ScrubsBothMnemonics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var master, child bip39.Mnemonic
		bip85SeedHook = func(m, c bip39.Mnemonic) { master, child = m, c }
		defer func() { bip85SeedHook = nil }()

		e := newEngraver()
		p := newPlatform()
		p.engraver = e
		ctx := NewContext(p)
		done := false
		frame, quit := runUI(ctx, func() {
			bip85DeriveFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		frame()

		// Master entry: word-count picker -> 12 words (choice 0), then type the
		// abandon-about phrase. (seedEntryFlow's master count is []int{12,24};
		// default index 0 = 12 words, so confirm with Button3.)
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		// Passphrase prompt: Skip (choice 0).
		if c, ok := pumpUntil(frame, "Passphrase", 160); !ok {
			t.Fatalf("did not reach the passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()
		// Param picker: word count = 12 (index 0), child index = 0 (index 0).
		// chooseEntry queues the Down presses, pumps a frame, confirms, pumps again.
		chooseEntry(frame, &ctx.Router, 0) // word count 12
		chooseEntry(frame, &ctx.Router, 0) // child index 0
		// Child-seed warning: hold Button3 to confirm (ConfirmYes).
		if c, ok := pumpUntil(frame, "Child Seed", 160); !ok {
			t.Fatalf("did not reach the child-seed warning; got %q", c)
		}
		press(&ctx.Router, Button3) // hold to confirm
		frame()
		time.Sleep(confirmDelay)
		frame()
		// Engrave screen: click to the connect step, hold to start engraving.
		click(&ctx.Router, Button3, Button3, Button3)
		press(&ctx.Router, Button3) // hold connect
		frame()
		time.Sleep(confirmDelay)
		// Pump until the engrave job closes (completes).
	loop:
		for {
			frame()
			select {
			case <-e.closes:
				break loop
			case <-p.wakeups:
			}
		}
		click(&ctx.Router, Button3) // dismiss the success screen -> Engrave returns true
		synctest.Wait()
		// Drain remaining frames until the flow goroutine returns and the scrub
		// defer has run.
		for i := 0; i < 32 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("bip85DeriveFlow did not return after a completed engrave")
		}
		if master == nil || child == nil {
			t.Fatal("hook never observed both mnemonics")
		}
		for i, w := range master {
			if w != 0 {
				t.Fatalf("master[%d] = %d, not scrubbed on exit (I-3)", i, w)
			}
		}
		for i, w := range child {
			if w != 0 {
				t.Fatalf("child[%d] = %d, not scrubbed on exit (I-3)", i, w)
			}
		}
	})
}

// FuzzDeriveBip85Child asserts the derive helper never panics across arbitrary
// word counts and indices (in-spec and out-of-spec). Out-of-spec inputs must
// return an error, never panic; the bip39.New bounds (16<=len<=32, len%4==0) and
// the bip85.Entropy 32-byte guard must hold for every in-spec path.
func FuzzDeriveBip85Child(f *testing.F) {
	f.Add(12, 0)
	f.Add(18, 5)
	f.Add(24, 9)
	f.Add(15, 0)  // out-of-spec word count
	f.Add(12, -1) // negative index
	f.Add(0, 0)
	f.Fuzz(func(t *testing.T, words, index int) {
		// Must not panic. Errors are fine for out-of-spec inputs.
		child, err := deriveBip85Child(abandonAboutMnemonic(), "", words, index)
		if err != nil {
			return
		}
		// On success the inputs were in-spec; the child must be valid.
		if !validBip85Words(words) || index < 0 {
			t.Fatalf("deriveBip85Child accepted out-of-spec words=%d index=%d", words, index)
		}
		if len(child) != words || !child.Valid() {
			t.Fatalf("words=%d index=%d: invalid child (%d words, valid=%v)", words, index, len(child), child.Valid())
		}
	})
}

// TestParseBip85Index pins the width-safe typed-index validator: it parses base-10
// via strconv.ParseUint (never a bare int), accepts leading zeros, and rejects
// anything > 2^31-1 (the BIP-85 hardened max), non-[0-9] runes, signs, whitespace,
// 0x, and empty input. The >2^31-1 reject is the validator's job, NOT a length cap
// (R0-M2): "9999999999" is 10 digits but still out of range.
func TestParseBip85Index(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"7", 7},
		{"007", 7},          // leading zeros ACCEPTED (R0 adjudication #1)
		{"1000000", 1000000},
		{"2147483647", 2147483647}, // = 2^31-1, the boundary, ACCEPTED
	}
	for _, tc := range ok {
		got, err := parseBip85Index(tc.in)
		if err != nil {
			t.Fatalf("parseBip85Index(%q): unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseBip85Index(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
	bad := []string{
		"",            // empty
		"12a",         // trailing letter
		"a12",         // leading letter
		"-1",          // sign
		"+1",          // sign
		" 1",          // leading whitespace
		"1 ",          // trailing whitespace
		"0x10",        // hex prefix
		"1.0",         // decimal point
		"2147483648",  // = 2^31, first out-of-range value
		"9999999999",  // 10 digits but > 2^31-1 (range, not length, is the authority)
		"9223372036854775808", // > 2^63, ParseUint(…,64) itself overflows
	}
	for _, in := range bad {
		if got, err := parseBip85Index(in); err == nil {
			t.Fatalf("parseBip85Index(%q) = %d, want an error", in, got)
		}
	}
}

// TestDeriveBip85Child_RejectsHighIndex pins the defense-in-depth upper-bound
// guard: an index > 2^31-1 MUST error, never silently truncate. On this 64-bit
// host, 1<<31 and 1<<31+1 fit an int and would otherwise wrap through
// uint32(index)+h into an UNHARDENED element with no error (the R0-reproduced
// bug). The lower bound (-1) still errors with its distinct message (R0-M3).
func TestDeriveBip85Child_RejectsHighIndex(t *testing.T) {
	for _, idx := range []int{1 << 31, 1<<31 + 1} { // 2147483648, 2147483649
		if _, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, idx); err == nil {
			t.Fatalf("index=%d: expected an error (silent uint32 truncation), got nil", idx)
		}
	}
	// Lower bound still errors (retained).
	if _, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, -1); err == nil {
		t.Fatal("index=-1: expected an error, got nil")
	}
}

// TestDeriveBip85Child_HighIndexGolden pins the boundary child at index 2^31-1.
// PROBE-VERIFIED at HEAD 8459654 two independent ways (in-tree derive + biptool's
// bip32.ParsePath path); re-probe-verify at impl time (Task 4 has the command).
// Index 0 stays byte-unchanged vs the shipped golden (typed path is additive).
func TestDeriveBip85Child_HighIndexGolden(t *testing.T) {
	child, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 2147483647) // = 2^31-1
	if err != nil {
		t.Fatalf("index=2147483647: %v", err)
	}
	const want = "jewel solution patient quarter elite grace quarter dinosaur taste parent dial clump"
	if got := child.String(); got != want {
		t.Fatalf("high-index child mismatch:\n got %q\nwant %q", got, want)
	}
	if len(child) != 12 || !child.Valid() {
		t.Fatalf("high-index child: %d words, valid=%v", len(child), child.Valid())
	}
	// Index 0 unchanged vs the shipped golden.
	c0, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 0)
	if err != nil {
		t.Fatalf("index=0: %v", err)
	}
	const want0 = "prosper short ramp prepare exchange stove life snack client enough purpose fold"
	if got := c0.String(); got != want0 {
		t.Fatalf("index-0 child changed:\n got %q\nwant %q", got, want0)
	}
}
