package bip39

import (
	"bytes"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

func TestVectors(t *testing.T) {
	for _, v := range testVectors {
		m, err := ParseMnemonic(v.mnemonic)
		if err != nil {
			t.Fatalf("ParseMnemonic failed to parse %q: %v", v.mnemonic, err)
		}
		e, err := hex.DecodeString(v.entropy)
		if err != nil {
			t.Error(err)
		}
		ent, check := splitMnemonic(m)
		if !bytes.Equal(e, ent) {
			t.Errorf("entropy mismatch")
		}
		if want := checksum(ent); want != check {
			t.Errorf("checksum mismatch, got %d, want %d", check, want)
		}
		checkWord := m[len(m)-1]
		if want := ChecksumWord(ent); want != checkWord {
			t.Errorf("checksum word mismatch, got %d, want %d", checkWord, want)
		}
		m2, err := Parse([]byte(v.mnemonic))
		if err != nil {
			t.Fatalf("Parse failed to parse %q: %v", v.mnemonic, err)
		}
		if !slices.Equal(m, m2) {
			t.Fatalf("Parse parsed differently than ParseMnemonic for %q", v.mnemonic)
		}
		shortWords := new(bytes.Buffer)
		// Shorten words to 3 or 4 characters.
		for w := range strings.SplitSeq(v.mnemonic, " ") {
			if len(w) > 4 {
				w = w[:4]
			}
			if shortWords.Len() > 0 {
				shortWords.WriteByte(' ')
			}
			shortWords.WriteString(w)
		}
		sw := shortWords.Bytes()
		m3, err := Parse(sw)
		if err != nil {
			t.Fatalf("Parse failed to parse %q: %v", sw, err)
		}
		if !slices.Equal(m, m3) {
			t.Fatalf("Parse parsed differently than ParseMnemonic for %q", v.mnemonic)
		}
		m4 := New(ent)
		if got := m4.String(); got != v.mnemonic {
			t.Errorf("%s: round-tripped to %s", v.mnemonic, got)
		}
		swu := bytes.ToUpper(sw)
		m5, err := Parse(swu)
		if err != nil {
			t.Fatalf("Parse failed to parse %q: %v", swu, err)
		}
		if !slices.Equal(m, m5) {
			t.Fatalf("Parse parsed differently than ParseMnemonic for %q", v.mnemonic)
		}
		m6, err := ParseMnemonic(strings.ToUpper(v.mnemonic))
		if err != nil {
			t.Fatalf("ParseMnemonic failed to parse %q: %v", swu, err)
		}
		if !slices.Equal(m, m6) {
			t.Fatalf("Parse parsed differently than ParseMnemonic for %q", v.mnemonic)
		}
	}
}

func TestInvalidSeeds(t *testing.T) {
	tests := []string{
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon",
	}
	for _, test := range tests {
		if _, err := ParseMnemonic(test); err == nil {
			t.Errorf("ParseMnemonic parsed invalid seed %q", test)
		}
		if _, err := Parse([]byte(test)); err == nil {
			t.Errorf("Parse parsed invalid seed %q", test)
		}
	}
}

func TestChecksumWord(t *testing.T) {
	mnemonic := make(Mnemonic, 12)
	for range int(1e4) {
		for j := range mnemonic {
			mnemonic[j] = RandomWord()
		}
		want, _ := splitMnemonic(mnemonic)
		got := mnemonic.FixChecksum().Entropy()
		if !bytes.Equal(want, got) {
			t.Errorf("checksum word changed the entropy")
		}
	}
}

var testVectors = []struct {
	entropy  string
	mnemonic string
}{
	{
		entropy:  "00000000000000000000000000000000",
		mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
	},
	{
		entropy:  "7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f",
		mnemonic: "legal winner thank year wave sausage worth useful legal winner thank yellow",
	},
	{
		entropy:  "80808080808080808080808080808080",
		mnemonic: "letter advice cage absurd amount doctor acoustic avoid letter advice cage above",
	},
	{
		entropy:  "ffffffffffffffffffffffffffffffff",
		mnemonic: "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
	},
	{
		entropy:  "000000000000000000000000000000000000000000000000",
		mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon agent",
	},
	{
		entropy:  "7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f",
		mnemonic: "legal winner thank year wave sausage worth useful legal winner thank year wave sausage worth useful legal will",
	},
	{
		entropy:  "808080808080808080808080808080808080808080808080",
		mnemonic: "letter advice cage absurd amount doctor acoustic avoid letter advice cage absurd amount doctor acoustic avoid letter always",
	},
	{
		entropy:  "ffffffffffffffffffffffffffffffffffffffffffffffff",
		mnemonic: "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo when",
	},
	{
		entropy:  "0000000000000000000000000000000000000000000000000000000000000000",
		mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art",
	},
	{
		entropy:  "7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f",
		mnemonic: "legal winner thank year wave sausage worth useful legal winner thank year wave sausage worth useful legal winner thank year wave sausage worth title",
	},
	{
		entropy:  "8080808080808080808080808080808080808080808080808080808080808080",
		mnemonic: "letter advice cage absurd amount doctor acoustic avoid letter advice cage absurd amount doctor acoustic avoid letter advice cage absurd amount doctor acoustic bless",
	},
	{
		entropy:  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		mnemonic: "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote",
	},
	{
		entropy:  "9e885d952ad362caeb4efe34a8e91bd2",
		mnemonic: "ozone drill grab fiber curtain grace pudding thank cruise elder eight picnic",
	},
	{
		entropy:  "6610b25967cdcca9d59875f5cb50b0ea75433311869e930b",
		mnemonic: "gravity machine north sort system female filter attitude volume fold club stay feature office ecology stable narrow fog",
	},
	{
		entropy:  "68a79eaca2324873eacc50cb9c6eca8cc68ea5d936f98787c60c7ebc74e6ce7c",
		mnemonic: "hamster diagram private dutch cause delay private meat slide toddler razor book happy fancy gospel tennis maple dilemma loan word shrug inflict delay length",
	},
	{
		entropy:  "c0ba5a8e914111210f2bd131f3d5e08d",
		mnemonic: "scheme spot photo card baby mountain device kick cradle pact join borrow",
	},
	{
		entropy:  "6d9be1ee6ebd27a258115aad99b7317b9c8d28b6d76431c3",
		mnemonic: "horn tenant knee talent sponsor spell gate clip pulse soap slush warm silver nephew swap uncle crack brave",
	},
	{
		entropy:  "9f6a2878b2520799a44ef18bc7df394e7061a224d2c33cd015b157d746869863",
		mnemonic: "panda eyebrow bullet gorilla call smoke muffin taste mesh discover soft ostrich alcohol speed nation flash devote level hobby quick inner drive ghost inside",
	},
	{
		entropy:  "23db8160a31d3e0dca3688ed941adbf3",
		mnemonic: "cat swing flag economy stadium alone churn speed unique patch report train",
	},
	{
		entropy:  "8197a4a47f0425faeaa69deebc05ca29c0a5b5cc76ceacc0",
		mnemonic: "light rule cinnamon wrap drastic word pride squirrel upgrade then income fatal apart sustain crack supply proud access",
	},
	{
		entropy:  "066dca1a2bb7e8a1db2832148ce9933eea0f3ac9548d793112d9a95c9407efad",
		mnemonic: "all hour make first leader extend hole alien behind guard gospel lava path output census museum junior mass reopen famous sing advance salt reform",
	},
	{
		entropy:  "f30f8c1da665478f49b001d94c5fc452",
		mnemonic: "vessel ladder alter error federal sibling chat ability sun glass valve picture",
	},
	{
		entropy:  "c10ec20dc3cd9f652c7fac2f1230f7a3c828389a14392f05",
		mnemonic: "scissors invite lock maple supreme raw rapid void congress muscle digital elegant little brisk hair mango congress clump",
	},
	{
		entropy:  "f585c11aec520db57dd353c69554b21a89b20fb0650966fa0a9d6f74fd989d8f",
		mnemonic: "void come effort suffer camp survey warrior heavy shoot primary clutch crush open amazing screen patrol group space point ten exist slush involve unfold",
	},
}

func TestLastWordCandidates(t *testing.T) {
	build := func(n int) Mnemonic {
		m := make(Mnemonic, n)
		for i := range m {
			m[i] = Word(i % int(NumWords))
		}
		return m.FixChecksum()
	}

	// 24-word: exactly 8 candidates, all valid, including the real last word.
	v24 := build(24)
	c24 := LastWordCandidates(v24)
	if len(c24) != 8 {
		t.Fatalf("24-word: got %d candidates, want 8", len(c24))
	}
	foundLast := false
	for _, w := range c24 {
		m := make(Mnemonic, len(v24))
		copy(m, v24)
		m[len(m)-1] = w
		if !m.Valid() {
			t.Errorf("24-word candidate %d is not checksum-valid", w)
		}
		if w == v24[len(v24)-1] {
			foundLast = true
		}
	}
	if !foundLast {
		t.Errorf("24-word candidates %v do not include the real last word %d", c24, v24[len(v24)-1])
	}

	// 12-word: exactly 128 candidates.
	v12 := build(12)
	if c12 := LastWordCandidates(v12); len(c12) != 128 {
		t.Fatalf("12-word: got %d candidates, want 128", len(c12))
	}

	// Incomplete prefix (an earlier word unset) -> nil.
	bad := make(Mnemonic, len(v24))
	copy(bad, v24)
	bad[5] = -1
	if got := LastWordCandidates(bad); got != nil {
		t.Errorf("incomplete prefix: got %v, want nil", got)
	}

	// Unsupported length (len%3 != 0) -> nil.
	if got := LastWordCandidates(make(Mnemonic, 13)); got != nil {
		t.Errorf("len 13: got %v, want nil", got)
	}

	// Empty mnemonic -> nil.
	if got := LastWordCandidates(Mnemonic{}); got != nil {
		t.Errorf("len 0: got %v, want nil", got)
	}

	// Must not mutate the input's final slot.
	before := v24[len(v24)-1]
	_ = LastWordCandidates(v24)
	if v24[len(v24)-1] != before {
		t.Errorf("LastWordCandidates mutated input final slot: %d -> %d", before, v24[len(v24)-1])
	}
}

func TestDiceToWord(t *testing.T) {
	counts := make([]int, len(index))
	dice := Roll{1, 1, 1, 1, 1}
loop:
	for {
		word, valid := DiceToWord(dice)
		// Increment roll.
		for i := len(dice) - 1; ; i-- {
			if i < 0 {
				break loop
			}
			dice[i]++
			if dice[i] <= 6 {
				break
			}
			dice[i] = 1
		}
		if valid {
			counts[word]++
		}
	}
	for word, count := range counts {
		if count != 3 {
			t.Errorf("word %v chosen %d times, expected 3", word, count)
		}
	}
}

// Parse must never GROW its result: append's reallocation orphans partial copies
// of the mnemonic that no caller's clear() can reach. Measured before the
// preallocation, a 12-word parse left orphans holding 1, 2, 4 and 8 words — two
// thirds of the seed, unwipeable. (B2a-ii whole-diff review, lens 1 pass 3.)
func TestParseNeverGrowsItsResult(t *testing.T) {
	for _, n := range []int{12, 24} {
		words := make([]string, n)
		for i := range words {
			words[i] = "abandon"
		}
		m := make(Mnemonic, n)
		for i := range m {
			m[i] = 0
		}
		m = m.FixChecksum()
		for i, w := range m {
			words[i] = strings.ToLower(LabelFor(w))
		}
		got, err := Parse([]byte(strings.Join(words, " ")))
		if err != nil {
			t.Fatalf("%d words: %v", n, err)
		}
		if c := cap(got); c != 24 {
			t.Errorf("%d words: cap is %d, want 24 — append grew and orphaned "+
				"partial copies of the seed that no clear() can reach", n, c)
		}
		// LENGTH too (lens 8 C5). Asserting only capacity is satisfied by a
		// Parse that returned a 24-long Mnemonic zero-padded to capacity, on
		// the test named for a seed-orphaning fix.
		if l := len(got); l != n {
			t.Errorf("%d words: len is %d, want %d", n, l, n)
		}
		for i, w := range got {
			if want := m[i]; w != want {
				t.Errorf("%d words: word %d is %v, want %v", n, i, w, want)
			}
		}
	}
}

// TestParseZeroesItsAccumulatorOnEveryErrorExit — lens 7 M2.
//
// The pass-3 fold stopped Parse orphaning PARTIAL copies through append
// reallocation. This is the other half: on each of the three error returns
// Parse hands back nil while the accumulator still holds every word it read,
// and the caller receives nothing it can clear.
//
// The materially interesting exit is ErrInvalidChecksum, where the accumulator
// holds the COMPLETE word list. seal.Classify calls Parse on every record of
// both sections, so a mnemonic-shaped record with a bad checksum leaves a full
// 12/24-word near-seed on a heap that neither Payload.Wipe nor RecordsResident
// can reach.
//
// Asserted ON THE BUFFER via the allocation seam, because all three exits
// return nil and there is no return value to read.
//
// EVERY fixture below is built from "zoo" (Word 2047) and NEVER from "abandon"
// (Word 0), and the premise is asserted per case. That is not fastidiousness:
// the first draft of this test used "abandon", so the accumulator filled with
// ZEROES and "reads as zeroed" was VACUOUSLY true -- two of the three mutants
// survived it. A zero-valued fixture on a zeroing test is a guaranteed false
// PASS over exactly the defect.
func TestParseZeroesItsAccumulatorOnEveryErrorExit(t *testing.T) {
	// Twelve "zoo" is all-valid words with a BAD checksum (measured: Parse
	// returns ErrInvalidChecksum), so the accumulator holds all twelve.
	twelveZoo := strings.TrimSpace(strings.Repeat("zoo ", 12))
	elevenZoo := strings.TrimSpace(strings.Repeat("zoo ", 11))
	twentyFiveZoo := strings.TrimSpace(strings.Repeat("zoo ", 25))

	for _, tc := range []struct {
		name     string
		input    string
		wantWord Word // the non-zero value the accumulator would hold
		wantLen  int  // how many slots it would hold it in
	}{
		{"invalid checksum (holds ALL twelve words)", twelveZoo, 2047, 12},
		{"unknown word after eleven good ones", elevenZoo + " notaword", 2047, 11},
		{"mnemonic too long (holds all 24)", twentyFiveZoo, 2047, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantWord == 0 {
				t.Fatal("premise broken: a zero-valued fixture makes this test vacuous")
			}
			var held Mnemonic
			parseWordsHook = func(m Mnemonic) { held = m }
			t.Cleanup(func() { parseWordsHook = nil })

			got, err := Parse([]byte(tc.input))
			if err == nil {
				t.Fatalf("premise broken: %q parsed cleanly as %v", tc.input, got)
			}
			if held == nil {
				t.Fatal("parseWordsHook never fired; this test asserted nothing")
			}
			if len(held) < tc.wantLen {
				t.Fatalf("the accumulator is %d long; this exit fills %d", len(held), tc.wantLen)
			}
			for i, w := range held {
				if w != 0 {
					t.Fatalf("Parse returned %v with word %d of its accumulator still set "+
						"to %v (%q)\nfull accumulator: %v\n"+
						"the caller gets nil and cannot reach this array, so a full "+
						"near-seed stays on the heap for the rest of the power cycle",
						err, i, w, LabelFor(w), held)
				}
			}
		})
	}
}

