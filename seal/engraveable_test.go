package seal

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

// SPEC §10.2.1a — an `ms1` record must be ENGRAVEABLE to be admitted.
//
// A codex32 secret longer than MaxEngraveableCodex32Len cannot be cut:
// backup.EngraveSeedString uppercases the share, encodes it at qr.M and refuses
// qrc.Size > 33. Before this rule such a record reached the engraver and
// surfaced as "Payload unreadable" AFTER a successful authentication and a
// ~31 s key derivation — telling an operator with an intact backup that their
// payload had been tampered with.
//
// THE BOUNDARY VECTORS, and where they come from. Real BIP-93 codex32 secrets,
// generated rather than hand-written:
//
//	head -c N /dev/zero | go run ./cmd/biptool seed -seedlen N -id entr
//
// N = 42, 43, 44, 63, 64 bytes → 90, 91, 93, 125, 127 characters. Measured.
//
//   - 92 is NOT constructible. A short code is 9 + ceil(8N/5) + 13 characters,
//     which steps 90 → 91 → 93.
//   - 124 CANNOT exercise this rule and is deliberately absent. It falls in the
//     dead zone between codex32's two length bands (short 48–93, long 125–127),
//     so codex32.New rejects it outright and the record never reaches the
//     length check. A test using 124 would prove nothing about §10.2.1a.
const (
	ms1Len90  = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqutd7mdh2lc8h2"
	ms1Len91  = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq2uk6ly9a0dmw4"
	ms1Len93  = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqmtf88e60hz9eu"
	ms1Len125 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqt042k235w95p5rd"
	ms1Len127 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqmk6rc3gq4c88nvp"
)

// The vectors must be what they claim, and they must be real codex32 secrets —
// otherwise every assertion below is testing the allow-list instead of the
// length rule, and would stay green with the rule deleted.
func TestEngraveableVectorsAreWhatTheyClaim(t *testing.T) {
	for _, c := range []struct {
		s    string
		want int
	}{
		{ms1Len90, 90},
		{ms1Len91, 91},
		{ms1Len93, 93},
		{ms1Len125, 125},
		{ms1Len127, 127},
	} {
		if got := len(c.s); got != c.want {
			t.Errorf("vector is %d characters, want %d", got, c.want)
		}
		if _, err := codex32.New(c.s); err != nil {
			t.Errorf("%d-character vector must be valid codex32: %v", c.want, err)
		}
		if got := Classify([]byte(c.s)); got != ClassCodex32Secret {
			t.Errorf("%d-character vector must classify as a codex32 secret, got %v", c.want, got)
		}
	}
	// 92 is not constructible, and 124 is in the dead zone. Pin the second
	// claim against the real engine rather than asserting it in prose.
	dead := "ms10entrsq" + strings.Repeat("q", 114)
	if len(dead) != 124 {
		t.Fatalf("dead-zone probe is %d characters, want 124", len(dead))
	}
	if _, err := codex32.New(dead); err == nil {
		t.Error("124 characters must be rejected by codex32.New itself — if this " +
			"ever becomes valid, 124 would need its own vector here")
	}
}

// 90 characters is the LAST length the seed plate can hold, so it is ADMITTED.
// This is the assertion that stops a fix for the over-length case from being
// written one character too tight and rejecting real seeds.
func TestAdmitsACodex32SecretAtTheEngraveableLimit(t *testing.T) {
	got, err := AdmitSection(bs([]string{ms1Len90}), SectionEncrypted)
	if err != nil {
		t.Fatalf("a 90-character codex32 secret must be admitted: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d admitted records, want 1", len(got))
	}
	if got[0].Class != ClassCodex32Secret {
		t.Errorf("admitted as %v, want %v", got[0].Class, ClassCodex32Secret)
	}
	if string(got[0].Record) != ms1Len90 {
		t.Errorf("admitted record was altered")
	}
}

// The rule itself. Both over-length bands: 91–93 short codes and every long
// code. That is why §10.2.1a is stated as ONE length and not as "reject long
// codes" — codex32.New admits 48–93 and 125–127, and both bands overhang the
// engraver.
func TestRefusesACodex32SecretTooLongToEngrave(t *testing.T) {
	for _, s := range []string{ms1Len91, ms1Len93, ms1Len125, ms1Len127} {
		got, err := AdmitSection(bs([]string{s}), SectionEncrypted)
		// §6.4: the payload is refused WHOLE. Phase A's expression of
		// "nothing was engraved" is that no record comes back at all.
		if len(got) != 0 {
			t.Errorf("%d-character secret: %d records admitted on a rejected payload",
				len(s), len(got))
		}
		if err == nil {
			t.Errorf("%d-character secret: admitted, want ErrCodex32TooLong", len(s))
			continue
		}
		if !errors.Is(err, ErrCodex32TooLong) {
			t.Errorf("%d-character secret: got %v, want ErrCodex32TooLong", len(s), err)
		}
		// §6.4 distinguishability: the operator must learn the record is too
		// long, not that their payload is corrupt. Length and classification
		// are authenticated plaintext, so naming them leaks nothing.
		if !strings.Contains(err.Error(), "characters") ||
			!strings.Contains(err.Error(), "engrave") {
			t.Errorf("%d-character secret: message must name the limit as an "+
				"engraving one, got %q", len(s), err)
		}
	}
}

