package gui

import (
	"testing"

	"seedhammer.com/bip39"
)

// canonicalBip85Master is the standard BIP-85 spec test-vector master seed.
func canonicalBip85Master(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic("install scatter logic circle pencil average fall shoe quantum disease suspect usage")
	if err != nil {
		t.Fatalf("ParseMnemonic(canonical master): %v", err)
	}
	return m
}

// TestDeriveBip85Child_AbandonGoldens pins the BIP-85 BIP-39 children of the
// canonical abandon-about master at index 0 for each word count. A trailing-bytes
// truncation bug, a wrong path element, or an unhardened element all yield a
// different child and fail here.
func TestDeriveBip85Child_AbandonGoldens(t *testing.T) {
	tests := []struct {
		words int
		want  string
	}{
		{12, "prosper short ramp prepare exchange stove life snack client enough purpose fold"},
		{18, "winter brother stamp provide uniform useful doctor prevent venue upper peasant auto view club next clerk tone fox"},
		{24, "stick exact spice sock filter ginger museum horse kit multiply manual wear grief demand derive alert quiz fault december lava picture immune decade jaguar"},
	}
	for _, tc := range tests {
		child, err := deriveBip85Child(abandonAboutMnemonic(), "", tc.words, 0)
		if err != nil {
			t.Fatalf("words=%d: %v", tc.words, err)
		}
		if got := child.String(); got != tc.want {
			t.Fatalf("words=%d child mismatch:\n got %q\nwant %q", tc.words, got, tc.want)
		}
		if len(child) != tc.words {
			t.Fatalf("words=%d: child has %d words", tc.words, len(child))
		}
		if !child.Valid() {
			t.Fatalf("words=%d: child fails BIP-39 checksum", tc.words)
		}
	}
}

// TestDeriveBip85Child_CanonicalVector cross-checks the helper against the
// canonical BIP-85 spec vector (master -> m/83696968'/39'/0'/12'/0').
func TestDeriveBip85Child_CanonicalVector(t *testing.T) {
	child, err := deriveBip85Child(canonicalBip85Master(t), "", 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	const want = "girl mad pet galaxy egg matter matrix prison refuse sense ordinary nose"
	if got := child.String(); got != want {
		t.Fatalf("canonical vector mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestDeriveBip85Child_IndexVaries confirms distinct indices yield distinct
// children (the index participates in the hardened path).
func TestDeriveBip85Child_IndexVaries(t *testing.T) {
	c0, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c0.String() == c1.String() {
		t.Fatal("index 0 and index 1 produced the same child")
	}
	const wantIdx1 = "sing slogan bar group gauge sphere rescue fossil loyal vital model desert"
	if got := c1.String(); got != wantIdx1 {
		t.Fatalf("idx1 child mismatch:\n got %q\nwant %q", got, wantIdx1)
	}
}

// TestDeriveBip85Child_RejectsBadWords: out-of-spec word counts error (never panic).
func TestDeriveBip85Child_RejectsBadWords(t *testing.T) {
	for _, w := range []int{0, 11, 13, 15, 21, 25, 27, -3} {
		if _, err := deriveBip85Child(abandonAboutMnemonic(), "", w, 0); err == nil {
			t.Fatalf("words=%d: expected an error, got nil", w)
		}
	}
}

// TestDeriveBip85Child_RejectsNegativeIndex: a negative index errors.
func TestDeriveBip85Child_RejectsNegativeIndex(t *testing.T) {
	if _, err := deriveBip85Child(abandonAboutMnemonic(), "", 12, -1); err == nil {
		t.Fatal("index=-1: expected an error, got nil")
	}
}
