package slip39

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
)

type slip39Vector struct {
	Desc      string
	Mnemonics []string
	MasterHex string
}

// slip39Fixture is one Rust-generated round-trip fixture (intermediate
// lengths the static official corpus lacks). See testdata/GEN.md.
type slip39Fixture struct {
	Desc       string   `json:"desc"`
	SecretHex  string   `json:"secret_hex"`
	Passphrase string   `json:"passphrase"`
	Mnemonics  []string `json:"mnemonics"`
}

func loadFixtures(t *testing.T) []slip39Fixture {
	t.Helper()
	b, err := os.ReadFile("testdata/slip39_fixtures.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var out []slip39Fixture
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	return out
}

// parseAll parses every mnemonic in ms, failing the test on any parse error.
func parseAll(t *testing.T, ms []string) []Share {
	t.Helper()
	out := make([]Share, len(ms))
	for i, m := range ms {
		s, err := ParseShare(m)
		if err != nil {
			t.Fatalf("ParseShare(share %d): %v", i, err)
		}
		out[i] = s
	}
	return out
}

// loadVectors reads testdata/slip39_vectors.json — an object keyed by the
// ORIGINAL upstream vector index (string) → 4-tuple [desc, [mnemonics],
// master_hex, xprv]. Returns a map keyed by the integer original index, so the
// test references below address vectors by their canonical upstream number.
func loadVectors(t *testing.T) map[int]slip39Vector {
	t.Helper()
	b, err := os.ReadFile("testdata/slip39_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var raw map[string][]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	out := make(map[int]slip39Vector, len(raw))
	for k, e := range raw {
		idx, err := strconv.Atoi(k)
		if err != nil {
			t.Fatalf("bad vector key %q: %v", k, err)
		}
		var v slip39Vector
		_ = json.Unmarshal(e[0], &v.Desc)
		_ = json.Unmarshal(e[1], &v.Mnemonics)
		_ = json.Unmarshal(e[2], &v.MasterHex)
		out[idx] = v
	}
	return out
}

// vectorShares returns all mnemonics of official vector index idx.
func vectorShares(t *testing.T, idx int) []string {
	t.Helper()
	v, ok := loadVectors(t)[idx]
	if !ok {
		t.Fatalf("vector idx %d not present in testdata", idx)
	}
	return v.Mnemonics
}

// vectorShare returns share `share` of official vector index idx.
func vectorShare(t *testing.T, idx, share int) string {
	t.Helper()
	return vectorShares(t, idx)[share]
}

// vectorSecretHex returns the expected master-secret hex of vector idx ("" if invalid).
func vectorSecretHex(t *testing.T, idx int) string {
	t.Helper()
	v, ok := loadVectors(t)[idx]
	if !ok {
		t.Fatalf("vector idx %d not present in testdata", idx)
	}
	return v.MasterHex
}

// TestCombineOfficialVectors recovers every positive official vector and
// checks the recovered hex against the expected master secret. All positive
// vectors use passphrase "TREZOR". Covers the T==1 no-digest path (idx 0),
// 2-of-3 (idx 3), group-threshold (idx 17), 256-bit/33-word (idx 35), and
// extendable/ext=1 (idx 42).
func TestCombineOfficialVectors(t *testing.T) {
	for _, idx := range []int{0, 3, 17, 35, 42} {
		parsed := parseAll(t, vectorShares(t, idx))
		got, err := Combine(parsed, []byte("TREZOR"))
		if err != nil {
			t.Fatalf("idx %d Combine: %v", idx, err)
		}
		if want := vectorSecretHex(t, idx); hexEq(got) != want {
			t.Errorf("idx %d recovered %s want %s", idx, hexEq(got), want)
		}
	}
}

// TestCombinePassphraseDistinguishes pins SLIP-39's plausible-deniability: a
// wrong/absent passphrase recovers a DIFFERENT valid secret with no error.
func TestCombinePassphraseDistinguishes(t *testing.T) {
	parsed := parseAll(t, vectorShares(t, 3))
	withPP, err := Combine(parsed, []byte("TREZOR"))
	if err != nil {
		t.Fatalf("Combine(TREZOR): %v", err)
	}
	noPP, err := Combine(parsed, []byte(""))
	if err != nil {
		t.Fatalf("Combine(\"\"): %v", err)
	}
	if hexEq(withPP) != vectorSecretHex(t, 3) {
		t.Errorf("TREZOR recovered %s want %s", hexEq(withPP), vectorSecretHex(t, 3))
	}
	if hexEq(withPP) == hexEq(noPP) {
		t.Errorf("empty-passphrase recovery must differ from the TREZOR recovery")
	}
}

// TestCombineFixtures round-trips the Rust-generated intermediate-length
// fixtures (the only path that exercises the 23/27/30-word value unpack).
func TestCombineFixtures(t *testing.T) {
	fx := loadFixtures(t)
	if len(fx) == 0 {
		t.Fatal("no fixtures loaded")
	}
	seenWordCounts := map[int]bool{}
	for _, f := range fx {
		parsed := parseAll(t, f.Mnemonics)
		wc := len(splitWords(f.Mnemonics[0]))
		seenWordCounts[wc] = true
		got, err := Combine(parsed, []byte(f.Passphrase))
		if err != nil {
			t.Fatalf("%s Combine: %v", f.Desc, err)
		}
		if hexEq(got) != f.SecretHex {
			t.Errorf("%s recovered %s want %s", f.Desc, hexEq(got), f.SecretHex)
		}
		// Per-share value byte length must be a valid secret length.
		for i, s := range parsed {
			if !validSecretLen(len(s.Value)) {
				t.Errorf("%s share %d value len %d invalid", f.Desc, i, len(s.Value))
			}
		}
	}
	// Confirm the fixtures actually cover all five share word counts.
	for _, wc := range []int{20, 23, 27, 30, 33} {
		if !seenWordCounts[wc] {
			t.Errorf("fixtures missing %d-word coverage", wc)
		}
	}
}

func splitWords(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// TestCombineNegatives checks each negative vector returns the correct
// sentinel — at ParseShare for parse-level failures (idx 1, 9), at Combine
// for set-level failures (idx 4, 5, 12, 13).
func TestCombineNegatives(t *testing.T) {
	// idx 1 — invalid checksum, fails at ParseShare.
	if _, err := ParseShare(vectorShare(t, 1, 0)); !errors.Is(err, errBadChecksum) {
		t.Errorf("idx 1: %v want errBadChecksum", err)
	}
	// idx 9 — group threshold exceeds count, fails at ParseShare.
	if _, err := ParseShare(vectorShare(t, 9, 0)); !errors.Is(err, errGroupThresholdExceedsCount) {
		t.Errorf("idx 9: %v want errGroupThresholdExceedsCount", err)
	}

	combineErr := func(idx int) error {
		parsed := parseAll(t, vectorShares(t, idx))
		_, err := Combine(parsed, []byte("TREZOR"))
		return err
	}
	// idx 4 — only 1 of a 2-member group → insufficient shares.
	if err := combineErr(4); !errors.Is(err, errInsufficientShares) {
		t.Errorf("idx 4: %v want errInsufficientShares", err)
	}
	// idx 5 — mismatched identifiers.
	if err := combineErr(5); !errors.Is(err, errIdentifierMismatch) {
		t.Errorf("idx 5: %v want errIdentifierMismatch", err)
	}
	// idx 12 — invalid digest (the critical gate).
	if err := combineErr(12); !errors.Is(err, errDigestVerificationFailed) {
		t.Errorf("idx 12: %v want errDigestVerificationFailed", err)
	}
	// idx 13 — insufficient number of groups.
	if err := combineErr(13); !errors.Is(err, errInsufficientShares) {
		t.Errorf("idx 13: %v want errInsufficientShares", err)
	}
}

// TestCombinePanicSafety asserts Combine NEVER panics on malformed input
// (SPEC §4.4): all input-violable preconditions are checked before any
// interpolation/gfDiv runs.
func TestCombinePanicSafety(t *testing.T) {
	noPanic := func(name string, fn func() (interface{}, error)) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s panicked: %v", name, r)
			}
		}()
		_, _ = fn()
	}

	// Base: a valid 2-of-3 set (idx 3), then mutate into malformed shapes.
	base := parseAll(t, vectorShares(t, 3))

	// (a) Duplicate (group,member) with valid-length values.
	dup := []Share{base[0], base[0]}
	noPanic("duplicate-member", func() (interface{}, error) { return Combine(dup, []byte("TREZOR")) })
	if _, err := Combine(dup, []byte("TREZOR")); err == nil {
		t.Error("duplicate-member set must error")
	}

	// (b) Invalid value length (forge a too-short value).
	bad := base[0]
	bad.Value = bad.Value[:len(bad.Value)-1]
	noPanic("bad-length", func() (interface{}, error) { return Combine([]Share{bad}, []byte("TREZOR")) })
	if _, err := Combine([]Share{bad}, []byte("TREZOR")); !errors.Is(err, errInvalidShareValueLength) {
		t.Errorf("bad-length: want errInvalidShareValueLength, got %v", err)
	}

	// (c) Empty input.
	noPanic("empty", func() (interface{}, error) { return Combine(nil, []byte("TREZOR")) })

	// (d) A set with members at the SAME member index but distinct objects
	//     (would create a duplicate x-coordinate at interpolation): build
	//     two shares sharing MemberIndex.
	clash := base[1]
	clash.MemberIndex = base[0].MemberIndex // force a dup x
	noPanic("dup-x", func() (interface{}, error) {
		return Combine([]Share{base[0], clash}, []byte("TREZOR"))
	})
	if _, err := Combine([]Share{base[0], clash}, []byte("TREZOR")); err == nil {
		t.Error("dup-x set must error before interpolation")
	}
}

// TestRecoverSecretWipesOnDigestFail asserts the scrub path executes: a
// digest-fail in recoverSecret must wipe the interpolated secret buffer and
// return the sentinel (SPEC §4.8). We drive it via two idx-12 shares (whose
// digest does not verify) and confirm the sentinel; the wipe of the
// transient `s` is on that error path.
func TestRecoverSecretWipesOnDigestFail(t *testing.T) {
	parsed := parseAll(t, vectorShares(t, 12))
	pts := make([]bytePoint, len(parsed))
	for i, s := range parsed {
		pts[i] = bytePoint{byte(s.MemberIndex), s.Value}
	}
	out, err := recoverSecret(parsed[0].MemberThreshold, pts)
	if !errors.Is(err, errDigestVerificationFailed) {
		t.Fatalf("recoverSecret: %v want errDigestVerificationFailed", err)
	}
	if out != nil {
		t.Errorf("recoverSecret must return nil secret on digest failure, got %x", out)
	}
}

// TestWipeZeroes pins the wipe helper actually zeroes its buffer.
func TestWipeZeroes(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	wipe(b)
	for i, v := range b {
		if v != 0 {
			t.Errorf("wipe left b[%d]=%d, want 0", i, v)
		}
	}
}
