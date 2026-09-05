package seal

import (
	"errors"
	"testing"

	"seedhammer.com/codex32"
)

// F-474: §10.2.1's allow-list refusal must say WHICH record it refused and WHAT
// that record is, so the screen can name it instead of reporting "Payload
// unreadable." after a successful authentication and a ~31 s derivation.
//
// ErrRecordNotPermitted already put both facts in its MESSAGE, and that is not
// enough: gui/unlock_kdf.go reaches the error through errors.Is and cannot take
// a message apart. The facts have to be structural, which is what
// RecordNotPermittedError is for.
//
// The error carries NO record bytes, deliberately. The index, the
// classification and the section are authenticated plaintext and naming them
// leaks nothing -- the same argument §6.4 already won for the record count --
// while the record itself is exactly the thing that must not travel.

// The plate is sealPreimagePlate (record_test.go), the seam corpus's own
// preimage-plate-0x03 row -- ONE literal in this package, not a second copy.
// Its shape is asserted below rather than trusted.

// A 2-of-N codex32 SHARE. IsPreimage requires an UNSHARED single, so a share is
// never a preimage however its payload begins -- which is what makes this the
// right second row: it is refused for a different reason and reports a
// different kind, so a Preimage flag that were hardwired true, or a Class field
// that were ignored, fails here.
const plainShare = "ms12testaqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdq7pl8qdc5tsp"

func TestPreimagePlateIsRefusedByIndexAndNamedAsAPreimage(t *testing.T) {
	// Premises, measured, so a mistyped vector fails loudly instead of
	// degenerating into a test of some other branch.
	c, err := codex32.New(sealPreimagePlate)
	if err != nil {
		t.Fatalf("the plate is not a valid codex32 string: %v", err)
	}
	if !codex32.IsPreimage(c) {
		t.Fatal("the plate is not IsPreimage -- this test would be about ClassUnknown in general")
	}
	if got := Classify([]byte(sealPreimagePlate)); got != ClassUnknown {
		t.Fatalf("H0 makes a preimage plate ClassUnknown; Classify says %v", got)
	}

	d := vectorNamed(t, "D")
	recs := insertAt(d.Public, 1, sealPreimagePlate)
	got, err := AdmitSection(bs(recs), SectionEncrypted)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Fatalf("refused with %v, want ErrRecordNotPermitted", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload -- rejection is whole-payload", len(got))
	}

	// MUTATION: `Index: 0` in place of `Index: i` -> `record 0, want 1`.
	// MUTATION: drop the codex32.IsPreimage arm that sets Preimage -> the
	// Preimage assertion fails and the gui falls back to the generic noun.
	var e *RecordNotPermittedError
	if !errors.As(err, &e) {
		t.Fatalf("the refusal is not a *RecordNotPermittedError (%T) -- the gui cannot name what it refused", err)
	}
	if e.Index != 1 {
		t.Errorf("record %d, want 1 (records count from 0, as `me` counts them)", e.Index)
	}
	if !e.Preimage {
		t.Error("the refusal does not report the record as a hashlock preimage")
	}
	if e.Class != ClassUnknown {
		t.Errorf("class %v, want %v", e.Class, ClassUnknown)
	}
	if e.Section != SectionEncrypted {
		t.Errorf("section %v, want encrypted", e.Section)
	}
	// The bytes never travel with the diagnosis.
	if msg := e.Error(); containsAny(msg, sealPreimagePlate) {
		t.Error("the error message carries the record itself")
	}
}

func TestPlainShareInThePublicSectionIsRefusedByIndexAndClass(t *testing.T) {
	c, err := codex32.New(plainShare)
	if err != nil {
		t.Fatalf("the share is not a valid codex32 string: %v", err)
	}
	if codex32.IsPreimage(c) {
		t.Fatal("a SHARE must never be IsPreimage -- IsPreimage requires an unshared single")
	}
	if got := Classify([]byte(plainShare)); got != ClassCodex32Secret {
		t.Fatalf("a share classifies as %v, want %v", got, ClassCodex32Secret)
	}

	d := vectorNamed(t, "D")
	recs := insertAt(d.Public, 2, plainShare)
	got, err := AdmitSection(bs(recs), SectionPublic)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Fatalf("refused with %v, want ErrRecordNotPermitted", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload", len(got))
	}
	var e *RecordNotPermittedError
	if !errors.As(err, &e) {
		t.Fatalf("the refusal is not a *RecordNotPermittedError (%T)", err)
	}
	// A DIFFERENT index and a DIFFERENT kind from the row above: that is what
	// makes these two rows a test of the fields rather than of two constants.
	if e.Index != 2 {
		t.Errorf("record %d, want 2", e.Index)
	}
	if e.Preimage {
		t.Error("a share was reported as a hashlock preimage")
	}
	if e.Class != ClassCodex32Secret {
		t.Errorf("class %v, want %v", e.Class, ClassCodex32Secret)
	}
	if e.Section != SectionPublic {
		t.Errorf("section %v, want public", e.Section)
	}
}

// Every existing caller reaches this through errors.Is, and none of them may
// break: the typed error is additive.
//
// MUTATION: drop Unwrap() -> every errors.Is call site in seal and in the gui
// stops matching, and this test names that directly.
func TestRecordNotPermittedErrorStillMatchesTheSentinel(t *testing.T) {
	e := &RecordNotPermittedError{Index: 3, Class: ClassAddress, Section: SectionEncrypted}
	if !errors.Is(e, ErrRecordNotPermitted) {
		t.Fatal("the typed error no longer matches ErrRecordNotPermitted -- every existing caller is broken")
	}
	if errors.Is(e, ErrCodex32TooLong) {
		t.Error("it matches the wrong sentinel")
	}
}

func containsAny(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
