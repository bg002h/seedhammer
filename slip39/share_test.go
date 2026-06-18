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
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{nil, ""},
		{errBadChecksum, "bad checksum"},
		{errNotInWordlist, "unknown word"},
		{errUnsupportedSize, "256-bit not supported"},
		{errWrongLength, "wrong length"},
		{errors.New("other"), "invalid"},
	}
	for _, c := range cases {
		if got := Describe(c.in); got != c.want {
			t.Errorf("Describe(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
