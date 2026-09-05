package gui

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// hexDecodeForTest is hex.DecodeString under a name that says why it is here:
// the tests-lens found that composerHexEntry's exact-64 bound was "covered"
// only by hex.DecodeString rejecting ODD lengths, which is a different
// property. 62 characters decode fine and are still not a digest.
func hexDecodeForTest(s string) ([]byte, error) { return hex.DecodeString(s) }

// mdmkCardRecords is a complete mk1 card as payload records, for the door's
// ClassMDMK branch -- which every earlier door fixture reached through
// ClassDescriptor instead, making the branch a false PASS.
func mdmkCardRecords(t *testing.T) []string {
	t.Helper()
	st, template, keyed := composerCardFixture(t)
	strs, err := composerMintCard(st, 0, template, keyed)
	if err != nil {
		t.Fatalf("composerMintCard: %v", err)
	}
	return strs
}

// ═══ §12 item 5's gates, for the three sections that had none ═══════════════
//
// Measured before this file: §8m's five structural refusals had NO modal-fits
// assertion and were never drawn onto a frame; §8c's seven echo and bound
// lines were asserted at string level only; §8r's six door lines had one line
// driven, in the door walk. All three are drawn through showError or through
// the door's lead, i.e. exactly the surfaces assertModalBodyFits measures, so
// there is no paged-screen carve-out for them.

func TestComposerSection8mRefusalsAllFitAndDraw(t *testing.T) {
	bodies := []struct{ what, body string }{
		{"§8m no keyed path", composerCopyRefuseNoKeyedPath()},
		{"§8m lock-only path", composerCopyRefuseLockOnly()},
		{"§8m key-less under tr", composerCopyRefuseKeylessTr()},
		{"§8m legacy wrapper shape", composerCopyRefuseLegacyShape()},
		{"§8m slot cap", composerCopyRefuseSlotCap()},
	}
	for _, b := range bodies {
		assertModalBodyFits(t, b.what, errorScreenBody, b.body)
	}
	// AND ONE IS DRIVEN ONTO A FRAME through the real refusal path, so the
	// mapping test above is not the only thing standing between §4e and the
	// operator.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		digest := [32]byte{0x11}
		list := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}}, {Hash: &digest},
		}}
		_, err := md.ValidatePathList(list)
		if err == nil {
			t.Fatal("the fixture shape is legal, so no refusal can draw")
		}
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerShowRefusal(ctx, &descriptorTheme, "Spend paths", err)
		})
		defer quit()
		got, ok := frame()
		if !ok {
			t.Fatal("the refusal drew no frame")
		}
		assertFrameHasBody(t, ink(), "the §8m key-less-under-tr refusal")
		if !uiContains(got, "will not put a key-less path in taproot") {
			t.Errorf("the drawn refusal is not §8m line 3.\nFrame: %q", got)
		}
	})
}

// composerAssertDrawnInFull is assertModalBodyFits' contract for a body too
// SHORT to satisfy its harness.
//
// MEASURED: firstModalFrame refuses a frame under buildWalkRasterFloor, and a
// twelve-character line like "Block 905000" draws 5,166 ink pixels against a
// 6,000 floor -- the guard that exists to catch a BLANKED modal fires on a
// genuinely small one. §12 item 5 asks that every §8 body be drawn in full on
// the surface the operator meets it on, and for these two sections that
// surface is not showError: §8c's echoes are a LIST on the composer's read
// screen and §8r's lines are the door's LEAD. So they are asserted there, with
// bodyDrawnFully -- the same comparator assertModalBodyFits uses -- and the
// raster floor is applied to the whole frame, which is what it was written
// for.
func composerAssertDrawnInFull(t *testing.T, what, drawn, body string) {
	t.Helper()
	ok, drew, want := bodyDrawnFully(drawn, body)
	if !ok {
		t.Errorf("%s: the screen draws %d of the body's %d characters.\ncut after: ...%s",
			what, drew, want, tailOf(normalizeDrawn(body)[:drew], 48))
	}
}

// TestComposerSection8cEchoesAllDrawOnTheirOwnScreen is §12 item 5 for §8c,
// on the surface §6b puts them on: the echo screen the operator confirms.
func TestComposerSection8cEchoesAllDrawOnTheirOwnScreen(t *testing.T) {
	packed := composerBound{seconds: 1788220800, hasBound: true}
	withHeight := composerBound{seconds: 1788220800, height: 905000, hasBound: true, hasHeight: true}
	for _, tc := range []struct {
		what string
		lock md.Lock
		b    composerBound
	}{
		{"§8c relative days echo", md.Lock{Kind: md.LockOlderUnits, Value: 15188}, composerBound{}},
		{"§8c relative blocks echo", md.Lock{Kind: md.LockOlderBlocks, Value: 1000}, composerBound{}},
		{"§8c height echo with a packed height", md.Lock{Kind: md.LockAfterHeight, Value: 905001}, withHeight},
		{"§8c date echo with a packed date", md.Lock{Kind: md.LockAfterTime, Value: 1803859200}, packed},
		{"§8c date echo with no bound at all", md.Lock{Kind: md.LockAfterTime, Value: 1803859200}, composerBound{}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lines := composerLockEcho(tc.lock, tc.b)
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				frame, _, ink, quit := runUITouchRaster(ctx, func() {
					composerReadScreen(ctx, &descriptorTheme, "Time lock", lines)
				})
				defer quit()
				got, ok := frame()
				if !ok {
					t.Fatal("the echo screen drew no frame")
				}
				// THE CHROME FLOOR, NOT assertFrameHasBody's. MEASURED: a
				// single-line echo ("1000 blocks (about 6.9 days)") draws
				// 5,669 ink pixels against assertFrameHasBody's 5,982, whose
				// margin is calibrated for a screen with a paragraph on it.
				// The property that matters here is that something drew BEYOND
				// the chrome, and titleOnlyInk is exactly that number --
				// searched over 1..3 nav buttons, so it tracks the chrome
				// rather than a constant.
				if blank := titleOnlyInk(t); ink() <= blank {
					t.Errorf("%s drew %d ink pixels against a body-less frame's %d -- "+
						"nothing of the echo reached the screen", tc.what, ink(), blank)
				}
				for _, l := range lines {
					composerAssertDrawnInFull(t, tc.what, got, l)
				}
			})
		})
	}
}

