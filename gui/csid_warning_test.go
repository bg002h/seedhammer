package gui

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	gscanner "go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── device-csid-warning: fixtures FROM the vendored mk corpus ───────────────
//
// SPEC design/SPEC_device_csid_warning.md, Acceptance: "fixtures come FROM
// the corpus, never hand-minted" for the mismatch/clean pair. This package
// does not embed mk/testdata/csid_ext_v0.1.json itself (only mk/ does, via
// go:embed), so these tests read the SAME vendored file directly, at test
// time, from its real path relative to gui/ -- binding the test to the
// corpus rather than to a transcribed copy of its strings (a near-miss
// transcription of a 21-row corpus is exactly the kind of drift that is
// invisible on read and wrong in a single character).

// csidFixtureRow is the subset of the corpus row schema these tests need.
type csidFixtureRow struct {
	Name                  string   `json:"name"`
	DeclaredCSID          string   `json:"declared_csid"`
	DerivedCSID           string   `json:"derived_csid"`
	ExpectMismatchWarning bool     `json:"expect_mismatch_warning"`
	Strings               []string `json:"strings"`
	WarningText           string   `json:"warning_text"`
}

type csidFixtureCorpus struct {
	Rows []csidFixtureRow `json:"rows"`
}

// csidVendoredCorpusPath is mk's vendored corpus, read directly rather than
// transcribed -- gofmt/go vet do not see through a relative path, so this is
// deliberately the SAME path every csid_warning_test.go helper uses.
const csidVendoredCorpusPath = "../mk/testdata/csid_ext_v0.1.json"

// loadCSIDFixture reads mk's vendored csid corpus and returns the named row.
func loadCSIDFixture(t *testing.T, name string) csidFixtureRow {
	t.Helper()
	b, err := os.ReadFile(csidVendoredCorpusPath)
	if err != nil {
		t.Fatalf("reading vendored csid corpus %s: %v", csidVendoredCorpusPath, err)
	}
	var corpus csidFixtureCorpus
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatalf("unmarshal csid corpus: %v", err)
	}
	for _, row := range corpus.Rows {
		if row.Name == name {
			if len(row.Strings) == 0 {
				t.Fatalf("corpus row %s carries no strings", name)
			}
			return row
		}
	}
	t.Fatalf("csid corpus row %q not found in %s", name, csidVendoredCorpusPath)
	return csidFixtureRow{}
}

// csidPinnedRow / csidCleanTwinRow are THE fixture pair this whole cycle is
// built on (R0 r1's "the fixture pair exists and is exactly right"):
// identical key content, one plate mis-stamped with a pinned id (12345), one
// clean (declares its own content-derived id, ef12f).
func csidPinnedRow(t *testing.T) csidFixtureRow    { return loadCSIDFixture(t, "SEED_pinned_12345_ef12f") }
func csidCleanTwinRow(t *testing.T) csidFixtureRow { return loadCSIDFixture(t, "SEED_plate_b_ef12f") }

// mustParseCSIDHex parses a corpus row's 5-hex-digit declared/derived id
// field into a uint32, failing the test (not silently defaulting) on a
// malformed corpus value.
func mustParseCSIDHex(t *testing.T, field, s string) uint32 {
	t.Helper()
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		t.Fatalf("corpus %s %q does not parse as hex: %v", field, s, err)
	}
	return uint32(v)
}

// TestCSIDFixturePairIsWhatTheSpecClaims is a self-check on the corpus read
// above: the pinned row's declared/derived disagree (12345/ef12f) and the
// clean twin's agree (ef12f/ef12f), both from mk.Decode + mk.DerivedChunkSetID
// run against the corpus's OWN strings -- not against a copied hex constant.
// If a future re-vendor changes this pair, this test is the first thing that
// fails, before any of the warning-firing tests below can be misread.
func TestCSIDFixturePairIsWhatTheSpecClaims(t *testing.T) {
	pinned := csidPinnedRow(t)
	if !pinned.ExpectMismatchWarning {
		t.Fatalf("%s: expect_mismatch_warning=false, want true", pinned.Name)
	}
	pinnedCard, err := mk.Decode(pinned.Strings)
	if err != nil {
		t.Fatalf("Decode(pinned.Strings): %v", err)
	}
	pinnedHdr, err := mk.ParseHeader(pinned.Strings[0])
	if err != nil {
		t.Fatalf("ParseHeader(pinned.Strings[0]): %v", err)
	}
	pinnedDerived, err := mk.DerivedChunkSetID(pinnedCard)
	if err != nil {
		t.Fatalf("DerivedChunkSetID(pinnedCard): %v", err)
	}
	wantDeclared := mustParseCSIDHex(t, "declared_csid", pinned.DeclaredCSID)
	wantDerived := mustParseCSIDHex(t, "derived_csid", pinned.DerivedCSID)
	if pinnedHdr.ChunkSetID != wantDeclared {
		t.Fatalf("pinned wire declared csid = %05x, corpus says %05x", pinnedHdr.ChunkSetID, wantDeclared)
	}
	if pinnedDerived != wantDerived {
		t.Fatalf("pinned DerivedChunkSetID = %05x, corpus says %05x", pinnedDerived, wantDerived)
	}
	if pinnedHdr.ChunkSetID == pinnedDerived {
		t.Fatalf("pinned row's declared == derived (%05x) -- not a mismatch fixture", pinnedDerived)
	}
	// M1 (whole-diff review): every OTHER assertion in this cycle goes through
	// uiContains, which lower-cases and space-strips both sides -- so an
	// uppercased or differently-spaced warning would still pass every rendered
	// test. This is the one STRICT, case- and whitespace-sensitive check:
	// csidMismatchWarningText's actual return value, byte for byte, against
	// the corpus's own warning_text.
	if got := csidMismatchWarningText(wantDeclared, wantDerived); got != pinned.WarningText {
		t.Errorf("csidMismatchWarningText(%05x, %05x) = %q, want corpus warning_text %q (byte-exact, not uiContains-normalized)",
			wantDeclared, wantDerived, got, pinned.WarningText)
	}

	clean := csidCleanTwinRow(t)
	if clean.ExpectMismatchWarning {
		t.Fatalf("%s: expect_mismatch_warning=true, want false", clean.Name)
	}
	cleanCard, err := mk.Decode(clean.Strings)
	if err != nil {
		t.Fatalf("Decode(clean.Strings): %v", err)
	}
	cleanHdr, err := mk.ParseHeader(clean.Strings[0])
	if err != nil {
		t.Fatalf("ParseHeader(clean.Strings[0]): %v", err)
	}
	cleanDerived, err := mk.DerivedChunkSetID(cleanCard)
	if err != nil {
		t.Fatalf("DerivedChunkSetID(cleanCard): %v", err)
	}
	if cleanHdr.ChunkSetID != cleanDerived {
		t.Fatalf("clean twin declared %05x != derived %05x -- not a clean fixture",
			cleanHdr.ChunkSetID, cleanDerived)
	}

	// The fixture pair: SAME key content, only the stamped id differs.
	if pinnedCard.Xpub != cleanCard.Xpub || pinnedCard.Path != cleanCard.Path {
		t.Fatalf("pinned/clean rows do not share the same key content: %+v vs %+v", pinnedCard, cleanCard)
	}
}

