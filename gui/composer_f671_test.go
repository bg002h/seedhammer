package gui

import (
	"image"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/gui/op"
	"seedhammer.com/md"
)

// F-671: a Liana-key kofn-recovery wallet with ONE seed seated into every slot
// is a wallet Liana v15.0 refuses ("derived from the same origin as another
// key present in the same spending path"). The Key-mapping review said so
// (§8g, "Liana will refuse it"), and the consent a few screens later still
// said "Liana (as of v15.0) and Bitcoin Core import this form". The consent is
// the promise the operator signs; its Liana claim must hold for the wallet AS
// SEATED.
//
// DRIVEN BY TOUCH AND THE NAV SLOTS ONLY: every row is taken by tapping the
// row's own hit area and every advance by tapping a nav button's target
// (tapNavSlot asserts the target is there). No Up/Down ButtonEvent, which the
// SH2 cannot produce (W-2).

// f671RowTargets lists the tappable ROW targets on the drawn frame, top to
// bottom: the Clickables with no Button, which is what ChoiceScreen children
// and composerPickScreen rows register. Nav buttons carry a Button and are
// excluded.
func f671RowTargets(ctx *Context, d *op.Drawer) []image.Point {
	dims := ctx.Platform.DisplaySize()
	var out []image.Point
	seen := map[*Clickable]bool{}
	for y := 0; y < dims.Y; y++ {
		for _, x := range []int{dims.X / 4, dims.X / 2} {
			tag, _, hit := d.Hit(image.Pt(x, y))
			if !hit {
				continue
			}
			c, ok := tag.(*Clickable)
			if !ok || c.Button != 0 || seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, image.Pt(x, y+2))
		}
	}
	return out
}

// f671TapRow taps row j of the frame just drawn, then takes it with the
// forward nav button.
func f671TapRow(t *testing.T, ctx *Context, frame func() (string, bool), drawer func() *op.Drawer, j int) {
	t.Helper()
	rows := f671RowTargets(ctx, drawer())
	if j >= len(rows) {
		t.Fatalf("row %d asked for, the frame has %d tappable rows", j, len(rows))
	}
	tap(&ctx.Router, drawer(), rows[j])
	frame()
	tapNavSlot(t, ctx, drawer(), Button3)
}

// f671HasNav reports whether the frame just drawn has a target bound to b.
func f671HasNav(ctx *Context, d *op.Drawer, b Button) bool {
	dims := ctx.Platform.DisplaySize()
	for y := 0; y < dims.Y; y++ {
		tag, _, hit := d.Hit(image.Pt(dims.X-4, y))
		if !hit {
			continue
		}
		if c, ok := tag.(*Clickable); ok && (c.Button == b || c.AltButton == b) {
			return true
		}
	}
	return false
}

