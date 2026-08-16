package gui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"seedhammer.com/bip39"
)

// ─── The walk's keyboard coordinates, machine-checked against the real screen ──
//
// WHY THIS EXISTS. Trace B holds two masters and cmd/emu's cards payload carries
// exactly ONE ClassMnemonic, and syswSession.take is first-match and
// non-consuming — so every "FROM PAYLOAD" entry in one flow hands back the SAME
// seed. The second master has to be TYPED, which made a keyboard driver gating
// for S5's walk after F-181 had recorded one as "genuinely optional".
//
// The driver types by TAPPING AT DEVICE COORDINATES. Nothing was added to the
// emulator for it: no rune injection, no new primitive, nothing the capacitive
// panel could not deliver. What that costs is a set of numbers in a JS file, and
// a number in a walk script is exactly the kind of claim this project's rules say
// must be machine-checked rather than remembered — F-181 measured, empirically,
// that "the hit regions do not fall on the grid a naive rowY-style formula
// predicts", so a wrong guess here is entirely plausible.
//
// So this test READS THE WALK'S OWN NUMBERS off cmd/emu/walk_trace_b.js and types
// a whole 12-word mnemonic with them, through the same pointer events the panel
// emits, asserting the drawn word line after every letter. There is one source of
// truth (the JS) and it is proved here rather than trusted.
//
// It is the mirror of cmd/emu/needle_test.go's arrangement — a Go test reading a
// walk script so the script's central claim is a fact rather than a comment —
// applied to the other half of what a walk does.
//
// BLIND SPOT, stated because a gate that hides one is worse than none: this
// renders at sh2DisplaySize with the shipped body face. A build that changed the
// display size or the keyboard style would move every key, and this test would
// catch it; a build that changed WHICH SCREEN the keyboard is on would not,
// because the flow is named here.

var (
	walkKeyPitchRe = regexp.MustCompile(`(?m)^export\s+const\s+KEY_PITCH\s*=\s*(\d+)\s*;`)
	walkKeyRowRe   = regexp.MustCompile(`\{\s*letters:\s*"([a-z]+)"\s*,\s*x0:\s*(\d+)\s*,\s*y:\s*(\d+)\s*\}`)
	walkPhraseRe   = regexp.MustCompile(`(?s)export\s+const\s+MASTER_B_WORDS\s*=\s*\n?\s*"([a-z ]+)"\s*;`)
)

type walkKeyRow struct {
	letters string
	x0, y   int
}

// walkKeyboard is the walk's own geometry, parsed rather than restated.
type walkKeyboard struct {
	pitch  int
	rows   []walkKeyRow
	phrase string
}

func loadWalkKeyboard(t *testing.T) walkKeyboard {
	t.Helper()
	path := filepath.Join("..", "cmd", "emu", "walk_trace_b.js")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	src := string(b)

	m := walkKeyPitchRe.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s declares no KEY_PITCH; this test can no longer read the walk's geometry, "+
			"so it would pass by checking nothing", path)
	}
	pitch, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("KEY_PITCH %q: %v", m[1], err)
	}

	var rows []walkKeyRow
	for _, r := range walkKeyRowRe.FindAllStringSubmatch(src, -1) {
		x0, err1 := strconv.Atoi(r[2])
		y, err2 := strconv.Atoi(r[3])
		if err1 != nil || err2 != nil {
			t.Fatalf("KEY_ROWS entry %q: %v %v", r[0], err1, err2)
		}
		rows = append(rows, walkKeyRow{letters: r[1], x0: x0, y: y})
	}
	// A FLOOR, so a regex that stopped matching cannot make this test green by
	// finding nothing to check. The BIP-39 keyboard is three rows and 26 letters.
	if len(rows) != 3 {
		t.Fatalf("%s declares %d KEY_ROWS, want 3 — the extractor is not reading the walk", path, len(rows))
	}
	letters := 0
	for _, r := range rows {
		letters += len(r.letters)
	}
	if letters != 26 {
		t.Fatalf("the walk's KEY_ROWS carry %d letters, want 26", letters)
	}

	p := walkPhraseRe.FindStringSubmatch(src)
	if p == nil {
		t.Fatalf("%s declares no MASTER_B_WORDS", path)
	}
	return walkKeyboard{pitch: pitch, rows: rows, phrase: strings.TrimSpace(p[1])}
}

// point is keyPoint() from the walk, in Go.
func (w walkKeyboard) point(ch byte) (image.Point, bool) {
	for _, r := range w.rows {
		if j := strings.IndexByte(r.letters, ch); j >= 0 {
			return image.Pt(r.x0+j*w.pitch, r.y), true
		}
	}
	return image.Point{}, false
}

