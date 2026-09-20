package gui

// Lens-4 (device flow, adversarial) walk tests written for the composer fable
// review r0. They script the SH2's real buttons through the Go harness and
// assert on what the plate planner was handed, not on screen text alone.
//
// A test whose name says "Spec" asserts the SPEC's promise; where it is RED at
// the tip it IS the counterexample. Tests that document a refusal or a
// correct path are mutation-checked in the report.

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/sysw"
)

// fableHold holds Button3 past confirmDelay: the only route through a
// ConfirmWarningScreen.
func fableHold(ctx *Context, frame func() (string, bool)) {
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	frame()
}

// fableCaptureConsent installs composerSelfCheckFaultHook as a READ seam and
// returns the chunks the consent was rendered from -- the set that reaches
// composerEngraveStep as `consent` (keyed when every slot is seated, the
// template otherwise). The hook returns its input unchanged.
func fableCaptureConsent(t *testing.T) *[]string {
	t.Helper()
	captured := new([]string)
	prev := composerSelfCheckFaultHook
	composerSelfCheckFaultHook = func(c []string) []string {
		*captured = append([]string(nil), c...)
		return c
	}
	t.Cleanup(func() { composerSelfCheckFaultHook = prev })
	return captured
}

// fableStartWsh walks composerFlow's opening pair: Segwit (wsh), then the
// blank row of "Start from?".
func fableStartWsh(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	if got, ok := pumpUntil(frame, "Which script?", 24); !ok {
		t.Fatalf("no wrapper picker.\nLast frame: %q", got)
	}
	click(&ctx.Router, Down) // Taproot -> Segwit (wsh)
	click(&ctx.Router, Button3)
	if got, ok := pumpUntil(frame, "Start from?", 24); !ok {
		t.Fatalf("no preset screen.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // row 0 = Build my own paths
	if got, ok := pumpUntil(frame, "Add a spend path", 24); !ok {
		t.Fatalf("no path list.\nLast frame: %q", got)
	}
}

// fableAddKeyedPath takes "Add a spend path" (rowsDown rows below the top of
// the list) and answers Keys, n, k.
func fableAddKeyedPath(t *testing.T, ctx *Context, frame func() (string, bool), rowsDown, n, k int) {
	t.Helper()
	if got, ok := pumpUntil(frame, "Add a spend path", 24); !ok {
		t.Fatalf("no path list.\nLast frame: %q", got)
	}
	for range rowsDown {
		click(&ctx.Router, Down)
	}
	click(&ctx.Router, Button3)
	pumpUntil(frame, "What can spend on this path?", 24)
	click(&ctx.Router, Button3) // Keys
	pumpUntil(frame, "how many keys?", 24)
	for range n - 1 {
		click(&ctx.Router, Down)
	}
	click(&ctx.Router, Button3)
	pumpUntil(frame, "how many must sign?", 24)
	for range k - 1 {
		click(&ctx.Router, Down)
	}
	click(&ctx.Router, Button3)
}

// fableDone takes the Done row of a list holding `paths` paths: the rows are
// the paths, "Add a spend path", "Change the script", "Done".
func fableDone(t *testing.T, ctx *Context, frame func() (string, bool), paths int) {
	t.Helper()
	if got, ok := pumpUntil(frame, "Add a spend path", 24); !ok {
		t.Fatalf("no path list.\nLast frame: %q", got)
	}
	for range paths + 2 {
		click(&ctx.Router, Down)
	}
	click(&ctx.Router, Button3)
}

// fableTypeSeed answers the word-count picker with 12 and types the abandon
// vector.
func fableTypeSeed(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	if got, ok := pumpUntil(frame, "Choose number of words", 48); !ok {
		t.Fatalf("no word-count picker.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // 12 words
	frame()
	typeWords(&ctx.Router, frame, fixtureMasterA)
}

// ─── 1. Key order: "Keep my order" is reversed by Back + Done + forward ──────

// TestFableSpecKeyOrderSurvivesBackFromTheStubScreen: §7b "Back preserves
// everything" and §8b "fires once per key set where sorted was legal and
// declined". The operator declines sorted with a hold-to-confirm, reaches the
// stub screen, steps Back to re-read the list, takes Done again and leaves the
// re-asked "Key order" screen by the forward button -- the control that has
// advanced every other screen. The policy the consent renders must still be
// the unsorted one they confirmed.
func TestFableSpecKeyOrderSurvivesBackFromTheStubScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		captured := fableCaptureConsent(t)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 3, 2)
		fableDone(t, ctx, frame, 1)
		if got, ok := pumpUntil(frame, "Sorted keys, or your order?", 24); !ok {
			t.Fatalf("no key-order question.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // Sorted (usual) -> Keep my order
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "UNSORTED KEYS (EXPERIMENTAL)", 24); !ok {
			t.Fatalf("§8b never fired on the decline.\nLast frame: %q", got)
		}
		fableHold(ctx, frame)
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("no stub screen.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1) // Back, to the path list
		fableDone(t, ctx, frame, 1)
		if got, ok := pumpUntil(frame, "Sorted keys, or your order?", 24); !ok {
			t.Fatalf("Done did not re-ask key order.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // forward, on whatever row the screen opened on

		_, experimentalAgain := pumpUntil(frame, "UNSORTED KEYS (EXPERIMENTAL)", 6)
		if experimentalAgain {
			fableHold(ctx, frame)
		}
		got, ok := pumpUntil(frame, "mk1 stub (template)", 32)
		if !ok {
			t.Fatalf("no second stub screen.\nLast frame: %q", got)
		}
		bannerFired := uiContains(got, "The shape changed")
		composerPageToEnd(t, ctx, frame)
		if _, ok := pumpUntil(frame, "Seat keys into this template?", 12); ok {
			click(&ctx.Router, Button3) // Engrave a key-less template
		}
		if got, ok = pumpUntil(frame, "Script:", 32); !ok {
			t.Fatalf("no consent.\nLast frame: %q", got)
		}
		if len(*captured) == 0 {
			t.Fatal("the consent hook saw no chunks")
		}
		shape, err := md.PolicyShapeChunks(*captured)
		if err != nil || len(shape.Branches) != 1 {
			t.Fatalf("consent chunks do not describe one branch: %v", err)
		}
		t.Logf("second pass: §8b re-fired=%v, stub banner=%v, consent has UNSORTED mark=%v, decoded Sorted=%v",
			experimentalAgain, bannerFired, uiContains(got, "UNSORTED"), shape.Branches[0].Sorted)
		if shape.Branches[0].Sorted {
			t.Errorf("the operator hold-confirmed 'Keep my order'; after Back from the stub "+
				"screen and Done again, the forward button on 'Key order' silently made the "+
				"policy sortedmulti (§8b re-fired=%v, only signal: stub banner %q=%v). "+
				"Consent chunks: %q", experimentalAgain, "The shape changed", bannerFired, *captured)
		}
	})
}

// ─── 2. Back inside the passphrase entry seats the seed WITHOUT it ───────────

// TestFableSpecBackOnThePassphraseKeyboardIsADecline: the digit pad's own rule
// ("Back is a decline everywhere on this device") and §7b's "going back should
// lose nothing". The operator chose "Add passphrase", started typing, and
// pressed Back to correct something. Expected: one screen back, the
// passphrase question. Measured: what the flow does instead, and the
// fingerprint that then reaches the mapping review.
func TestFableSpecBackOnThePassphraseKeyboardIsADecline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 2, 1)
		fableDone(t, ctx, frame, 1)
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3) // Sorted
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("no stub screen.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Slot @0", 24); !ok {
			t.Fatalf("no seat prompt.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down) // K1, K2 -> Type a seed
		click(&ctx.Router, Button3)
		fableTypeSeed(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 48); !ok {
			t.Fatalf("no passphrase question.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // Skip -> Add passphrase
		click(&ctx.Router, Button3)
		// The keyboard: same title as the question, no "Add a BIP-39" lead.
		var got string
		onKeyboard := false
		for i := 0; i < 8; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			got = c
			if uiContains(c, "Passphrase seed 1") && !uiContains(c, "Add a BIP-39 passphrase?") {
				onKeyboard = true
				break
			}
		}
		if !onKeyboard {
			t.Fatalf("the passphrase keyboard never drew.\nLast frame: %q", got)
		}
		runes(&ctx.Router, "x")
		frame()
		click(&ctx.Router, Button1) // Back, mid-passphrase

		got, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 6)
		if ok {
			return // spec behaviour: one screen back
		}
		t.Errorf("Back on the passphrase keyboard did not return to the passphrase question; "+
			"the device drew: %q", got)

		// Measure what it did instead.
		got, ok = pumpUntil(frame, "choose a key", 8)
		if !ok {
			t.Fatalf("after Back, neither the passphrase question nor the seat prompt drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "seed 1") {
			t.Fatalf("the seat prompt after Back offers no seed row.\nFrame: %q", got)
		}
		click(&ctx.Router, Down, Down) // K1, K2 -> seed 1 (any slots)
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Slot @1", 24); !ok {
			t.Fatalf("slot @1 never asked.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // K1 -> K2 (K1 would be the same key as seed@0)
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Key mapping", 24); !ok {
			t.Fatalf("no mapping review.\nLast frame: %q", got)
		}
		m, err := bip39.ParseMnemonic(fixtureMasterA)
		if err != nil {
			t.Fatal(err)
		}
		_, bareFP, _ := deriveAccountXpub(m, "", &chaincfg.MainNetParams, nil)
		_, ppFP, _ := deriveAccountXpub(m, "x", &chaincfg.MainNetParams, nil)
		var bare, pp [4]byte
		binary.BigEndian.PutUint32(bare[:], bareFP)
		binary.BigEndian.PutUint32(pp[:], ppFP)
		t.Logf("mapping review after Back mid-passphrase: %q", got)
		t.Logf("bare-seed fingerprint %x (what the review shows); with passphrase 'x' it would be %x", bare, pp)
		if !uiContains(got, "@0: "+hex.EncodeToString(bare[:])) {
			t.Errorf("expected the mapping review to seat the BARE seed at @0 (%x); frame %q", bare, got)
		}
	})
}

// ─── 3. The date pad: 2009-01-01 and 2009-01-02 get the CEILING body ─────────

// TestFableSpecDateBelowTheFloorInsideTwoThousandNineNamesTheFloor: §6b "the
// entry refuses every date before 2009-01-03 with §8t". The refusal body for a
// date below the floor must be §8t's, not the 2038 ceiling's.
func TestFableSpecDateBelowTheFloorInsideTwoThousandNineNamesTheFloor(t *testing.T) {
	for _, tc := range []struct {
		digits string
		want   string
	}{
		{"20081231", composerCopyDateFloor()},
		{"20090101", composerCopyDateFloor()},
		{"20090102", composerCopyDateFloor()},
		{"20380120", composerCopyDateCeiling()},
	} {
		line, valid := composerDateBandEcho(tc.digits)
		if valid {
			t.Errorf("%s was accepted", tc.digits)
			continue
		}
		if line != tc.want {
			t.Errorf("%s refused with %q, want %q", tc.digits, line, tc.want)
		}
	}
	if line, valid := composerDateBandEcho("20090103"); !valid {
		t.Errorf("20090103 (the floor itself) refused with %q", line)
	}
	if line, valid := composerDateBandEcho("20380119"); !valid {
		t.Errorf("20380119 (the ceiling itself) refused with %q", line)
	}
}

// ─── 4. A key-less path cleared to empty is refused as a lock-only path ──────

// TestFableEmptiedKeylessPathIsRefusedWithTheLockOnlyBody documents the body
// an operator meets after "Hash lock -> No hash lock" on a key-less path: the
// row says "empty", the refusal talks about "only a time lock".
//
// REWORDED IN THE FOLD (M-5). The reviewer's version pinned the body that was
// SHIPPED -- composerCopyRefuseLockOnly -- and named the defect in its own
// title: the row reads "Path 2: empty" and the refusal spoke about a time
// lock nobody had set. The fold chose the REFUSAL over silent removal (see
// composerCopyRefuseEmptyPath for why), so the test now asserts the body that
// names the actual state, and keeps the other arm under test beside it so the
// two states cannot collapse back onto one body.
func TestFableEmptiedKeylessPathIsRefusedWithABodyThatNamesIt(t *testing.T) {
	for _, tc := range []struct {
		what string
		path md.SpendPath
		row  string
		want string
	}{
		{"a key-less path whose hash was cleared: no key, no hash, no lock",
			md.SpendPath{}, "Path 2: empty", composerCopyRefuseEmptyPath()},
		{"a path carrying only a time lock",
			md.SpendPath{Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}},
			"Path 2: empty + 5 blocks", composerCopyRefuseLockOnly()},
	} {
		list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
			tc.path,
		}}
		if got := composerPathLine(list.Paths[1], 1); got != tc.row {
			t.Fatalf("%s: row reads %q, want %q", tc.what, got, tc.row)
		}
		_, err := md.ValidatePathList(list)
		if err == nil {
			t.Fatalf("%s: the path validated", tc.what)
		}
		body, ok := composerRefusalBody(err)
		if !ok {
			t.Fatalf("%s: no §8m body for %v", tc.what, err)
		}
		if body != tc.want {
			t.Errorf("%s: row %q is refused with %q, want %q", tc.what, tc.row, body, tc.want)
		}
	}
}