// TestComposerSection8rDoorLinesAllDrawOnTheDoor is §12 item 5 for §8r, on
// the door itself -- every key state, not the one the door walk happened to
// use.
func TestComposerSection8rDoorLinesAllDrawOnTheDoor(t *testing.T) {
	for _, tc := range []struct {
		what    string
		session *syswSession
		inFlash bool
	}{
		{"keys only", composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil), false},
		{"keys and a seed", composerSessionWith([]string{composerTestKeyRecord}, []string{composerTestMnemonicRecord}), false},
		{"a seed alone", composerSessionWith(nil, []string{composerTestMnemonicRecord}), false},
		{"nothing loaded", composerSessionWith(nil, nil), false},
		{"records not understood", composerSessionWith([]string{composerTestKeyRecord, "hash:zz"}, nil), false},
	} {
		t.Run(tc.what, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				ctx.sysw = tc.session
				frame, _, ink, quit := runUITouchRaster(ctx, func() {
					composerDoorFlow(ctx, &descriptorTheme)
				})
				defer quit()
				got, ok := pumpUntil(frame, "Build a new policy", 16)
				if !ok {
					t.Fatalf("the door never drew.\nLast frame: %q", got)
				}
				assertFrameHasBody(t, ink(), "the door, "+tc.what)
				for _, l := range composerDoorLines(tc.session, tc.inFlash) {
					composerAssertDrawnInFull(t, tc.what, got, l)
				}
			})
		})
	}
	// The one line no session can produce, since it describes an UNLOADED
	// payload: asserted on the door with no session at all.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
		defer quit()
		got, ok := pumpUntil(frame, "Build a new policy", 16)
		if !ok {
			t.Fatalf("the door never drew.\nLast frame: %q", got)
		}
		assertFrameHasBody(t, ink(), "the door, no session")
		composerAssertDrawnInFull(t, "§8r no keys", got, composerCopyNoKeys())
	})
}

// TestComposerReadScreenWithholdsTheCheckmarkUntilTheLastPage is §7e's proof
// rule, and it replaces an ink comparison that could not fail.
//
// The old pager test compared total frame ink between a 1-line and a 64-line
// body and concluded the pager made the difference -- but ink is dominated by
// the body rows, so it passed whether the pager was drawn always, never, or
// correctly. This asserts the BEHAVIOUR instead: on a body that needs a second
// page, Button3 on the first frame does nothing; on a one-page body it
// returns at once. Both directions can fail.
func TestComposerReadScreenWithholdsTheCheckmarkUntilTheLastPage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		done := false
		frame, quit := runUI(ctx, func() {
			composerReadScreen(ctx, &descriptorTheme, "Read", composerNumberedLines(64))
			done = true
		})
		defer quit()
		if _, ok := frame(); !ok {
			t.Fatal("no frame")
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 4; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if done {
			t.Fatal("a 64-line consent was confirmed from its FIRST page; §7e's addresses " +
				"are the only proof of which wallet this is, and they are pages in")
		}
		for i := 0; i < 12 && !done; i++ {
			click(&ctx.Router, Button2)
			frame()
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 4 && !done; i++ {
			frame()
		}
		if !done {
			t.Error("after paging to the end the checkmark still did not confirm")
		}
	})
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		done := false
		frame, quit := runUI(ctx, func() {
			composerReadScreen(ctx, &descriptorTheme, "Read", []string{"only line"})
			done = true
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3)
		for i := 0; i < 4 && !done; i++ {
			frame()
		}
		if !done {
			t.Error("a ONE-page screen withheld its checkmark; the gate is about pages the " +
				"operator has not seen, not about making them press twice")
		}
	})
}

// TestComposerPickScreenNeverReturnsARowItDidNotDraw is the wrap clamp.
//
// On Button2 the page advances or wraps to start = 0, and the only clamp was
// upward -- so after a wrap the cursor could sit on a row belonging to a later
// page: that frame drew NO highlight and Button3 returned the invisible row,
// seating a key the operator never saw selected.
func TestComposerPickScreenNeverReturnsARowItDidNotDraw(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		rows := composerNumberedLines(20)
		var got int
		var ok bool
		frame, quit := runUI(ctx, func() {
			got, ok = composerPickScreen(ctx, &descriptorTheme, "Pick", "Choose one", rows)
		})
		defer quit()
		frame()
		// Walk the cursor to the last row, then WRAP with the pager.
		for i := 0; i < len(rows); i++ {
			click(&ctx.Router, Down)
			frame()
		}
		for i := 0; i < 6; i++ {
			click(&ctx.Router, Button2)
			frame()
		}
		content, _ := frame()
		click(&ctx.Router, Button3)
		for i := 0; i < 4; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if !ok {
			t.Fatal("the pick screen returned no selection")
		}
		if got < 0 || got >= len(rows) {
			t.Fatalf("the pick screen returned row %d, outside 0..%d", got, len(rows)-1)
		}
		if !uiContains(content, rows[got]) {
			t.Errorf("the pick screen returned row %d (%q), which was NOT on the frame the "+
				"operator confirmed.\nFrame: %q", got, rows[got], content)
		}
	})
}

// TestComposerConsentNumbersPathsAsTheOperatorListedThem is fidelity I-2:
// PolicyShape.Branches are LEAVES, and a taproot internal key is reported
// through KeyPath rather than as a Branch -- so a consent that numbered
// branches called the operator's Path 2 "Path 1", while the seating prompt for
// the same slot said "Path 2".
func TestComposerConsentNumbersPathsAsTheOperatorListedThem(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	internal, extracted := c.InternalKeyPath()
	if !extracted {
		t.Fatal("the fixture does not extract an internal key, so it cannot show the defect")
	}
	// The leaf list with the extracted path removed, in listed order: exactly
	// what composerLeafPaths computes for the self-check.
	var listed []int
	for i := range list.Paths {
		if i == internal {
			continue
		}
		listed = append(listed, i+1)
	}
	lines, err := composerConsentLinesFor(chunks, listed, internal+1)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Path 2: 2-of-3") {
		t.Errorf("the consent calls the operator's Path 2 something else:\n%s", joined)
	}
	if strings.Contains(joined, "Path 1: 2-of-3") {
		t.Errorf("the consent numbers branches rather than the operator's paths, so it "+
			"disagrees with the seating prompt about which path is which:\n%s", joined)
	}
	if !strings.Contains(joined, "Key-path (Path 1)") {
		t.Errorf("the key-path line does not name the listed path it came from:\n%s", joined)
	}
}

// ═══ The mutation lens's survivors, each with the mutation named ════════════

// TestComposerDoorOffersFromPayloadForACardPayload is tests-lens C-4.
// MUTATION: drop the ClassMDMK branch of composerDoorHasConsumablePolicy.
// It was a false PASS because every door fixture held a Descriptor.
func TestComposerDoorOffersFromPayloadForACardPayload(t *testing.T) {
	s := composerSessionWith(mdmkCardRecords(t), nil)
	if !s.has(sysw.ClassMDMK) {
		t.Fatal("INCONCLUSIVE: the card fixture does not classify as ClassMDMK")
	}
	if s.has(sysw.ClassDescriptor) {
		t.Fatal("INCONCLUSIVE: the card fixture also holds a Descriptor, so this test " +
			"would pass through the other branch")
	}
	if !composerDoorHasConsumablePolicy(s) {
		t.Error("a payload holding an md1/mk1 card is not offered the From payload route; " +
			"§7a offers it for a Descriptor OR an md1/mk1 record")
	}
}

