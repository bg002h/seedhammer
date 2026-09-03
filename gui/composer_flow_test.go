package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/md"
)

// TestComposerConsentLinesDescribeEveryPathFromTheDecodedMd1 is §7e.
//
// The input is CHUNKS, not a PathList: the consent must be derivable from
// what the device is about to engrave, or §8q's self-check has nothing to
// compare against.
func TestComposerConsentLinesDescribeEveryPathFromTheDecodedMd1(t *testing.T) {
	digest := [32]byte{0xab, 0xcd}
	digest[31] = 0xef
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}, Hash: &digest},
	}}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := composerConsentLinesFor(chunks, nil, 0)
	if err != nil {
		t.Fatalf("composerConsentLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Path 1: 2-of-3",
		"Path 2:",
		"1000 blocks",  // §6b echo form, in operator units
		"abcd",         // the digest's first bytes
		"Template-ID:", // the id, NAMED by kind (§7c)
		"mk1 stub (template):",
		"Keyless template - no addresses.", // D4
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the consent surface does not say %q:\n%s", want, joined)
		}
	}
	// NOT the shipped Wallet Policy consent's words: md1Summary prints
	// "Complex policy - cannot display safely." for every shape this composer
	// exists to author (md/md_test.go:337,416), which is exactly why §7e gives
	// the composer its own surface.
	if strings.Contains(joined, "cannot display safely") {
		t.Errorf("the composer's consent fell back to md1Summary's complex-policy line, "+
			"which describes nothing:\n%s", joined)
	}
}

// TestComposerConsentMarksTheExperimentalForms is §7e's "EXPERIMENTAL marks",
// derived from the decoded shape rather than from the operator's answers.
func TestComposerConsentMarksTheExperimentalForms(t *testing.T) {
	digest := [32]byte{0x01}
	keyless := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Hash: &digest},
	}}
	c, err := md.Compose(keyless)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, _ := c.Chunks()
	lines, err := composerConsentLinesFor(chunks, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "KEY-LESS") {
		t.Errorf("a key-less path is not marked on the consent surface:\n%s", strings.Join(lines, "\n"))
	}

	unsorted := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: false}},
	}}
	c2, err := md.Compose(unsorted)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks2, _ := c2.Chunks()
	lines2, err := composerConsentLinesFor(chunks2, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines2, "\n"), "UNSORTED") {
		t.Errorf("a sole unsorted key set is not marked EXPERIMENTAL:\n%s", strings.Join(lines2, "\n"))
	}
	// And a LOWERING-FORCED multi carries no such mark: the operator declined
	// nothing, and a mark there would teach them to discount the real one.
	forced := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}},
	}}
	c3, err := md.Compose(forced)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks3, _ := c3.Chunks()
	lines3, err := composerConsentLinesFor(chunks3, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(lines3, "\n"), "UNSORTED") {
		t.Errorf("a lowering-forced multi is marked UNSORTED (EXPERIMENTAL); §5a says the "+
			"mark belongs only where sorted was legal and declined:\n%s", strings.Join(lines3, "\n"))
	}
}

// TestComposerNUMSNoteFiresOnlyForATaprootFallback is §8f's condition test.
func TestComposerNUMSNoteFiresOnlyForATaprootFallback(t *testing.T) {
	digest := [32]byte{0x02}
	// tr with no unlocked single-key path: §5 extracts no internal key, so
	// the policy falls back to NUMS.
	nums := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}, Hash: &digest},
	}}
	c, err := md.Compose(nums)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, _ := c.Chunks()
	lines, err := composerConsentLinesFor(chunks, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "KEY PATH: NONE (NUMS)") {
		t.Errorf("a NUMS taproot policy does not carry §8f:\n%s", strings.Join(lines, "\n"))
	}
	// An EXTRACTED internal key says the opposite thing, and it is the line
	// that must never be missing: a spendable key path moves funds without
	// satisfying any leaf.
	extracted := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	c2, err := md.Compose(extracted)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks2, _ := c2.Chunks()
	lines2, err := composerConsentLinesFor(chunks2, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines2, "\n")
	if !strings.Contains(joined, "A KEY CAN SPEND ALONE") {
		t.Errorf("an extracted internal key is not stated:\n%s", joined)
	}
	if strings.Contains(joined, "NUMS") {
		t.Errorf("§8f fired on a policy with a real key path:\n%s", joined)
	}
	assertModalBodyFits(t, "the §8f NUMS note", errorScreenBody, composerCopyNUMS())
}

// ═══ PART A's EXIT: SPEC §12 item 3, the C26 no-payload walk ════════════════
//
// A device with NO payload reaches Wallet Policy, chooses Build, composes a
// shape, reads the stub screen with its per-slot expected origins, consents,
// and engraves a keyless template whose md1 DECODES on the device.
//
// THIS WALK LIVES IN PART A because §12 item 3 is PART A's declared exit, and
// a claim that "Part A ships alone" is only measured if the test that
// discharges it runs with Part B absent. It is written to pass in BOTH states:
// once Part B's seating step lands between the stub screen and consent, the
// walk takes that step's key-less choice and the rest of the journey is
// unchanged. A walk that had to be rewritten when the next task landed would
// stop being the same claim.
//
// It stops at the census: no hardware, no plate.