// ─── 5. The Keys editor opens on n = 1, k = 1 ────────────────────────────────

// TestFableSpecKeysEditorShowsTheKeySetInForce is journey C-1's class on the
// count pickers: opening "Keys" on a 2-of-3 path and leaving by the forward
// button must not rewrite the path.
func TestFableSpecKeysEditorShowsTheKeySetInForce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 3, 2)
		if got, ok := pumpUntil(frame, "Path 1: 2-of-3", 24); !ok {
			t.Fatalf("no path row.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // row 0 = Path 1
		if got, ok := pumpUntil(frame, "Remove path", 24); !ok {
			t.Fatalf("no path menu.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Keys
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Button3) // forward, on the row the picker opened on
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Button3) // forward again
		got, ok := pumpUntil(frame, "Remove path", 24)
		if !ok {
			t.Fatalf("the path menu never came back.\nLast frame: %q", got)
		}
		if !uiContains(got, "Path 1: 2-of-3") {
			t.Errorf("opening Keys on a 2-of-3 path and leaving by the forward button twice "+
				"rewrote it; the menu now reads: %q", got)
		}
	})
}

// ─── 6. A typed seed that seats nothing still asks Full vs Watch-only ────────

// TestFableSpecEngraveModeIsAskedOnlyForSeedDerivedSlots: §7f "For
// seed-derived slots: Full (seed + keys) or Watch-only (keys)". A seed typed
// and then not used for any slot must not raise the question; if it does,
// "Full" cuts no seed plate.
func TestFableSpecEngraveModeIsAskedOnlyForSeedDerivedSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 2, 1)
		fableDone(t, ctx, frame, 1)
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "mk1 stub (template)", 32)
		composerPageToEnd(t, ctx, frame)
		pumpUntil(frame, "Slot @0", 24)
		click(&ctx.Router, Down, Down) // Type a seed
		click(&ctx.Router, Button3)
		fableTypeSeed(t, ctx, frame)
		pumpUntil(frame, "Add a BIP-39 passphrase?", 48)
		click(&ctx.Router, Button3) // Skip
		// Back at @0 with the seed on offer; seat the two key records instead.
		if got, ok := pumpUntil(frame, "Slot @0", 24); !ok || !uiContains(got, "seed 1") {
			t.Fatalf("no seat prompt with the seed row.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // K1
		pumpUntil(frame, "Slot @1", 24)
		click(&ctx.Router, Button3) // K2 (K1 is used)
		if got, ok := pumpUntil(frame, "Key mapping", 24); !ok {
			t.Fatalf("no mapping review.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok := pumpUntil(frame, "mk1 stub (policy)", 32); !ok {
			t.Fatalf("no keyed stub screen.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Script:", 32); !ok {
			t.Fatalf("no consent.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Nothing outside this device", 32); !ok {
			t.Fatalf("no §8l.\nLast frame: %q", got)
		}
		fableHold(ctx, frame)
		if got, ok := pumpUntil(frame, "Which form?", 24); !ok {
			t.Fatalf("no form pick.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // The policy itself
		got, asked := pumpUntil(frame, "What to engrave?", 6)
		if asked {
			t.Errorf("Full vs Watch-only was asked though no slot is seed-derived: %q", got)
			click(&ctx.Router, Button3) // Full (seed + keys)
		}
		if got, ok := pumpUntil(frame, "Plates To Cut", 32); !ok {
			t.Fatalf("no census.\nLast frame: %q", got)
		} else {
			t.Logf("census after choosing Full with no seed-derived slot: %q", got)
			if uiContains(got, "ms1") {
				t.Errorf("a seed plate was planned for a seed that seats nothing: %q", got)
			}
		}
	})
}

// ─── 7. One seed registered twice is cut twice ───────────────────────────────

// TestFableSpecOneSeedTypedTwiceIsCutOnce: §7f "A seed that filled several
// slots is cut ONCE". Typing the same words at two "Type a seed" prompts
// registers two ids for one secret.
func TestFableSpecOneSeedTypedTwiceIsCutOnce(t *testing.T) {
	st := &composerState{reg: &seedRegistry{}, list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}},
	}}}
	for i := 0; i < 2; i++ {
		id, err := st.reg.add("seed", composerTestMnemonic(t), "", &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		seed, _ := st.reg.at(id)
		var fp [4]byte
		binary.BigEndian.PutUint32(fp[:], seed.MasterFP)
		st.sources = append(st.sources, composerSource{
			kind: composerSourceSeed, label: "seed", fingerprint: fp, fpPresent: true, seedID: id,
		})
	}
	st.assigned = make([]composerAssignment, 2)
	for i := range st.assigned {
		a, err := composerSeedDerive(st, uint8(i), i)
		if err != nil {
			t.Fatal(err)
		}
		st.assigned[i] = a
	}
	if st.assigned[0].account != 0 || st.assigned[1].account != 1 {
		t.Fatalf("accounts %d/%d: the two registrations were not seen as one master",
			st.assigned[0].account, st.assigned[1].account)
	}
	if st.assigned[0].xpub == st.assigned[1].xpub {
		t.Fatal("the two slots hold one key; the mapping review would refuse this")
	}
	cards, err := composerSecretCards(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		same := len(cards) == 2 && cards[0].strings[0] == cards[1].strings[0]
		t.Errorf("one seed at two slots planned %d ms1 plates (byte-identical: %v); §7f says a seed is cut ONCE",
			len(cards), same)
	}
	// Positive control: ONE registration at two slots is cut once.
	st.assigned[1].src = 0
	st.sources = st.sources[:1]
	cards, err = composerSecretCards(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Errorf("control: one registration at two slots planned %d ms1 plates", len(cards))
	}
}

// ─── 10. The key-less EXPERIMENTAL door: a tap does not pass it, Back removes the path ───

// TestFableKeylessDoorIsHoldOnlyAndBackReturnsToTheShape: §8 header, "every
// confirm-to-proceed screen is dismissed only by a tap on CONTINUE, and Back
// returns to the shape". A plain click on Button3 must leave the door up; Back
// must return to the path list with the half-made path gone.
func TestFableKeylessDoorIsHoldOnlyAndBackReturnsToTheShape(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		click(&ctx.Router, Button3) // Add a spend path
		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Down) // Keys -> A hash, no keys
		click(&ctx.Router, Button3)
		got, ok := pumpUntil(frame, "KEY-LESS PATH (EXPERIMENTAL)", 24)
		if !ok {
			t.Fatalf("§8a never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // a TAP, not a hold
		for i := 0; i < 4; i++ {
			got, _ = frame()
		}
		if !uiContains(got, "KEY-LESS PATH (EXPERIMENTAL)") {
			t.Errorf("a plain tap dismissed the EXPERIMENTAL door: %q", got)
		}
		click(&ctx.Router, Button1) // Back
		if got, ok = pumpUntil(frame, "Add a spend path", 24); !ok {
			t.Fatalf("Back did not return to the shape.\nLast frame: %q", got)
		}
		if uiContains(got, "Path 1:") {
			t.Errorf("Back at the EXPERIMENTAL door left a path behind: %q", got)
		}
		if !uiContains(got, "slots: 0") {
			t.Errorf("the list after Back does not read slots: 0: %q", got)
		}
	})
}