// gatherRow offers every string of a corpus row to a fresh mk1Gatherer, in
// order, and fails the test if any offer is not gatherAdded (i.e. the fixture
// itself is malformed) or if the gatherer is not complete afterward.
func gatherRow(t *testing.T, row csidFixtureRow) *mk1Gatherer {
	t.Helper()
	g := &mk1Gatherer{}
	for i, s := range row.Strings {
		if st := g.offer(s); st != gatherAdded {
			t.Fatalf("%s: offer(strings[%d]) = %v, want gatherAdded", row.Name, i, st)
		}
	}
	if !g.complete() {
		t.Fatalf("%s: gatherer not complete after offering every string", row.Name)
	}
	return g
}

// ─── Contract 2: the inspect flow (decodeGathered) ────────────────────────────

// TestDecodeGatheredWarnsOnCSIDMismatch drives decodeGathered (gui/mk1_inspect.go)
// with the pinned corpus row: a non-blocking notice fires BEFORE the card is
// returned, its body is the host warning text byte-exact (read from the
// corpus at test time, not transcribed), and BACK dismisses it and PROCEEDS
// -- decodeGathered still returns the decoded card, ok=true.
func TestDecodeGatheredWarnsOnCSIDMismatch(t *testing.T) {
	row := csidPinnedRow(t)
	g := gatherRow(t, row)

	// sh2DisplaySize, matching R0's own raster measurement: the host warning
	// text is 258 normalized chars with 302 chars of headroom on THIS panel
	// size (r1/r2). The package's smaller default test display would wrap the
	// body across more lines than the real panel and misreport a truncation
	// that is a test-fixture artifact, not a device defect.
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	var card mk.Card
	var ok bool
	frame, quit := runUI(ctx, func() { card, ok = decodeGathered(ctx, &descriptorTheme, g) })
	defer quit()

	content, fok := frame()
	if !fok {
		t.Fatalf("no notice frame drawn for a csid-mismatched key set; decodeGathered must warn before returning")
	}
	if !uiContains(content, row.WarningText) {
		t.Errorf("notice body is not the corpus warning_text verbatim; got %q, want it to contain %q",
			content, row.WarningText)
	}
	// BACK dismisses -- and proceeding continues to the card (R1: every modal
	// answers BACK; proceeding continues).
	click(&ctx.Router, Button1)
	if _, fok2 := frame(); fok2 {
		t.Fatal("decodeGathered drew a second frame after BACK dismissed the notice")
	}
	if !ok {
		t.Fatal("decodeGathered returned ok=false after a dismissed non-blocking notice; the mismatch must not refuse (R1)")
	}
	if card.Path == "" || card.Xpub == "" {
		t.Fatalf("decodeGathered did not return the decoded card after the notice; got %+v", card)
	}
}

// TestDecodeGatheredSilentOnCleanTwin: the clean twin (same key content,
// declared csid == derived) draws no notice at all -- decodeGathered returns
// immediately with the decoded card.
func TestDecodeGatheredSilentOnCleanTwin(t *testing.T) {
	row := csidCleanTwinRow(t)
	g := gatherRow(t, row)

	ctx := NewContext(newPlatform())
	var card mk.Card
	var ok bool
	frame, quit := runUI(ctx, func() { card, ok = decodeGathered(ctx, &descriptorTheme, g) })
	defer quit()

	if content, fok := frame(); fok {
		t.Fatalf("decodeGathered drew a frame for a clean (non-mismatched) key set; want silent return; got %q", content)
	}
	if !ok {
		t.Fatal("decodeGathered returned ok=false for a valid clean key set")
	}
	if card.Path == "" || card.Xpub == "" {
		t.Fatalf("decodeGathered did not return the decoded card; got %+v", card)
	}
}