// TestComposerNoPayloadWalkEngravesAKeylessTemplate is Part A's declared exit,
// §12 item 3, WALKED.
//
// The test that carried this name before asserted only that the door drew. §12
// item 3 asks for "door line present, shape, stub screen with per-slot
// expected origins, consent stating no addresses, form choice collapsed,
// keyless-template engrave whose md1 decodes" -- six clauses, of which one was
// covered, by a separate artifact-level test.
func TestComposerNoPayloadWalkEngravesAKeylessTemplate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		// NO payload at all: ctx.sysw stays nil, which is C26's whole case.

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		got, ok := pumpUntil(frame, "Build a new policy", 24)
		if !ok {
			t.Fatalf("the door never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, composerCopyNoKeys()) {
			t.Errorf("the door does not say the build will be key-less.\nFrame: %q", got)
		}
		if uiContains(got, "From payload") {
			t.Errorf("From payload was offered with no payload loaded.\nFrame: %q", got)
		}
		click(&ctx.Router, Down) // Scan cards -> Build a new policy
		click(&ctx.Router, Button3)

		if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // -> Segwit (wsh)
		click(&ctx.Router, Button3)

		// The preset picker (§4d, task A10) sits between the wrapper and the
		// path list. Back is §7b's BLANK route, which is the one C26 walks:
		// §12 item 3 composes a shape by hand on a machine with no payload.
		if got, ok = pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("the preset picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1)

		// §7b's live line carries NO key count with no payload loaded.
		if got, ok = pumpUntil(frame, "Add a spend path", 24); !ok {
			t.Fatalf("the path list never drew.\nLast frame: %q", got)
		}
		if uiContains(got, "keys available") {
			t.Errorf("the path list counts keys on a machine with no payload; §7b scopes "+
				"that line to \"whenever a payload is loaded\".\nFrame: %q", got)
		}
		click(&ctx.Router, Button3) // Add a spend path

		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Button3) // Keys
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Down, Down) // 1 -> 3
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Down) // 1 -> 2
		click(&ctx.Router, Button3)

		if got, ok = pumpUntil(frame, "Path 1: 2-of-3", 24); !ok {
			t.Fatalf("the path list does not show the new path.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down)
		click(&ctx.Router, Button3) // Done
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3) // Sorted

		// THE STUB SCREEN, with a per-slot expected origin for every slot.
		if got, ok = pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
		}
		// The per-slot "expects a key at" lines are pages in: the body grows
		// one line per slot and the grammar admits 32, which is why §7c makes
		// this a paged widget at all.
		if got, ok = composerPageUntil(t, ctx, frame, "expects a key at", 10); !ok {
			t.Fatalf("the stub screen never named an expected origin.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// PART B'S SEATING STEP, IF IT IS PRESENT. With nothing to seat from it
		// asks whether to engrave the key-less template; in Part A alone there
		// is no such screen and consent follows the stub directly. The walk
		// tolerates both, because it is the same journey either way, and a walk
		// rewritten when the next task lands is no longer the same claim.
		if got, ok = pumpUntil(frame, "Seat keys into this template?", 12); ok {
			click(&ctx.Router, Button3) // Engrave a key-less template
		}

		// CONSENT, STATING NO ADDRESSES (D4).
		if got, ok = composerPageUntil(t, ctx, frame, "Keyless template - no addresses", 12); !ok {
			t.Fatalf("the consent never says there are no addresses.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		if got, ok = pumpUntil(frame, "Nothing outside this device", 48); !ok {
			t.Fatalf("§8l never drew.\nLast frame: %q", got)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		// THE FORM CHOICE COLLAPSED, and saying so is §7f's rule.
		if got, ok = pumpUntil(frame, "No slot is seated", 48); !ok {
			t.Fatalf("the collapsed form choice never said why it collapsed.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)

		if got, ok = pumpUntil(frame, "This engraves", 64); !ok {
			t.Fatalf("the plate census never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "md1 template") {
			t.Errorf("the census does not name the key-less template.\nFrame: %q", got)
		}
		if uiContains(got, "mk1 key") {
			t.Errorf("a key-less composition cut a key card.\nFrame: %q", got)
		}
	})
}

// TestComposerKeylessTemplateDecodesOnTheDevice is the artifact half of
// §12 item 3, at the layer that can assert it: the chunk set this flow
// engraves is read back by the device's own decoder, every slot declares an
// origin, and no two slots share one (§4f's invariant, which is what makes
// the template seatable at all).
func TestComposerKeylessTemplateDecodesOnTheDevice(t *testing.T) {
	for _, w := range []md.ComposeWrapper{md.ComposeTr, md.ComposeWsh, md.ComposeShWsh, md.ComposeSh} {
		list := md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		}}
		c, err := md.Compose(list)
		if err != nil {
			t.Fatalf("wrapper %v: md.Compose: %v", w, err)
		}
		chunks, err := c.Chunks()
		if err != nil {
			t.Fatalf("wrapper %v: Chunks: %v", w, err)
		}
		tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("wrapper %v: the device cannot decode what it just built: %v", w, err)
		}
		if tpl.N != 3 {
			t.Errorf("wrapper %v: decoded N = %d, want 3", w, tpl.N)
		}
		seen := map[string]bool{}
		for _, k := range keys {
			if k.XpubPresent {
				t.Errorf("wrapper %v: slot @%d carries a key in a KEYLESS template", w, k.Index)
			}
			if len(k.OriginPath) == 0 {
				t.Errorf("wrapper %v: slot @%d declares no origin; the fork's decoder "+
					"refuses a pathless slot (F-166)", w, k.Index)
			}
			o := k.OriginPath.String()
			if seen[o] && !k.FingerprintPresent {
				t.Errorf("wrapper %v: two slots declare %s with no fingerprints, which "+
					"errSeatSlotContested makes unseatable (§4f's invariant)", w, o)
			}
			seen[o] = true
		}
		// And the consent surface reads it.
		if _, err := composerConsentLinesFor(chunks, nil, 0); err != nil {
			t.Errorf("wrapper %v: composerConsentLines: %v", w, err)
		}
	}
}

// composerPageUntil pages forward looking for a string, returning the frame it
// was found on. Paging is forward-only with wrap, so a bounded number of
// pages either finds it or proves it is not on the screen at all.
func composerPageUntil(t *testing.T, ctx *Context, frame func() (string, bool), want string, pages int) (string, bool) {
	t.Helper()
	var last string
	for i := 0; i < pages; i++ {
		c, ok := frame()
		if !ok {
			return last, false
		}
		last = c
		if uiContains(c, want) {
			return c, true
		}
		click(&ctx.Router, Button2)
	}
	return last, false
}

// composerPageToEnd pages a composer read screen to its last page and
// confirms.
//
// The checkmark is withheld until the last page has been laid out once (§7e's
// proof is the addresses, and consenting before they are drawn is consenting
// to a wallet nobody saw), so a walk has to page as an operator would.
func composerPageToEnd(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	for i := 0; i < 12; i++ {
		click(&ctx.Router, Button2)
		if _, ok := frame(); !ok {
			return
		}
	}
	click(&ctx.Router, Button3)
	frame()
}

// TestComposerBackAtThePathListKeepsTheComposition is §7b's rule, walked --
// and before this the answer to "is any Back asserted by a test that would
// fail if Back lost state" was no, for every one of them.
func TestComposerBackAtThePathListKeepsTheComposition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		pumpUntil(frame, "Which script?", 24)
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		// The preset picker (§4d, task A10) sits between the wrapper and
		// the path list on the FIRST pass; Back is the blank route. The
		// re-pick after the Back below does NOT pass through it, which is
		// why this step appears once.
		pumpUntil(frame, "Start from?", 24)
		click(&ctx.Router, Button1)
		pumpUntil(frame, "Add a spend path", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Button3)
		got, ok := pumpUntil(frame, "Path 1: 1-of-2", 24)
		if !ok {
			t.Fatalf("the path was never added.\nLast frame: %q", got)
		}

		// BACK at the path list goes back ONE STEP, to the wrapper.
		click(&ctx.Router, Button1)
		if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("Back at the path list did not land on the wrapper picker; it used "+
				"to drop the whole composition.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)

		// ...and the list is still there.
		if got, ok = pumpUntil(frame, "Path 1: 1-of-2", 24); !ok {
			t.Fatalf("the path list lost its path across a Back; §7b's rule is that going "+
				"back loses nothing.\nLast frame: %q", got)
		}
	})
}

