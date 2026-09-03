package gui

import (
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ─── The composer walk's DIGIT PAD coordinates, machine-checked ──────────────
//
// WHY THIS EXISTS. cmd/emu/shots_composer.js types the S4 fixture's 12960-block
// relative lock by TAPPING AT DEVICE COORDINATES. Nothing was added to the
// emulator for it: no rune injection, no new driving primitive, nothing the
// capacitive panel could not deliver. What that costs is a set of numbers in a
// JS file, and a number in a walk script is exactly the kind of claim this
// project's rules say must be machine-checked rather than remembered.
//
// So this READS THE WALK'S OWN NUMBERS off cmd/emu/shots_composer.js and types
// 12960 with them, through the same pointer events the panel emits, asserting
// the drawn fragment after every digit. It is gui/keyboard_geometry_test.go's
// arrangement applied to the second keyboard in this tree -- one source of
// truth (the JS), proved here rather than trusted.
//
// THE LAST ROW IS NOT LIKE THE OTHERS, and that is the case a "derive it from a
// pitch" formula gets wrong: gui.NewKeyboard appends its own backspace key, so
// the row spelled "0" is laid out as TWO keys and centred on them -- it starts
// one pitch to the right of the three-key rows above it. Measured, not assumed.
//
// BLIND SPOT, stated because a gate that hides one is worse than none: this
// renders at sh2DisplaySize with the shipped body face, and it drives
// composerDigitEntry directly. A build that changed the display size or the
// keyboard style would move every key and this test would catch it; a build
// that moved the lock flow to a different WIDGET would not, because the entry
// point is named here.

var (
	digitPitchRe = regexp.MustCompile(`(?m)^export\s+const\s+DIGIT_KEY_PITCH\s*=\s*(\d+)\s*;`)
	digitRowRe   = regexp.MustCompile(`\{\s*digits:\s*"(\d+)"\s*,\s*x0:\s*(\d+)\s*,\s*y:\s*(\d+)\s*\}`)
)

type digitRow struct {
	digits string
	x0, y  int
}

func loadWalkDigitPad(t *testing.T) (int, []digitRow) {
	t.Helper()
	path := filepath.Join("..", "cmd", "emu", "shots_composer.js")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	src := string(b)

	m := digitPitchRe.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s declares no DIGIT_KEY_PITCH; this test can no longer read the walk's "+
			"geometry, so it would pass by checking nothing", path)
	}
	pitch, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("DIGIT_KEY_PITCH %q: %v", m[1], err)
	}

	var rows []digitRow
	for _, r := range digitRowRe.FindAllStringSubmatch(src, -1) {
		x0, err1 := strconv.Atoi(r[2])
		y, err2 := strconv.Atoi(r[3])
		if err1 != nil || err2 != nil {
			t.Fatalf("DIGIT_KEY_ROWS entry %q: %v %v", r[0], err1, err2)
		}
		rows = append(rows, digitRow{digits: r[1], x0: x0, y: y})
	}
	// A FLOOR, so a regex that stopped matching cannot make this test green by
	// finding nothing to check. The pad is "123\n456\n789\n0": four rows, ten
	// digits, each exactly once.
	if len(rows) != 4 {
		t.Fatalf("%s declares %d DIGIT_KEY_ROWS, want 4 -- the extractor is not reading the walk",
			path, len(rows))
	}
	var all string
	for _, r := range rows {
		all += r.digits
	}
	if len(all) != 10 {
		t.Fatalf("the walk's DIGIT_KEY_ROWS carry %d digits, want 10 (%q)", len(all), all)
	}
	for d := byte('0'); d <= '9'; d++ {
		if strings.Count(all, string(d)) != 1 {
			t.Fatalf("the walk's digit pad spells %q; digit %c appears %d time(s), want 1",
				all, d, strings.Count(all, string(d)))
		}
	}
	return pitch, rows
}

// point is digitPoint() from the walk, in Go.
func digitPoint(pitch int, rows []digitRow, ch byte) (image.Point, bool) {
	for _, r := range rows {
		if j := strings.IndexByte(r.digits, ch); j >= 0 {
			return image.Pt(r.x0+j*pitch, r.y), true
		}
	}
	return image.Point{}, false
}

// TestWalkDigitPadCoordinatesTypeTheIntendedNumber is the pin.
//
// It types the S4 fixture's own lock value at the walk's own coordinates,
// through runUITouch's pointer events -- the route a fingertip takes -- and
// requires the fragment to read back after EVERY keystroke. A coordinate that
// drifted by one key fails on the digit it drifted on, naming it, rather than
// composing a policy with a lock nobody chose. A lock is not cosmetic: it is
// how long the funds behind the other path are unspendable.
func TestWalkDigitPadCoordinatesTypeTheIntendedNumber(t *testing.T) {
	pitch, rows := loadWalkDigitPad(t)
	const number = "12960" // the S4 fixture's relative lock, in blocks

	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	var got string
	done := false
	frame, drawer, quit := runUITouch(ctx, func() {
		got, _ = composerDigitEntry(ctx, &descriptorTheme, "Path 2 lock", "How many blocks?", 5,
			composerBlocksBandEcho)
		done = true
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the digit pad drew no frame")
	}

	for i := 0; i < len(number); i++ {
		pt, ok := digitPoint(pitch, rows, number[i])
		if !ok {
			t.Fatalf("the walk has no key for %q", number[i])
		}
		// A tap that lands on NOTHING is a different failure from a tap that
		// lands on the wrong key, and only one of them is a geometry error.
		if _, _, hit := drawer().Hit(pt); !hit {
			t.Fatalf("digit %d (%c): the walk taps %v and there is no touch target there at all",
				i+1, number[i], pt)
		}
		tap(&ctx.Router, drawer(), pt)
		content, ok := frame()
		if !ok {
			t.Fatalf("digit %d (%c): the pad returned mid-number", i+1, number[i])
		}
		want := number[:i+1]
		if !uiContains(content, want) {
			t.Fatalf("digit %d (%c): tapped %v and the field does not read %q; the frame is %q",
				i+1, number[i], pt, want, content)
		}
	}

	// The ECHO the walk asserts, from the production copy rather than a literal.
	content, _ := frame()
	if !uiContains(content, composerCopyLockEchoBlocks(12960)) {
		t.Errorf("the pad does not echo %q after the walk typed %s.\nFrame: %q",
			composerCopyLockEchoBlocks(12960), number, content)
	}
	if done || got != "" {
		t.Fatalf("the pad returned before the walk confirmed (got %q)", got)
	}
}