// TestDecodeGatheredWarnsUsingChunkedFieldNotProxies is Contract 2's I3 guard
// (R0 r1): the comparison gates on the EXPLICIT g.chunked field set at prime
// time, never on the setID==0 or total==1 proxies. Directly asserts the field
// is set from the header's own Chunked bit.
func TestMK1GathererChunkedFieldSetAtPrimeTime(t *testing.T) {
	row := csidPinnedRow(t)
	g := &mk1Gatherer{}
	if g.chunked {
		t.Fatal("chunked is true before any offer -- zero value must be false")
	}
	if st := g.offer(row.Strings[0]); st != gatherAdded {
		t.Fatalf("offer(strings[0]) = %v, want gatherAdded", st)
	}
	if !g.chunked {
		t.Fatal("chunked was not set true on priming from a genuinely Chunked header")
	}
}

// ─── decodeGathered's decode-failure path stays untouched ─────────────────────

// TestDecodeGatheredStillRefusesOnDecodeFailure: the csid comparison must not
// run (and must not panic) ahead of a failed mk.Decode -- the pre-existing
// "Can't decode this key set." refusal is unchanged.
func TestDecodeGatheredStillRefusesOnDecodeFailure(t *testing.T) {
	g := &mk1Gatherer{set: map[int]string{0: "not an mk1 string"}, total: 1, primed: true, chunked: false}
	ctx := NewContext(newPlatform())
	var card mk.Card
	var ok bool
	frame, quit := runUI(ctx, func() { card, ok = decodeGathered(ctx, &descriptorTheme, g) })
	defer quit()
	content, fok := frame()
	if !fok {
		t.Fatal("decodeGathered drew no frame for an undecodable set; want the existing refusal screen")
	}
	if !uiContains(content, "Can't decode this key set") {
		t.Errorf("refusal wording changed or missing; got %q", content)
	}
	click(&ctx.Router, Button1)
	frame()
	if ok || card.Xpub != "" {
		t.Fatalf("decode failure must still yield (zero, false); got ok=%v card=%+v", ok, card)
	}
}

// ─── Contract 3: the bundle-gatherer flow's comparison point ─────────────────

// gatherRowIntoBundle offers every string of a corpus row into a fresh
// bundleGatherer (through mdmkText, the same admission path a scanned card
// takes) and returns the ONE completed card. Fails the test if the row does
// not yield exactly one completed card (the fixture would then not be
// exercising offerChunkedMK1's completion path this test targets).
func gatherRowIntoBundle(t *testing.T, row csidFixtureRow) bundleCard {
	t.Helper()
	g := &bundleGatherer{}
	var lastStatus bundleOfferStatus
	for i, s := range row.Strings {
		lastStatus = g.offer(mdmkText(s))
		if i < len(row.Strings)-1 && lastStatus != bundleChunkProgress {
			t.Fatalf("%s: offer(strings[%d]) = %v, want bundleChunkProgress", row.Name, i, lastStatus)
		}
	}
	if lastStatus != bundleCardComplete {
		t.Fatalf("%s: final offer status = %v, want bundleCardComplete", row.Name, lastStatus)
	}
	if len(g.cards) != 1 {
		t.Fatalf("%s: bundleGatherer holds %d cards, want 1", row.Name, len(g.cards))
	}
	return g.cards[0]
}

// TestOfferChunkedMK1ComputesCSIDMismatch: offerChunkedMK1 (gui/bundle.go)
// computes the Contract-3 comparison ONCE at set completion and stores it on
// the resulting bundleCard, for the pinned corpus row.
func TestOfferChunkedMK1ComputesCSIDMismatch(t *testing.T) {
	row := csidPinnedRow(t)
	c := gatherRowIntoBundle(t, row)
	if c.kind != cardMK1 {
		t.Fatalf("kind = %v, want cardMK1", c.kind)
	}
	if !c.csidMismatch {
		t.Fatal("csidMismatch = false for the pinned mismatch row")
	}
	wantDeclared := mustParseCSIDHex(t, "declared_csid", row.DeclaredCSID)
	wantDerived := mustParseCSIDHex(t, "derived_csid", row.DerivedCSID)
	if c.declaredCSID != wantDeclared {
		t.Errorf("declaredCSID = %05x, want %05x", c.declaredCSID, wantDeclared)
	}
	if c.derivedCSID != wantDerived {
		t.Errorf("derivedCSID = %05x, want %05x", c.derivedCSID, wantDerived)
	}
}

// TestOfferChunkedMK1SilentOnCleanTwin: the clean twin completes with
// csidMismatch left false.
func TestOfferChunkedMK1SilentOnCleanTwin(t *testing.T) {
	row := csidCleanTwinRow(t)
	c := gatherRowIntoBundle(t, row)
	if c.csidMismatch {
		t.Fatalf("csidMismatch = true for the clean twin (declared %05x derived %05x)",
			c.declaredCSID, c.derivedCSID)
	}
}