// TestWalkKeyboardCoordinatesTypeTheIntendedWords is the pin.
//
// It types the walk's own phrase, letter by letter, at the walk's own
// coordinates, through runUITouch's pointer events — the same route a fingertip
// takes — and requires the word line to read back the fragment after EVERY
// keystroke. A coordinate that drifted by one key fails on the letter it drifted
// on, naming the word, rather than assembling a wallet from a seed nobody chose.
func TestWalkKeyboardCoordinatesTypeTheIntendedWords(t *testing.T) {
	w := loadWalkKeyboard(t)
	words := strings.Fields(w.phrase)
	if len(words) != 12 {
		t.Fatalf("the walk's typed phrase has %d words, want 12", len(words))
	}
	if _, err := bip39.ParseMnemonic(w.phrase); err != nil {
		t.Fatalf("the walk types a phrase that is not a valid BIP-39 mnemonic: %v", err)
	}

	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	m := emptyBIP39Mnemonic(len(words))
	done := false
	frame, drawer, quit := runUITouch(ctx, func() {
		// The Build path's own seed screen: a titled slot prefix and the checksum
		// gate on the last word, exactly as buildSeedForSlot reaches it.
		inputWordsFlow(ctx, &descriptorTheme, m, 0, "", wordEntryOpts{checksumGate: true, titlePrefix: "@2"})
		done = true
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the seed screen drew no frame")
	}

	for wi, word := range words {
		for li := 0; li < len(word); li++ {
			pt, ok := w.point(word[li])
			if !ok {
				t.Fatalf("the walk has no key for %q (word %d %q)", word[li], wi+1, word)
			}
			// A tap that lands on NOTHING is a different failure from a tap that
			// lands on the wrong key, and only one of them is a geometry error.
			if _, _, hit := drawer().Hit(pt); !hit {
				t.Fatalf("word %d (%s) letter %d (%c): the walk taps %v and there is no touch "+
					"target there at all", wi+1, word, li+1, word[li], pt)
			}
			tap(&ctx.Router, drawer(), pt)
			content, ok := frame()
			if !ok {
				t.Fatalf("word %d (%s): the flow returned mid-word", wi+1, word)
			}
			want := fmt.Sprintf("%d: %s", wi+1, strings.ToUpper(word[:li+1]))
			if !uiContains(content, want) {
				t.Fatalf("word %d (%s) letter %d (%c): tapped %v and the word line does not read "+
					"%q; the screen reads %q", wi+1, word, li+1, word[li], pt, want, content)
			}
		}
		click(&ctx.Router, Button3) // accept the word
		if _, ok := frame(); !ok && wi != len(words)-1 {
			t.Fatalf("the flow returned after word %d of %d", wi+1, len(words))
		}
	}
	for i := 0; i < 64 && !done; i++ {
		frame()
	}
	if !done {
		t.Fatalf("the flow did not return after %d words", len(words))
	}
	got := make([]string, len(m))
	for i, x := range m {
		if x == -1 {
			t.Fatalf("word %d was never filled; the walk's coordinates do not complete a phrase", i+1)
		}
		got[i] = bip39.LabelFor(x)
	}
	// Case-insensitively: bip39.LabelFor renders the wordlist in upper case, which
	// is how the keys are drawn, while the walk's phrase is written the way an
	// operator would say it.
	if !strings.EqualFold(strings.Join(got, " "), w.phrase) {
		t.Fatalf("the walk's coordinates typed %q, want %q", strings.Join(got, " "), w.phrase)
	}
	t.Logf("typed all %d words of the walk's phrase at the walk's own coordinates", len(words))
}

// TestWalkKeyboardExtractorCanExtract is the mutation proof for the parser.
// Without it a regex that silently stopped matching would make the test above
// fail loudly rather than pass quietly — but the FLOOR checks in
// loadWalkKeyboard are what guarantee that, so this pins that they can fire.
func TestWalkKeyboardExtractorCanExtract(t *testing.T) {
	w := loadWalkKeyboard(t)
	if w.pitch <= 0 {
		t.Fatalf("KEY_PITCH parsed as %d", w.pitch)
	}
	// Every letter must resolve, and no two letters may share a point.
	seen := map[image.Point]byte{}
	for c := byte('a'); c <= 'z'; c++ {
		pt, ok := w.point(c)
		if !ok {
			t.Errorf("no key for %c", c)
			continue
		}
		if prev, dup := seen[pt]; dup {
			t.Errorf("keys %c and %c both resolve to %v", prev, c, pt)
		}
		seen[pt] = c
	}
	if len(seen) != 26 {
		t.Errorf("%d distinct key points, want 26", len(seen))
	}
}
