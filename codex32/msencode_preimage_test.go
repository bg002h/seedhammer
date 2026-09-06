package codex32

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

// H6 §3.5: the DEVICE half of the ms1 preimage encoder, a PORT of
// ms_codec::encode(Tag::HASH, &Payload::Preimage(x)).
//
// Without it a composer-derived preimage has no ms1 string to engrave and the
// string form — decision 1's DEFAULT — silently becomes payload-only.

// TestEncodeMS1PreimageMatchesTheCorpusKindRow is the lockstep: the wrapper
// reproduces the vendored ms-codec corpus's `kind` row byte for byte, so a
// drift in either repo reds here rather than on a plate.
//
// MUTATION: pass "entr" to NewSeed -> the ms1 comparison fails with
// `ms10entrsqw46...kp9wv63u5a0u7q, want the corpus ms10hashsqw46...kzv2ncy60u7z9c`.
// MUTATION: drop the msPrefixPreimage byte -> New() still parses but the
// string is 73 characters and the comparison fails.
func TestEncodeMS1PreimageMatchesTheCorpusKindRow(t *testing.T) {
	c := loadHashlockCorpus(t)
	var x [32]byte
	copy(x[:], mustHexT(t, c.Kind[0].PreimageHex))

	got, err := EncodeMS1Preimage(x)
	if err != nil {
		t.Fatalf("EncodeMS1Preimage: %v", err)
	}
	if got != c.Kind[0].MS1 {
		t.Fatalf("EncodeMS1Preimage = %s, want the corpus %s", got, c.Kind[0].MS1)
	}
	if len(got) != 75 {
		t.Errorf("len = %d, want 75 (SPEC_ms_hashlock §1 rule 2)", len(got))
	}
	s, err := New(got)
	if err != nil {
		t.Fatalf("New(EncodeMS1Preimage(x)): %v", err)
	}
	if id, _, _ := s.Split(); id != "hash" {
		t.Errorf("id = %q, want \"hash\"", id)
	}
	if !IsPreimage(s) {
		t.Error("IsPreimage(EncodeMS1Preimage(x)) = false")
	}
	if !IsPreimagePlate(s) {
		t.Error("IsPreimagePlate(EncodeMS1Preimage(x)) = false")
	}
}

// TestEncodeMS1PreimageRoundTrips is the inverse over random X.
//
// MUTATION: append x[:31] -> DecodeMS1Preimage returns errMSBadLength.
func TestEncodeMS1PreimageRoundTrips(t *testing.T) {
	for i := 0; i < 64; i++ {
		var x [32]byte
		if _, err := rand.Read(x[:]); err != nil {
			t.Fatal(err)
		}
		enc, err := EncodeMS1Preimage(x)
		if err != nil {
			t.Fatalf("EncodeMS1Preimage: %v", err)
		}
		s, err := New(enc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		back, err := DecodeMS1Preimage(s)
		if err != nil {
			t.Fatalf("DecodeMS1Preimage: %v", err)
		}
		if !bytes.Equal(back[:], x[:]) {
			t.Fatalf("round trip: got %x, want %x", back, x)
		}
	}
}

// TestEncodeMS1PreimageCanNeverEmitIDEntr asserts the property on the OUTPUT,
// not on the source, so a future refactor that reintroduces an id parameter
// reds here rather than on steel.
//
// MUTATION: pass "entr" to NewSeed -> every iteration fails with
// `id = "entr"`; the string produced satisfies IsPreimage and FAILS
// IsPreimagePlate, which is the copy-paste hazard reproduced.
// MUTATION: reintroduce an `id string` parameter -> this test no longer
// compiles against a one-argument call, which is the point.
func TestEncodeMS1PreimageCanNeverEmitIDEntr(t *testing.T) {
	for i := 0; i < 256; i++ {
		var x [32]byte
		if _, err := rand.Read(x[:]); err != nil {
			t.Fatal(err)
		}
		enc, err := EncodeMS1Preimage(x)
		if err != nil {
			t.Fatalf("EncodeMS1Preimage: %v", err)
		}
		s, err := New(enc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		id, _, _ := s.Split()
		if id == "entr" {
			t.Fatalf("EncodeMS1Preimage emitted id %q for x=%x", id, x)
		}
		if id != "hash" {
			t.Fatalf("id = %q, want \"hash\"", id)
		}
	}
}

// TestIsPreimagePlateNarrowsByID is §4.3: the ADMISSION predicate consults the
// id, the INERTNESS predicate does not, and the mistagged string is exactly
// where they part.
//
// MUTATION: drop the id test from IsPreimagePlate -> the mistagged row is
// admitted, which is the row that stands between a plain BIP-93 33-byte secret
// beginning 0x03 (roughly 1 in 256 of them) and a flow that engraves it.
func TestIsPreimagePlateNarrowsByID(t *testing.T) {
	c := loadHashlockCorpus(t)
	var x [32]byte
	copy(x[:], mustHexT(t, c.Kind[0].PreimageHex))

	// The mistagged plate: the SAME kind-0x03 payload under id "entr", which
	// NewSeed mints without complaint and the Rust encoder refuses outright.
	payload := append([]byte{msPrefixPreimage}, x[:]...)
	mis, err := NewSeed("ms", 0, "entr", 's', payload)
	if err != nil {
		t.Fatalf("NewSeed(entr, 0x03‖X): %v -- if this now REFUSES, the hazard "+
			"IsPreimagePlate guards has moved into the primitive and this test "+
			"needs rewriting, not deleting", err)
	}
	if !IsPreimage(mis) {
		t.Fatal("the mistagged plate does not satisfy IsPreimage; the two predicates no longer part here")
	}
	if IsPreimagePlate(mis) {
		t.Errorf("IsPreimagePlate(%s) = true: a kind-0x03 payload under id %q was admitted", mis.String(), "entr")
	}

	good, err := New(c.Kind[0].MS1)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPreimagePlate(good) {
		t.Error("IsPreimagePlate(the corpus plate) = false")
	}

	// THE UPPERCASE PLATE -- the QR-alphanumeric spelling -- is a preimage by
	// the wide predicate and is NOT admissible. Split() reports the id as it is
	// written, and section 5.3 hashes a record in its canonical LOWERCASE form,
	// so an uppercase string is not the record it looks like. The host's
	// `preimage_plate_admissible` compares the id bytes case-SENSITIVELY for
	// exactly this reason, and `a_preimage_plate_is_named_not_misdiagnosed`
	// pins the same row over there.
	//
	// MUTATION: lowercase the id before comparing (or use EqualFold) -> this
	// row admits the uppercase plate and the two sides disagree.
	up, err := New(strings.ToUpper(c.Kind[0].MS1))
	if err != nil {
		t.Fatalf("New(the UPPERCASE plate): %v", err)
	}
	if !IsPreimage(up) {
		t.Error("the uppercase plate is not a preimage by the WIDE predicate; the two no longer part here")
	}
	if IsPreimagePlate(up) {
		t.Error("IsPreimagePlate admitted the UPPERCASE spelling of a plate")
	}

	// The entr-32 pair row: a real 32-byte entr secret, neither predicate.
	pair, err := New(c.Kind[0].Entr32PairMS1)
	if err != nil {
		t.Fatal(err)
	}
	if IsPreimage(pair) || IsPreimagePlate(pair) {
		t.Error("the entr-32 pair row answered a preimage predicate")
	}
}
