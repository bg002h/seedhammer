package slip39

import (
	"errors"
	"strings"
	"testing"
)

const (
	vecDuckling    = "duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard"
	vecTestify     = "testify swimming academic academic column loyalty smear include exotic bedroom exotic wrist lobe cover grief golden smart junior estimate learn"
	vecDucklingBad = "duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision kidney"
)

func TestParseShare(t *testing.T) {
	s, err := ParseShare(vecDuckling)
	if err != nil {
		t.Fatalf("ParseShare(duckling): %v", err)
	}
	if s.Identifier != 7945 {
		t.Errorf("Identifier = %d, want 7945", s.Identifier)
	}
	if s.Extendable {
		t.Errorf("Extendable = true, want false")
	}
	if s.GroupThreshold != 1 || s.GroupCount != 1 || s.MemberIndex != 0 || s.MemberThreshold != 1 {
		t.Errorf("fields = %+v, want 1-of-1 single-group", s)
	}
	if len(s.Mnemonic) != 20 || s.Mnemonic[0] != "DUCKLING" {
		t.Errorf("Mnemonic = %v (len %d), want 20 canonical-uppercase words starting DUCKLING", s.Mnemonic, len(s.Mnemonic))
	}

	// ext=1 vector exercises the shamir_extendable customization string.
	s, err = ParseShare(vecTestify)
	if err != nil {
		t.Fatalf("ParseShare(testify): %v", err)
	}
	if s.Identifier != 29019 || !s.Extendable {
		t.Errorf("testify fields = %+v, want Identifier=29019 Extendable=true", s)
	}

	// Uppercase input (the GUI feeds LabelFor's uppercase) parses identically.
	if _, err := ParseShare(strings.ToUpper(vecDuckling)); err != nil {
		t.Errorf("uppercase parse: %v", err)
	}

	// Bad checksum.
	if _, err := ParseShare(vecDucklingBad); !errors.Is(err, errBadChecksum) {
		t.Errorf("bad checksum: %v, want errBadChecksum", err)
	}
	// Unknown word.
	bad := "zzzz" + vecDuckling[len("duckling"):]
	if _, err := ParseShare(bad); !errors.Is(err, errNotInWordlist) {
		t.Errorf("unknown word: %v, want errNotInWordlist", err)
	}
	// Wrong length.
	if _, err := ParseShare("duckling enlarge"); !errors.Is(err, errWrongLength) {
		t.Errorf("wrong length: %v, want errWrongLength", err)
	}
	// Empty input → wrong length (0 words).
	if _, err := ParseShare(""); !errors.Is(err, errWrongLength) {
		t.Errorf("empty: %v, want errWrongLength", err)
	}
	// A word count outside {20,23,27,30,33} → wrong length (21 is invalid).
	if _, err := ParseShare(strings.TrimSpace(strings.Repeat("duckling ", 21))); !errors.Is(err, errWrongLength) {
		t.Errorf("21 words: %v, want errWrongLength", err)
	}
	// A prefix of a real word ("ducklin" ⊂ "duckling") must be rejected as
	// not-in-wordlist — exactWord must be exact, not ClosestWord's prefix match.
	if _, err := ParseShare("ducklin" + vecDuckling[len("duckling"):]); !errors.Is(err, errNotInWordlist) {
		t.Errorf("prefix word: %v, want errNotInWordlist", err)
	}
	// Extra interior whitespace is tolerated (strings.Fields collapses runs).
	if _, err := ParseShare(strings.ReplaceAll(vecDuckling, " ", "  ")); err != nil {
		t.Errorf("double-spaced valid share: %v", err)
	}
}

func TestParseShareExtractsValue(t *testing.T) {
	s, err := ParseShare(vectorShare(t, 3, 0)) // official idx 3, 128-bit/20-word
	if err != nil {
		t.Fatalf("ParseShare: %v", err)
	}
	if len(s.Value) != 16 {
		t.Errorf("Value len=%d want 16 (128-bit)", len(s.Value))
	}
	// Long path: idx 35 is 256-bit/33-word.
	s32, err := ParseShare(vectorShare(t, 35, 0))
	if err != nil {
		t.Fatalf("ParseShare(33-word): %v", err)
	}
	if len(s32.Value) != 32 {
		t.Errorf("Value len=%d want 32 (256-bit)", len(s32.Value))
	}
}

func TestParseShareGroupThresholdExceedsCount(t *testing.T) {
	_, err := ParseShare(vectorShare(t, 9, 0)) // official idx 9 — group thr > count
	if !errors.Is(err, errGroupThresholdExceedsCount) {
		t.Errorf("want errGroupThresholdExceedsCount, got %v", err)
	}
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{nil, ""},
		{errBadChecksum, "bad checksum"},
		{errNotInWordlist, "unknown word"},
		{errWrongLength, "wrong length"},
		{errBadPadding, "bad padding"},
		{errGroupThresholdExceedsCount, "group threshold exceeds count"},
		{errors.New("other"), "invalid"},
	}
	for _, c := range cases {
		if got := Describe(c.in); got != c.want {
			t.Errorf("Describe(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