// ─── 11. Digit-pad bands: what each edge entry echoes ────────────────────────

// TestFableDigitPadBands records what the four pads make of their edges, and
// pins the two relative ceilings and the height ceiling.
func TestFableDigitPadBands(t *testing.T) {
	type row struct {
		pad, digits string
		echo        func(string) (string, bool)
	}
	rows := []row{
		{"blocks", "0", composerBlocksBandEcho},
		{"blocks", "00012", composerBlocksBandEcho},
		{"blocks", "65535", composerBlocksBandEcho},
		{"blocks", "65536", composerBlocksBandEcho},
		{"days", "0", composerDaysBandEcho},
		{"days", "036", composerDaysBandEcho},
		{"days", "388", composerDaysBandEcho},
		{"days", "389", composerDaysBandEcho},
		{"height", "0", composerHeightBandEcho},
		{"height", "499999999", composerHeightBandEcho},
		{"height", "500000000", composerHeightBandEcho},
		{"date", "20270230", composerDateBandEcho},
		{"date", "19851105", composerDateBandEcho},
		{"date", "20090103", composerDateBandEcho},
		{"date", "20380119", composerDateBandEcho},
	}
	for _, r := range rows {
		line, valid := r.echo(r.digits)
		t.Logf("%-6s %-9s valid=%-5v %q", r.pad, r.digits, valid, line)
	}
	if _, valid := composerBlocksBandEcho("65535"); !valid {
		t.Error("65535 blocks refused")
	}
	if _, valid := composerBlocksBandEcho("65536"); valid {
		t.Error("65536 blocks accepted")
	}
	if _, valid := composerDaysBandEcho("389"); valid {
		t.Error("389 days accepted")
	}
	if _, valid := composerHeightBandEcho("500000000"); valid {
		t.Error("height 500000000 accepted")
	}
	// The encoded operands at the edges pass md.Lock.Check.
	for _, l := range []md.Lock{
		{Kind: md.LockOlderBlocks, Value: 65535},
		{Kind: md.LockOlderUnits, Value: composerDaysToUnits(388)},
		{Kind: md.LockAfterHeight, Value: 499_999_999},
		{Kind: md.LockAfterTime, Value: composerDateFloorUnix},
		{Kind: md.LockAfterTime, Value: composerDateCeilingUnix},
	} {
		if err := l.Check(); err != nil {
			t.Errorf("%v/%d: %v", l.Kind, l.Value, err)
		}
	}
	if u := composerDaysToUnits(388); u > 65535 {
		t.Errorf("388 days = %d units, over the wire's 65535", u)
	}
}

