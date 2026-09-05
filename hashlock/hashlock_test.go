package hashlock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const corpusPath = "testdata/hashlock-v0.8.json"
const corpusSHA256 = "a46c197a3640fe8af4ca4370b46a9637466649227163ce6761bb032354811d30"

type corpus struct {
	Derivation []struct {
		Phrase      string `json:"phrase"`
		PhraseChars int    `json:"phrase_chars"`
		HardenedX   string `json:"hardened_x"`
		HardenedH   string `json:"hardened_h"`
		SHA256X     string `json:"sha256_x"`
		SHA256H     string `json:"sha256_h"`
	} `json:"derivation"`
	Refusals []struct {
		Input         *string `json:"input"`
		InputBytesHex *string `json:"input_bytes_hex"`
		Channel       string  `json:"channel"`
		Rule          *string `json:"rule"`
		Remedy        *string `json:"remedy"`
		Note          *string `json:"note"`
	} `json:"refusals"`
	Kind []struct {
		PreimageHex   string `json:"preimage_hex"`
		Digest        string `json:"digest"`
		MS1           string `json:"ms1"`
		Entr32PairMS1 string `json:"entr32_pair_ms1"`
	} `json:"kind"`
	Lockstep []string `json:"lockstep"`
}

func loadCorpus(t *testing.T) corpus {
	t.Helper()
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("%s: %v", corpusPath, err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != corpusSHA256 {
		t.Fatalf("%s hashes to %x, not the pinned %s -- the vendored copy and ms-codec 0.8.0's have drifted",
			corpusPath, sum, corpusSHA256)
	}
	var c corpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("%s: %v", corpusPath, err)
	}
	if len(c.Derivation) != 11 || len(c.Refusals) != 15 || len(c.Kind) < 1 {
		t.Fatalf("corpus shape: %d derivation, %d refusals, %d kind rows", len(c.Derivation), len(c.Refusals), len(c.Kind))
	}
	return c
}

func mustHex(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad 32-byte hex %q: %v", s, err)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

// Every derivation row, both methods, compared against the corpus CONSTANTS.
// MUTATIONS: zero-pad Salt to 16 bytes -> every hardened row fails; Iterations
// 99999 -> every hardened row fails; NormalisePassphrase the phrase first ->
// the "Correct Horse Battery Staple" and "  a  b " rows fail; strip display
// separators first -> the "correct-horse,battery staple" row fails.
func TestDerivationRowsLockstep(t *testing.T) {
	c := loadCorpus(t)
	for _, r := range c.Derivation {
		phrase := []byte(r.Phrase)
		if len(phrase) != r.PhraseChars {
			t.Errorf("%q: %d bytes, corpus says %d", r.Phrase, len(phrase), r.PhraseChars)
		}
		x := PreimageHardened(phrase)
		if want := mustHex(t, r.HardenedX); x != want {
			t.Errorf("%q hardened X: got %x want %x", r.Phrase, x, want)
		}
		if h, want := Digest(&x), mustHex(t, r.HardenedH); h != want {
			t.Errorf("%q hardened H: got %x want %x", r.Phrase, h, want)
		}
		x2 := PreimageSHA256(phrase)
		if want := mustHex(t, r.SHA256X); x2 != want {
			t.Errorf("%q sha256 X: got %x want %x", r.Phrase, x2, want)
		}
		if h, want := Digest(&x2), mustHex(t, r.SHA256H); h != want {
			t.Errorf("%q sha256 H: got %x want %x", r.Phrase, h, want)
		}
		// The stepwise driver derives the same bytes as the one-shot function.
		if d, ok := DeriveHardened(phrase, func(int, int) bool { return true }); !ok || d != x {
			t.Errorf("%q DeriveHardened != PreimageHardened (ok=%v)", r.Phrase, ok)
		}
	}
}

// The three rows that are NOT fixed points of the folds spec §2 forbids exist in
// the corpus; without them the mutations above could not fail.
func TestCorpusCarriesTheNonFixedPointRows(t *testing.T) {
	c := loadCorpus(t)
	want := map[string]bool{"Correct Horse Battery Staple": false, "  a  b ": false, "correct-horse,battery staple": false}
	for _, r := range c.Derivation {
		if _, ok := want[r.Phrase]; ok {
			want[r.Phrase] = true
		}
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("corpus has no derivation row %q", p)
		}
	}
}

