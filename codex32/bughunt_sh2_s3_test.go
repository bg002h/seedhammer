package codex32

import (
	"fmt"
	"strings"
	"testing"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

func bhNewNoPanic(s string) (err error, panicked any) {
	defer func() { panicked = recover() }()
	_, err = New(s)
	return
}

// Bug 11: New panics ("slice bounds out of range [6:5]") on a string whose
// human-readable part is long and whose data part is too short to hold the
// 6-character header and the 13-character checksum, although the string is
// long enough to pass the length gate and its checksum is valid.
func TestBughuntSh211NewShortDataPanics(t *testing.T) {
	// Positive control: a valid string parses.
	if err, p := bhNewNoPanic("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM"); err != nil || p != nil {
		t.Fatalf("control: err=%v panic=%v", err, p)
	}
	const s = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa10leetwl7wjd64zfny6"
	if n := len(s); n < shortCodeMinLength || n > shortCodeMaxLength {
		t.Fatalf("input length %d is outside the band that reaches partsInner", n)
	}
	err, p := bhNewNoPanic(s)
	if p != nil {
		t.Errorf("New panicked: %v", p)
		return
	}
	if err == nil {
		t.Errorf("New accepted a string whose data part (%d chars) cannot hold a 6-char header and a 13-char checksum", len(s)-strings.LastIndex(s, "1")-1)
	}
}

// Bug 47: NewSeed returns err == nil with strings that New rejects (a payload
// whose string lands between the short band, 48-93, and the long band,
// 125-127). The correct behaviour is an error from NewSeed or a string that New
// accepts; which of the two is a product decision, so only the round trip is
// asserted.
func TestBughuntSh247NewSeedOutputRejectedByNew(t *testing.T) {
	t.Skip("deferred-minor: sh-codex32-newseed-length")
	// Positive control: the BIP-93 master seed sizes round-trip.
	for _, n := range []int{16, 20, 24, 28, 32, 64} {
		s, err := NewSeed("ms", 0, "test", 's', make([]byte, n))
		if err != nil {
			t.Fatalf("control %d bytes: NewSeed: %v", n, err)
		}
		if _, err := New(s.String()); err != nil {
			t.Fatalf("control %d bytes: New(NewSeed) = %v", n, err)
		}
	}
	// The sizes the report names, plus the rest of the dead zone for the message.
	var bad []string
	for n := 1; n <= 80; n++ {
		s, err := NewSeed("ms", 0, "test", 's', make([]byte, n))
		if err != nil {
			continue
		}
		if _, err := New(s.String()); err != nil {
			bad = append(bad, fmt.Sprintf("%d bytes->%d chars", n, len(s.String())))
			if n == 47 || n == 48 || n == 50 || n == 51 {
				t.Errorf("NewSeed(ms, 0, test, s, %d bytes) = err nil, %d chars; New: %v", n, len(s.String()), err)
			}
		}
	}
	t.Logf("all payload sizes 1..80 bytes whose NewSeed output New rejects (%d): %s", len(bad), strings.Join(bad, "; "))
}