// TestComposerKeylessConfirmFiresAgainForANewPathAtAReusedIndex is C16's
// unskippable-confirm rule, and the reproduction is the one the fidelity lens
// constructed: the confirm was memoised by path INDEX, and "Remove path"
// splices the slice without touching the memo.
func TestComposerKeylessConfirmFiresAgainForANewPathAtAReusedIndex(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}, reg: &seedRegistry{}}
		frame, quit := runUI(ctx, func() { composerShapeFlow(ctx, &descriptorTheme, st) })
		defer quit()

		add := func(which string) {
			t.Helper()
			pumpUntil(frame, "Add a spend path", 24)
			// An empty list opens on "Add a spend path".
			for i := 0; i < len(st.list.Paths); i++ {
				click(&ctx.Router, Down)
			}
			click(&ctx.Router, Button3)
			pumpUntil(frame, "What can spend on this path?", 24)
			click(&ctx.Router, Down) // Keys -> A hash, no keys
			click(&ctx.Router, Button3)
			got, ok := pumpUntil(frame, "KEY-LESS PATH", 24)
			if !ok {
				t.Fatalf("%s: §8a did not fire for a key-less path -- an unskippable "+
					"confirm that can be skipped is the defect §12 item 4 exists "+
					"for.\nLast frame: %q", which, got)
			}
			// Decline it: the path is truncated and we are back at the list.
			click(&ctx.Router, Button1)
			frame()
		}
		add("first key-less path")
		add("a second key-less path at the same index")
	})
}