// f671ReadToEnd pages a paged read screen by its page nav target, collecting
// every page's text, then takes it with the forward target once that target
// is drawn (composerReadScreen withholds it until the last page was laid out).
func f671ReadToEnd(t *testing.T, ctx *Context, frame func() (string, bool), drawer func() *op.Drawer, first string) string {
	t.Helper()
	pages := []string{first}
	for i := 0; i < 16; i++ {
		if f671HasNav(ctx, drawer(), Button3) && i > 0 {
			break
		}
		if !f671HasNav(ctx, drawer(), Button2) {
			break
		}
		tapNavSlot(t, ctx, drawer(), Button2)
		c, ok := frame()
		if !ok {
			t.Fatal("the flow ended while paging")
		}
		pages = append(pages, c)
	}
	if !f671HasNav(ctx, drawer(), Button3) {
		t.Fatalf("no forward target after paging.\nPages: %q", pages)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	return strings.Join(pages, "\n")
}

// f671ConsentForSeedEverywhere walks tr -> kofn-recovery -> the Liana key ->
// the one loaded seed into all four slots, and returns the mapping review's
// and the consent's text.
func f671ConsentForSeedEverywhere(t *testing.T) (mapping, consent string) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith(nil, []string{composerTestMnemonicRecord})
		frame, drawer, quit := runUITouch(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		if got, ok := pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("no wrapper picker.\nLast frame: %q", got)
		}
		f671TapRow(t, ctx, frame, drawer, 0) // Taproot (tr)
		if got, ok := pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("no preset picker.\nLast frame: %q", got)
		}
		// Build my own paths, plain-multisig, simple-timelocked-inheritance,
		// kofn-recovery.
		f671TapRow(t, ctx, frame, drawer, 3)
		got, ok := pumpUntil(frame, "Add a spend path", 24)
		if !ok || !uiContains(got, "Path 1: 2-of-3") {
			t.Fatalf("no kofn-recovery path list.\nLast frame: %q", got)
		}
		// Path 1, Path 2, Add a spend path, Change the script, Done.
		f671TapRow(t, ctx, frame, drawer, 4)
		if got, ok = pumpUntil(frame, "Which key path?", 24); !ok {
			t.Fatalf("no key-path choice.\nLast frame: %q", got)
		}
		f671TapRow(t, ctx, frame, drawer, 1) // Liana key
		if got, ok = pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("no stub screen.\nLast frame: %q", got)
		}
		f671ReadToEnd(t, ctx, frame, drawer, got)

		// With no key records loaded the seed is not yet a source: the
		// seating step asks, and "Type a seed" opens the seed picker, whose
		// first row is the payload's own seed.
		if got, ok = pumpUntil(frame, "Seat keys into this template?", 32); !ok {
			t.Fatalf("no seating question.\nLast frame: %q", got)
		}
		f671TapRow(t, ctx, frame, drawer, 1) // Type a seed
		if got, ok = pumpUntil(frame, "Where from?", 32); !ok {
			t.Fatalf("no seed-source picker.\nLast frame: %q", got)
		}
		f671TapRow(t, ctx, frame, drawer, 0) // FROM PAYLOAD
		if got, ok = pumpUntil(frame, "Source:", 32); !ok {
			t.Fatalf("no source acceptance.\nLast frame: %q", got)
		}
		tapNavSlot(t, ctx, drawer(), Button3)
		if got, ok = pumpUntil(frame, "Add a BIP-39 passphrase?", 32); !ok {
			t.Fatalf("no passphrase question.\nLast frame: %q", got)
		}
		f671TapRow(t, ctx, frame, drawer, 0) // Skip

		for slot := 0; slot < 4; slot++ {
			want := "Slot @" + string(rune('0'+slot))
			if got, ok = pumpUntil(frame, want, 32); !ok {
				t.Fatalf("%s was never asked.\nLast frame: %q", want, got)
			}
			f671TapRow(t, ctx, frame, drawer, 0) // the loaded seed
		}
		if got, ok = pumpUntil(frame, "cannot confirm", 48); !ok {
			t.Fatalf("no mapping review.\nLast frame: %q", got)
		}
		mapping = f671ReadToEnd(t, ctx, frame, drawer, got)
		if got, ok = pumpUntil(frame, "mk1 stub (policy)", 32); !ok {
			t.Fatalf("no keyed stub screen.\nLast frame: %q", got)
		}
		f671ReadToEnd(t, ctx, frame, drawer, got)
		if got, ok = pumpUntil(frame, "Script:", 32); !ok {
			t.Fatalf("no consent.\nLast frame: %q", got)
		}
		consent = f671ReadToEnd(t, ctx, frame, drawer, got)
		if _, ok = pumpUntil(frame, "Nothing outside this device", 32); !ok {
			t.Fatalf("the consent did not advance to §8l.\nConsent: %q", consent)
		}
	})
	return mapping, consent
}

// TestComposerConsentDoesNotClaimLianaImportsASameSeedWallet is F-671.
//
// Mutation: make composerLianaRefusesSeating return false and the consent
// assertions fail (the Liana claim is back); the mapping assertion is the
// control that the walk reached the seating the claim is about.
func TestComposerConsentDoesNotClaimLianaImportsASameSeedWallet(t *testing.T) {
	mapping, consent := f671ConsentForSeedEverywhere(t)

	m := normalizeDrawn(mapping)
	if !strings.Contains(m, normalizeDrawn("SAME SEED, SAME PATH")) ||
		!strings.Contains(m, normalizeDrawn("Liana will refuse it.")) {
		t.Fatalf("the walk did not produce the same-seed seating: the mapping review "+
			"carries no §8g warning.\nMapping: %q", mapping)
	}

	c := normalizeDrawn(consent)
	if !strings.Contains(c, normalizeDrawn("KEY PATH: NONE (LIANA KEY)")) {
		t.Fatalf("the consent names no kind-1 key path.\nConsent: %q", consent)
	}
	for _, claim := range []string{
		"Liana (as of v15.0) and Bitcoin Core import this form",
		"Liana (v15.0) and Bitcoin Core import",
	} {
		if strings.Contains(c, normalizeDrawn(claim)) {
			t.Errorf("the consent still says %q for a wallet the mapping review says "+
				"Liana will refuse.\nConsent: %q", claim, consent)
		}
	}
	if !strings.Contains(c, normalizeDrawn(composerCopyLianaKeyPathSameSeed())) {
		t.Errorf("the consent does not state the refusal (§8y's same-seed body).\nConsent: %q", consent)
	}
}