// TestParseAccumulatorIsPopulatedBeforeTheErrorExit is the anti-vacuity control
// for the test above -- the assertion that would have caught the "abandon"
// fixture. It proves the accumulator really does carry words by reading the SAME
// buffer on the SUCCESS path, where nothing clears it.
func TestParseAccumulatorIsPopulatedBeforeTheErrorExit(t *testing.T) {
	// A checksum-VALID twelve words made of "zoo": FixChecksum over Word 2047.
	m := make(Mnemonic, 12)
	for i := range m {
		m[i] = 2047
	}
	m = m.FixChecksum()
	words := make([]string, len(m))
	for i, w := range m {
		words[i] = strings.ToLower(LabelFor(w))
	}

	var held Mnemonic
	parseWordsHook = func(mm Mnemonic) { held = mm }
	t.Cleanup(func() { parseWordsHook = nil })

	got, err := Parse([]byte(strings.Join(words, " ")))
	if err != nil {
		t.Fatalf("the good mnemonic did not parse: %v", err)
	}
	if held == nil {
		t.Fatal("parseWordsHook never fired on the success path")
	}
	nonZero := 0
	for _, w := range held[:len(got)] {
		if w != 0 {
			nonZero++
		}
	}
	if nonZero < 11 {
		t.Fatalf("only %d of %d accumulator slots are non-zero on a SUCCESSFUL parse, so "+
			"the error-path assertions are reading a buffer that was never populated",
			nonZero, len(got))
	}
}
