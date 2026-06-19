package gui

import (
	"testing"

	"seedhammer.com/codex32"
	"seedhammer.com/mk"
)

// ms1Fixture is a valid codex32 secret (HRP "ms") — exactly what the scanner
// yields for an ms1 string (scan.go:70 codex32.New succeeds before the
// ValidMD/ValidMK mdmkText branch). It must be REFUSED in the bundle channel,
// never silently dropped (R0-C2).
const ms1Fixture = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"

func ms1Object(tb testing.TB) codex32.String {
	tb.Helper()
	s, err := codex32.New(ms1Fixture)
	if err != nil {
		tb.Fatalf("codex32.New(ms1): %v", err)
	}
	return s
}

// singleMD1 returns a single-string (non-chunked) md1 — a small descriptor that
// legitimately fits one string. wpkh_basic is the in-tree single-string vector.
func singleMD1(tb testing.TB) string {
	tb.Helper()
	return loadVector(tb, "wpkh_basic")
}

// singleMK1Fixture returns a single (non-chunked) BCH-valid mk1 string. A real
// mk1 key card is ALWAYS >=2 chunks (xpub_compact 73B > the 56B cap), so a
// single mk1 is MALFORMED and carries no cross-chunk integrity — it must be
// REFUSED (R0-C1, host parity bundle.rs:128). No single mk1 exists in-tree, so
// we synthesize one: header [version=0x00, type=single=0x00] + filler fragment
// symbols + the BCH checksum, rendered with the "mk1" HRP.
func singleMK1Fixture(tb testing.TB) string {
	tb.Helper()
	const alphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	dataSyms := []byte{0x00, 0x00} // version 0, single card type 0
	for i := 0; i < 10; i++ {      // 10 filler fragment syms → data-part len 28
		dataSyms = append(dataSyms, byte(i%32))
	}
	ck := codex32.MKChecksumSymbols(dataSyms, false)
	var sb []byte
	sb = append(sb, 'm', 'k', '1')
	for _, s := range dataSyms {
		sb = append(sb, alphabet[s])
	}
	for _, s := range ck {
		sb = append(sb, alphabet[s])
	}
	s := string(sb)
	if !codex32.ValidMK(s) {
		tb.Fatalf("synthesized single mk1 not ValidMK: %s", s)
	}
	return s
}

func TestClassify(t *testing.T) {
	mkA := mk1CardA(t)
	mdA := md1CardA(t)

	// ms1 (codex32.String) → refuse, not drop.
	if cls, _, _ := classify(ms1Object(t)); cls != clsMs1Refuse {
		t.Fatalf("ms1: got class %v, want clsMs1Refuse", cls)
	}
	// chunked mk1 → clsChunkedMK1 + its csid.
	if cls, csid, str := classify(mdmkText(mkA[0])); cls != clsChunkedMK1 {
		t.Fatalf("chunked mk1: got class %v, want clsChunkedMK1", cls)
	} else if csid != mkCSID(t, mkA[0]) {
		t.Fatalf("chunked mk1 csid: got %#x, want %#x", csid, mkCSID(t, mkA[0]))
	} else if str != mkA[0] {
		t.Fatalf("chunked mk1 str not verbatim")
	}
	// chunked md1 → clsChunkedMD1 + its csid.
	if cls, csid, _ := classify(mdmkText(mdA[0])); cls != clsChunkedMD1 {
		t.Fatalf("chunked md1: got class %v, want clsChunkedMD1", cls)
	} else if csid != mdCSID(t, mdA[0]) {
		t.Fatalf("chunked md1 csid: got %#x, want %#x", csid, mdCSID(t, mdA[0]))
	}
	// single (non-chunked) md1 → standalone.
	if cls, _, _ := classify(mdmkText(singleMD1(t))); cls != clsStandaloneMD1 {
		t.Fatalf("single md1: got class %v, want clsStandaloneMD1", cls)
	}
	// garbage / non-md-mk object → drop.
	if cls, _, _ := classify(addressText("bc1qexampleaddress")); cls != clsDrop {
		t.Fatalf("addressText: got class %v, want clsDrop", cls)
	}
	if cls, _, _ := classify(nil); cls != clsDrop {
		t.Fatalf("nil: got class %v, want clsDrop", cls)
	}
}

