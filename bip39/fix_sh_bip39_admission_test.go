package bip39

import (
	"fmt"
	"strings"
	"testing"
)

// FIX-sh-bip39-admission (bh-sh-I1, bh-sh-I2; ruling T35). Design:
// design/FIX_sh_bip39_admission.md.

// fixShPhrase returns a checksum-valid n-word phrase whose first word is
// first and whose other words are "abandon" (the last one is the checksum
// word FixChecksum picks). n may be any multiple of 3 up to 24, so 3-, 6-
// and 9-word fixtures pass the checksum and only the length rule refuses them.
func fixShPhrase(n int, first Word) (Mnemonic, string) {
	m := make(Mnemonic, n)
	m[0] = first
	m = m.FixChecksum()
	return m, strings.ToLower(m.String())
}

// fixShParseMnemonic reports a panic instead of crashing the test binary
// (ParseMnemonic of 27 words panics at base, sh-bip39-valid-bounds).
func fixShParseMnemonic(s string) (m Mnemonic, err error, panicked any) {
	defer func() { panicked = recover() }()
	m, err = ParseMnemonic(s)
	return
}

func fixShWord(t *testing.T, label string) Word {
	t.Helper()
	w, ok := ClosestWord(strings.ToUpper(label))
	if !ok || LabelFor(w) != strings.ToUpper(label) {
		t.Fatalf("premise: %q is not a BIP-39 word", label)
	}
	return w
}

// R1: an abbreviation that matches more than one word is refused, with the
// exact error text; the four words it could mean all parse when spelled out.
func TestFixShBip39AmbiguousPrefixRefused(t *testing.T) {
	const tail = " abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon bargain"
	for _, full := range []string{"absent", "absorb"} {
		if _, err := Parse([]byte(full + tail)); err != nil {
			t.Fatalf("control %q: %v", full, err)
		}
	}
	for _, in := range []string{"abs", "ABS"} {
		m, err := Parse([]byte(in + tail))
		want := fmt.Sprintf("bip39: parse: ambiguous word: %q", in)
		if err == nil {
			t.Errorf("Parse(%q ...) accepted as %v; want error %q", in, m, want)
		} else if err.Error() != want {
			t.Errorf("Parse(%q ...) error %q; want %q", in, err, want)
		}
	}
}

// R1 and R2 over the whole wordlist: for every prefix of 3 or more letters of
// every word, Parse accepts it exactly when it IS a word or exactly one word
// starts with it, and then yields that word; otherwise it refuses as ambiguous.
func TestFixShBip39EveryPrefixNamesExactlyOneWord(t *testing.T) {
	seen := map[string]bool{}
	accepted, refused := 0, 0
	for w := Word(0); w < NumWords; w++ {
		label := LabelFor(w)
		for l := 3; l <= len(label); l++ {
			p := label[:l]
			if seen[p] {
				continue
			}
			seen[p] = true
			var matches []Word
			exact := Word(-1)
			for v := Word(0); v < NumWords; v++ {
				if strings.HasPrefix(LabelFor(v), p) {
					matches = append(matches, v)
				}
				if LabelFor(v) == p {
					exact = v
				}
			}
			want := exact
			if want < 0 && len(matches) == 1 {
				want = matches[0]
			}
			// The phrase's checksum is fixed for the intended word, so a
			// wrong resolution shows as a checksum error or a different word.
			target := want
			if target < 0 {
				target = matches[0]
			}
			_, phrase := fixShPhrase(12, target)
			in := strings.ToLower(p) + phrase[strings.IndexByte(phrase, ' '):]
			got, err := Parse([]byte(in))
			if want >= 0 {
				accepted++
				if err != nil || got[0] != want {
					t.Errorf("prefix %q: Parse gave (%v, %v); want word %q", p, got, err, LabelFor(want))
				}
				continue
			}
			refused++
			wantErr := fmt.Sprintf("bip39: parse: ambiguous word: %q", strings.ToLower(p))
			if err == nil || err.Error() != wantErr {
				t.Errorf("prefix %q (%d words): Parse gave (%v, %v); want error %q", p, len(matches), got, err, wantErr)
			}
		}
	}
	if accepted == 0 || refused == 0 {
		t.Fatalf("premise: accepted %d, refused %d prefixes", accepted, refused)
	}
	t.Logf("prefixes: %d accepted, %d refused as ambiguous", accepted, refused)
}

// R2 control: every word of the list, spelled in full, parses as itself,
// including the 3- and 4-letter words that are also prefixes of longer
// words ("act" of action/actor/actress/actual).
func TestFixShBip39ExactWordAccepted(t *testing.T) {
	act := fixShWord(t, "act")
	if _, p := fixShPhrase(12, act); !strings.HasPrefix(p, "act ") {
		t.Fatalf("premise: %q", p)
	}
	for w := Word(0); w < NumWords; w++ {
		m, phrase := fixShPhrase(12, w)
		got, err := Parse([]byte(phrase))
		if err != nil || got[0] != m[0] {
			t.Errorf("%q: Parse gave (%v, %v)", LabelFor(w), got, err)
		}
	}
}