// ─── 12. A testnet xpub in a key: record ─────────────────────────────────────

// TestFableTestnetXpubKeyRecord is lens 4's M-6, INVERTED by the fold (it is
// also lens 1's M-2 and lens 3's M-2 -- three lenses, one defect).
//
// As the reviewer ran it, it MEASURED: sysw.ParseKeyRecord accepted
// `[73c5da0a/48'/1'/0'/2']tpub...` because it checked depth and the last
// child index and never the version bytes; Classify returned ClassKey, so
// the door counted the record and seating offered it; the consent then
// printed mainnet bc1q addresses for material derived under coin type 1';
// and the mk1 card the composer minted carried the composer's
// Network: "mainnet" label while mk.Encode serialised the tpub's version
// bytes, so mk.Decode of that same card reported Network="testnet".
//
// It now asserts the refusal. §4f: complex-policy derivation is mainnet-only
// by construction. The host half landed FIRST (mnemonic-engrave 1cbecbfd,
// the Rust-primary rule) and the shared record_class_vectors table came back
// with the old `key-testnet-tpub-valid` row retired in place; the device rule
// is the convergence port, driven by that table in
// sysw/composer_records_test.go. This test is the same measurement from the
// device side, on a tpub derived here rather than one read out of a fixture.
func TestFableTestnetXpubKeyRecord(t *testing.T) {
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatal(err)
	}
	const h = 0x80000000
	tpub, fp, err := deriveAccountXpub(m, "", &chaincfg.TestNet3Params, []uint32{48 | h, 1 | h, 0 | h, 2 | h})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tpub, "tpub") {
		t.Fatalf("derived %s, not a tpub", tpub[:8])
	}
	var fpb [4]byte
	binary.BigEndian.PutUint32(fpb[:], fp)
	rec := composerRecord("key:", "[73c5da0a/48'/1'/0'/2']"+tpub)
	if _, err := sysw.ParseKeyRecord(rec); err == nil {
		t.Errorf("ParseKeyRecord admitted a testnet key (%s...): the consent would print "+
			"mainnet bc1q addresses for material derived under coin type 1h, and the mk1 "+
			"card minted from it decodes as Network=\"testnet\" under the composer's "+
			"mainnet label", tpub[:8])
	}
	if got := sysw.Classify(rec); got != sysw.ClassUnknown {
		t.Errorf("Classify(tpub record) = %v, want ClassUnknown -- the door must not count "+
			"it and seating must never offer it", got)
	}
	// THE CONTROL: the same origin and depth with a MAINNET xpub is still a
	// key, so the refusal is about the network and not about the shape.
	xpub, _, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, []uint32{48 | h, 0 | h, 0 | h, 2 | h})
	if err != nil {
		t.Fatal(err)
	}
	ok := composerRecord("key:", "[73c5da0a/48'/0'/0'/2']"+xpub)
	if _, err := sysw.ParseKeyRecord(ok); err != nil {
		t.Errorf("the mainnet control is refused too (%v), so the rule is not about the "+
			"network", err)
	}
	if got := sysw.Classify(ok); got != sysw.ClassKey {
		t.Errorf("Classify(mainnet control) = %v, want ClassKey", got)
	}
}

