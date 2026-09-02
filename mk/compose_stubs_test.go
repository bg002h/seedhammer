package mk

import (
	"reflect"
	"testing"
)

// The journey's cosigner @0 at m/48'/0'/0'/2' (BIP-39 "abandon" mnemonic).
const composeTestXpub = "xpub6DkFAXWQ2dHxq2vatrt9qyA3bXYU4ToWQwCHbf5XB2mSTexcHZCeKS1VZYcPoBd5X8yVcbXFHJR9R8UCVpt82VX1VhR28mCyxUFL4r6KFrf"

func TestAppendStubsPreservesExistingAndAddsEachOnce(t *testing.T) {
	existing := [4]byte{0xde, 0xad, 0xbe, 0xef}
	tmpl := [4]byte{1, 2, 3, 4}
	pol := [4]byte{5, 6, 7, 8}
	card := Card{Network: "mainnet", Path: "m/48'/0'/0'/2'", Fingerprint: "73c5da0a", Stubs: [][4]byte{existing}, Xpub: composeTestXpub}
	got := AppendStubs(card, tmpl, pol, tmpl)
	if want := [][4]byte{existing, tmpl, pol}; !reflect.DeepEqual(got.Stubs, want) {
		t.Fatalf("stubs = %x, want %x", got.Stubs, want)
	}
	// A stub the card already carries is not repeated.
	again := AppendStubs(got, existing)
	if !reflect.DeepEqual(again.Stubs, got.Stubs) {
		t.Fatalf("re-appending an existing stub changed the list: %x", again.Stubs)
	}
	// The re-minted card round-trips through the wire with all three, in order.
	strs, err := Encode(got)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	back, err := Decode(strs)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(back.Stubs, got.Stubs) || back.Xpub != composeTestXpub || back.Fingerprint != "73c5da0a" {
		t.Fatalf("round trip: %+v", back)
	}
}

// AppendStubs must not alias its input's backing array (tests-lens C-2: a
// length/content check on the input cannot see aliasing, because append
// never changes the caller's slice header). So: give the input SPARE
// capacity, append through it afterwards, and the result must not change --
// which it would if AppendStubs had appended into the same array.
func TestAppendStubsDoesNotShareTheInputsBackingArray(t *testing.T) {
	existing := make([][4]byte, 1, 4)
	existing[0] = [4]byte{0xde, 0xad, 0xbe, 0xef}
	card := Card{Network: "mainnet", Path: "m/48'/0'/0'/2'", Fingerprint: "73c5da0a", Stubs: existing, Xpub: composeTestXpub}
	got := AppendStubs(card, [4]byte{1, 2, 3, 4})
	want := [][4]byte{existing[0], {1, 2, 3, 4}}
	if !reflect.DeepEqual(got.Stubs, want) {
		t.Fatalf("stubs = %x, want %x", got.Stubs, want)
	}
	// Write through the ORIGINAL's spare capacity.
	sentinel := [4]byte{0xff, 0xff, 0xff, 0xff}
	_ = append(card.Stubs, sentinel)
	if !reflect.DeepEqual(got.Stubs, want) {
		t.Fatalf("appending to the input changed the result to %x: AppendStubs aliased the input's array", got.Stubs)
	}
	if got.Stubs[1] == sentinel {
		t.Fatal("the result's second stub is the sentinel written through the input")
	}
}