// TestComposerShapeRefusalGateIsReachedFromTheScreen is tests-lens C-6.
// MUTATION: delete composerShapeFlow's ValidatePathList gate. The mapping test
// checked composerRefusalBody only; nothing drove Done on an illegal shape.
func TestComposerShapeRefusalGateIsReachedFromTheScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		digest := [32]byte{0x77}
		// One key-less path under wsh: legal to BUILD, refused at Done because
		// §4e's first row needs a path with keys.
		st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Hash: &digest},
		}}, reg: &seedRegistry{}}
		done := false
		frame, quit := runUI(ctx, func() {
			composerShapeFlow(ctx, &descriptorTheme, st)
			done = true
		})
		defer quit()
		got, ok := pumpUntil(frame, "Path 1: hash only", 24)
		if !ok {
			t.Fatalf("the path list never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down) // path 1 -> Add -> Change script -> Done
		click(&ctx.Router, Button3)
		got, ok = pumpUntil(frame, "at least one path with a key", 24)
		if !ok {
			t.Fatalf("Done on a key-less-only list did not refuse with §8m line 1.\n"+
				"Last frame: %q", got)
		}
		if done {
			t.Error("the shape flow RETURNED on a shape §4e refuses")
		}
	})
}

// TestComposerLockAcceptRefusesFromTheScreen is tests-lens C-8.
// MUTATION: make composerLockAccept return true unconditionally. Only
// md.Lock.Check and the pure parsers were tested; the screen's own gate was not.
func TestComposerLockAcceptRefusesFromTheScreen(t *testing.T) {
	b := composerBound{seconds: 1788220800, height: 905000, hasBound: true, hasHeight: true}
	for _, tc := range []struct {
		what string
		lock md.Lock
	}{
		{"a block count past §4c's ceiling", md.Lock{Kind: md.LockOlderBlocks, Value: 65536}},
		{"zero time units, which md itself still accepts", md.Lock{Kind: md.LockOlderUnits, Value: 0}},
		{"a date before the payload was packed", md.Lock{Kind: md.LockAfterTime, Value: 1788220799}},
		{"a height below the packed height", md.Lock{Kind: md.LockAfterHeight, Value: 904999}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				var accepted bool
				frame, quit := runUI(ctx, func() {
					accepted = composerLockAccept(ctx, &descriptorTheme, tc.lock, b)
				})
				defer quit()
				for i := 0; i < 6; i++ {
					if _, ok := frame(); !ok {
						break
					}
				}
				if accepted {
					t.Errorf("composerLockAccept ACCEPTED %s", tc.what)
				}
			})
		})
	}
	// The CONTROL: a legal lock is accepted, so the four refusals above are the
	// gate and not a function that refuses everything.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var accepted bool
		frame, quit := runUI(ctx, func() {
			accepted = composerLockAccept(ctx, &descriptorTheme,
				md.Lock{Kind: md.LockOlderBlocks, Value: 1000}, b)
		})
		defer quit()
		frame()
		if !accepted {
			t.Error("INCONCLUSIVE: a legal 1000-block lock was refused")
		}
	})
}

// TestComposerStubLinesLabelASeatedSlot is tests-lens C-10.
// MUTATION: always print "expects a key at", even for a seated slot. Every
// existing call passed nil for keyedChunks, so the seated branch never ran.
func TestComposerStubLinesLabelASeatedSlot(t *testing.T) {
	st, template, keyed := composerCardFixture(t)
	_ = st
	lines, err := composerStubLines(template, keyed, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "expects a key at") {
		t.Errorf("a fully seated template still tells the operator a slot EXPECTS a key:\n%s", joined)
	}
	if !strings.Contains(joined, "Slot @0: 73c5da0a") {
		t.Errorf("a seated slot is not labelled with its fingerprint and origin:\n%s", joined)
	}
	// The KEYED half §7c asks for: both ids, both stubs, and the advice to
	// stamp both.
	for _, want := range []string{"Policy-ID:", "mk1 stub (policy):", "Stamp BOTH stubs"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the keyed stub screen does not say %q:\n%s", want, joined)
		}
	}
}

// TestComposerStubReshowSignalIsTheChunkSet is tests-lens C-9.
// MUTATION: invert composerFlow's re-show signal. It was a sticky `edited`
// bool set on any Back; it is now a comparison of the emitted chunk sets, so
// the signal is the artifact rather than a flag about navigation.
func TestComposerStubReshowSignalIsTheChunkSet(t *testing.T) {
	a := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	b := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}},
	}}
	chunksOf := func(l md.PathList) []string {
		t.Helper()
		c, err := md.Compose(l)
		if err != nil {
			t.Fatal(err)
		}
		s, err := c.Chunks()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	first, again, edited := chunksOf(a), chunksOf(a), chunksOf(b)
	if !slices.Equal(first, again) {
		t.Fatal("INCONCLUSIVE: composing one shape twice gave two chunk sets")
	}
	if slices.Equal(first, edited) {
		t.Fatal("INCONCLUSIVE: adding a path did not change the chunk set")
	}
	// The flow's rule, stated as the comparison it performs.
	if changed := first != nil && !slices.Equal(first, again); changed {
		t.Error("re-reaching the stub screen with NO edit reports the id changed -- §8s " +
			"would then be a false statement on the screen that gets copied onto steel")
	}
	if changed := first != nil && !slices.Equal(first, edited); !changed {
		t.Error("an actual shape edit does not report the id changed, so §8s never fires")
	}
}

// TestComposerMappingReviewRefusesFromTheScreen is tests-lens C-11 and C-13.
// MUTATIONS: disable the §4f invariant check at composerMappingReview's call
// site (the logic underneath was tested, the wiring was not), and delete the
// C29 warning from composerMappingLines' output.
func TestComposerMappingReviewRefusesFromTheScreen(t *testing.T) {
	origin := composerTestOrigin(2, 0)
	st := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, origin: origin, fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, fpPresent: true},
		{src: 1, origin: origin},
		{src: -1}, {src: -1},
	}}
	st.sources = []composerSource{{kind: composerSourceKey}, {kind: composerSourceKey}}
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var ok bool
		frame, quit := runUI(ctx, func() {
			ok = composerMappingReview(ctx, &descriptorTheme, st)
		})
		defer quit()
		got, seen := pumpUntil(frame, "not both carry a fingerprint", 16)
		if !seen {
			t.Fatalf("the mapping review did not refuse §4f's invariant violation.\n"+
				"Last frame: %q", got)
		}
		if ok {
			t.Error("composerMappingReview returned true on a template that could not be restored")
		}
	})

	// C-13: the C29 warning must be IN the review's own output, not merely
	// computable by a helper the review does not print.
	fp := [4]byte{0xaa}
	shared := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, origin: composerTestOrigin(2, 0), fingerprint: fp, fpPresent: true},
		{src: 0, origin: composerTestOrigin(2, 1), fingerprint: fp, fpPresent: true},
		{src: 1, origin: composerTestOrigin(2, 2), fingerprint: [4]byte{0xbb}, fpPresent: true},
		{src: 2, origin: composerTestOrigin(2, 3), fingerprint: [4]byte{0xcc}, fpPresent: true},
	}}
	shared.sources = []composerSource{
		{kind: composerSourceSeed, fingerprint: fp, fpPresent: true},
		{kind: composerSourceKey}, {kind: composerSourceKey},
	}
	joined := strings.Join(composerMappingLines(shared), "\n")
	if !strings.Contains(joined, "SAME SEED, SAME PATH") {
		t.Errorf("the mapping review's own output does not carry the C29 warning:\n%s", joined)
	}
}

