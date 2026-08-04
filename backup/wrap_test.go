package backup

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// fixed returns a widthAt closure of constant width.
func fixed(w int) func(int) int { return func(int) int { return w } }

func TestWrapWordBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  []string
	}{
		{"greedy fill", "aa bb cc dd", 5, []string{"aa bb", "cc dd"}},
		{"exact fit", "aaa bbb", 7, []string{"aaa bbb"}},
		{"one over", "aaa bbb", 6, []string{"aaa", "bbb"}},
		{"single word", "hello", 10, []string{"hello"}},
		// U+0020 is the ONLY break opportunity. A hyphen or a slash inside a
		// token must not create one, or an xpub's '/' would silently split a
		// key path across a line.
		{"hyphen is not a break", "aaa-bbb ccc", 7, []string{"aaa-bbb", "ccc"}},
		{"slash is not a break", "aa/bb/cc dd", 8, []string{"aa/bb/cc", "dd"}},
		{"tab is not a break", "aa\tbb cc", 5, []string{"aa\tbb", "cc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := WrapText(tc.s, fixed(tc.width), 20)
			if !ok {
				t.Fatalf("WrapText(%q, %d) refused", tc.s, tc.width)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("WrapText(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
			}
			for i, l := range got {
				if len(l) > tc.width {
					t.Errorf("line %d %q is %d wide, limit %d", i, l, len(l), tc.width)
				}
			}
		})
	}
}

func TestWrapOverlongToken(t *testing.T) {
	// An xpub or a URL must not deadlock the wrap: alone at the start of a
	// line it is character-broken at EXACTLY widthAt(i), never at a smaller
	// "safe" width and never left whole.
	const xpub = "xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE"
	got, ok := WrapText(xpub, fixed(22), 20)
	if !ok {
		t.Fatal("refused")
	}
	if strings.Join(got, "") != xpub {
		t.Errorf("character break lost or duplicated data:\n got %q\nwant %q", strings.Join(got, ""), xpub)
	}
	for i, l := range got[:len(got)-1] {
		if len(l) != 22 {
			t.Errorf("line %d is %d chars, want exactly 22 (break at exactly widthAt)", i, len(l))
		}
	}
}

func TestWrapOverlongTokenNotAlone(t *testing.T) {
	got, ok := WrapText("ab cdefghijkl", fixed(6), 20)
	if !ok {
		t.Fatal("refused")
	}
	// "ab" is committed, so "cdefghijkl" is not "alone at the start of a
	// line": it moves down whole and is broken only once it IS alone.
	want := []string{"ab", "cdefgh", "ijkl"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWrapSpacePrecedence(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  []string
	}{
		// (a) A break consumes exactly the run of spaces at the break point --
		// all three, not one.
		{"break consumes the run", "aa   bb", 5, []string{"aa", "bb"}},
		{"break consumes one space", "aa bb", 3, []string{"aa", "bb"}},
		// (b) Runs NOT at a break are preserved verbatim, so a plate can be a
		// table.
		{"leading indent kept", "  ab cd", 5, []string{"  ab", "cd"}},
		{"interior run kept", "a  b", 6, []string{"a  b"}},
		{"indent kept on every block", "x\n    y", 8, []string{"x", "    y"}},
		// (c) Trailing spaces are stripped from every emitted line, and (c)
		// beats (b) at end of line.
		{"trailing stripped", "ab   ", 10, []string{"ab"}},
		{"interior run at line end stripped", "a  b", 3, []string{"a", "b"}},
		// A line whose whole content would be a space run is emitted EMPTY.
		{"all-space block", "   ", 10, []string{""}},
		{"all-space block narrow", " ", 1, []string{""}},
		{"all-space line between words", "a\n   \nb", 10, []string{"a", "", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := WrapText(tc.s, fixed(tc.width), 20)
			if !ok {
				t.Fatalf("WrapText(%q, %d) refused", tc.s, tc.width)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("WrapText(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
			}
			for i, l := range got {
				if strings.HasSuffix(l, " ") {
					t.Errorf("line %d %q has a trailing space", i, l)
				}
			}
		})
	}
}

func TestWrapVaryingWidth(t *testing.T) {
	// The screw-hole band makes widthAt a function of the line, so a wrap that
	// silently reuses line 0's width would put ink through a screw hole. Line 0
	// is narrow, line 1 wide, line 2 narrow again.
	widths := []int{4, 10, 4}
	widthAt := func(i int) int {
		if i < len(widths) {
			return widths[i]
		}
		return 10
	}
	got, ok := WrapText("aaaa bbbbbbbbbb cccc", widthAt, 20)
	if !ok {
		t.Fatal("refused")
	}
	want := []string{"aaaa", "bbbbbbbbbb", "cccc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i, l := range got {
		if len(l) > widthAt(i) {
			t.Errorf("line %d %q exceeds widthAt(%d)=%d", i, l, i, widthAt(i))
		}
	}

	// And the varying width must be applied to the character-break fallback
	// too, not just to word fitting.
	got, ok = WrapText("abcdefghijklmnopqr", widthAt, 20)
	if !ok {
		t.Fatal("refused")
	}
	want = []string{"abcd", "efghijklmn", "opqr"}
	if !slices.Equal(got, want) {
		t.Errorf("varying width ignored by the character break: got %q, want %q", got, want)
	}
}

func TestWrapExplicitNewlines(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  []string
	}{
		{"blocks", "one\ntwo", 10, []string{"one", "two"}},
		// An empty block emits ONE empty line occupying a full row -- the
		// operator sees a blank row in the preview and gets a blank row on the
		// plate. Mapping it onto a backup.Paragraph would advance offy by 1mm
		// instead (spec 5.3).
		{"empty block", "a\n\nb", 10, []string{"a", "", "b"}},
		{"leading newline", "\na", 10, []string{"", "a"}},
		{"trailing newline", "a\n", 10, []string{"a", ""}},
		{"only newline", "\n", 10, []string{"", ""}},
		{"empty string", "", 10, []string{""}},
		{"consecutive", "a\n\n\nb", 10, []string{"a", "", "", "b"}},
		{"block wraps too", "aa bb\ncc", 2, []string{"aa", "bb", "cc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := WrapText(tc.s, fixed(tc.width), 20)
			if !ok {
				t.Fatalf("WrapText(%q) refused", tc.s)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("WrapText(%q) = %q, want %q", tc.s, got, tc.want)
			}
			// Output lines contain no '\n': each is engraved by its own
			// engrave.String call at its own (offx, offy).
			for i, l := range got {
				if strings.ContainsRune(l, '\n') {
					t.Errorf("line %d %q contains a newline", i, l)
				}
			}
		})
	}
}

func TestWrapRefusesPastMaxLines(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		width    int
		maxLines int
		want     []string
	}{
		{"words", "aa bb cc", 2, 2, []string{"aa", "bb"}},
		// Mid-token: the refusal must not wait for a word boundary that never
		// comes.
		{"mid-token", "aaaaaaaaaa", 4, 2, []string{"aaaa", "aaaa"}},
		{"blocks", "a\nb\nc", 10, 2, []string{"a", "b"}},
		{"empty blocks count", "\n\n\n", 10, 2, []string{"", ""}},
		{"zero lines allowed", "a", 10, 0, nil},
		{"exact fit is not a refusal", "aa bb", 2, 2, []string{"aa", "bb"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := WrapText(tc.s, fixed(tc.width), tc.maxLines)
			wantOK := tc.name == "exact fit is not a refusal"
			if ok != wantOK {
				t.Errorf("WrapText(%q, maxLines=%d) ok = %v, want %v", tc.s, tc.maxLines, ok, wantOK)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("WrapText(%q, maxLines=%d) = %q, want %q", tc.s, tc.maxLines, got, tc.want)
			}
			if len(got) > tc.maxLines {
				t.Errorf("returned %d lines, over maxLines %d", len(got), tc.maxLines)
			}
		})
	}
}

