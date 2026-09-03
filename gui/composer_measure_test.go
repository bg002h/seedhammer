package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// TestComposerMeasureSection13Numbers prints, in ONE block, every number
// SPEC §13 item 1 lists as unverified.
//
// IT ASSERTS ALMOST NOTHING, deliberately. Its job is to MEASURE, and a
// measurement test that also pins a threshold becomes a test nobody dares
// re-run when the font moves. The one assertion it does make is that each
// paged screen actually pages -- because a capacity equal to the whole body
// would mean the screen never pages and the number is meaningless.
//
// SPEC §13 item 1 QUOTES THIS TEST'S OUTPUT VERBATIM, so when a number here
// moves the spec is stale until it is re-pasted. W-3 moved one:
//
//	                    3cc71d9b      after the W-3 band fix
//	stub_screen         7 per frame   6 per frame  (6 pages -> 7)
//	pick_list           7             7
//	consent             7             7
//	descriptor_plate    596 chars     596 chars
//
// The stub screen lost a row because `Template-ID: <32 hex>` no longer fits one
// line inside the band left of the navigation column, so it WRAPS to two -- the
// honest cost of not drawing the last hex digit under the Back button. The spec
// sentence "all three paged screens hold 7 rows per frame" is false from this
// commit; the stub screen holds 6.
func TestComposerMeasureSection13Numbers(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)

	measure := func(name string, lines []string) {
		t.Helper()
		_, shown, _ := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, 0, -1)
		pages := 0
		if shown > 0 {
			pages = (len(lines) + shown - 1) / shown
		}
		t.Logf("SPEC13 %-14s lines=%3d per_frame=%2d pages=%d", name, len(lines), shown, pages)
		if shown >= len(lines) && len(lines) > 12 {
			t.Errorf("%s: %d lines all fit one frame, so the paging number is meaningless",
				name, len(lines))
		}
	}

	// (a) THE STUB SCREEN at the grammar's maximum: 32 slots.
	maxList := md.PathList{Wrapper: md.ComposeWsh}
	for i := 0; i < 4; i++ {
		maxList.Paths = append(maxList.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 8}})
	}
	c, err := md.Compose(maxList)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	stub, err := composerStubLines(chunks, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	measure("stub_screen", stub)

	// (b) THE PICK LIST at a payload the composer plausibly meets: 32 keys.
	rows := make([]string, 0, 34)
	for i := 0; i < 32; i++ {
		rows = append(rows, composerKeyLabel([4]byte{0x73, 0xc5, 0xda, byte(i)}, composerTestPath(i)))
	}
	rows = append(rows, "Type a seed", "Leave unseated")
	measure("pick_list", append([]string{composerCopySeatPrompt(2, 1, 2, 3), ""}, rows...))

	// (c) THE CONSENT at §7e's own worst case: eight paths plus four addresses.
	eight := md.PathList{Wrapper: md.ComposeWsh}
	for i := 0; i < 8; i++ {
		eight.Paths = append(eight.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 4}})
	}
	c2, err := md.Compose(eight)
	if err != nil {
		t.Fatal(err)
	}
	chunks2, err := c2.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	consent, err := composerConsentLinesFor(chunks2, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	measure("consent", consent)

	// (d) THE CONCRETE DESCRIPTOR PLATE CEILING, by the same search
	// qrCeilingBytes uses on the QR side (gui/transaction.go:1359-1391).
	n := composerDescriptorCeilingChars(p)
	t.Logf("SPEC13 %-14s ceiling_chars=%d  c10_688_fits=%v", "descriptor_plate", n,
		composerDescriptorPlateFits(p, strings.Repeat("a", 688)))
}