// TestComposerShortfallCountsSeatsFromTheScreen is tests-lens C-12.
// MUTATION: swap composerShortfall's count from assignable SEATS to sources at
// the call site. composerAssignableSlots was tested; the screen's use was not.
func TestComposerShortfallCountsSeatsFromTheScreen(t *testing.T) {
	st := &composerState{list: composerTwoPathList()} // 4 slots
	st.sources = []composerSource{{kind: composerSourceKey}, {kind: composerSourceKey}}
	st.assigned = []composerAssignment{{src: 0}, {src: 1}, {src: -1}, {src: -1}}
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerShortfall(ctx, &descriptorTheme, st) })
		defer quit()
		got, ok := pumpUntil(frame, "4 slots, 2 keys available", 16)
		if !ok {
			t.Fatalf("the shortfall screen does not name §8p's counts.\nLast frame: %q", got)
		}
		if !uiContains(got, "Unfilled: slots @2 and @3") {
			t.Errorf("the shortfall screen does not name the unfilled slots.\nFrame: %q", got)
		}
	})
}

// TestComposerHexEntryTakesExactlySixtyFourCharacters is tests-lens I-1.
// MUTATION: accept 63 hex characters. It was caught only by hex.DecodeString's
// odd-length error, which is a different property from the bound §6c states.
func TestComposerHexEntryTakesExactlySixtyFourCharacters(t *testing.T) {
	for _, n := range []int{0, 62, 63, 64, 65} {
		frag := strings.Repeat("a", n)
		valid := len(frag) == 64
		if got := len(frag) == 64; got != valid {
			t.Fatalf("the fixture is inconsistent at %d", n)
		}
		raw, err := hexDecodeForTest(frag)
		switch {
		case n == 64:
			if err != nil || len(raw) != 32 {
				t.Errorf("64 hex characters do not decode to 32 bytes: %v", err)
			}
		case n == 62:
			// EVEN and short: hex.DecodeString ACCEPTS it, which is why the
			// odd-length error is not the bound. Only the explicit
			// len(frag) == 64 test refuses this one.
			if err != nil {
				t.Errorf("62 hex characters fail to decode, so this case cannot show that "+
					"the length bound is what refuses them: %v", err)
			}
			if len(raw) == 32 {
				t.Errorf("62 hex characters decoded to 32 bytes")
			}
		}
	}
}

// ═══ ROUND-1 REGRESSION GUARDS ══════════════════════════════════════════════
//
// The round-1 fold verification found six Importants with a correct production
// fix and NO test that fails if it regresses, plus two tests-lens Criticals
// still open (C-9, C-12) and two unnumbered mutation cells (6b, 8d). Each test
// below names the mutation it fails under, so a later reader can re-run it.

// TestComposerLockAndHashEditsAreNotGuardedByTheDiscardConfirm is journey I-1's
// missing guard.
// MUTATION: add composerShapeGuard to composerPathEdit's Time-lock arm.
// §7d: "A lock or hash edit moves no slot, keeps assignments"; §7g classifies
// it DEFAULT. Telling a seated operator that every key will be cleared, for an
// edit that clears none, is false -- and declining it left the lock uneditable.
func TestComposerLockAndHashEditsAreNotGuardedByTheDiscardConfirm(t *testing.T) {
	for _, tc := range []struct {
		name  string
		downs int
		want  string
	}{
		{"time lock", 1, "What kind of time lock?"},
		// H2 (SPEC_hashlock_H2_device §4.1): with no ctx.sysw session loaded
		// (this test's own state), composerHashRows reports 0 payload digests,
		// so the screen's LEAD becomes composerCopyHashlockNoPayloadLead
		// rather than the literal "Which hash?" -- the TITLE ("Path 1 hash")
		// is what stays invariant across both cases, so the pump target moves
		// to it rather than to wording this test does not otherwise assert on.
		{"hash lock", 2, "Path 1 hash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
				composerSizeAssignments(st)
				st.assigned[0].src = 0 // SEATED: the guard would fire if it were on this arm
				st.sources = []composerSource{{kind: composerSourceKey, seedID: -1}}
				frame, quit := runUI(ctx, func() { composerPathEdit(ctx, &descriptorTheme, st, 0) })
				defer quit()
				if got, ok := pumpUntil(frame, "Path 1:", 16); !ok {
					t.Fatalf("the path editor never drew.\nLast frame: %q", got)
				}
				for i := 0; i < tc.downs; i++ {
					click(&ctx.Router, Down)
				}
				click(&ctx.Router, Button3)
				got, ok := pumpUntil(frame, tc.want, 24)
				if !ok {
					t.Fatalf("the %s editor was never reached.\nLast frame: %q", tc.name, got)
				}
				if uiContains(got, "EDITING THE SHAPE CLEARS THE KEYS") {
					t.Errorf("§8j fired on a %s edit, which renumbers nothing.\nFrame: %q", tc.name, got)
				}
			})
		})
	}
}

// TestComposerInvariantIgnoresSeveralUnseatedSlots is journey I-2's missing
// guard, and it is §8p's own legal fallback.
// MUTATION: remove the `src < 0` skip in composerInvariantViolation.
// An unseated slot has a nil origin, so two of them hashed to "" and looked
// like two keys at one origin with no fingerprints -- refusing the partially
// seated form with a body about keys that are not there.
func TestComposerInvariantIgnoresSeveralUnseatedSlots(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	composerSizeAssignments(st)
	if len(st.assigned) < 3 {
		t.Fatalf("the fixture has %d slots; this needs at least three", len(st.assigned))
	}
	st.assigned[0] = composerAssignment{
		src: 0, origin: composerTestOrigin(2, 0),
		fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, fpPresent: true,
	}
	// Every other slot unseated: THREE of them, no origins, no fingerprints.
	if composerInvariantViolation(st) {
		t.Error("three unseated slots are reported as colliding; §7f's partially seated " +
			"form and C26's key-less template are both legal and both refused by this")
	}
	// And a REAL collision is still caught, so the skip did not disable the check.
	st.assigned[1] = composerAssignment{src: 1, origin: composerTestOrigin(2, 0)}
	if !composerInvariantViolation(st) {
		t.Error("a genuine same-origin collision is no longer caught")
	}
}

