package gui

import (
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// `[mdmk-decode]` (§12.6) where the device actually uses it: classification is
// AT LOAD (§3.2.1), and confirmation rides with it. Re-deciding it at the point
// of use would let one byte string be admitted under one answer and consumed
// under another.

// The same cards vector S-J carries, so the session test and the cross-language
// vector cannot drift apart.
const (
	gMD1A     = "md1fv9wjpqpqpm6jzzqqvqpdqnf4ztqq4gy99tzyzyzdv7xh9vpdwu3t7dhhesk2tl3"
	gMD1B     = "md1fv9wjpqg0yq82l0czvx85ae43vtfd26hsmngjecmqy44k2pgttqh74qwxlawq374"
	gMD1C     = "md1fv9wjpqsp2026hh65xpvugtfhd9792zxgunymm0a82pdju6442q0jskj9gzfaqmz"
	gSeed     = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	gFreeText = "text:6869"
)

func TestSyswSessionMarksUnconfirmedRecordsAtLoad(t *testing.T) {
	var s syswSession
	// A complete 3-chunk card plus one record of another class: nothing here is
	// unconfirmed, which is the direction that catches a marker stuck on.
	p := &sysw.Payload{Public: []string{gMD1A, gMD1B, gMD1C, gFreeText}}
	s.load(p, [32]byte{}, false, true, true, true)
	for i, r := range s.records {
		if r.unconfirmed {
			t.Errorf("record %d of a complete card set is marked unconfirmed", i)
		}
	}

	// And the other direction: one chunk of the same declared set, alone.
	var t2 syswSession
	t2.load(&sysw.Payload{Public: []string{gFreeText, gMD1A}}, [32]byte{}, false, true, true, true)
	if t2.records[0].unconfirmed {
		t.Error("a text: record is not ClassMDMK and must never be marked")
	}
	if !t2.records[1].unconfirmed {
		t.Error("a lone chunk of a declared 3-chunk set must be marked unconfirmed")
	}
}

// The indices `MDMKUnconfirmed` returns are into the list it was GIVEN, and the
// session's list is Public then Secret. Marking the wrong record would tell the
// operator the wrong card is a secret -- and, worse, tell them a real one is fine.
func TestUnconfirmedMarkingSurvivesTheSecretRecordsBeingAppended(t *testing.T) {
	var s syswSession
	s.load(&sysw.Payload{
		Public: []string{gFreeText, gMD1A},
		Secret: []string{gSeed},
	}, [32]byte{}, true, true, true, true)
	if len(s.records) != 3 {
		t.Fatalf("INCONCLUSIVE: %d records, want 3", len(s.records))
	}
	if s.records[0].unconfirmed || s.records[2].unconfirmed {
		t.Error("only the md1 may be marked")
	}
	if !s.records[1].unconfirmed {
		t.Error("the lone chunk must still be the one marked once secrets are appended")
	}
	if s.records[2].class != sysw.ClassMnemonic {
		t.Fatalf("INCONCLUSIVE: the secret record did not survive the load: %v", s.records[2].class)
	}
}

// §12.6's flag consequence, end to end: the load screen must NAME this case
// rather than folding it into "A SECRET is stored unencrypted in flash". The
// operator who wrote a partial card set deliberately needs to know that is what
// the machine is complaining about, or the warning reads as data loss.
func TestSyswLoadWarningsNamesTheUnconfirmedCaseDistinctly(t *testing.T) {
	var s syswSession
	s.load(&sysw.Payload{Public: []string{gMD1A}}, [32]byte{}, false, true, true, true)
	lines := syswLoadWarnings(&s)
	if len(lines) == 0 {
		t.Fatal("an unconfirmed md1 in a plaintext container must raise F1")
	}
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "md1/mk1") || !strings.Contains(joined, "could not confirm") {
		t.Errorf("the warning does not name the case: %q", lines)
	}

	// The other direction. A COMPLETE card set in the same container warns about
	// nothing, and a real secret still gets the plain sentence.
	var ok syswSession
	ok.load(&sysw.Payload{Public: []string{gMD1A, gMD1B, gMD1C}}, [32]byte{}, false, true, true, true)
	if got := syswLoadWarnings(&ok); len(got) != 0 {
		t.Errorf("a complete card set must warn about nothing; got %q", got)
	}

	var secret syswSession
	secret.load(&sysw.Payload{Public: []string{gSeed}}, [32]byte{}, false, true, true, true)
	got := strings.Join(syswLoadWarnings(&secret), " ")
	if !strings.Contains(got, "SECRET") {
		t.Errorf("a real secret must still raise the plain F1 line; got %q", got)
	}
	if strings.Contains(got, "could not confirm") {
		t.Errorf("a real secret must NOT be described as an unconfirmed card; got %q", got)
	}
}

// Both lines can be needed at once -- a payload holding a seed AND a lone chunk
// raises F1 twice for two different reasons, and de-duplicating by flag alone
// would silently drop one of them.
func TestBothF1LinesAppearWhenBothCausesArePresent(t *testing.T) {
	var s syswSession
	s.load(&sysw.Payload{Public: []string{gSeed, gMD1A}}, [32]byte{}, false, true, true, true)
	lines := syswLoadWarnings(&s)
	var plain, named bool
	for _, l := range lines {
		if strings.Contains(l, "could not confirm") {
			named = true
		} else if strings.Contains(l, "SECRET") {
			plain = true
		}
	}
	if !plain || !named {
		t.Errorf("want both F1 sentences, got %q", lines)
	}
}