// ─── 8. Malformed record spellings are inert ─────────────────────────────────

// TestFableRecordSpellingsAreInert: a key: record with CRLF, a leading or
// trailing space, upper-case hex, or an upper-case prefix must not classify
// as a key (§6a: malformed records are inert on the device).
func TestFableRecordSpellingsAreInert(t *testing.T) {
	if got := sysw.Classify(composerTestKeyRecord); got != sysw.ClassKey {
		t.Fatalf("the well-formed fixture classifies as %v", got)
	}
	body := strings.TrimPrefix(composerTestKeyRecord, "key:")
	for _, tc := range []struct{ name, rec string }{
		{"CRLF", composerTestKeyRecord + "\r\n"},
		{"LF", composerTestKeyRecord + "\n"},
		{"leading space", " " + composerTestKeyRecord},
		{"trailing space", composerTestKeyRecord + " "},
		{"upper-case hex", "key:" + strings.ToUpper(body)},
		{"upper-case prefix", "KEY:" + body},
	} {
		got := sysw.Classify(tc.rec)
		t.Logf("%-17s -> %v", tc.name, got)
		if got == sysw.ClassKey {
			t.Errorf("%s: classified as a key", tc.name)
		}
	}
}

// ─── 9. Key -> seed -> Back: what reaches the planner ────────────────────────