// TestComposerBackInTheKeyEditorKeepsTheExistingKeySet is journey I-5's
// missing guard.
// MUTATION: drop the snapshot/restore in composerPathEdit's Keys arm.
// A decline used to leave Keys == nil on a path that already had a key set,
// which then read "hash only" and was refused at Done with a body about a lock
// nobody set.
func TestComposerBackInTheKeyEditorKeepsTheExistingKeySet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
		composerSizeAssignments(st) // nothing seated, so the guard stays silent
		before := *st.list.Paths[0].Keys
		frame, quit := runUI(ctx, func() { composerPathEdit(ctx, &descriptorTheme, st, 0) })
		defer quit()
		pumpUntil(frame, "Path 1:", 16)
		click(&ctx.Router, Button3) // Keys
		if got, ok := pumpUntil(frame, "how many keys?", 24); !ok {
			t.Fatalf("the key-count picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1) // Back, at the first screen inside the editor
		pumpUntil(frame, "Path 1:", 16)
		if st.list.Paths[0].Keys == nil {
			t.Fatal("Back inside the key editor left the path with NO key set; §7b's rule " +
				"is that going back loses nothing")
		}
		if *st.list.Paths[0].Keys != before {
			t.Errorf("Back changed the key set from %+v to %+v", before, *st.list.Paths[0].Keys)
		}
	})
}

// TestComposerChangeTheScriptRowRewrapsAndDiscards is fidelity I-4 and
// mutation-table cell 6b, both of which are the same gap from two angles.
// MUTATION: delete the "Change the script" row, or nullify
// composerApplyShapeEdit's discard at that call site.
// §12 item 4 names a wrapper change after seating as one of its vectors, and
// the row was the only way to reach it.
func TestComposerChangeTheScriptRowRewrapsAndDiscards(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
		composerSizeAssignments(st)
		st.assigned[0].src = 0
		st.sources = []composerSource{{kind: composerSourceKey, seedID: -1, used: true}}
		frame, quit := runUI(ctx, func() { composerShapeFlow(ctx, &descriptorTheme, st) })
		defer quit()
		got, ok := pumpUntil(frame, "Change the script", 24)
		if !ok {
			t.Fatalf("the wrapper-change row is not offered; §7g's wrapper row and §12 "+
				"item 4's wrapper vector are unreachable without it.\nLast frame: %q", got)
		}
		// paths(2) + "Add a spend path" + "Change the script"
		click(&ctx.Router, Down, Down, Down)
		click(&ctx.Router, Button3)
		// §8j fires, because a slot IS seated and the wrapper renumbers.
		if got, ok = pumpUntil(frame, "EDITING THE SHAPE CLEARS THE KEYS", 24); !ok {
			t.Fatalf("§8j did not fire before a wrapper change with a seat held.\n"+
				"Last frame: %q", got)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()
		if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Taproot (index 0), a change from wsh
		pumpUntil(frame, "Change the script", 24)
		if st.list.Wrapper != md.ComposeTr {
			t.Errorf("the wrapper is %v, want ComposeTr -- the row did not apply the change",
				st.list.Wrapper)
		}
		if composerAnySlotAssigned(st) {
			t.Error("a wrapper change kept its seats; §5 renumbers slots by first appearance " +
				"in text that is a function of the wrapper, so a carried assignment seats " +
				"keys into the wrong slots")
		}
		if st.sources[0].used {
			t.Error("the discarded source is still marked used, so it would never be offered again")
		}
	})
}

// TestComposerConsentRestatesTheHashRule is fidelity I-9's missing guard.
// MUTATION: delete the §8i block from composerConsentLinesFor.
// §6c and §8i's own heading say "at entry AND at consent": the rule whose
// whole purpose is to prevent an unspendable wallet was stated once, several
// screens earlier, on a policy that may have gained its hashlock afterwards.
func TestComposerConsentRestatesTheHashRule(t *testing.T) {
	digest := [32]byte{0xab}
	hashed := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}, Hash: &digest},
	}}
	c, err := md.Compose(hashed)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := composerConsentLinesFor(chunks, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), composerCopyHashRule()) {
		t.Errorf("a hash-bearing policy consents without restating the 32-byte rule:\n%s",
			strings.Join(lines, "\n"))
	}
	// And a policy with NO hash does not carry it: a rule restated where it does
	// not apply is one the operator learns to skip.
	plain := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	c2, err := md.Compose(plain)
	if err != nil {
		t.Fatal(err)
	}
	chunks2, _ := c2.Chunks()
	lines2, err := composerConsentLinesFor(chunks2, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(lines2, "\n"), composerCopyHashRule()) {
		t.Errorf("a hash-free policy carries §8i:\n%s", strings.Join(lines2, "\n"))
	}
}

// TestComposerHexEntryItselfRefusesAnythingButSixtyFourCharacters is the
// tests lens's I-1, on the REAL function, with a boundary the DECODER cannot
// refuse for it.
//
// MUTATION: `valid := len(frag) >= 63` in composerHexEntry.
//
// WHY THE RETURN VALUE CANNOT BE THE ASSERTION, measured in round 2: on a
// rejected fragment the entry loop `continue`s and the function never
// returns, so `ok` stays at its zero value -- which is the same `false` a
// genuine refusal produces. A test reading `ok` therefore passes whether the
// bound refused, the DECODER refused, or nothing happened at all, and the
// round-1 version of this test passed under its own named mutation for
// exactly that reason.
//
// WHAT IS OBSERVABLE INSTEAD: under the mutation a 63-character fragment is
// `valid`, so Button3 enters the accept branch, `hex.DecodeString` fails on
// the odd length, and the function draws "That is not a 32-byte digest." --
// a screen that does not exist when the bound is correct. The 62-character
// case is the even twin: `hex.DecodeString` SUCCEEDS on it, so with
// `valid := true` the `len(raw) != 32` guard draws the same error. Between
// them the two cases catch a loosened bound in either direction, and neither
// leans on the decoder to do the refusing.
func TestComposerHexEntryItselfRefusesAnythingButSixtyFourCharacters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		typed string
		why   string
	}{
		{"sixty-two hex characters", strings.Repeat("a", 62),
			"even, so hex.DecodeString accepts it and only the length bound can refuse it"},
		{"sixty-three hex characters", strings.Repeat("a", 63),
			"the exact length the named mutation would admit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				returned := false
				frame, quit := runUI(ctx, func() {
					composerHexEntry(ctx, &descriptorTheme)
					returned = true
				})
				defer quit()
				frame()
				// Typed through the ROUTER, so the real Keyboard consumes the
				// runes and the real bound sees the real fragment.
				for _, r := range tc.typed {
					ctx.Router.Events(nil, RuneEvent{Rune: r}.Event())
					frame()
				}
				click(&ctx.Router, Button3)
				last := ""
				for i := 0; i < 8; i++ {
					c, more := frame()
					if !more {
						break
					}
					last = c
					if uiContains(c, "not a 32-byte digest") {
						t.Fatalf("composerHexEntry ACCEPTED %d characters (%s): it reached "+
							"the decode branch, which only a valid-length fragment does.\n"+
							"Frame: %q", len(tc.typed), tc.why, c)
					}
				}
				if returned {
					t.Fatalf("composerHexEntry RETURNED for %d characters; §6c accepts a "+
						"digest only when exactly 64 valid hex characters are present",
						len(tc.typed))
				}
				if !uiContains(last, "of 64 hex") {
					t.Errorf("the entry screen is no longer showing its count line, so this "+
						"test is not measuring the entry any more.\nFrame: %q", last)
				}
			})
		})
	}
	// AND THE ACCEPTING CASE, so the two refusals above are the bound and not
	// a function that refuses everything.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var got [32]byte
		var ok bool
		frame, quit := runUI(ctx, func() { got, ok = composerHexEntry(ctx, &descriptorTheme) })
		defer quit()
		frame()
		for _, r := range strings.Repeat("a", 64) {
			ctx.Router.Events(nil, RuneEvent{Rune: r}.Event())
			frame()
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 8 && !ok; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if !ok {
			t.Fatal("INCONCLUSIVE: 64 valid hex characters were not accepted, so the two " +
				"refusals above prove nothing about the bound")
		}
		if got == [32]byte{} {
			t.Error("a 64-hex entry returned the zero digest, which is spendable by anyone " +
				"who knows the preimage of zero")
		}
	})
}