// TestCSIDMarkerForm pins the marker's rendered form for both rows, so a
// future edit to csidMarker is visible as a diff here rather than only in the
// screenshot gate.
func TestCSIDMarkerForm(t *testing.T) {
	mismatched := gatherRowIntoBundle(t, csidPinnedRow(t))
	if got, want := csidMarker(mismatched), " [csid 12345!ef12f]"; got != want {
		t.Errorf("csidMarker(mismatched) = %q, want %q", got, want)
	}
	clean := gatherRowIntoBundle(t, csidCleanTwinRow(t))
	if got := csidMarker(clean); got != "" {
		t.Errorf("csidMarker(clean) = %q, want \"\"", got)
	}
}

// ─── The review-list marker (Engrave Bundle / Wallet Policy) ─────────────────

// TestBundleReviewFlowMarksCSIDMismatch: bundleReviewFlow's per-card line
// carries the marker for the pinned row and none for the clean twin.
func TestBundleReviewFlowMarksCSIDMismatch(t *testing.T) {
	cards := []bundleCard{
		gatherRowIntoBundle(t, csidPinnedRow(t)),
		gatherRowIntoBundle(t, csidCleanTwinRow(t)),
	}
	ctx := NewContext(newPlatform())
	var ok bool
	frame, quit := runUI(ctx, func() { ok = bundleReviewFlow(ctx, &descriptorTheme, cards) })
	defer quit()
	content, found := pumpUntil(frame, "Bundle", 8)
	if !found {
		t.Fatalf("review screen not shown; got %q", content)
	}
	if !uiContains(content, "12345!ef12f") {
		t.Errorf("review list carries no marker for the mismatched card; got %q", content)
	}
	click(&ctx.Router, Button3)
	frame()
	if !ok {
		t.Fatal("Confirm did not advance the review flow")
	}
}

// ─── The Build Policy census / restore-doc / payload-cards markers ───────────

// TestBuildPlateCensusLinesMarksCSIDMismatch is a HELPER-LEVEL PIN on
// buildPlateCensusLines AND buildPlateInventoryLines (multisig_build_census.go),
// corrected 2026-09-01 per the whole-diff review's C1: it constructs the
// mismatched bundleCard directly and calls both functions, which is the ONLY
// way either currently sees one. NO PRODUCTION FLOW FEEDS THESE TWO FUNCTIONS
// A GATHERED CARD — Build Policy's cosigner cards are device-minted
// bundleCard literals from buildEngraveTail/multisigEngraveCardsMulti, never
// routed through the bundle gatherer that computes csidMismatch, so
// csidMarker(c) at multisig_build_census.go:53,89 returns "" on every
// production path today. This test proves the two helpers render the marker
// correctly WHEN GIVEN a mismatched card; it does NOT prove any on-device
// path reaches that state, and does not stand in for the reachable
// "restore-doc consumer" acceptance row (see design/journeys/csid-tags/
// README.md and design/SPEC_device_csid_warning.md Contract 3, both amended
// to match).
func TestBuildPlateCensusLinesMarksCSIDMismatch(t *testing.T) {
	mismatched := gatherRowIntoBundle(t, csidPinnedRow(t))
	clean := gatherRowIntoBundle(t, csidCleanTwinRow(t))
	cards := []bundleCard{mismatched, clean}

	census := buildPlateCensusLines(engraverParams, cards)
	censusText := strings.Join(census, "\n")
	if !strings.Contains(censusText, "12345!ef12f") {
		t.Errorf("buildPlateCensusLines carries no marker for the mismatched card; got %q", census)
	}
	if strings.Count(censusText, "12345!ef12f") != 1 {
		t.Errorf("buildPlateCensusLines marker count = %d, want exactly 1 (only the mismatched card)",
			strings.Count(censusText, "12345!ef12f"))
	}

	inv := buildPlateInventoryLines(engraverParams, cards, oneSeedPassphraseFact(false), seedCapacityMany, false)
	invText := strings.Join(inv, "\n")
	if !strings.Contains(invText, "12345!ef12f") {
		t.Errorf("buildPlateInventoryLines carries no marker for the mismatched card; got %q", inv)
	}
}

// TestBuildPayloadCardsLinesMarksCSIDMismatch covers buildPayloadCardsLines
// (multisig_build_payload.go), the payload-review screen (SPEC P0 item 6)
// shown before the operator picks/confirms cosigner cards.
func TestBuildPayloadCardsLinesMarksCSIDMismatch(t *testing.T) {
	mismatched := gatherRowIntoBundle(t, csidPinnedRow(t))
	clean := gatherRowIntoBundle(t, csidCleanTwinRow(t))
	lines := buildPayloadCardsLines([]bundleCard{mismatched, clean}, 2, false)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "12345!ef12f") {
		t.Errorf("buildPayloadCardsLines carries no marker for the mismatched card; got %q", lines)
	}
}

// ─── The set-completion notice modal (Engrave Bundle / Wallet Policy / Build) ─

// TestShowBundleCSIDMismatchNoticesFiresOnMismatch: showBundleCSIDMismatchNotices
// is the shared call the three interactive-gather callers make right after
// their gather returns (Contract 3). One notice, host-warning-text-verbatim,
// per mismatched mk1 card; silent for the clean twin.
func TestShowBundleCSIDMismatchNoticesFiresOnMismatch(t *testing.T) {
	row := csidPinnedRow(t)
	c := gatherRowIntoBundle(t, row)

	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() {
		showBundleCSIDMismatchNotices(ctx, &descriptorTheme, "Engrave Bundle", []bundleCard{c})
	})
	defer quit()
	content, fok := frame()
	if !fok {
		t.Fatal("no notice frame drawn for a csid-mismatched completed card")
	}
	if !uiContains(content, row.WarningText) {
		t.Errorf("notice body is not the corpus warning_text verbatim; got %q", content)
	}
	click(&ctx.Router, Button1) // BACK dismisses
	if _, fok2 := frame(); fok2 {
		t.Fatal("a second frame was drawn after the sole mismatch was dismissed")
	}
}