// Every refusals row through ValidatePhrase; the ms1-shaped rows are built from
// kind[0].ms1 exactly as the corpus describes them.
func TestRefusalRowsMatchTheHost(t *testing.T) {
	c := loadCorpus(t)
	plate := c.Kind[0].MS1
	grouped := func(s string, n int) string {
		var b strings.Builder
		for i, r := range s {
			if i > 0 && i%n == 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	for i, r := range c.Refusals {
		var in []byte
		switch {
		case r.InputBytesHex != nil:
			b, err := hex.DecodeString(*r.InputBytesHex)
			if err != nil {
				t.Fatalf("row %d: %v", i, err)
			}
			in = b
		case r.Input != nil:
			s := *r.Input
			switch s {
			case "<the kind[0].ms1 string, lowercase>":
				s = strings.ToLower(plate)
			case "<the kind[0].ms1 string, UPPERCASE>":
				s = strings.ToUpper(plate)
			case "<the kind[0].ms1 string, grouped by 5 with spaces>":
				s = grouped(plate, 5)
			case "<the kind[0].ms1 string, with two leading and two trailing spaces>":
				s = "  " + plate + "  "
			case "<the kind[0].ms1 string, grouped by 2 (112 chars)>":
				s = grouped(plate, 2)
			}
			in = []byte(s)
		default:
			t.Fatalf("row %d has neither input nor input_bytes_hex", i)
		}
		err := ValidatePhrase(in)
		if r.Rule == nil {
			if err != nil {
				t.Errorf("row %d (%s) must be ACCEPTED, got %v", i, r.Channel, err)
			}
			continue
		}
		want := map[string]error{
			"empty": ErrEmpty, "printable-ascii": ErrNotPrintableASCII, "64-hex": ErrHex64,
			"ms1-shaped": ErrMS1Shaped, "too-long": ErrTooLong,
		}[*r.Rule]
		if want == nil {
			t.Fatalf("row %d: unknown rule %q", i, *r.Rule)
		}
		if err != want {
			t.Errorf("row %d rule %s: got %v want %v", i, *r.Rule, err, want)
		}
	}
}

// The kind row: the plate's preimage bytes are the corpus's preimage_hex, and
// Digest of that preimage is the corpus's own `digest` CONSTANT -- what the
// confirm modal must show for a --hex X. Compared against the corpus, never
// against a value this Go recomputed (Global Constraints, Rust-primary).
//
// MUTATION: double-hash in Digest (`inner := sha256.Sum256(x[:]); return
// sha256.Sum256(inner[:])`) -> this test fails with
// `kind[0] digest: got 88b8f02c...  want 9a2db2e2...`. The identity check this
// replaced could NOT fail on that mutation (r0 tests C-1 executed it: the
// mutated Digest still returned a value != x, so the test reported PASS).
func TestKindRowPreimageDigest(t *testing.T) {
	c := loadCorpus(t)
	if c.Kind[0].Digest == "" {
		t.Fatal("kind[0] carries no digest constant -- the corpus and this test have drifted")
	}
	x := mustHex(t, c.Kind[0].PreimageHex)
	if got, want := Digest(&x), mustHex(t, c.Kind[0].Digest); got != want {
		t.Fatalf("kind[0] digest: got %x want %x", got, want)
	}
	if h := Digest(&x); h == x {
		t.Fatalf("Digest is the identity")
	}
}

// PhraseMaxChars is the single source of the cap (mutation: change the literal
// in ValidatePhrase to 99 -> the 100-character corpus row is refused here).
func TestPhraseMaxCharsIsTheCap(t *testing.T) {
	if PhraseMaxChars != 100 {
		t.Fatalf("PhraseMaxChars = %d", PhraseMaxChars)
	}
	if err := ValidatePhrase([]byte(strings.Repeat("k", PhraseMaxChars))); err != nil {
		t.Errorf("100 characters must be accepted: %v", err)
	}
	if err := ValidatePhrase([]byte(strings.Repeat("k", PhraseMaxChars+1))); err != ErrTooLong {
		t.Errorf("101 characters: got %v want ErrTooLong", err)
	}
}

// DeriveHardened's OWN abandon contract (r0 tests I-3): `progress` returning
// false must stop the KDF and report ok=false, PROMPTLY -- not after running to
// completion. TestDerivationRowsLockstep passes an always-true progress func, and
// the GUI's hashlockDeriveFlow tracks its own `abandoned` flag, so a
// DeriveHardened that ignored the callback's return value would ship green
// through both. This is the only test that can see it.
//
// MUTATION: drop the early return (`progress(d.Done(), d.Total())` in place of
// `if !progress(...) { return x, false }`) -> ok becomes true, calls becomes 199
// instead of 3, and both assertions below fail.
func TestDeriveHardenedAbandonsWhenProgressSaysStop(t *testing.T) {
	calls := 0
	x, ok := DeriveHardened([]byte("correct horse battery staple"), func(done, total int) bool {
		calls++
		if total != Iterations {
			t.Errorf("progress total = %d, want %d", total, Iterations)
		}
		return calls < 3
	})
	if ok {
		t.Errorf("DeriveHardened returned ok=true after progress abandoned it")
	}
	if calls != 3 {
		t.Errorf("progress was called %d times; abandoning must stop the KDF at the "+
			"third call, not run it to completion (%d calls)", calls, Iterations/500)
	}
	if x != ([32]byte{}) {
		t.Error("an abandoned derivation returned a non-zero value, want the zero value " +
			"(the bytes are deliberately not logged)")
	}
}

// minMS1Len's OWN boundary (r0 tests I-5): the corpus's ms1-shaped refusals are
// all 75-character plates, nowhere near 47/48, so nothing else in this package
// can see the constant move.
//
// MUTATION: minMS1Len = 47 -> the 47-character row is reported ms1-shaped and
// this test fails; minMS1Len = 49 -> the 48-character row fails.
func TestIsMS1ShapedMinLengthBoundary(t *testing.T) {
	if minMS1Len != 48 {
		t.Errorf("minMS1Len = %d -- ms-cli's MIN_MS1_LEN is 48", minMS1Len)
	}
	// The two inputs are LITERAL 47 and 48 characters, not derived from
	// minMS1Len: a test that built its own boundary out of the constant it is
	// pinning would move with the mutation and never fail on it.
	short := "ms1" + strings.Repeat("q", 44) // 47 characters
	long := "ms1" + strings.Repeat("q", 45)  // 48 characters
	if len(short) != 47 || len(long) != 48 {
		t.Fatalf("boundary inputs are %d and %d characters", len(short), len(long))
	}
	if IsMS1Shaped(short) {
		t.Errorf("%d characters must be BELOW the ms1 shape bound", len(short))
	}
	if !IsMS1Shaped(long) {
		t.Errorf("%d characters is the bound and must be ms1-shaped", len(long))
	}
	// The bound is applied to the STRIPPED length, not the typed one: the same
	// 47 characters grouped by 5 are still too short, and the same 48 still are not.
	if IsMS1Shaped(displaySpaced(short)) {
		t.Errorf("grouping must not lift a 47-character string over the bound")
	}
	if !IsMS1Shaped(displaySpaced(long)) {
		t.Errorf("grouping must not push a 48-character string under the bound")
	}
}

func displaySpaced(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsMS1Shaped's strings.TrimSpace call is LOAD-BEARING, and this test is what
// says so (r0 tests I-6 claimed it was redundant with the strip loop and should
// be deleted -- measured false, see below).
//
// The strip loop skips exactly ' ', '\t', '\n', '\r', '-' and ','. TrimSpace
// removes every character unicode.IsSpace reports at the ENDS, which is a
// strictly larger set: '\v', '\f', U+0085 and U+00A0 among them. Removing the
// call therefore changes the answer for real inputs -- and in the WRONG
// direction, since the host's own looks_like_ms1 is `is_ms1_shaped(&raw.trim()
// .to_ascii_lowercase())` (ms-cli argv_guard.rs) and Rust's str::trim uses the
// White_Space property, which covers all of them.
//
// MUTATION: `t := strings.ToLower(s)` in place of
// `t := strings.ToLower(strings.TrimSpace(s))` -> every row below except the
// first two fails, measured: '\v', '\f', U+0085, U+00A0 and U+2003 all flip
// from true to false while the host still refuses the plate.
func TestIsMS1ShapedTrimsWhatTheStripLoopCannot(t *testing.T) {
	c := loadCorpus(t)
	plate := c.Kind[0].MS1
	for _, pad := range []string{" ", "\t", "\v", "\f", "\u0085", "\u00a0", "\u2003"} {
		if !IsMS1Shaped(pad + plate) {
			t.Errorf("%q + the plate is not ms1-shaped -- the host trims this character "+
				"before its own shape test, so the port must too", pad)
		}
		if !IsMS1Shaped(plate + pad) {
			t.Errorf("the plate + %q is not ms1-shaped", pad)
		}
	}
}

// The lockstep list is the corpus's own statement of what this file drives; if
// ms-codec adds a clause, this test names it so the port grows with it.
func TestLockstepListIsTheOneWeDrive(t *testing.T) {
	c := loadCorpus(t)
	if len(c.Lockstep) != 4 {
		t.Fatalf("lockstep clauses: %d (this test drives 4 -- read the new one)", len(c.Lockstep))
	}
}