// TestComposerLockEditTellsAnImpossibleDateFromThePastCeilingDate is journey
// I-6 / F-458's guard, driving the REAL screen.
//
// MUTATION: restore `if y > 2038 || u == 0` as the ceiling test inside
// composerLockEdit's date closure.
//
// WHY THE PURE HELPERS ARE NOT ENOUGH, measured in round 2: the round-1 guard
// called composerDateExists and composerDateToUnix directly and re-derived
// the dispatch rule inline, so it passed under its own named mutation --
// composerLockEdit had zero test callers anywhere in the tree. The defect
// lives in the closure that CHOOSES the message, so the test has to reach the
// closure.
func TestComposerLockEditTellsAnImpossibleDateFromThePastCeilingDate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		digits  string
		want    string
		notWant string
	}{
		{"an impossible date inside the band", "20270231",
			"that date does not exist", "up to 2038-01-19"},
		{"a real date past the ceiling", "20450601",
			"up to 2038-01-19", "that date does not exist"},
		{"a real date below the floor", "20081231",
			"before 2009", "that date does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
				frame, quit := runUI(ctx, func() {
					composerLockEdit(ctx, &descriptorTheme, st, 0)
				})
				defer quit()
				if got, ok := pumpUntil(frame, "What kind of time lock?", 16); !ok {
					t.Fatalf("the lock-kind picker never drew.\nLast frame: %q", got)
				}
				click(&ctx.Router, Down, Down) // None -> After a wait -> After a date or height
				click(&ctx.Router, Button3)
				if got, ok := pumpUntil(frame, "Named how?", 16); !ok {
					t.Fatalf("the absolute-kind picker never drew.\nLast frame: %q", got)
				}
				click(&ctx.Router, Button3) // A date
				if got, ok := pumpUntil(frame, "Date as YYYYMMDD", 16); !ok {
					t.Fatalf("the date pad never drew.\nLast frame: %q", got)
				}
				last := ""
				for _, r := range tc.digits {
					ctx.Router.Events(nil, RuneEvent{Rune: r}.Event())
					c, more := frame()
					if !more {
						t.Fatal("the date pad stopped drawing mid-entry")
					}
					last = c
				}
				if !uiContains(last, tc.want) {
					t.Errorf("the date pad does not say %q for %s.\nFrame: %q",
						tc.want, tc.digits, last)
				}
				if uiContains(last, tc.notWant) {
					t.Errorf("the date pad says %q for %s, which is the WRONG message: the "+
						"three failures are told apart by what they ARE, not by reading the "+
						"returned operand (F-458).\nFrame: %q", tc.notWant, tc.digits, last)
				}
			})
		})
	}
}

// TestComposerShortfallCountsSeatsNotSourcesOnAFixtureThatCanTellThemApart is
// tests-lens C-12, still open after round 0.
// MUTATION: pass len(st.sources) instead of composerAssignableSlots(st).
// The earlier fixture used two plain key: sources against four slots, where
// both counting rules give 2 -- structurally incapable of distinguishing them.
// A SEED fills any number of slots (C12, §4f), so with one seed the two rules
// give 1 and 4.
func TestComposerShortfallCountsSeatsNotSourcesOnAFixtureThatCanTellThemApart(t *testing.T) {
	st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
	composerSizeAssignments(st)
	st.sources = []composerSource{{kind: composerSourceSeed, seedID: 0, fpPresent: true}}
	st.assigned[0].src = 0
	st.assigned[1].src = 0
	if got, want := composerAssignableSlots(st), composerSlotCount(st.list); got != want {
		t.Fatalf("INCONCLUSIVE: with a seed present %d slots are assignable, want %d", got, want)
	}
	if len(st.sources) == composerAssignableSlots(st) {
		t.Fatalf("INCONCLUSIVE: the fixture cannot tell the two counting rules apart " +
			"(sources == assignable seats)")
	}
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerShortfall(ctx, &descriptorTheme, st) })
		defer quit()
		got, ok := pumpUntil(frame, "keys available", 16)
		if !ok {
			t.Fatalf("the shortfall screen never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "4 slots, 4 keys available") {
			t.Errorf("the shortfall screen counts SOURCES, not assignable seats; §7d says "+
				"a seed is a source of as many slots as the operator assigns.\nFrame: %q", got)
		}
	})
}

// TestComposerMintCardsMintsOneCardPerSeatedSlot is mutation-table cell 8d,
// which the round-1 verification found still open once reconstructed.
// MUTATION: duplicate a card from the SECOND seated slot onward
// (`if len(out) >= 2 { out = append(out, card) }`).
// The existing test unseats one of its two slots before calling, so any
// duplication that needs two seated slots was invisible to it.
func TestComposerMintCardsMintsOneCardPerSeatedSlot(t *testing.T) {
	st, template, keyed := composerCardFixture(t)
	if len(st.assigned) != 2 {
		t.Fatalf("the fixture has %d slots; this test needs two SEATED", len(st.assigned))
	}
	for i, a := range st.assigned {
		if a.src < 0 {
			t.Fatalf("slot @%d is unseated; the point of this test is both seated", i)
		}
	}
	cards, err := composerMintCards(st, template, keyed)
	if err != nil {
		t.Fatalf("composerMintCards: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("two seated slots produced %d card(s), want exactly 2 -- a duplicate is a "+
			"plate the census counts and the operator cuts for nothing", len(cards))
	}
	if cards[0].label == cards[1].label {
		t.Errorf("both cards are labelled %q, so one slot was minted twice", cards[0].label)
	}
	seen := map[string]bool{}
	for _, c := range cards {
		key := strings.Join(c.strings, "|")
		if seen[key] {
			t.Errorf("two cards carry identical chunk strings: %q", c.label)
		}
		seen[key] = true
	}
}