// fableDecodeCards groups a run of mk1 chunk lines into cards the way
// composerCardSources does: a growing window that decodes is one card.
func fableDecodeCards(t *testing.T, lines []string) []mk.Card {
	t.Helper()
	var out []mk.Card
	for start := 0; start < len(lines); {
		decoded := false
		for end := start + 1; end <= len(lines); end++ {
			c, err := mk.Decode(lines[start:end])
			if err == nil {
				out = append(out, c)
				start = end
				decoded = true
				break
			}
		}
		if !decoded {
			t.Fatalf("mk1 lines from %d never decode: %q", start, lines[start:])
		}
	}
	return out
}

// TestFableKeyThenSeedThenBackReachesThePlannerAsShown seats @0 from a key:
// record, steps Back from @1 (releasing @0), re-seats @0 from a typed seed,
// seats @1 from the other record, takes form B watch-only, and cuts every
// plate. It asserts on the strings the engraver was handed: the md1 template
// and the two mk1 cards, decoded, must carry exactly what the mapping review
// showed.
func TestFableKeyThenSeedThenBackReachesThePlannerAsShown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil)
		captured := fableCaptureConsent(t)
		frame, _, _, quit := runUITouchRaster(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 2, 1)
		fableDone(t, ctx, frame, 1)
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("no stub screen.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// @0 = K1.
		if got, ok := pumpUntil(frame, "Slot @0", 24); !ok {
			t.Fatalf("no seat prompt.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		// At @1, Back: releases @0.
		if got, ok := pumpUntil(frame, "Slot @1", 24); !ok {
			t.Fatalf("no @1 prompt.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1)
		got, ok := pumpUntil(frame, "Slot @0", 24)
		if !ok {
			t.Fatalf("Back at @1 did not land on @0.\nLast frame: %q", got)
		}
		if !uiContains(got, "48h/0h/0h/2h") {
			t.Fatalf("K1 was not released back onto the list.\nFrame: %q", got)
		}
		// @0 = a typed seed.
		click(&ctx.Router, Down, Down) // K1, K2 -> Type a seed
		click(&ctx.Router, Button3)
		fableTypeSeed(t, ctx, frame)
		pumpUntil(frame, "Add a BIP-39 passphrase?", 48)
		click(&ctx.Router, Button3) // Skip
		if got, ok = pumpUntil(frame, "Slot @0", 24); !ok || !uiContains(got, "seed 1") {
			t.Fatalf("no seat prompt with the seed row.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down) // K1, K2 -> seed 1 (any slots)
		click(&ctx.Router, Button3)
		// @1 = K2.
		if got, ok = pumpUntil(frame, "Slot @1", 24); !ok {
			t.Fatalf("no @1 prompt.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // K1 -> K2
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Key mapping", 24); !ok {
			t.Fatalf("no mapping review.\nLast frame: %q", got)
		}
		for _, want := range []string{"@0: 73c5da0a m/48'/0'/0'/2'", "@1: 73c5da0a m/48'/0'/1'/2'"} {
			if !uiContains(got, want) {
				t.Errorf("mapping review lacks %q: %q", want, got)
			}
		}
		t.Logf("mapping review page 1: %q", got)
		// §8g: the seed at @0 and the record at @1 share one master inside one
		// 1-of-2 path -- one person can satisfy it. The body is on a later page.
		if got, ok = composerPageUntil(t, ctx, frame, "SAME SEED, SAME PATH", 6); !ok {
			t.Errorf("§8g never drew on the mapping review for two slots of one master in one path.\nLast page: %q", got)
		} else {
			t.Logf("§8g page: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok = pumpUntil(frame, "mk1 stub (policy)", 32); !ok {
			t.Fatalf("no keyed stub screen.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok = pumpUntil(frame, "Script:", 32); !ok {
			t.Fatalf("no consent.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok = pumpUntil(frame, "Nothing outside this device", 32); !ok {
			t.Fatalf("no §8l.\nLast frame: %q", got)
		}
		fableHold(ctx, frame)
		if got, ok = pumpUntil(frame, "Which form?", 24); !ok {
			t.Fatalf("no form pick.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // The policy itself -> Template plus key cards
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "What to engrave?", 24); !ok {
			t.Fatalf("no engrave-mode pick.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // Full -> Watch-only
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Plates To Cut", 32); !ok {
			t.Fatalf("no census.\nLast frame: %q", got)
		}
		for _, want := range []string{"md1 template", "mk1 key @0", "mk1 key @1"} {
			if !uiContains(got, want) {
				t.Errorf("census lacks %q: %q", want, got)
			}
		}
		if uiContains(got, "ms1") {
			t.Errorf("watch-only census plans a seed plate: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		plates := 0
		for {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3)
			frame()
			engraveOnePlate(t, ctx, frame, e)
			plates++
			if plates > 12 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		if plates == 0 {
			t.Fatal("no plate was cut")
		}
		t.Logf("%d plates cut, %d engraved texts recorded", plates, len(p.engraved))

		var md1Lines, mk1Lines []string
		for _, text := range p.engraved {
			for _, l := range strings.Split(text, "\n") {
				switch {
				case strings.HasPrefix(l, "md1"):
					md1Lines = append(md1Lines, l)
				case strings.HasPrefix(l, "mk1"):
					mk1Lines = append(mk1Lines, l)
				default:
					t.Errorf("an engraved line is neither md1 nor mk1: %q", l)
				}
			}
		}
		// The template on steel.
		_, keys, err := md.ExpandWalletPolicyChunks(md1Lines)
		if err != nil {
			t.Fatalf("the engraved md1 does not expand: %v (%q)", err, md1Lines)
		}
		if len(keys) != 2 {
			t.Fatalf("engraved template has %d slots", len(keys))
		}
		for i, wantPath := range []string{"m/48h/0h/0h/2h", "m/48h/0h/1h/2h"} {
			k := keys[i]
			if k.XpubPresent {
				t.Errorf("template slot @%d carries an xpub", i)
			}
			if !k.FingerprintPresent || hex.EncodeToString(k.Fingerprint[:]) != "73c5da0a" {
				t.Errorf("template slot @%d fingerprint %x present=%v", i, k.Fingerprint, k.FingerprintPresent)
			}
			if k.OriginPath.String() != wantPath {
				t.Errorf("template slot @%d origin %s, want %s", i, k.OriginPath, wantPath)
			}
		}
		// The cards on steel.
		cards := fableDecodeCards(t, mk1Lines)
		if len(cards) != 2 {
			t.Fatalf("%d cards engraved, want 2", len(cards))
		}
		tstub, _ := md.FormAwareStubChunks(md1Lines)
		kstub, _ := md.FormAwareStubChunks(*captured)
		// mk.Decode renders hardening as `h`; the wire carries no notation.
		for i, want := range []struct{ path, xpub string }{
			{"m/48h/0h/0h/2h", composerTestXpubA},
			{"m/48h/0h/1h/2h", composerTestXpubB},
		} {
			c := cards[i]
			if c.Path != want.path || c.Xpub != want.xpub || c.Fingerprint != "73c5da0a" {
				t.Errorf("card %d: path %s fp %s xpub %s; want %s 73c5da0a %s",
					i, c.Path, c.Fingerprint, c.Xpub, want.path, want.xpub)
			}
			hasT, hasK := false, false
			for _, s := range c.Stubs {
				hasT = hasT || s == tstub
				hasK = hasK || s == kstub
			}
			if !hasT || !hasK {
				t.Errorf("card %d stubs %x lack template %x / policy %x", i, c.Stubs, tstub, kstub)
			}
		}
		// The consent chunks (the keyed policy) hold the same two keys.
		_, ckeys, err := md.ExpandWalletPolicyChunks(*captured)
		if err != nil || len(ckeys) != 2 {
			t.Fatalf("consent chunks: %v, %d keys", err, len(ckeys))
		}
		for i, xpub := range []string{composerTestXpubA, composerTestXpubB} {
			cc, pk, _, err := decodeXpubBytes(xpub)
			if err != nil {
				t.Fatal(err)
			}
			var b [65]byte
			copy(b[0:32], cc[:])
			copy(b[32:65], pk[:])
			if !ckeys[i].XpubPresent || ckeys[i].Xpub != b {
				t.Errorf("consent slot @%d does not carry %s", i, xpub[:12])
			}
		}
	})
}

// ═══ ADDED IN THE FOLD, NOT PART OF APPENDIX A ══════════════════════════════
//
// Everything above is lens 4's file as the reviewer ran it. What follows was
// written while folding its findings, for the halves the adopted walks did
// not cover.

// TestFableSpecBackOnThePassphraseQuestionUnRegistersTheSeed is the OTHER
// half of lens 4 I-2.
//
// The adopted walk covers Back on the KEYBOARD ("I mis-typed" -> re-ask).
// Back on the QUESTION means "not this seed", and st.reg.add runs BEFORE the
// question -- deliberately, so the deferred scrub owns the words from the
// moment they are entered -- so declining there has to undo the registration
// or the bare seed stays a source and is offered for seating.
func TestFableSpecBackOnThePassphraseQuestionUnRegistersTheSeed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		fableStartWsh(t, ctx, frame)
		fableAddKeyedPath(t, ctx, frame, 0, 2, 1)
		fableDone(t, ctx, frame, 1)
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3) // Sorted
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("no stub screen.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Slot @0", 24); !ok {
			t.Fatalf("no seat prompt.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down) // K1, K2 -> Type a seed
		click(&ctx.Router, Button3)
		fableTypeSeed(t, ctx, frame)
		if got, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 48); !ok {
			t.Fatalf("no passphrase question.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1) // Back, ON THE QUESTION: not this seed

		if got, ok := pumpUntil(frame, "choose a key", 24); !ok {
			t.Fatalf("the seat prompt did not come back.\nLast frame: %q", got)
		}
		// THE OBSERVABLE IS THE NEXT SEED'S LABEL, not the seat prompt: a
		// declined source is never appended to st.sources either way, so the
		// prompt looks the same whether or not the REGISTRY kept the seed.
		// What the stale entry changes is everything keyed on the registry --
		// the per-seed label (st.reg.count()+1), usesPassphrase(), which
		// decides the engrave-mode label, and the restore document's
		// passphrase facts. Typing a second seed and reading the title it is
		// given is the cheapest of those to drive, and it fails if and only if
		// the first registration survived.
		click(&ctx.Router, Down, Down) // K1, K2 -> Type a seed
		click(&ctx.Router, Button3)
		fableTypeSeed(t, ctx, frame)
		got, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 48)
		if !ok {
			t.Fatalf("no passphrase question on the second seed.\nLast frame: %q", got)
		}
		if !uiContains(got, "Passphrase seed 1") {
			t.Errorf("Back on the passphrase question left the bare seed in the registry: "+
				"the NEXT seed is labelled from st.reg.count()+1 and the question drew "+
				"%q, so usesPassphrase(), the engrave-mode label and the restore "+
				"document's passphrase facts all still count a seed the operator "+
				"declined", got)
		}
	})
}

// TestFableSeedRegistryDiscardLastOnlyDropsTheLastEntry pins discardLast's
// bound, because seedIDs are INDICES: dropping an interior entry would
// renumber every id after it, and callers hold those ids.
func TestFableSeedRegistryDiscardLastOnlyDropsTheLastEntry(t *testing.T) {
	reg := &seedRegistry{}
	m, err := bip39.ParseMnemonic(
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := bip39.ParseMnemonic("zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong")
	if err != nil {
		t.Fatal(err)
	}
	id0, err := reg.add("a", m, "", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	id1, err := reg.add("b", m2, "", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	if reg.discardLast(id0) {
		t.Error("discardLast dropped an INTERIOR entry, which renumbers every id after it")
	}
	if reg.count() != 2 {
		t.Fatalf("count = %d after a refused discard, want 2", reg.count())
	}
	if !reg.discardLast(id1) {
		t.Fatal("discardLast refused the last entry")
	}
	if reg.count() != 1 {
		t.Fatalf("count = %d, want 1", reg.count())
	}
	// And the words it dropped are zeroed, not merely unreferenced.
	dropped := reg.seeds[:2][1]
	for _, w := range dropped.Mnemonic {
		if w != 0 {
			t.Fatalf("discardLast left a live word in the dropped entry: %v", dropped.Mnemonic)
		}
	}
	if s, ok := reg.at(id0); !ok || s.Label != "a" {
		t.Errorf("the surviving entry moved: %+v ok=%v", s, ok)
	}
}