func TestShowBundleCSIDMismatchNoticesSilentOnCleanTwin(t *testing.T) {
	c := gatherRowIntoBundle(t, csidCleanTwinRow(t))
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		showBundleCSIDMismatchNotices(ctx, &descriptorTheme, "Engrave Bundle", []bundleCard{c})
	})
	defer quit()
	if content, fok := frame(); fok {
		t.Fatalf("a notice was drawn for a clean (non-mismatched) card; got %q", content)
	}
}

// ─── Contract 3's Engrave Multisig disposition: NO marker, NO modal ──────────

// TestEngraveMultisigRefusesAnyMK1BeforeCSIDCouldMatter re-confirms (R0 r2/r3)
// that extractSuppliedMd1 refuses unconditionally on ANY mk1 presence, so the
// csid comparison is unreachable there regardless of match/mismatch — this is
// the reason Contract 3 names for that surface's silence, and it is real,
// failable behavior, not merely cited. TestExtractSuppliedMd1's own
// "any mk1 present -> refuse" subtest (gui/multisig_supply_test.go) pins the
// same fact; this restates it against a REAL mismatched card from the corpus,
// not a synthetic bundleCard{kind: cardMK1}, to close the last inch between
// "a stray mk1" and "a csid-mismatched mk1 specifically".
func TestEngraveMultisigRefusesAnyMK1BeforeCSIDCouldMatter(t *testing.T) {
	mismatched := gatherRowIntoBundle(t, csidPinnedRow(t))
	md1card := bundleCard{kind: cardMD1, label: "md1 descriptor", strings: []string{"md1yqpqqxqq8xtwhw4xwn4qh"}}
	if _, ok := extractSuppliedMd1([]bundleCard{md1card, mismatched}); ok {
		t.Fatal("extractSuppliedMd1 accepted a set carrying a csid-mismatched mk1 -- Contract 3's Engrave Multisig disposition (unconditional refusal) does not hold")
	}
}

// ─── The Build Policy program, end to end, off a real payload ────────────────