// WHERE the check runs, and this is the whole point of §10.2.1a's placement
// paragraph. In AdmitSection's PER-RECORD pass, so an over-length secret at
// index 1 is caught before index 2 is ever copied. In the post-loop section
// block it would leak every ms1 already copied — unreachable to both
// Payload.Wipe and RecordsResident().
//
// The over-length record sits at index 1 of three, NOT index 0: at index 0 this
// test passes under an implementation that checks the first record and trusts
// the rest.
func TestTooLongSecretIsCaughtInThePerRecordPass(t *testing.T) {
	d := vectorNamed(t, "D")
	good := d.Secret[0]
	if Classify([]byte(good)) != ClassCodex32Secret {
		t.Fatalf("premise broken: vector D's first secret must be an ms1, got %v",
			Classify([]byte(good)))
	}
	recs := []string{good, ms1Len127, good}
	got, err := AdmitSection(bs(recs), SectionEncrypted)
	if !errors.Is(err, ErrCodex32TooLong) {
		t.Fatalf("got %v, want ErrCodex32TooLong", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload", len(got))
	}

	// THE DISCRIMINATOR between the per-record pass and the post-loop section
	// block, and the only one there is through the public API.
	//
	// The wipe() call itself is NOT observable: `out` is never returned on this
	// path and every AdmittedRecord.Record is its own allocation, so no test can
	// reach those bytes without unsafe -- the same honest limitation the wipe
	// doc comment already records for the two allow-list call sites. What IS
	// observable is WHICH record the section stops at. Put an over-length secret
	// at index 0 and a record the allow-list refuses at index 1:
	//
	//	per-record pass  -> record 0 is refused before record 1 is classified
	//	                    => ErrCodex32TooLong
	//	post-loop block  -> the whole loop runs first and meets record 1
	//	                    => ErrRecordNotPermitted
	//
	// So a check moved out of the loop fails here even though it returns the
	// same "nothing admitted".
	got, err = AdmitSection(bs([]string{ms1Len127, "command: lock-boot"}), SectionEncrypted)
	if !errors.Is(err, ErrCodex32TooLong) {
		t.Errorf("got %v, want ErrCodex32TooLong -- an over-length secret at index 0 "+
			"must stop the section THERE. Reporting record 1's failure means the "+
			"check ran after the copy loop, where it leaks every ms1 already copied",
			err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload", len(got))
	}
}

// SCOPE: `ms1` ONLY. md/mk records CHUNK — a descriptor too long for one record
// is split across records that reassemble by (HRP, chunk_set_id) — and md/mk
// plates have VARIANTS, so a plate that does not fit can be replaced by one
// that does. A seed share has neither, which is why it alone needs this rule.
//
// Nothing here may start rejecting md1/mk1 by length. Vector D's public section
// carries records well past the limit.
func TestEngraveableLimitDoesNotCoverMDMKRecords(t *testing.T) {
	d := vectorNamed(t, "D")
	var long []string
	for _, r := range d.Public {
		if len(r) > MaxEngraveableCodex32Len {
			long = append(long, r)
		}
	}
	if len(long) == 0 {
		t.Fatalf("premise broken: vector D's public section has no record over %d "+
			"characters, so this test asserts nothing", MaxEngraveableCodex32Len)
	}
	t.Logf("vector D public section carries %d record(s) over %d characters",
		len(long), MaxEngraveableCodex32Len)
	if _, err := AdmitSection(bs(d.Public), SectionPublic); err != nil {
		t.Fatalf("md1/mk1 records over the limit must still be admitted: %v", err)
	}
}

// A BIP-39 mnemonic is not covered either (§10.2.1a scope). Vector A's secret
// is a bare 24-word mnemonic, which is 155 characters — well past the limit.
func TestEngraveableLimitDoesNotCoverMnemonics(t *testing.T) {
	a := vectorNamed(t, "A")
	var mnem string
	for _, r := range a.Secret {
		if Classify([]byte(r)) == ClassMnemonic {
			mnem = r
		}
	}
	if mnem == "" {
		t.Fatal("premise broken: vector A must carry a bare mnemonic")
	}
	if len(mnem) <= MaxEngraveableCodex32Len {
		t.Fatalf("premise broken: the mnemonic is %d characters, not over the %d limit",
			len(mnem), MaxEngraveableCodex32Len)
	}
	if _, err := AdmitSection(bs([]string{mnem}), SectionEncrypted); err != nil {
		t.Fatalf("a %d-character mnemonic must still be admitted: %v", len(mnem), err)
	}
}

// ORDER inside the per-record pass: the §10.2.1 allow-list runs FIRST. An
// over-length codex32 secret in the PUBLIC section is a secret shipped in the
// clear, which is a far more serious finding than a plate that does not fit,
// and the operator must be told the serious one.
func TestPublicSectionStillReportsAnOverLongSecretAsNotPermitted(t *testing.T) {
	got, err := AdmitSection(bs([]string{ms1Len127}), SectionPublic)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("a secret in the public section must report ErrRecordNotPermitted, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload", len(got))
	}
}
