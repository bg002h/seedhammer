package sysw

import (
	"reflect"
	"testing"
)

// `[mdmk-decode]` -- SPEC_systemwide_payloads §12.6, ruled in as §13 D6.
//
// REAL CARDS, not hand-built fixtures, and every chunk header below was MEASURED
// in the Rust primary rather than assumed. They are the same strings the primary
// uses, so a disagreement between the two implementations shows up here as well
// as in the shared vector:
//
//	md1fv9wjpq...  chunk_set_id 398802, chunks 0,1,2 of 3
//	md1fe4dazs...  chunk_set_id 841149, chunk  0    of 6
//	md1yqpqq...    NOT chunked, decodes on its own
//	mk1qpzg69p...  a complete 2-chunk mk1 card (74565)
//	mk1qpykrep...  chunk 0 of a 2-chunk mk1 card (153721)
const (
	tMD1A      = "md1fv9wjpqpqpm6jzzqqvqpdqnf4ztqq4gy99tzyzyzdv7xh9vpdwu3t7dhhesk2tl3"
	tMD1B      = "md1fv9wjpqg0yq82l0czvx85ae43vtfd26hsmngjecmqy44k2pgttqh74qwxlawq374"
	tMD1C      = "md1fv9wjpqsp2026hh65xpvugtfhd9792zxgunymm0a82pdju6442q0jskj9gzfaqmz"
	tMD1Other  = "md1fe4dazspq3m67zzqqvzrs3pstucnf4ztqz4pk6ujgjycfn6zhs79nmzdp9frd6dzth6asfu2za4mwgfkg6"
	tMD1Single = "md1yqpqqxqq8xtwhw4xwn4qh"
	tMK1A      = "mk1qpzg69pqqsq3zg3ngj4thnxaq5zg3vs7zqsrqqdt4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4vp3kx98j76m4mjlwphf"
	tMK1B      = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	tMK1Lone   = "mk1qpykrepqqspjtpuhfqjc096gykrewjy6dgjcqpcy3zepaggqseet8ky6z2jxm56yh04m5mqslrmueekdmecm0js2h978k03jfvkwz2rxj8r8"
	tSeed      = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// 32 bytes of 0xAB -- seed entropy -- wrapped into a BCH-valid md1 by the
	// Rust primary's md_codec::codex32::wrap_payload(&[0xAB; 32], 256). This is
	// the smuggling case §5.3.2 names: ValidMD is a pure BCH verifier, so it
	// says yes, and only a real decode says no. Pasted rather than generated
	// because the fork has no md1 payload writer and Rust is primary anyway.
	tSmuggled = "md14w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w4sltupm5etwtesk"

	// The same thing with 0x01 instead of 0xAB, which leaves the chunked-flag
	// bit CLEAR -- so both implementations take the NON-CHUNKED path and this
	// record actually reaches the grouping.
	//
	// It exists because the first version of this file used tSmuggled for the
	// two-cards test below and the collapse-the-uniq mutant SURVIVED: tSmuggled
	// sets the chunked flag with a wrong wire version, so md.ParseChunkHeader
	// errors and the record is reported from the fail-closed arm without ever
	// being grouped. The test read as coverage of a line it never executed.
	// The primary's mirror of this test was corrected the same way.
	tSmuggledNonChunked = "md1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs27czt2jar0dnj"
)

func eq(t *testing.T, got, want []int, ctx string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s\n got %v\nwant %v", ctx, got, want)
	}
}

func TestALoneChunkOfADeclaredSetIsUnconfirmed(t *testing.T) {
	// The set declares 3 chunks and one is present, so nothing reassembles and
	// §12.6 leaves it unconfirmed. Nothing is REFUSED -- that is D6.
	eq(t, MDMKUnconfirmed([]string{tMD1A}), []int{0}, "one chunk of three")
}

func TestTheCompleteSetIsConfirmed(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tMD1A, tMD1B, tMD1C}), nil, "the whole card")
}

