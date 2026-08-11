package backup

import (
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/font/constant"
	"seedhammer.com/seal"
)

// SPEC §10.2.1a: "90 MUST be pinned by a test, not trusted as a literal."
//
// seal.MaxEngraveableCodex32Len is the number seal.AdmitSection refuses above,
// and it is DERIVED -- from EngraveSeedString, which uppercases the share,
// encodes it at qr.M and refuses qrc.Size > 33. Two things underneath can move
// it independently:
//
//   - the qrc.Size > 33 cap (F-117/F-118 may raise it deliberately), and
//   - the error-correction level. MEASURED here, not asserted in prose: at qr.Q
//     the limit drops to 67 -- below EncodeMS1's ordinary output -- so that
//     change would BOTH silently reopen F-113 and start rejecting ordinary
//     seeds. Nothing else in the tree would have noticed.
//
// qrScale is NOT among them: the boundary is decided by qr.Encode before
// qrScale is ever read. It changes how big the QR is cut, not which version the
// string needs.
//
// So this test re-runs the real encoder rather than restating 90.
func TestEngraveableLimitIsDerivedFromTheRealQREncoder(t *testing.T) {
	// The seed plate's QR alphabet: uppercased bech32 is all [0-9A-Z], which is
	// QR alphanumeric mode, so the encoded version depends on the LENGTH alone.
	// Sweep past both codex32 bands (short 48-93, long 125-127).
	//
	// seedQRLevel and seedQRMaxSize come from EngraveSeedString, NOT from
	// literals here. A duplicated `qr.M` would keep computing 90 while the real
	// function had moved to another level -- the sweep would agree with the
	// constant and both would be wrong.
	maxAtM := boundary(t, seedQRLevel)
	if maxAtM != seal.MaxEngraveableCodex32Len {
		t.Errorf("the real encoder caps the seed plate at %d characters, but "+
			"seal.MaxEngraveableCodex32Len is %d -- §10.2.1a's constant has drifted "+
			"from what backup.EngraveSeedString will actually cut",
			maxAtM, seal.MaxEngraveableCodex32Len)
	}
	if maxAtM != 90 {
		t.Errorf("boundary moved: qr.M + the qrc.Size > 33 cap now allows %d "+
			"characters, not 90", maxAtM)
	}
	// The other side of the boundary, stated explicitly so a fix cannot pass by
	// making everything refuse.
	c, err := qr.Encode(strings.Repeat("Q", seal.MaxEngraveableCodex32Len+1), seedQRLevel)
	if err != nil || c.Size <= seedQRMaxSize {
		t.Errorf("%d characters must need a QR bigger than %d: got size=%v err=%v",
			seal.MaxEngraveableCodex32Len+1, seedQRMaxSize, c, err)
	}

	// The error-correction level is the silent lever. Record what it would do.
	maxAtQ := boundary(t, qr.Q)
	t.Logf("engraveable limit: qr.M -> %d characters, qr.Q -> %d", maxAtM, maxAtQ)
	if maxAtQ >= maxAtM {
		t.Errorf("premise broken: qr.Q (%d) should be STRICTER than qr.M (%d); "+
			"if it is not, the warning in this test's doc comment is wrong",
			maxAtQ, maxAtM)
	}
}

// boundary returns the longest uppercase-alphanumeric string EngraveSeedString's
// QR step would accept at error-correction level lvl -- i.e. the largest n with
// qr.Encode(n chars, lvl).Size <= seedQRMaxSize.
func boundary(t *testing.T, lvl qr.Level) int {
	t.Helper()
	max := 0
	for n := 1; n <= 140; n++ {
		c, err := qr.Encode(strings.Repeat("Q", n), lvl)
		if err != nil {
			continue
		}
		if c.Size <= seedQRMaxSize {
			max = n
		}
	}
	if max == 0 {
		t.Fatalf("no length encodes to a QR of size <= %d; the sweep is broken", seedQRMaxSize)
	}
	return max
}

// The constant must bind the REAL call, not just the encoder inside it. These
// are the same two vectors seal/engraveable_test.go admits and refuses, driven
// through EngraveSeedString itself.
//
// 42 and 43 bytes of entropy from `biptool seed -seedlen N -id entr`, giving 90
// and 91 characters. 92 is not constructible.
func TestEngraveSeedStringCutsAt90AndRefusesAt91(t *testing.T) {
	const (
		len90 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqutd7mdh2lc8h2"
		len91 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq2uk6ly9a0dmw4"
	)
	if len(len90) != seal.MaxEngraveableCodex32Len {
		t.Fatalf("vector is %d characters, but the limit is %d -- this test no "+
			"longer sits on the boundary", len(len90), seal.MaxEngraveableCodex32Len)
	}
	if len(len91) != seal.MaxEngraveableCodex32Len+1 {
		t.Fatalf("vector is %d characters, want %d",
			len(len91), seal.MaxEngraveableCodex32Len+1)
	}
	if _, err := EngraveSeedString(params, seedPlate(len90)); err != nil {
		t.Errorf("a %d-character share must engrave, got %v",
			seal.MaxEngraveableCodex32Len, err)
	}
	if _, err := EngraveSeedString(params, seedPlate(len91)); err == nil {
		t.Errorf("a %d-character share must be refused by EngraveSeedString; "+
			"if it now cuts, §10.2.1a's limit is too tight and rejects engraveable seeds",
			seal.MaxEngraveableCodex32Len+1)
	}
}

// seedPlate builds the same SeedString gui/unlock_session.go hands to
// EngraveSeedString for an admitted ms1.
func seedPlate(s string) SeedString {
	return SeedString{Title: "entr", Seed: s, Font: constant.Font}
}