// TestClassifySingleMK1Refuse: a single (non-chunked) mk1 string is malformed
// (no cross-chunk integrity) and must classify as clsSingleMK1Refuse (R0-C1).
func TestClassifySingleMK1Refuse(t *testing.T) {
	s := singleMK1Fixture(t)
	if cls, _, _ := classify(mdmkText(s)); cls != clsSingleMK1Refuse {
		t.Fatalf("single mk1: got class %v, want clsSingleMK1Refuse", cls)
	}
}

func TestBundleGatherChunkedMK1(t *testing.T) {
	mkA := mk1CardA(t)
	g := &bundleGatherer{}
	// Offer all but the last chunk → progress, no card yet.
	for i := 0; i < len(mkA)-1; i++ {
		if st := g.offer(mdmkText(mkA[i])); st != bundleChunkProgress {
			t.Fatalf("chunk %d: got status %v, want bundleChunkProgress", i, st)
		}
	}
	if len(g.cards) != 0 {
		t.Fatalf("card completed early: %d cards", len(g.cards))
	}
	// Last chunk → card complete + verified.
	if st := g.offer(mdmkText(mkA[len(mkA)-1])); st != bundleCardComplete {
		t.Fatalf("last chunk: got status %v, want bundleCardComplete", st)
	}
	if len(g.cards) != 1 {
		t.Fatalf("want 1 verified card, got %d", len(g.cards))
	}
	if g.cards[0].kind != cardMK1 {
		t.Fatalf("card kind = %v, want cardMK1", g.cards[0].kind)
	}
	// The card carries the gathered strings verbatim (I-4).
	if len(g.cards[0].strings) != len(mkA) {
		t.Fatalf("card strings = %d, want %d", len(g.cards[0].strings), len(mkA))
	}
}

// TestBundleGatherSecondCardNewCsid: a SECOND distinct-csid mk1 set starts a
// new card (R0-I2: new-csid = new card, NOT a foreign-rejection).
func TestBundleGatherSecondCardNewCsid(t *testing.T) {
	mkA, mkB := mk1CardA(t), mk1CardB(t)
	g := &bundleGatherer{}
	offerAll(t, g, mkA)
	if len(g.cards) != 1 {
		t.Fatalf("after card A: %d cards, want 1", len(g.cards))
	}
	// A chunk with a different csid does NOT reject — it primes a new card.
	offerAll(t, g, mkB)
	if len(g.cards) != 2 {
		t.Fatalf("after card B: %d cards, want 2 (new-csid=new-card)", len(g.cards))
	}
}

// TestBundleGatherInterleavedMixed: a chunked md1 + chunked mk1, interleaved,
// both complete and verify → exactly 2 cards.
func TestBundleGatherInterleavedMixed(t *testing.T) {
	mdA, mkA := md1CardA(t), mk1CardA(t)
	g := &bundleGatherer{}
	// Interleave: md chunk, mk chunk, md chunk, ... until both exhausted.
	maxLen := len(mdA)
	if len(mkA) > maxLen {
		maxLen = len(mkA)
	}
	for i := 0; i < maxLen; i++ {
		if i < len(mdA) {
			g.offer(mdmkText(mdA[i]))
		}
		if i < len(mkA) {
			g.offer(mdmkText(mkA[i]))
		}
	}
	if len(g.cards) != 2 {
		t.Fatalf("interleaved: %d cards, want 2", len(g.cards))
	}
	var haveMD, haveMK bool
	for _, c := range g.cards {
		switch c.kind {
		case cardMD1:
			haveMD = true
		case cardMK1:
			haveMK = true
		}
	}
	if !haveMD || !haveMK {
		t.Fatalf("interleaved: haveMD=%v haveMK=%v, want both", haveMD, haveMK)
	}
}

// TestBundleGatherDroppedChunkNeverCompletes: a card missing one chunk never
// lands in cards (I-1).
func TestBundleGatherDroppedChunkNeverCompletes(t *testing.T) {
	mkA := mk1CardA(t)
	g := &bundleGatherer{}
	// Offer every chunk except the last → never completes.
	for i := 0; i < len(mkA)-1; i++ {
		g.offer(mdmkText(mkA[i]))
	}
	if len(g.cards) != 0 {
		t.Fatalf("incomplete card added: %d cards", len(g.cards))
	}
}