// Group by (hrp, chunk_set_id), NEVER by HRP alone. Grouping by HRP puts four
// md1 records in one bucket against a declared total of 3, and reports a
// COMPLETE card as unconfirmed -- the R1-I2 shape.
func TestGroupingIsByChunkSetIDNotByHRPAlone(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tMD1A, tMD1B, tMD1C, tMD1Other}), []int{3},
		"the complete 398802 set must confirm beside a stray chunk of 841149")
}

// R1-I2 the other way round: filter the ITERATION, never the indices. A walk
// that collected the ClassMDMK records first and reported positions in THAT list
// would name record 0 here, which is the seed.
func TestTheIndicesAreIntoTheCallersList(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tSeed, "text:6869", tMD1A}), []int{2},
		"indices belong to the caller's slice")
}

func TestABCHValidMD1CarryingEntropyIsUnconfirmed(t *testing.T) {
	if c := Classify(tSmuggled); c != ClassMDMK {
		t.Fatalf("INCONCLUSIVE: the premise is that BCH validity alone makes this a "+
			"non-secret class, but Classify says %v", c)
	}
	if ClassMDMK.IsSecret() {
		t.Fatal("INCONCLUSIVE: the class would already be secret, so §12.6 would be moot")
	}
	eq(t, MDMKUnconfirmed([]string{tSmuggled}), []int{0}, "smuggled entropy")
}

// The non-chunked arm, and the mutant it kills: a walk that gave every
// non-chunked record the same group key would decode only the first of them and
// call the rest confirmed. Here the first decodes and the second does not.
func TestTwoNonChunkedMD1RecordsAreTwoCards(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tMD1Single, tSmuggledNonChunked}), []int{1},
		"one card decoding must not vouch for another")
	eq(t, MDMKUnconfirmed([]string{tSmuggledNonChunked, tMD1Single}), []int{0},
		"and order must not decide it either")
	eq(t, MDMKUnconfirmed([]string{tMD1Single}), nil, "the one that decodes is confirmed alone")
}

func TestMK1SetsAreWalkedTheSameWay(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tMK1A, tMK1B}), nil, "a complete mk1 card")
	eq(t, MDMKUnconfirmed([]string{tMK1A}), []int{0}, "half of one")
	eq(t, MDMKUnconfirmed([]string{tMK1Lone}), []int{0}, "half of another")
	eq(t, MDMKUnconfirmed([]string{tMK1A, tMK1B, tMK1Lone}), []int{2},
		"two mk1 cards, grouped apart by chunk_set_id")
}

// §12.6 is about ClassMDMK records and nothing else. A mnemonic is already
// secret by class and a text: record is already not; reporting either would
// double-count.
func TestRecordsOfOtherClassesAreNeverReported(t *testing.T) {
	eq(t, MDMKUnconfirmed([]string{tSeed, "text:6869", "ms1notacard"}), nil, "other classes")
	eq(t, MDMKUnconfirmed(nil), nil, "no records")
}

// The fail-closed arm, and the one the Rust primary's author first wrote off as
// unreachable. Classification TRIMS and the decoders do not, so a record with a
// leading space classifies ClassMDMK and then has no readable card identity at
// all. Unconfirmed is the only honest answer, and both implementations give it
// -- by different routes, which is what "bind semantics, not code" means: Rust
// reads the chunk header and gets a non-chunked answer it then fails to decode,
// while md.ParseChunkHeader here refuses the string outright.
func TestARecordWhoseCardIdentityCannotBeReadIsUnconfirmed(t *testing.T) {
	padded := " " + tMD1A
	if c := Classify(padded); c != ClassMDMK {
		t.Fatalf("INCONCLUSIVE: the premise is that the classifier trims; got %v", c)
	}
	eq(t, MDMKUnconfirmed([]string{padded}), []int{0}, "an unreadable card identity")
}
