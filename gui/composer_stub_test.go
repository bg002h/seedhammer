package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

// composerTemplateChunks composes a two-path wsh template with no keys, the
// shape Part A's exit engraves.
func composerTemplateChunks(t *testing.T) []string {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
	}}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatalf("Chunks: %v", err)
	}
	return chunks
}

// TestComposerStubLinesTeachTheStubAndTheOrigins is §7c: the labels are
// LITERAL, the mk encode command is present, §8d is present, and every
// unseated slot names the origin a card must declare to seat there.
func TestComposerStubLinesTeachTheStubAndTheOrigins(t *testing.T) {
	chunks := composerTemplateChunks(t)
	lines, err := composerStubLines(chunks, nil, false)
	if err != nil {
		t.Fatalf("composerStubLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Template-ID:",
		"mk1 stub (template):",
		"mk encode --xpub",
		"--policy-id-stub",
		composerCopyOwnWallet(),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the stub screen does not say %q:\n%s", want, joined)
		}
	}
	// FOUR SLOTS, FOUR EXPECTED-ORIGIN LINES, each at a DISTINCT account: the
	// §4f invariant is what makes the template seatable at all.
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		line := k.OriginPath.String()
		if !strings.Contains(joined, line) {
			t.Errorf("no line names slot @%d's expected origin %s:\n%s", k.Index, line, joined)
		}
		if seen[line] {
			t.Errorf("two slots declare the same origin %s with no fingerprints, which "+
				"errSeatSlotContested makes unseatable (§4f's invariant)", line)
		}
		seen[line] = true
	}
	if len(keys) != 4 {
		t.Fatalf("the fixture has %d slots, want 4", len(keys))
	}
}

// TestComposerStubScreenSaysTheIdChangedAfterAnEdit is §8s's first body and
// §12 item 5's condition test for it.
func TestComposerStubScreenSaysTheIdChangedAfterAnEdit(t *testing.T) {
	chunks := composerTemplateChunks(t)
	fresh, err := composerStubLines(chunks, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(fresh, "\n"), composerCopyIdChanged()) {
		t.Error("a first showing carries the changed-id line, which would be false")
	}
	after, err := composerStubLines(chunks, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(after, "\n"), composerCopyIdChanged()) {
		t.Error("a re-show after an edit does not carry §8s's changed-id line")
	}
	assertModalBodyFits(t, "the §8s changed-id line", errorScreenBody, composerCopyIdChanged())
	assertModalBodyFits(t, "the §8d own-wallet line", errorScreenBody, composerCopyOwnWallet())
}

// TestComposerStubScreenIsPagedAtItsMeasuredCapacity is §12 item 5's rule for
// a variable-length screen: assert the PAGING, since a fits assertion cannot
// pin a body with no single source string.
func TestComposerStubScreenIsPagedAtItsMeasuredCapacity(t *testing.T) {
	// A 32-slot template: the grammar's maximum, and the case the screen
	// exists to survive.
	list := md.PathList{Wrapper: md.ComposeWsh}
	for i := 0; i < 4; i++ {
		list.Paths = append(list.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 8}})
	}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := composerStubLines(chunks, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	_, shown := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, 0, -1)
	if shown >= len(lines) {
		t.Fatalf("a 32-slot stub screen claims all %d lines fit one frame; it must page", len(lines))
	}
	t.Logf("stub screen: %d lines for 32 slots, %d per frame, %d pages",
		len(lines), shown, (len(lines)+shown-1)/shown)
	// And the LAST page is reachable: paging forward by the reported count
	// terminates rather than looping short of the end.
	start, pages := 0, 0
	for start < len(lines) && pages < 64 {
		_, n := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, start, -1)
		if n == 0 {
			t.Fatalf("paging stalled at line %d: composerPageLines drew nothing", start)
		}
		start += n
		pages++
	}
	if start < len(lines) {
		t.Errorf("paging reached line %d of %d before the page cap; the tail is unreachable",
			start, len(lines))
	}
	// It draws.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerStubFlow(ctx, &descriptorTheme, chunks, nil, false)
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the stub screen never drew")
		}
		assertFrameHasBody(t, ink(), "the composer stub-teaching screen")
		if !uiContains(content, "mk1 stub (template):") {
			t.Errorf("the first frame does not carry the stub label.\nFrame: %q", content)
		}
	})
}

// TestComposerTemplateEngraveScreenUsesTheStubLabel pins the §7c relabelling
// on the SHIPPED screen: its 4-byte value is a STUB, and calling it
// "Template-ID" beside a 16-byte id of the same name is how an operator comes
// to compare the wrong one against a coordinator.
func TestComposerTemplateEngraveScreenUsesTheStubLabel(t *testing.T) {
	lines := templateConsentLines(md.Template{N: 3, Renderable: true, Policy: md.PolicySortedMulti, K: 2},
		[4]byte{0xde, 0xad, 0xbe, 0xef}, 0, md.PolicyShape{})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "mk1 stub (template): deadbeef") {
		t.Errorf("the template-engrave screen does not label its 4-byte value as a stub:\n%s", joined)
	}
	if strings.Contains(joined, "Template-ID: deadbeef") {
		t.Errorf("the template-engrave screen still calls a 4-byte stub Template-ID, which "+
			"is the label md.WalletIdKind gives the 16-byte id:\n%s", joined)
	}
}
