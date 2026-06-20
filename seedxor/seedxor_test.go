package seedxor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"seedhammer.com/bip39"
)

type vector struct {
	Name   string   `json:"name"`
	Words  int      `json:"words"`
	Parts  []string `json:"parts"`
	Result string   `json:"result"`
}

func loadVectors(t *testing.T) []vector {
	t.Helper()
	buf, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatalf("read vectors.json: %v", err)
	}
	var vs []vector
	if err := json.Unmarshal(buf, &vs); err != nil {
		t.Fatalf("parse vectors.json: %v", err)
	}
	if len(vs) == 0 {
		t.Fatal("no vectors loaded")
	}
	return vs
}

// parseM parses a space-separated phrase into a checksum-valid Mnemonic.
func parseM(t *testing.T, s string) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	if !m.Valid() {
		t.Fatalf("parsed mnemonic not checksum-valid: %q", s)
	}
	return m
}

func TestCombineVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			parts := make([]bip39.Mnemonic, len(v.Parts))
			for i, p := range v.Parts {
				parts[i] = parseM(t, p)
			}
			want := parseM(t, v.Result)

			got, err := Combine(parts)
			if err != nil {
				t.Fatalf("Combine: unexpected error: %v", err)
			}
			if got.String() != want.String() {
				t.Errorf("Combine result = %q, want %q", got.String(), want.String())
			}
			// Pin the recovered entropy bytewise too (independent of the
			// word-rendering path).
			if string(got.Entropy()) != string(want.Entropy()) {
				t.Errorf("Combine entropy = %x, want %x", got.Entropy(), want.Entropy())
			}
		})
	}
}

func TestCombineOrderIndependent(t *testing.T) {
	// XOR commutes: any permutation of the parts yields the same result.
	for _, v := range loadVectors(t) {
		if len(v.Parts) < 3 {
			continue // need >=3 to make a meaningful reorder
		}
		t.Run(v.Name, func(t *testing.T) {
			parts := make([]bip39.Mnemonic, len(v.Parts))
			for i, p := range v.Parts {
				parts[i] = parseM(t, p)
			}
			fwd, err := Combine(parts)
			if err != nil {
				t.Fatalf("Combine(forward): %v", err)
			}
			// reversed
			rev := make([]bip39.Mnemonic, len(parts))
			for i := range parts {
				rev[i] = parts[len(parts)-1-i]
			}
			rgot, err := Combine(rev)
			if err != nil {
				t.Fatalf("Combine(reversed): %v", err)
			}
			if rgot.String() != fwd.String() {
				t.Errorf("reversed result = %q, want %q", rgot.String(), fwd.String())
			}
			// rotated (shuffle a third order)
			rot := append([]bip39.Mnemonic{parts[1], parts[2], parts[0]}, parts[3:]...)
			ogot, err := Combine(rot)
			if err != nil {
				t.Fatalf("Combine(rotated): %v", err)
			}
			if ogot.String() != fwd.String() {
				t.Errorf("rotated result = %q, want %q", ogot.String(), fwd.String())
			}
		})
	}
}

func TestCombineNoCallerMutation(t *testing.T) {
	// Combine must copy parts[0]'s entropy, never XOR into the caller's slice.
	v := loadVectors(t)[0]
	parts := make([]bip39.Mnemonic, len(v.Parts))
	for i, p := range v.Parts {
		parts[i] = parseM(t, p)
	}
	before := string(parts[0].Entropy())
	if _, err := Combine(parts); err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if after := string(parts[0].Entropy()); after != before {
		t.Errorf("Combine mutated parts[0] entropy: before %x after %x", before, after)
	}
}

func TestCombineTooFewParts(t *testing.T) {
	v := loadVectors(t)[0]
	one := []bip39.Mnemonic{parseM(t, v.Parts[0])}
	_, err := Combine(one)
	if !errors.Is(err, errTooFewParts) {
		t.Errorf("Combine(1 part) err = %v, want errTooFewParts", err)
	}
	_, err = Combine(nil)
	if !errors.Is(err, errTooFewParts) {
		t.Errorf("Combine(nil) err = %v, want errTooFewParts", err)
	}
}

