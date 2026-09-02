package sysw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

const (
	recordClassVectors    = "testdata/record_class_vectors.json"
	recordClassProvenance = "testdata/record_class_vectors.provenance.json"
)

type recordClassRow struct {
	Name     string  `json:"name"`
	Record   string  `json:"record"`
	Class    string  `json:"class"`
	HostLine *string `json:"host_line"`
}

var recordClassByName = map[string]Class{
	"Key": ClassKey, "Hash": ClassHash, "Now": ClassNow, "Unknown": ClassUnknown,
}

func loadRecordClassRows(t *testing.T) []recordClassRow {
	t.Helper()
	raw, err := os.ReadFile(recordClassVectors)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no vendored fixture at %s: %v", recordClassVectors, err)
	}
	pinRaw, err := os.ReadFile(recordClassProvenance)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin at %s: %v", recordClassProvenance, err)
	}
	var pin struct {
		SHA256  string `json:"sha256"`
		Vectors int    `json:"vectors"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(pinRaw, &pin); err != nil {
		t.Fatalf("parsing pin: %v", err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != pin.SHA256 {
		t.Fatalf("fixture sha256 %x, pin says %s", sum, pin.SHA256)
	}
	if len(pin.Commit) != 40 {
		t.Fatalf("pin commit %q is not a full SHA", pin.Commit)
	}
	var rows []recordClassRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if len(rows) != pin.Vectors || len(rows) != 45 {
		t.Fatalf("fixture has %d rows, pin says %d, plan says 45", len(rows), pin.Vectors)
	}
	return rows
}

// SPEC §12 item 8: each key:, hash:, now: record (valid and each §6a
// malformation) classifies identically on the host and on the device. Rust's
// answer is the row; a disagreement is fixed in Go.
func TestComposerRecordsClassifyExactlyAsTheHost(t *testing.T) {
	rows := loadRecordClassRows(t)
	seen := map[string]int{}
	for _, row := range rows {
		want, ok := recordClassByName[row.Class]
		if !ok {
			t.Fatalf("%s: fixture class %q is not one this test knows", row.Name, row.Class)
		}
		seen[row.Class]++
		if got := Classify(row.Record); got != want {
			t.Errorf("%s: Classify(%.60q) = %v, want %v (host's answer)", row.Name, row.Record, got, want)
		}
		// A malformed composer record is Unknown on the device and carries the
		// host's line; a valid one has none. The two must agree row by row.
		if (row.HostLine != nil) != (want == ClassUnknown) {
			t.Errorf("%s: host_line present=%v but class %s", row.Name, row.HostLine != nil, row.Class)
		}
	}
	for _, cls := range []string{"Key", "Hash", "Now", "Unknown"} {
		if seen[cls] == 0 {
			t.Errorf("fixture exercises no %s row; the gate would prove nothing for that class", cls)
		}
	}
}

// The parsed values behind the classes, on the fixture's valid rows.
func TestComposerRecordParsersReturnTheHostsValues(t *testing.T) {
	rows := loadRecordClassRows(t)
	byName := map[string]recordClassRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	k, err := ParseKeyRecord(byName["key-journey-cosigner-0"].Record)
	if err != nil {
		t.Fatalf("key-journey-cosigner-0: %v", err)
	}
	// bip32.Path.String renders hardening as `h` (bip32/bip32.go:20-35).
	if hex.EncodeToString(k.Fingerprint[:]) != "73c5da0a" || k.Origin.String() != "m/48h/0h/0h/2h" {
		t.Errorf("key = fp %x origin %s", k.Fingerprint, k.Origin.String())
	}
	if k.Xpub != "xpub6DkFAXWQ2dHxq2vatrt9qyA3bXYU4ToWQwCHbf5XB2mSTexcHZCeKS1VZYcPoBd5X8yVcbXFHJR9R8UCVpt82VX1VhR28mCyxUFL4r6KFrf" {
		t.Errorf("xpub = %s", k.Xpub)
	}
	if k.Text != "[73c5da0a/48'/0'/0'/2']"+k.Xpub {
		t.Errorf("text = %s", k.Text)
	}
	if _, err := ParseKeyRecord(byName["key-depth-3-valid"].Record); err != nil {
		t.Errorf("depth-3 key: %v", err)
	}
	if _, err := ParseKeyRecord(byName["key-testnet-tpub-valid"].Record); err != nil {
		t.Errorf("tpub key: %v", err)
	}
	h, err := ParseHashRecord(byName["hash-valid"].Record)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range h {
		if b != 0xa8 {
			t.Fatalf("digest[%d] = %x", i, b)
		}
	}
	n, err := ParseNowRecord(byName["now-seconds-and-height"].Record)
	if err != nil {
		t.Fatal(err)
	}
	if n.Seconds != 1_756_684_800 || !n.HasHeight || n.Height != 910_000 {
		t.Errorf("now = %+v", n)
	}
	n, err = ParseNowRecord(byName["now-max-both"].Record)
	if err != nil {
		t.Fatal(err)
	}
	if n.Seconds != 2_147_483_647 || n.Height != 499_999_999 {
		t.Errorf("now max = %+v", n)
	}
	n, err = ParseNowRecord(byName["now-seconds-only"].Record)
	if err != nil || n.HasHeight {
		t.Errorf("seconds-only = %+v, %v", n, err)
	}
}

// Classification ORDER: a composer prefix is matched before the constellation
// sniffers, so a record that happens to be BCH-valid or mnemonic-shaped after
// its prefix is never claimed by them; and the three classes are not secret.
func TestComposerClassesArePrefixMatchedAndNotSecret(t *testing.T) {
	for _, c := range []Class{ClassKey, ClassHash, ClassNow} {
		if c.IsSecret() {
			t.Errorf("%v reports secret; key:/hash:/now: are public (SPEC §6a)", c)
		}
	}
	for _, r := range []string{"key", "hash", "now", "Key:00", "KEY:00", "key :00", " key:00"} {
		if got := Classify(r); got == ClassKey || got == ClassHash || got == ClassNow {
			t.Errorf("%q classified as a composer record", r)
		}
		if IsComposerRecord(r) {
			t.Errorf("IsComposerRecord(%q) = true", r)
		}
	}
	if !IsComposerRecord("key:") || !IsComposerRecord("hash:zz") || !IsComposerRecord("now:") {
		t.Error("a prefixed record is ours even when malformed (it is refused, not passed on)")
	}
}

// §6a's digit-COUNT bound is independent of the range bound (composer-S2-plan-R0-r0-tests
// I-1): an in-range value padded with leading zeros past 10 (seconds) or 9 (height) digits
// refuses. The fixture rows now-seconds-eleven-digits / now-height-ten-digits are the
// lockstep leg; this is the unit leg, so a mutation of digitsInRange's length check fails
// here even if the fixture is re-pinned.
func TestNowRecordDigitCountIsBoundedIndependentlyOfRange(t *testing.T) {
	now := func(text string) string { return NowPrefix + hex.EncodeToString([]byte(text)) }
	for _, ok := range []string{"1756684800", "0001756800", "1756684800,499999999", "1756684800,000000001"} {
		if _, err := ParseNowRecord(now(ok)); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"01756684800", "00000000001", "1756684800,0499999999", "1756684800,0000000001"} {
		if _, err := ParseNowRecord(now(bad)); err == nil {
			t.Errorf("%q accepted: the digit count is not bounded", bad)
		}
	}
}

// The path grammar the host accepts: ' and h harden; H, signs and blanks refuse.
func TestKeyRecordPathGrammarMatchesTheHost(t *testing.T) {
	const xpub = "xpub6DkFAXWQ2dHxq2vatrt9qyA3bXYU4ToWQwCHbf5XB2mSTexcHZCeKS1VZYcPoBd5X8yVcbXFHJR9R8UCVpt82VX1VhR28mCyxUFL4r6KFrf"
	rec := func(origin string) string { return KeyPrefix + hex.EncodeToString([]byte("["+origin+"]"+xpub)) }
	for _, ok := range []string{"73c5da0a/48'/0'/0'/2'", "73c5da0a/48h/0h/0h/2h", "73c5da0a/48'/0h/0'/2h"} {
		if _, err := ParseKeyRecord(rec(ok)); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"73c5da0a/48H/0H/0H/2H", "73c5da0a/+48'/0'/0'/2'", "73c5da0a/-48'/0'/0'/2'", "73c5da0a/48'/0'/0'/2'/",
		"73c5da0a/48'/0'//2'", "73c5da0a/ 48'/0'/0'/2'", "73c5da0a/48'/0'/0'/2147483648'", "73C5DA0A/48'/0'/0'/2'",
		"73c5da0/48'/0'/0'/2'", "73c5da0a", "73c5da0a/", "73c5da0a/48'/0'/0'/3'", "73c5da0a/48'/0'/2'",
	} {
		if _, err := ParseKeyRecord(rec(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