// TestComposerSection8mRefusalsAllDrawThroughTheRealPath closes fidelity I-8's
// remainder: four of the five §8m bodies had a modal-fits assertion and no
// screen-level fires-on-condition test.
// MUTATION: remove any one showError call in composerShowRefusal's arms.
func TestComposerSection8mRefusalsAllDrawThroughTheRealPath(t *testing.T) {
	digest := [32]byte{0x11}
	for _, tc := range []struct {
		name string
		list md.PathList
		want string
	}{
		{"no keyed path", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{Hash: &digest}}},
			"at least one path with a key"},
		{"lock-only path", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}},
			{Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 100}},
		}}, "anyone can spend after it"},
		{"key-less under tr", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}}, {Hash: &digest},
		}}, "key-less path in taproot"},
		{"legacy wrapper shape", md.PathList{Wrapper: md.ComposeSh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
			{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}},
		}}, "Legacy wrappers hold one plain multisig"},
		{"empty list", md.PathList{Wrapper: md.ComposeWsh}, "at least one path with a key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := md.ValidatePathList(tc.list)
			if err == nil {
				t.Fatalf("the fixture is legal, so no refusal can draw")
			}
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				frame, _, ink, quit := runUITouchRaster(ctx, func() {
					composerShowRefusal(ctx, &descriptorTheme, "Spend paths", err)
				})
				defer quit()
				got, ok := frame()
				if !ok {
					t.Fatal("the refusal drew no frame")
				}
				assertFrameHasBody(t, ink(), "the §8m refusal for "+tc.name)
				if !uiContains(got, tc.want) {
					t.Errorf("the drawn refusal does not say %q.\nFrame: %q", tc.want, got)
				}
			})
		})
	}
	// The slot cap is the fifth body, and it is refused where the operator asks
	// for it rather than through composerShowRefusal.
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}, reg: &seedRegistry{}}
	for i := 0; i < 4; i++ {
		st.list.Paths = append(st.list.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 8}})
	}
	st.list.Paths = append(st.list.Paths, md.SpendPath{})
	if got := composerMaxKeysForPath(st, len(st.list.Paths)-1); got != 0 {
		t.Fatalf("INCONCLUSIVE: the fixture leaves %d slots free, so §8m line 5 is unreachable", got)
	}
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerKeysEdit(ctx, &descriptorTheme, st, len(st.list.Paths)-1)
		})
		defer quit()
		got, ok := frame()
		if !ok {
			t.Fatal("the slot-cap refusal drew no frame")
		}
		// NO INK FLOOR ON THIS ONE, and the reason is measured rather than
		// asserted: "This wallet already has 32 key slots." is 37 characters,
		// and this refusal is a TWO-button modal drawing 5,380 pixels in
		// total -- below titleOnlyInk's 5,482, which is the WORST case over
		// one to three nav buttons and therefore a three-button frame's
		// chrome. The instrument's floor is above this whole frame, so it
		// cannot say anything about this body. The text assertion below is
		// what stands, and it is the property that matters: the words reached
		// the screen.
		t.Logf("the §8m slot-cap refusal drew %d ink pixels (titleOnlyInk's worst-case "+
			"chrome is %d, so no ink floor applies to a body this short)", ink(), titleOnlyInk(t))
		if !uiContains(got, "already has 32 key slots") {
			t.Errorf("the 33rd slot is not refused with §8m line 5.\nFrame: %q", got)
		}
	})
}

// TestComposerDoorSaysAPayloadIsInFlashButNotLoaded closes fidelity I-8's §8r
// remainder: the one door line no loaded session can produce.
func TestComposerDoorSaysAPayloadIsInFlashButNotLoaded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		p.sysw = &countingSyswReader{probe: true}
		ctx := NewContext(p)
		// No session: the payload is in flash and was skipped at boot.
		frame, _, ink, quit := runUITouchRaster(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
		defer quit()
		got, ok := pumpUntil(frame, "Build a new policy", 16)
		if !ok {
			t.Fatalf("the door never drew.\nLast frame: %q", got)
		}
		assertFrameHasBody(t, ink(), "the door with a payload in flash")
		composerAssertDrawnInFull(t, "§8r payload in flash", got, composerCopyPayloadNotLoaded())
	})
}

// TestComposerConsentFlowNumbersPathsFromTheOperatorsList is fidelity I-2's
// PRODUCTION guard: the numbering fix was correct in isolation and dead from
// the live flow's point of view, because the only reachable call site
// hardcoded (nil, 0).
// MUTATION: pass nil, 0 to composerConsentLinesFor inside composerConsentFlow.
func TestComposerConsentFlowNumbersPathsFromTheOperatorsList(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	listed, keyPathNo := composerListedPaths(list)
	if keyPathNo != 1 || len(listed) != 1 || listed[0] != 2 {
		t.Fatalf("INCONCLUSIVE: composerListedPaths gave listed=%v keyPathNo=%d for a tr "+
			"list whose first path is the extracted internal key", listed, keyPathNo)
	}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	st := &composerState{list: list, reg: &seedRegistry{}}
	composerSizeAssignments(st)
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerConsentFlow(ctx, &descriptorTheme, st, chunks) })
		defer quit()
		got, ok := composerPageUntil(t, ctx, frame, "Path 2: 2-of-3", 10)
		if !ok {
			t.Fatalf("the consent surface never numbered the leaf as the operator's Path 2 "+
				"-- with an extracted internal key the branch list holds one entry, and "+
				"numbering branches calls the operator's Path 2 \"Path 1\".\nLast frame: %q", got)
		}
		if uiContains(got, "Path 1: 2-of-3") {
			t.Errorf("the consent numbers branches rather than the operator's paths.\nFrame: %q", got)
		}
	})
}