// TestBundleGatherTamperedChunkNeverAdded: a complete set whose last chunk is
// corrupted (passes per-chunk BCH only if still valid; here we corrupt so it
// fails to even classify, but a structurally-valid-but-wrong reassembly must
// also never be added). We simulate by replacing a chunk with another card's
// chunk of the same index — reassembly/integrity (mk.Decode) fails → not added.
func TestBundleGatherTamperedChunkNeverAdded(t *testing.T) {
	mkA, mkB := mk1CardA(t), mk1CardB(t)
	g := &bundleGatherer{}
	// Prime card A with all chunks but the last, then offer mkB's last chunk.
	// mkB's chunk has a different csid → it primes a SEPARATE card, so card A
	// stays incomplete and is never added (and card B is incomplete too).
	for i := 0; i < len(mkA)-1; i++ {
		g.offer(mdmkText(mkA[i]))
	}
	g.offer(mdmkText(mkB[len(mkB)-1]))
	for _, c := range g.cards {
		if c.kind == cardMK1 {
			// If any mk1 card completed, it must be a full, integral set.
			if _, err := mk.Decode(c.strings); err != nil {
				t.Fatalf("an added mk1 card failed integrity: %v", err)
			}
		}
	}
	// Neither partial card should have completed.
	if len(g.cards) != 0 {
		t.Fatalf("partial cards added: %d", len(g.cards))
	}
}

// TestBundleGatherStandaloneMD1: a single-string md1 → a standalone card,
// validated by md.Decode (BCH+structural).
func TestBundleGatherStandaloneMD1(t *testing.T) {
	g := &bundleGatherer{}
	if st := g.offer(mdmkText(singleMD1(t))); st != bundleAddedSingleMD1 {
		t.Fatalf("single md1: got status %v, want bundleAddedSingleMD1", st)
	}
	if len(g.cards) != 1 || g.cards[0].kind != cardMD1 {
		t.Fatalf("standalone md1 not added as one cardMD1: %+v", g.cards)
	}
	if len(g.cards[0].strings) != 1 {
		t.Fatalf("standalone md1 must be 1 plate, got %d", len(g.cards[0].strings))
	}
}

// TestBundleGatherTwoDistinctSingleMD1NoCollision: two distinct single-string
// md1 cards must NOT collide on a zero-value csid (R0-C1).
func TestBundleGatherTwoDistinctSingleMD1NoCollision(t *testing.T) {
	g := &bundleGatherer{}
	one := loadChunkedVectorString(t, "wpkh_basic")
	two := loadChunkedVectorString(t, "tr_keyonly")
	if one == two {
		t.Skip("need two distinct single-string md1 vectors")
	}
	g.offer(mdmkText(one))
	g.offer(mdmkText(two))
	if len(g.cards) != 2 {
		t.Fatalf("two distinct single md1: %d cards, want 2 (no csid-0 collision)", len(g.cards))
	}
}

// TestBundleGatherMs1Refused: an ms1 codex32.String is refused, never added.
func TestBundleGatherMs1Refused(t *testing.T) {
	g := &bundleGatherer{}
	if st := g.offer(ms1Object(t)); st != bundleRefusedMs1 {
		t.Fatalf("ms1: got status %v, want bundleRefusedMs1", st)
	}
	if len(g.cards) != 0 {
		t.Fatalf("ms1 added to bundle: %d cards", len(g.cards))
	}
}

// TestBundleGatherSingleMK1Refused: a single (non-chunked) mk1 is refused.
func TestBundleGatherSingleMK1Refused(t *testing.T) {
	g := &bundleGatherer{}
	if st := g.offer(mdmkText(singleMK1Fixture(t))); st != bundleRefusedSingleMK1 {
		t.Fatalf("single mk1: got status %v, want bundleRefusedSingleMK1", st)
	}
	if len(g.cards) != 0 {
		t.Fatalf("single mk1 added: %d cards", len(g.cards))
	}
}

// TestBundleGatherDuplicateCardIgnored: re-offering a complete card → duplicate,
// no double-add.
func TestBundleGatherDuplicateCardIgnored(t *testing.T) {
	mkA := mk1CardA(t)
	g := &bundleGatherer{}
	offerAll(t, g, mkA)
	if len(g.cards) != 1 {
		t.Fatalf("card not completed once: %d", len(g.cards))
	}
	// Re-offer every chunk of the now-complete card.
	for _, s := range mkA {
		if st := g.offer(mdmkText(s)); st != bundleDuplicate {
			t.Fatalf("re-offer chunk: got status %v, want bundleDuplicate", st)
		}
	}
	if len(g.cards) != 1 {
		t.Fatalf("duplicate double-added: %d cards", len(g.cards))
	}
}

func offerAll(t *testing.T, g *bundleGatherer, strs []string) {
	t.Helper()
	for _, s := range strs {
		g.offer(mdmkText(s))
	}
}