// TestComposerKeyPathChoiceDoesNotClaimLianaImportsASameSeedSeating is F-671's
// second print site. The key-path choice sits before seating, so on first
// entry nothing is seated and its Liana row stands; but Back from the stub,
// the mapping review or the consent returns through it with the seating kept,
// and then its "Liana ... import it" is the same false claim.
//
// Mutation: make composerLianaRefusesSeating return false and the seated case
// fails; the unseated case is the control that the row still names Liana
// where nothing refutes it.
func TestComposerKeyPathChoiceDoesNotClaimLianaImportsASameSeedSeating(t *testing.T) {
	fp := [4]byte{0x73, 0xc5, 0xda, 0x0a}
	list := composerTrPreset(t, "kofn-recovery")
	unseated := &composerState{list: list, assigned: []composerAssignment{{src: -1}, {src: -1}, {src: -1}, {src: -1}}}
	seated := &composerState{list: list, assigned: []composerAssignment{
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 0, fingerprint: fp, fpPresent: true},
	}}
	seated.sources = []composerSource{{kind: composerSourceSeed, fingerprint: fp, fpPresent: true}}

	claim := normalizeDrawn("Liana (v15.0) and Bitcoin Core import it")
	if got := composerUnspendableRows(unseated); !strings.Contains(normalizeDrawn(got[1]), claim) {
		t.Errorf("control: with nothing seated the Liana row no longer names Liana: %q", got[1])
	}
	got := composerUnspendableRows(seated)
	if strings.Contains(normalizeDrawn(got[1]), claim) {
		t.Errorf("the Liana row claims Liana imports a seating the mapping review says "+
			"Liana will refuse: %q", got[1])
	}
	if got[1] != composerCopyUnspendableRowLianaSameSeed() {
		t.Errorf("the Liana row for a same-seed seating is %q, want §8y's same-seed row", got[1])
	}
	if got[0] != composerCopyUnspendableRowNUMS() {
		t.Errorf("the NUMS row changed with the seating: %q", got[0])
	}
}

// TestComposerKeyPathChoiceSameSeedRowFitsTheFirstPage: the same-seed Liana
// row is longer than the one it replaces, and the key-path screen's gate is
// that BOTH rows are on the first page (TestComposerUnspendableScreenFirstPage
// measures the unseated form). Measured with the picker's own layout.
func TestComposerKeyPathChoiceSameSeedRowFitsTheFirstPage(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	rows := []string{composerCopyUnspendableRowNUMS(), composerCopyUnspendableRowLianaSameSeed()}
	_, shown, _ := composerPickPageLayout(ctx, &descriptorTheme, sh2DisplaySize, composerCopyUnspendableLead(), rows, -1)
	if want := composerPickRowBase + len(rows); shown < want {
		t.Errorf("the key-path page draws %d of %d lines with the same-seed row; a row is off the first page", shown, want)
	}
}

// TestComposerNoConsentClaimsLianaImportsANUMSWallet: F-671's NUMS half. Liana
// v15.0 refuses every NUMS-keyed composer wallet (e2e-live-site-wallets.md,
// all four NUMS rows), so no consent may say Liana imports one -- seated with
// one seed twice or not.
func TestComposerNoConsentClaimsLianaImportsANUMSWallet(t *testing.T) {
	chunks := lianaKofnChunks(t, md.UnspendableNums)
	for _, refuses := range []bool{false, true} {
		lines, err := composerConsentLinesFor(chunks, nil, 0, refuses)
		if err != nil {
			t.Fatal(err)
		}
		c := normalizeDrawn(strings.Join(lines, "\n"))
		if !strings.Contains(c, normalizeDrawn("KEY PATH: NONE (NUMS)")) {
			t.Fatalf("control: the NUMS twin's consent names no NUMS key path:\n%s", strings.Join(lines, "\n"))
		}
		for _, claim := range []string{"Liana (as of v15.0) and Bitcoin Core import",
			"Liana (v15.0) and Bitcoin Core import", "Liana imports"} {
			if strings.Contains(c, normalizeDrawn(claim)) {
				t.Errorf("a NUMS consent (same seed %v) says %q", refuses, claim)
			}
		}
	}
}