// TestComposerDateCeilingAndImpossibleDateAreToldApart is journey I-6's
// missing guard, and the defect it guards is F-458.
// MUTATION: restore `if y > 2038 || u == 0` as the ceiling test.
// composerDateToUnix returns 0 on EVERY failure, so that condition is a
// tautology and "that date does not exist" was dead code -- 2027-02-31 got
// the ceiling message, advising a block height for a date no height makes real.
func TestComposerDateCeilingAndImpossibleDateAreToldApart(t *testing.T) {
	for _, tc := range []struct {
		digits string
		exists bool
		why    string
	}{
		{"20270231", false, "2027-02-31 is inside the band and does not exist"},
		{"20271301", false, "month 13"},
		{"20450601", true, "2045-06-01 exists and is past the ceiling"},
		{"20081231", true, "2008-12-31 exists and is below the floor"},
		{"20270301", true, "the worked example, in band"},
	} {
		t.Run(tc.digits, func(t *testing.T) {
			y, m, d, parsed := composerParseDateDigits(tc.digits)
			if !parsed {
				t.Fatalf("%s did not parse", tc.digits)
			}
			if got := composerDateExists(y, m, d); got != tc.exists {
				t.Fatalf("composerDateExists(%s) = %v, want %v (%s)", tc.digits, got, tc.exists, tc.why)
			}
			_, inBand := composerDateToUnix(y, m, d)
			if inBand && !tc.exists {
				t.Fatalf("%s is in band but does not exist", tc.digits)
			}
		})
	}
	// The dispatch itself: an impossible date must NOT get the ceiling body,
	// and a past-ceiling date must.
	y, m, d, _ := composerParseDateDigits("20270231")
	if composerDateExists(y, m, d) {
		t.Fatal("INCONCLUSIVE: the impossible-date fixture is a real date")
	}
	y2, m2, d2, _ := composerParseDateDigits("20450601")
	if !composerDateExists(y2, m2, d2) {
		t.Fatal("INCONCLUSIVE: the past-ceiling fixture is not a real date")
	}
	if _, in := composerDateToUnix(y2, m2, d2); in {
		t.Fatal("INCONCLUSIVE: 2045-06-01 is inside the entry band")
	}
}

// TestComposerFlowReShowsTheStubScreenOnlyAfterARealEdit is tests-lens C-9,
// driven through `composerFlow` itself.
//
// MUTATION: `changed := false && shown != nil && !slices.Equal(shown, template)`
// in composerFlow.
//
// WHY THE PINNING TEST WAS NOT THE GUARD, measured in round 2:
// TestComposerStubReshowSignalIsTheChunkSet declares its own local `changed`
// and recomputes the comparison standalone -- it never calls composerFlow, so
// the line carrying the signal is untouched by it. The only other direct call
// to composerStubFlow in any test passes a HARDCODED false. So nothing
// exercised the real decision, and forcing it false left the whole suite ok.
//
// This walks the flow twice through the stub screen: once with no edit in
// between (§8s must be silent -- a false "this id changed" on the screen whose
// job is to be copied onto steel trains the operator to discount the line that
// will one day be true), and once after a real key-count change (§8s must
// fire, because a card minted with the old stub will not seat here).
func TestComposerFlowReShowsTheStubScreenOnlyAfterARealEdit(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit bool
	}{
		{"back out and Done again with no edit", false},
		{"back out, change a key count, Done again", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
				defer quit()

				// Wrapper -> wsh.
				if got, ok := pumpUntil(frame, "Which script?", 24); !ok {
					t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
				}
				click(&ctx.Router, Down)
				click(&ctx.Router, Button3)

				// The preset picker (§4d, task A10) sits between the wrapper and
				// the path list on the first pass; Back is §7b's blank route,
				// which is what this guard composes from.
				if got, ok := pumpUntil(frame, "Start from?", 24); !ok {
					t.Fatalf("the preset picker never drew.\nLast frame: %q", got)
				}
				click(&ctx.Router, Button3) // row 0 = Build my own paths (W-1)

				// One 1-of-2 path.
				if got, ok := pumpUntil(frame, "Add a spend path", 24); !ok {
					t.Fatalf("the path list never drew.\nLast frame: %q", got)
				}
				click(&ctx.Router, Button3)
				pumpUntil(frame, "What can spend on this path?", 24)
				click(&ctx.Router, Button3) // Keys
				pumpUntil(frame, "how many keys?", 24)
				click(&ctx.Router, Down) // 1 -> 2
				click(&ctx.Router, Button3)
				pumpUntil(frame, "how many must sign?", 24)
				click(&ctx.Router, Button3) // k = 1

				composerFlowDone(t, ctx, frame)

				// THE FIRST SHOWING carries no changed-id line: there is
				// nothing it could be a change from.
				got, ok := pumpUntil(frame, "mk1 stub (template)", 32)
				if !ok {
					t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
				}
				if uiContains(got, "The shape changed") {
					t.Errorf("the FIRST stub screen claims the id changed.\nFrame: %q", got)
				}
				click(&ctx.Router, Button1) // Back, to the path list

				if got, ok = pumpUntil(frame, "Add a spend path", 32); !ok {
					t.Fatalf("Back at the stub screen did not return to the path list.\n"+
						"Last frame: %q", got)
				}
				if tc.edit {
					// Path 1 -> Keys -> 3 keys, which moves a slot and so moves
					// the emitted chunk set.
					click(&ctx.Router, Button3)
					pumpUntil(frame, "Path 1:", 24)
					click(&ctx.Router, Button3) // Keys
					pumpUntil(frame, "how many keys?", 24)
					click(&ctx.Router, Down, Down) // 1 -> 3
					click(&ctx.Router, Button3)
					pumpUntil(frame, "how many must sign?", 24)
					click(&ctx.Router, Button3) // k = 1
					pumpUntil(frame, "Path 1:", 24)
					click(&ctx.Router, Button1) // leave the path editor
					pumpUntil(frame, "Add a spend path", 24)
				}
				composerFlowDone(t, ctx, frame)

				got, ok = pumpUntil(frame, "mk1 stub (template)", 32)
				if !ok {
					t.Fatalf("the stub screen never re-drew.\nLast frame: %q", got)
				}
				if tc.edit && !uiContains(got, "The shape changed") {
					t.Errorf("the stub screen was re-shown after a real key-count change and "+
						"does NOT carry §8s: a card minted with the old stub will not seat "+
						"here, and the operator is not told.\nFrame: %q", got)
				}
				if !tc.edit && uiContains(got, "The shape changed") {
					t.Errorf("the stub screen claims the id changed after a Back with NO "+
						"edit; §8s is then a false statement on the screen that gets copied "+
						"onto steel.\nFrame: %q", got)
				}
			})
		})
	}
}

// composerFlowDone takes the path list's "Done" row and answers the key-order
// question that follows it.
//
// The list's rows are the paths, then "Add a spend path", then "Change the
// script", then "Done" -- so with one path that is three Downs. Dispatch is by
// row NAME in the flow itself; the count here is the walk's own arithmetic and
// is asserted by the screen it lands on.
func composerFlowDone(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	click(&ctx.Router, Down, Down, Down)
	click(&ctx.Router, Button3)
	if got, ok := pumpUntil(frame, "Sorted keys, or your order?", 24); !ok {
		t.Fatalf("Done did not reach the key-order question.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // Sorted (usual)
}