func TestCombineMismatchedLengths(t *testing.T) {
	// A 12-word part XOR a 24-word part → errMismatchedLengths.
	twelve := parseM(t, "romance wink lottery autumn shop bring dawn tongue range crater truth ability")
	twentyfour := parseM(t, "silent toe meat possible chair blossom wait occur this worth option bag nurse find fish scene bench asthma bike wage world quit primary indoor")
	_, err := Combine([]bip39.Mnemonic{twelve, twentyfour})
	if !errors.Is(err, errMismatchedLengths) {
		t.Errorf("Combine(12+24) err = %v, want errMismatchedLengths", err)
	}
}

func TestCombineBadLength(t *testing.T) {
	// A 15-word (20-byte) part is a valid BIP-39 mnemonic but not a
	// Coldcard-interop Seed XOR length: errBadLength (the load-bearing guard;
	// bip39.New would otherwise accept 20 bytes).
	fifteen := make(bip39.Mnemonic, 15)
	for i := range fifteen {
		fifteen[i] = bip39.Word(i)
	}
	fifteen = fifteen.FixChecksum()
	if !fifteen.Valid() {
		t.Fatal("constructed 15-word mnemonic is not valid")
	}
	if len(fifteen.Entropy()) != 20 {
		t.Fatalf("15-word entropy = %d bytes, want 20", len(fifteen.Entropy()))
	}
	_, err := Combine([]bip39.Mnemonic{fifteen, fifteen})
	if !errors.Is(err, errBadLength) {
		t.Errorf("Combine(15-word) err = %v, want errBadLength", err)
	}
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errTooFewParts, "need at least 2 parts"},
		{errBadLength, "unsupported length (use 12/18/24 words)"},
		{errMismatchedLengths, "all parts must be the same length"},
		{errors.New("boom"), "invalid"},
	}
	for _, c := range cases {
		if got := Describe(c.err); got != c.want {
			t.Errorf("Describe(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

// TestCombineScrubNoCorruption is the M3 regression guard. The per-part entropy
// copies wiped by the fix (e0 at seedxor.go:38, e at :44) are function-local and
// unobservable seam-free (spec R0 Q1/Minor-2), so this is NOT a buffer-zeroed
// assertion. It proves the additive in-loop `wipe(e)` does not corrupt the XOR:
// each part's entropy is wiped only AFTER it has been XORed into out, so the
// combined result must still match the vector. Also re-confirms the
// errMismatchedLengths path (where wipe(e) now runs alongside wipe(out)) still
// returns its sentinel.
func TestCombineScrubNoCorruption(t *testing.T) {
	// Success path: the result must be byte-identical to the vector even though
	// each per-part entropy copy is wiped immediately after use.
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			parts := make([]bip39.Mnemonic, len(v.Parts))
			for i, p := range v.Parts {
				parts[i] = parseM(t, p)
			}
			want := parseM(t, v.Result)
			got, err := Combine(parts)
			if err != nil {
				t.Fatalf("Combine: %v", err)
			}
			if string(got.Entropy()) != string(want.Entropy()) {
				t.Fatalf("Combine entropy = %x, want %x (wipe(e) corrupted the XOR?)",
					got.Entropy(), want.Entropy())
			}
		})
	}

	// Mismatched-lengths path still returns its sentinel (now wipes e too).
	twelve := parseM(t, "romance wink lottery autumn shop bring dawn tongue range crater truth ability")
	twentyfour := parseM(t, "silent toe meat possible chair blossom wait occur this worth option bag nurse find fish scene bench asthma bike wage world quit primary indoor")
	if _, err := Combine([]bip39.Mnemonic{twelve, twentyfour}); !errors.Is(err, errMismatchedLengths) {
		t.Fatalf("mismatched: err = %v, want errMismatchedLengths", err)
	}
}