// TestBuildPolicyGatherShowsCSIDMismatchNoticeLive drives buildMultisigPolicyFlow
// (gui/multisig_build.go) — not just the shared helper — with the pinned
// corpus row supplied as the systemwide payload's sole cosigner card, through
// the SAME default param-picker path TestBuildGatherIsNotTitledEngraveBundle
// uses to reach the cosigner gather. This is the "funds-most path" SPEC
// Contract 3 names explicitly, so it gets a live drive in addition to the
// shared-helper test above.
func TestBuildPolicyGatherShowsCSIDMismatchNoticeLive(t *testing.T) {
	row := csidPinnedRow(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding(row.Strings...)
	frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
	defer quit()
	buildWalkParamPickers(t, ctx, frame)
	// The card completes off the pre-loaded payload before any interactive
	// scan, but bundleGatherFlow(Resume) still WAITS at its own tally screen
	// until "Done adding cards" (Button3) -- the notice fires once the whole
	// gather SET completes (Contract 3: "at set completion"), not mid-loop.
	if c, ok := pumpUntil(frame, "mk1 keys: 1", 64); !ok {
		t.Fatalf("Build Policy's cosigner gather tally was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	content, ok := pumpUntil(frame, row.WarningText, 64)
	if !ok {
		t.Fatalf("Build Policy's cosigner gather did not show the csid-mismatch notice; last frame %q", content)
	}
	click(&ctx.Router, Button1) // BACK dismisses the notice, non-blocking
	// The flow must continue past the dismissed notice into Build's own
	// payload-cards review (SPEC P0 item 6) rather than dead-ending -- and
	// that screen carries the SAME csid marker (buildPayloadCardsLines),
	// confirming Contract 3's two surfaces agree on one card.
	c, ok := pumpUntil(frame, "Payload cards", 32)
	if !ok {
		t.Fatalf("Build Policy did not continue past the dismissed notice to the payload-cards review; got %q", c)
	}
	if !uiContains(c, "12345!ef12f") {
		t.Errorf("the payload-cards review does not carry the csid marker; got %q", c)
	}
}

// TestBuildPolicyGatherSilentOnCleanTwinLive is the clean-twin control for the
// same live path.
func TestBuildPolicyGatherSilentOnCleanTwinLive(t *testing.T) {
	row := csidCleanTwinRow(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding(row.Strings...)
	frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
	defer quit()
	buildWalkParamPickers(t, ctx, frame)
	content, ok := pumpUntil(frame, "mk1 keys: 1", 64)
	if !ok {
		t.Fatalf("Build Policy's cosigner gather was not reached; got %q", content)
	}
	if uiContains(content, "warning:") {
		t.Errorf("a csid-mismatch notice fired for the clean twin; got %q", content)
	}
	click(&ctx.Router, Button3) // Done adding cards
	// After Done, silence must hold too -- not just before the notice would
	// have had a chance to fire. M4 (whole-diff review): this budget must
	// match the mismatch twin's pumpUntil(..., 64) above -- an 8-frame budget
	// let a notice firing on frame 9 pass unnoticed, weaker than what the
	// positive live test actually proves reachable.
	for i := 0; i < 64; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		if uiContains(c, "warning:") {
			t.Fatalf("a csid-mismatch notice fired for the clean twin after Done; got %q", c)
		}
	}
}

// ─── Contract 3: the two verify-readback line-markers ────────────────────────

// TestBundleCSIDNoteFiresOnMismatch: bundleCSIDNote (gui/bundle.go), the pure
// helper both verify flows append to their terminal verdict text, carries the
// marker for the pinned row and is "" for the clean twin.
func TestBundleCSIDNoteFiresOnMismatch(t *testing.T) {
	mismatched := gatherRowIntoBundle(t, csidPinnedRow(t))
	note := bundleCSIDNote([]bundleCard{mismatched})
	if !strings.Contains(note, "12345!ef12f") {
		t.Errorf("bundleCSIDNote carries no marker for the mismatched card; got %q", note)
	}
}

func TestBundleCSIDNoteSilentOnCleanTwin(t *testing.T) {
	clean := gatherRowIntoBundle(t, csidCleanTwinRow(t))
	if note := bundleCSIDNote([]bundleCard{clean}); note != "" {
		t.Errorf("bundleCSIDNote(clean) = %q, want \"\"", note)
	}
}

// TestSingleSigVerifyCSIDNoteOnFailureLive drives the REAL singleSigVerifyFlow
// (gui/singlesig_verify.go) with the pinned corpus row supplied as the mk1
// readback (paired with an unrelated, syntactically valid md1 so the readback
// accounting succeeds) and a seed that cannot possibly match that card's
// content -- guaranteeing a comparator FAIL, which is where Contract 3 wires
// bundleCSIDNote in. Confirms: (a) the note fires on the FAIL screen, and (b)
// no separate modal is shown for the mismatch itself -- exactly one "Verify
// Failed" screen appears, carrying both the seed-mismatch text AND the marker.
func TestSingleSigVerifyCSIDNoteOnFailureLive(t *testing.T) {
	row := csidPinnedRow(t)
	// An unrelated but real, decodable md1 -- content is irrelevant here since
	// the mk1 leg alone guarantees disagreement; it only needs to satisfy
	// singleSigReadbackCards' "exactly one md1" accounting.
	unrelated, _, _, _, err := deriveSingleSigBundle(abandonAboutMnemonic(), "",
		&chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriving the unrelated md1 fixture: %v", err)
	}

	var rec verifyRecord
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append(append([]string(nil), row.Strings...), unrelated.MD1...)
	frame, quit := runUI(ctx, func() {
		singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only: no ms1 step needed */, false, false, &rec)
	})
	defer quit()

	if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
		t.Fatalf("the verify did not ask for the seed; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, abandonAboutPhrase())
	if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
		t.Fatalf("the verify did not reach the wallet-type picker; got %q", c)
	}
	click(&ctx.Router, Button3) // BIP-84 default
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
		t.Fatalf("the verify did not reach the passphrase prompt; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	body, ok := pumpUntil(frame, "Verify Failed", 96)
	if !ok {
		t.Fatalf("a foreign mk1 readback did not FAIL the verify; got %q", body)
	}
	if !uiContains(body, "12345!ef12f") {
		t.Errorf("the Verify Failed screen carries no csid-mismatch marker; got %q", body)
	}
	if !uiContains(body, "does NOT match the seed") {
		t.Errorf("the Verify Failed screen lost its own verdict text; got %q", body)
	}
	if !rec.adverse {
		t.Error("the comparator ran and disagreed but the record says nothing adverse")
	}
	// NO SEPARATE MODAL: dismissing this ONE screen must exit the flow, not
	// reveal a second csid-specific popup underneath it (Contract 3: "line-
	// marker only, NO modal").
	click(&ctx.Router, Button3)
	if _, fok := frame(); fok {
		t.Fatal("a second modal was drawn after the Verify Failed screen was dismissed -- Contract 3 forbids a separate csid modal here")
	}
}

// TestSingleSigVerifyCSIDNoteSilentOnCleanTwinLive is the clean-twin control
// for the same live path -- silence extends to the note too, not merely to
// the absence of a separate modal.
func TestSingleSigVerifyCSIDNoteSilentOnCleanTwinLive(t *testing.T) {
	row := csidCleanTwinRow(t)
	unrelated, _, _, _, err := deriveSingleSigBundle(abandonAboutMnemonic(), "",
		&chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriving the unrelated md1 fixture: %v", err)
	}

	var rec verifyRecord
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append(append([]string(nil), row.Strings...), unrelated.MD1...)
	frame, quit := runUI(ctx, func() {
		singleSigVerifyFlow(ctx, &descriptorTheme, false, false, false, &rec)
	})
	defer quit()

	pumpUntil(frame, "Choose number of words", 96)
	click(&ctx.Router, Button3)
	frame()
	driveWords(&ctx.Router, abandonAboutPhrase())
	pumpUntil(frame, "Wallet Type", 240)
	click(&ctx.Router, Button3)
	pumpUntil(frame, "Add a BIP-39 passphrase?", 96)
	click(&ctx.Router, Button3)
	frame()
	pumpUntil(frame, "mk1 keys:", 96)
	click(&ctx.Router, Button3)
	frame()

	body, ok := pumpUntil(frame, "Verify Failed", 96)
	if !ok {
		t.Fatalf("a foreign mk1 readback did not FAIL the verify; got %q", body)
	}
	if uiContains(body, "csid") {
		t.Errorf("a csid note appeared for the clean twin; got %q", body)
	}
}

// TestMultisigVerifyFlowWiresCSIDNoteIntoVerdicts is a mechanical wiring guard
// (mirroring TestGatherTitleReachesTheRefusalsToo's funcBody idiom,
// gui/multisig_build_title_test.go), covering the ONE surface this cycle does
// not also drive live end to end: multisigVerifyFlow's per-seed/per-slot loop
// makes a full live drive (typed seed x N legs, passphrase, ms1 per leg)
// disproportionate to what a wiring check needs to prove. The pure
// bundleCSIDNote (tested directly above) and the singlesig sibling's live
// drive (TestSingleSigVerifyCSIDNoteOnFailureLive) already prove the NOTE
// TEXT and the NO-SEPARATE-MODAL property; this proves multisigVerifyFlow's
// SOURCE actually appends it at exactly the three verdict sites Contract 3
// names (the PASS and both comparator-FAIL exits) -- so a future edit that
// drops the append is a source-level regression this test catches even
// though nothing here renders a frame.
func TestMultisigVerifyFlowWiresCSIDNoteIntoVerdicts(t *testing.T) {
	// M2 (whole-diff review): funcBody returns RAW source, so a commented-out
	// call would satisfy a bare strings.Contains below. stripGoComments
	// removes every Go comment via go/scanner first.
	body := stripGoComments(t, funcBody(t, "multisig_verify.go", "func multisigVerifyFlow("))
	if !strings.Contains(body, "bundleCSIDNote(cards)") {
		t.Fatal("multisigVerifyFlow no longer computes bundleCSIDNote(cards) -- Contract 3's verify-readback line-marker is unwired")
	}
	if !strings.Contains(body, "multisigVerifyOKMessage(len(legs), full)+csidNote") {
		t.Error("the PASS verdict (showNotice) does not append csidNote")
	}
	// M2: >= 2, not == 2 -- the exact-equality form fails on a LEGITIMATE third
	// comparator-FAIL site (e.g. a future verdict split), which is the wrong
	// direction for a wiring guard to be brittle in. The two sites this cycle
	// wired (the partial and the full sweep) must still both be present.
	if got := strings.Count(body, "multisigVerifyFailureText(err)+csidNote"); got < 2 {
		t.Errorf("expected at least 2 comparator-FAIL verdicts appending csidNote (the partial and the full sweep), found %d", got)
	}
}

// stripGoComments removes every Go comment from src using the standard
// tokenizer (go/scanner), so a commented-out call cannot satisfy a wiring
// guard's strings.Contains the way funcBody's raw source slice can (M2,
// whole-diff review). src need not be a syntactically complete declaration --
// funcBody deliberately returns a fragment (signature to the next top-level
// `func`) -- the scanner only needs valid token boundaries, not a parseable
// tree, so it tokenizes a fragment exactly as it would the whole file. Output
// is NOT valid Go source (tokens are rejoined with single spaces, discarding
// original formatting); callers only run strings.Contains/strings.Count
// against it.
func stripGoComments(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s gscanner.Scanner
	s.Init(file, []byte(src), nil, gscanner.ScanComments)
	// Deletes comment BYTE RANGES from the original src and keeps every other
	// byte exactly as written, including its original spacing. Reconstructing
	// from token literals instead (joining with a single space) would insert
	// spaces around punctuation and break substring checks like
	// strings.Contains(body, "bundleCSIDNote(cards)") -- caught live: an
	// earlier version of this helper failed that exact check.
	keep := make([]bool, len(src))
	for i := range keep {
		keep[i] = true
	}
	base := file.Base()
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		start := int(pos) - base
		end := start + len(lit)
		if start < 0 || end > len(src) {
			continue
		}
		for i := start; i < end; i++ {
			keep[i] = false
		}
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		if keep[i] {
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

// ─── M2: nothing else guards the CONSUMER SET; a seventh caller must trip ────

// bundleGatherConsumer records what a caller of bundleGatherFlow(Resume) is
// expected to also call, per Contract 3 (SPEC design/SPEC_device_csid_warning.md
// "ALL SIX consumers, enumerated").
type bundleGatherConsumer struct {
	modal bool // must call showBundleCSIDMismatchNotices (the three interactive-gather surfaces)
	note  bool // must call bundleCSIDNote (the two verify-readback line-markers)
	// exempt names an identifier the function must still call, standing in
	// for the reason neither of the above applies (Engrave Multisig: refused
	// before a card could render).
	exempt string
}

// expectedBundleGatherConsumers is Contract 3's six named consumers, keyed by
// the enclosing top-level function's name. Anything that calls
// bundleGatherFlow/bundleGatherFlowResume and is NOT one of these is a
// SEVENTH consumer this cycle never audited.
var expectedBundleGatherConsumers = map[string]bundleGatherConsumer{
	"bundleFlow":               {modal: true},
	"walletPolicyFlow":         {modal: true},
	"buildMultisigPolicyFlow":  {modal: true},
	"supplyMultisigPolicyFlow": {exempt: "extractSuppliedMd1"},
	"multisigVerifyFlow":       {note: true},
	"singleSigVerifyFlow":      {note: true},
}

// TestBundleGatherConsumersAreAccountedFor is Contract 3's "ALL SIX
// consumers, enumerated" turned into a regression guard (whole-diff review
// M2). Unlike funcBody's raw strings.Index (comment-blind -- see
// stripGoComments above), this walks the real AST (go/parser) of every
// non-test gui/*.go source file: a commented-out call cannot satisfy an AST
// CallExpr, and a genuinely new caller of bundleGatherFlow/
// bundleGatherFlowResume anywhere outside the six functions below fails this
// test rather than shipping unaudited.
func TestBundleGatherConsumersAreAccountedFor(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing gui/*.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("filepath.Glob(\"*.go\") found nothing -- this guard is not running from gui/ (cwd wrong?)")
	}

	fset := token.NewFileSet()
	// found maps a top-level function's name to the set of plain-identifier
	// calls (CallExpr with an *ast.Ident callee) anywhere in its body,
	// including nested closures -- for every function that calls
	// bundleGatherFlow or bundleGatherFlowResume at least once.
	found := map[string]map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// bundleGatherFlow's OWN body delegates to bundleGatherFlowResume
			// (the thin wrapper, gui/bundle_flow.go) -- that is the
			// definition, not a consumer, and must not count as one.
			if fn.Name.Name == "bundleGatherFlow" || fn.Name.Name == "bundleGatherFlowResume" {
				continue
			}
			calls := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok {
					calls[id.Name] = true
				}
				return true
			})
			if calls["bundleGatherFlow"] || calls["bundleGatherFlowResume"] {
				found[fn.Name.Name] = calls
			}
		}
	}

	for name, calls := range found {
		want, ok := expectedBundleGatherConsumers[name]
		if !ok {
			t.Errorf("%s calls bundleGatherFlow(Resume) but is not on this guard's named "+
				"list of six audited consumers -- a SEVENTH caller has appeared and Contract "+
				"3's csid comparison has not been reviewed for it", name)
			continue
		}
		switch {
		case want.modal:
			if !calls["showBundleCSIDMismatchNotices"] {
				t.Errorf("%s gathers mk1 cards but no longer calls showBundleCSIDMismatchNotices "+
					"-- a csid mismatch would now be silent here", name)
			}
		case want.note:
			if !calls["bundleCSIDNote"] {
				t.Errorf("%s gathers mk1 cards but no longer calls bundleCSIDNote -- the "+
					"verify-readback line-marker is unwired", name)
			}
		case want.exempt != "":
			if !calls[want.exempt] {
				t.Errorf("%s no longer calls %s -- its exemption (refuses before a card "+
					"could render) no longer holds, and Contract 3's csid comparison may now "+
					"be reachable here unmarked", name, want.exempt)
			}
		}
	}
	// Every expected consumer must actually have been found -- a renamed or
	// deleted function would otherwise pass this test vacuously.
	for name := range expectedBundleGatherConsumers {
		if _, ok := found[name]; !ok {
			t.Errorf("expected consumer %s no longer calls bundleGatherFlow(Resume) anywhere "+
				"in gui/*.go -- this guard's site list is stale", name)
		}
	}
}

// ─── M3: the notice re-fires on every re-entry into a gather ────────────────

// TestBundleFlowNoticeRefiresOnReviewBackReentry pins the ACCEPTED M3 shape
// (whole-diff review): all three modal callers loop back into the SAME
// gather on a review Back (bundleFlow: `gathered = cards; continue`), so a
// still-mismatched card's set-completion notice fires again on re-entry
// rather than latching "already shown this session" -- see
// showBundleCSIDMismatchNotices's doc comment (gui/bundle.go). Warning-only
// and non-blocking, so accepted rather than debounced (not a correctness
// defect, per the review); this test exists so the behaviour is pinned
// rather than an unreviewed accident -- a future change that adds a latch,
// or that stops re-firing for an unrelated reason, shows up here rather than
// only at the flash gate.
func TestBundleFlowNoticeRefiresOnReviewBackReentry(t *testing.T) {
	row := csidPinnedRow(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), row.Strings...)
	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	// The preloaded seeds complete the card immediately, but the gather
	// screen still waits for Done -- it does not auto-proceed.
	if _, ok := pumpUntil(frame, "mk1 keys: 1", 32); !ok {
		t.Fatal("the initial gather never reached the completed-card tally")
	}
	click(&ctx.Router, Button3) // Done adding cards

	// First fire, at initial set completion.
	if _, ok := pumpUntil(frame, row.WarningText, 32); !ok {
		t.Fatal("the first notice never fired for the mismatched card")
	}
	click(&ctx.Router, Button1) // BACK dismisses the notice
	if _, ok := pumpUntil(frame, "Bundle", 32); !ok {
		t.Fatal("the review screen was not reached after the first notice was dismissed")
	}
	click(&ctx.Router, Button1) // Back at review -- resumes the gather WITH the card still on it

	// Re-entry: bundleGatherFlowResume re-offers the same card from `prev`, so
	// it is already complete -- Done proceeds immediately, without a rescan.
	if _, ok := pumpUntil(frame, "mk1 keys: 1", 32); !ok {
		t.Fatal("the gather was not resumed with the card still on the pile")
	}
	click(&ctx.Router, Button3) // Done adding cards, again

	// SECOND fire: the same card, the same mismatch, re-computed and re-shown.
	if _, ok := pumpUntil(frame, row.WarningText, 32); !ok {
		t.Fatal("the notice did not re-fire on review-Back re-entry -- if this is now " +
			"debounced, update this test to assert the new (intentional) behaviour " +
			"rather than deleting the coverage")
	}
}