// R3: a phrase of a length BIP-39 does not define is refused by Valid, Parse
// and ParseMnemonic, without a panic, even when its checksum passes.
func TestFixShBip39WordCountRefused(t *testing.T) {
	for _, n := range []int{3, 6, 9} {
		m, phrase := fixShPhrase(n, 0)
		if ok, p := bhValid(m); ok || p != nil {
			t.Errorf("Valid(%d words) = %v, panic %v; want false", n, ok, p)
		}
		want := fmt.Sprintf("bip39: parse: wrong word count: %d", n)
		if got, err := Parse([]byte(phrase)); err == nil || err.Error() != want {
			t.Errorf("Parse(%d words) = (%v, %v); want error %q", n, got, err, want)
		}
		want = fmt.Sprintf("bip39: wrong word count: %d", n)
		if got, err := ParseMnemonic(phrase); err == nil || err.Error() != want {
			t.Errorf("ParseMnemonic(%d words) = (%v, %v); want error %q", n, got, err, want)
		}
	}
	for _, n := range []int{0, 1, 2, 4, 11, 13, 14, 23, 25, 26, 27, 30} {
		if ok, p := bhValid(make(Mnemonic, n)); ok || p != nil {
			t.Errorf("Valid(%d words) = %v, panic %v; want false", n, ok, p)
		}
	}
	for _, n := range []int{1, 13, 27} {
		phrase := strings.TrimSpace(strings.Repeat("abandon ", n))
		want := fmt.Sprintf("bip39: wrong word count: %d", n)
		if got, err, p := fixShParseMnemonic(phrase); p != nil || err == nil || err.Error() != want {
			t.Errorf("ParseMnemonic(%d words) = (%v, %v), panic %v; want error %q", n, got, err, p, want)
		}
	}
	for _, n := range []int{3, 6, 9, 27} {
		var got []Word
		p := func() (p any) {
			defer func() { p = recover() }()
			got = LastWordCandidates(make(Mnemonic, n))
			return nil
		}()
		if p != nil || got != nil {
			t.Errorf("LastWordCandidates(%d words) = %d candidates, panic %v; want nil", n, len(got), p)
		}
	}
	// Unchanged texts at the edges Parse already refused.
	if _, err := Parse([]byte(strings.TrimSpace(strings.Repeat("abandon ", 25)))); err == nil ||
		err.Error() != "bip39: parse: mnemonic too long" {
		t.Errorf("Parse(25 words) error %v; want the unchanged \"mnemonic too long\"", err)
	}
	if _, err := Parse([]byte(strings.TrimSpace(strings.Repeat("abandon ", 13)))); err == nil ||
		err.Error() != "bip39: parse: wrong word count: 13" {
		t.Errorf("Parse(13 words) error %v; want wrong word count: 13", err)
	}
}

// R3 control: the five lengths BIP-39 defines (12, 15, 18, 21, 24) parse.
func TestFixShBip39DefinedLengthsAccepted(t *testing.T) {
	for _, n := range []int{12, 15, 18, 21, 24} {
		m, phrase := fixShPhrase(n, 2047)
		if !m.Valid() {
			t.Errorf("Valid(%d words) = false", n)
		}
		if got, err := Parse([]byte(phrase)); err != nil || len(got) != n {
			t.Errorf("Parse(%d words) = (%v, %v)", n, got, err)
		}
		if got, err := ParseMnemonic(phrase); err != nil || len(got) != n {
			t.Errorf("ParseMnemonic(%d words) = (%v, %v)", n, got, err)
		}
		if got := LastWordCandidates(m); len(got) != 1<<(11-n/3) {
			t.Errorf("LastWordCandidates(%d words) = %d candidates; want %d", n, len(got), 1<<(11-n/3))
		}
	}
}

// R4: the two new error exits zero Parse's accumulator, as the three existing
// ones do (TestParseZeroesItsAccumulatorOnEveryErrorExit). Fixtures are "zoo"
// (Word 2047), never "abandon" (Word 0), so a zeroed buffer is not vacuous.
func TestFixShBip39NewErrorExitsZeroAccumulator(t *testing.T) {
	elevenZoo := strings.TrimSpace(strings.Repeat("zoo ", 11))
	for _, tc := range []struct {
		name, input, wantErr string
		wantLen              int
	}{
		{"ambiguous word after eleven good ones", elevenZoo + " abs", `bip39: parse: ambiguous word: "abs"`, 11},
		{"wrong word count (holds all nine)", strings.TrimSpace(strings.Repeat("zoo ", 9)), "bip39: parse: wrong word count: 9", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var held Mnemonic
			parseWordsHook = func(m Mnemonic) { held = m }
			t.Cleanup(func() { parseWordsHook = nil })
			got, err := Parse([]byte(tc.input))
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("Parse(%q) = (%v, %v); want error %q", tc.input, got, err, tc.wantErr)
			}
			if len(held) < tc.wantLen {
				t.Fatalf("the accumulator is %d long; this exit fills %d", len(held), tc.wantLen)
			}
			for i, w := range held {
				if w != 0 {
					t.Fatalf("word %d of the accumulator is still %q after %v", i, LabelFor(w), err)
				}
			}
		})
	}
}