// TestWrapAssertsWidthAtPositive: spec 5.2's fallback slices at exactly
// widthAt(i), so a zero width consumes nothing and appends forever. On a device
// with no OOM killer that is a hang, not a crash -- the machine stops
// responding mid-flow. It must panic instead.
func TestWrapAssertsWidthAtPositive(t *testing.T) {
	for _, w := range []int{0, -1} {
		t.Run(fmt.Sprintf("width%d", w), func(t *testing.T) {
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				WrapText("aaaa", fixed(w), 10)
				done <- nil
			}()
			select {
			case r := <-done:
				if r == nil {
					t.Fatalf("widthAt returning %d did not panic", w)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("widthAt returning %d hung instead of panicking", w)
			}
		})
	}
}

// TestWrapNeverExceedsWidth is the invariant behind every capacity number: no
// emitted line may be wider than the width its own line index allows.
func TestWrapNeverExceedsWidth(t *testing.T) {
	widthAt := func(i int) int { return 3 + i%7 }
	inputs := []string{
		"the quick brown fox jumps over the lazy dog",
		"   indented    and   spaced   out   ",
		"one\n\ntwo\n   \nthree",
		strings.Repeat("z", 200),
		"a b c d e f g h i j k l m n o p",
	}
	for _, s := range inputs {
		lines, _ := WrapText(s, widthAt, 100)
		for i, l := range lines {
			if len(l) > widthAt(i) {
				t.Errorf("input %q line %d %q is %d wide, allowed %d", s, i, l, len(l), widthAt(i))
			}
		}
	}
}

// TestWrapPreservesNonSpaceCharacters: wrapping may drop spaces at break
// points, but it may never drop or reorder anything else. A dropped character
// on a free-text plate is a silently wrong plate.
func TestWrapPreservesNonSpaceCharacters(t *testing.T) {
	inputs := []string{
		"the quick brown fox jumps over the lazy dog",
		"xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE",
		"a  b\nc   d\n\ne",
		"!\"$%&+=?\\^_`|~",
	}
	strip := func(s string) string {
		return strings.NewReplacer(" ", "", "\n", "").Replace(s)
	}
	for _, s := range inputs {
		lines, ok := WrapText(s, func(i int) int { return 5 + i%11 }, 500)
		if !ok {
			t.Fatalf("%q refused", s)
		}
		if got, want := strip(strings.Join(lines, "")), strip(s); got != want {
			t.Errorf("input %q: characters lost\n got %q\nwant %q", s, got, want)
		}
	}
}
