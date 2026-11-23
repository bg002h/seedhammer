package gui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/engrave"
	"seedhammer.com/gui/op"
	"seedhammer.com/sysw"
)

// The kit door (SPEC_systemwide_payloads.md section 14e, plan S2 phase SH-2).
//
// FIXTURES. The plan reads `me sysw pack` blobs for the load-time rows; these
// rows build the UNSEALED container in Go (sysw.Header.Encode "exists for
// tests") and TestKitFixturePackerMatchesMe pins that packer byte for byte
// against containers `me` did write (chain-codex32, chain-text). For kit-2of3
// and kit-with-ms1, me at engrave 73209a1e writes the same bytes (architect
// 1122-late); their chain_payloads entries come with SH-5's me-packed kit blob.

// kitVectorFile is the vendored S1 vectors, kit section.
func kitVectorRecords(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "sysw", "testdata", "sysw_mr_v1_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		KitVectors []struct {
			Name    string   `json:"name"`
			Records []string `json:"records"`
		} `json:"kit_vectors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, v := range f.KitVectors {
		if v.Name == name {
			return v.Records
		}
	}
	t.Fatalf("no kit vector %q", name)
	return nil
}

// plaintextContainer is the unsealed systemwide container `me sysw pack
// --no-passphrase` writes: the header, then the public records joined by LF.
func plaintextContainer(records ...string) []byte {
	pub := []byte(strings.Join(records, "\n"))
	h := sysw.Header{PubLen: uint32(len(pub))}.Encode()
	return append(h[:], pub...)
}

func plaintextRegion(t *testing.T, records ...string) sysw.Reader {
	t.Helper()
	region := make([]byte, sysw.RegionLen)
	for i := range region {
		region[i] = 0xFF
	}
	copy(region, plaintextContainer(records...))
	f := filepath.Join(t.TempDir(), "region.bin")
	if err := os.WriteFile(f, region, 0o600); err != nil {
		t.Fatal(err)
	}
	return sysw.FileReader{Path: f}
}

func TestKitFixturePackerMatchesMe(t *testing.T) {
	for _, name := range []string{"chain-codex32", "chain-text"} {
		p := chainPayloadNamed(t, name)
		want, err := hex.DecodeString(p.Blob)
		if err != nil {
			t.Fatal(err)
		}
		if got := plaintextContainer(p.Records...); string(got) != string(want[:len(got)]) || len(got) != p.Bytes {
			t.Errorf("%s: the Go packer differs from the me-packed container", name)
		}
	}
}

// kitPlatform is a platform with the sysw region reader set, at the real panel.
func kitPlatform(r sysw.Reader) *testPlatform {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = r
	return p
}

func kitSentences(t *testing.T, content string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !uiContains(content, w) {
			t.Errorf("screen lacks %q; got %q", w, content)
		}
	}
}

// indexOfAll returns the position of each needle in the frame text, -1 absent.
func orderOf(content string, needles ...string) []int {
	txt := strings.ReplaceAll(strings.ToLower(content), " ", "")
	out := make([]int, len(needles))
	for i, n := range needles {
		out[i] = strings.Index(txt, strings.ReplaceAll(strings.ToLower(n), " ", ""))
	}
	return out
}

func TestKitMenuOffersEngraveKit(t *testing.T) {
	for _, tc := range []struct{ vec, lead string }{
		{"kit-2of3", "Engrave kit now (6 cards, 6 plates)?"},
		{"kit-single", "Engrave kit now (1 card, 1 plate)?"},
	} {
		// From a session (the second opening, from the carousel).
		p := kitPlatform(nil)
		ctx := NewContext(p)
		ctx.sysw = sessionWith(kitVectorRecords(t, tc.vec)...)
		frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
		content, ok := pumpUntil(frame, "ENGRAVE KIT", 32)
		if !ok {
			t.Fatalf("%s: no ENGRAVE KIT; got %q", tc.vec, content)
		}
		kitSentences(t, content, "Load Payload", tc.lead, "LOAD AGAIN", "UNLOAD")
		if uiContains(content, "It holds") {
			t.Errorf("%s: the menu kept today's lead: %q", tc.vec, content)
		}
		ord := orderOf(content, "ENGRAVE KIT", "LOAD AGAIN", "UNLOAD")
		if !(ord[0] >= 0 && ord[0] < ord[1] && ord[1] < ord[2]) {
			t.Errorf("%s: choices out of order %v in %q", tc.vec, ord, content)
		}
		click(&ctx.Router, Button3) // ENGRAVE KIT
		list, ok := pumpUntil(frame, "Start at:", 32)
		if !ok {
			t.Fatalf("%s: ENGRAVE KIT did not open the kit list; got %q", tc.vec, list)
		}
		kitSentences(t, list, "Engrave Kit")
		_ = drawer
		quit()
	}

	// At boot: the load flow, then the menu, exactly as gui.go does.
	p := kitPlatform(plaintextRegion(t, kitVectorRecords(t, "kit-2of3")...))
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() {
		if syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true) {
			syswPayloadMenu(ctx, &descriptorTheme)
		}
	})
	defer quit()
	if c, ok := pumpUntil(frame, "Load it?", 32); !ok {
		t.Fatalf("no boot prompt; got %q", c)
	}
	click(&ctx.Router, Button3) // LOAD
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("no digest; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	content, ok := pumpUntil(frame, "Engrave kit now (6 cards, 6 plates)?", 64)
	if !ok {
		t.Fatalf("the boot route did not reach the kit menu; got %q", content)
	}
}

func TestKitMenuRefusedShowsReasonThenMenu(t *testing.T) {
	const tail = "Nothing from this payload can be engraved as a kit. Write the kit payload again."
	for _, tc := range []struct{ vec, sentence string }{
		{"kit-text", "Record 3 of this payload is not an md1, mk1 or mr1 string (it is: free text), so this payload is not a kit."},
		{"kit-upper", "Record 1 of this payload is an mr1 string with capital letters; the mr1 strings of a kit are all lowercase, as mr prints them."},
		{"kit-mk1-split", "The mk1 card that begins mk1qppc9 is split: record 4 of this payload sits between its strings but is not part of it. Keep each card's strings together."},
	} {
		ctx := NewContext(kitPlatform(nil))
		ctx.sysw = sessionWith(kitVectorRecords(t, tc.vec)...)
		frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
		content, ok := pumpUntil(frame, "Kit Refused", 32)
		if !ok {
			t.Fatalf("%s: no Kit Refused screen; got %q", tc.vec, content)
		}
		kitSentences(t, content, tc.sentence, tail)
		if o := orderOf(content, tc.sentence, tail); !(o[0] >= 0 && o[0] < o[1]) {
			t.Errorf("%s: the sentence must precede the tail: %v", tc.vec, o)
		}
		click(&ctx.Router, Button3) // dismiss
		menu, ok := pumpUntil(frame, "It holds", 32)
		if !ok {
			t.Fatalf("%s: today's menu did not follow; got %q", tc.vec, menu)
		}
		if uiContains(menu, "ENGRAVE KIT") {
			t.Errorf("%s: a refused kit offers ENGRAVE KIT", tc.vec)
		}
		quit()
	}
}

// GUARD: a payload with no mr1 candidate sees today's menu, first and second
// opening, with neither the offer nor the refusal.
func TestKitMenuNoMr1Unchanged(t *testing.T) {
	sess := sessionWith(kitVectorRecords(t, "no-mr1")...)
	for opening := 1; opening <= 2; opening++ {
		ctx := NewContext(kitPlatform(nil))
		ctx.sysw = sess
		frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
		content, ok := pumpUntil(frame, "It holds", 32)
		if !ok {
			t.Fatalf("opening %d: no menu; got %q", opening, content)
		}
		if uiContains(content, "ENGRAVE KIT") || uiContains(content, "Kit Refused") || uiContains(content, "Engrave kit now") {
			t.Errorf("opening %d: a payload with no mr1 shows the kit surface: %q", opening, content)
		}
		quit()
	}
}

// kitWithMs1 is kit-single's mr1, then a codex32 secret.
func kitWithMs1(t *testing.T) []string {
	return append(append([]string{}, kitVectorRecords(t, "kit-single")...), chainPayloadNamed(t, "chain-codex32").Records...)
}

func TestKitRefusedAfterWarnings(t *testing.T) {
	const sentence = "Record 2 of this payload is not an md1, mk1 or mr1 string (it is: codex32 secret), so this payload is not a kit."
	run := func(unload bool) (frames []string) {
		p := kitPlatform(plaintextRegion(t, kitWithMs1(t)...))
		ctx := NewContext(p)
		frame, drawer, quit := runUITouch(ctx, func() {
			if syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true) {
				syswPayloadMenu(ctx, &descriptorTheme)
			}
		})
		defer quit()
		pumpUntil(frame, "Load it?", 32)
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Compare this against", 32); ok {
			frames = append(frames, c)
		}
		tapNavSlot(t, ctx, drawer(), Button3)
		c, ok := pumpUntil(frame, "Payload Warnings", 32)
		if !ok {
			t.Fatalf("no Payload Warnings; got %q", c)
		}
		frames = append(frames, "WARN:"+c)
		tapNavSlot(t, ctx, drawer(), Button3) // acknowledge
		if c, ok = pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
			t.Fatalf("no KEEP / UNLOAD; got %q", c)
		}
		if unload {
			click(&ctx.Router, Down)
		}
		click(&ctx.Router, Button3)
		if unload {
			if c, ok = pumpUntil(frame, "BACK", 32); !ok {
				t.Fatalf("no unload confirmation; got %q", c)
			}
			click(&ctx.Router, Down, Button3) // UNLOAD, confirmed
			if c, ok = pumpUntil(frame, "Payload Unloaded", 32); !ok {
				t.Fatalf("no Payload Unloaded; got %q", c)
			}
			click(&ctx.Router, Button3)
		}
		for i := 0; i < 24; i++ {
			c, more := frame()
			if !more {
				break
			}
			frames = append(frames, c)
		}
		return frames
	}
	kept := run(false)
	joined := strings.Join(kept, "\n")
	if !uiContains(joined, "Kit Refused") || !uiContains(joined, sentence) {
		t.Fatalf("KEEP did not lead to Kit Refused: %q", joined)
	}
	if strings.Index(strings.ToLower(joined), "payloadwarnings") > strings.Index(strings.ToLower(joined), "kitrefused") {
		t.Error("Kit Refused came before the Payload Warnings screen")
	}
	if !uiContains(joined, "It holds") && !uiContains(joined, "Nothing from this payload") {
		t.Errorf("no menu after Kit Refused: %q", joined)
	}
	if un := strings.Join(run(true), "\n"); uiContains(un, "Kit Refused") {
		t.Errorf("UNLOAD still showed Kit Refused: %q", un)
	}
}

func TestKitBundleDoor(t *testing.T) {
	// Eligible: the kit list at once, no "Cards from where?".
	ctx := NewContext(kitPlatform(nil))
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
	frame, _, quit := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	content, ok := pumpUntil(frame, "Start at:", 32)
	if !ok || uiContains(content, "Cards from where") {
		t.Fatalf("eligible kit: no list, or the gather prompt showed; got %q", content)
	}
	quit()

	// Refused: Kit Refused, then the carousel (the flow returns).
	ctx = NewContext(kitPlatform(nil))
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-text")...)
	returned := false
	frame, _, quit = runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme); returned = true })
	if c, ok := pumpUntil(frame, "Kit Refused", 32); !ok {
		t.Fatalf("refused kit: no Kit Refused; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame()
	if !returned {
		t.Error("a refused kit did not return to the carousel")
	}
	quit()

	// No payload, and a payload with no mr1: the gather screen as today.
	for _, s := range []*syswSession{nil, sessionWith(kitVectorRecords(t, "no-mr1")...)} {
		ctx = NewContext(kitPlatform(nil))
		ctx.sysw = s
		frame, _, quit = runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		c, _ := frame()
		if uiContains(c, "Start at:") || uiContains(c, "Kit Refused") {
			t.Errorf("no kit offered, yet the kit door opened: %q", c)
		}
		quit()
	}
}

func TestKitListEntries(t *testing.T) {
	ctx := NewContext(kitPlatform(nil))
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
	frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	pumpUntil(frame, "ENGRAVE KIT", 32)
	click(&ctx.Router, Button3)
	content, ok := pumpUntil(frame, "Start at:", 32)
	if !ok {
		t.Fatalf("no list; got %q", content)
	}
	kitSentences(t, content, "Engrave Kit", "6 cards, 6 plates. Start at:",
		"Card 1 of 6: mr1, begins mr1lg2c0, ends 06uf", "Card 2 of 6: mk1, begins mk1qppc9, ends 0uvz")
	if uiContains(content, "(cut)") {
		t.Errorf("a fresh list marks a card cut: %q", content)
	}
	_ = drawer
}

// kitHarness records the layouts the run asks for and restores the seams.
type kitLayoutCall struct {
	strs          []string
	title, footer string
}

func kitSeams(t *testing.T, onePerPlate bool, failLayout bool) *[]kitLayoutCall {
	t.Helper()
	var calls []kitLayoutCall
	origLayout, origPlates := kitPlateLayout, kitCardPlates
	kitPlateLayout = func(pl Platform, strs []string, title, footer string) ([]string, []Plate, error) {
		calls = append(calls, kitLayoutCall{append([]string{}, strs...), title, footer})
		if failLayout {
			return nil, nil, fmt.Errorf("test: no layout")
		}
		return origLayout(pl, strs, title, footer)
	}
	if onePerPlate {
		kitCardPlates = func(_ engrave.Params, strs []string) [][]string {
			var out [][]string
			for i := range strs {
				out = append(out, strs[i:i+1:i+1])
			}
			return out
		}
	}
	t.Cleanup(func() { kitPlateLayout, kitCardPlates = origLayout, origPlates })
	return &calls
}

// kitSelectCard taps list entry n (1-based) on the current frame.
func kitSelectCard(t *testing.T, ctx *Context, frame func() (string, bool), drawer func() *op.Drawer, n int) {
	t.Helper()
	frame()
	pts := plateHitPoints(ctx, drawer())
	if len(pts) < n {
		t.Fatalf("only %d list entries have touch targets, want entry %d", len(pts), n)
	}
	tap(&ctx.Router, drawer(), pts[n-1])
	frame()
}

// kitCut chooses TEXT ONLY and drives one real engrave to its end, as the other
// bundle walks do (composerEngraveOnePlate's loop, which this reuses in shape).
func kitCut(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) {
	t.Helper()
	click(&ctx.Router, Button3) // TEXT ONLY
	composerEngraveOnePlate(t, ctx, frame, e)
}

func kitRunPlatform() (*engravedAwarePlatform, *testEngraver) {
	e := newEngraver()
	p := newEngravedAwarePlatform()
	p.engraver = e
	p.display = sh2DisplaySize
	return p, e
}

func TestKitDoorRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := kitSeams(t, false, false)
		p, e := kitRunPlatform()
		ctx := NewContext(p)
		ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
		frame, drawer, quit := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()
		if c, ok := pumpUntil(frame, "Start at:", 32); !ok {
			t.Fatalf("no list; got %q", c)
		}
		// Start at card 2: Down once, OK.
		kitSelectCard(t, ctx, frame, drawer, 2)
		click(&ctx.Router, Button3)
		for k := 2; k <= 6; k++ {
			title := fmt.Sprintf("Card %d of 6 | Plate 1 of 1", k)
			c, ok := pumpUntil(frame, title, 64)
			if !ok {
				t.Fatalf("no choice screen %q; got %q", title, c)
			}
			kitSentences(t, c, "Choose engraving", "TEXT ONLY")
			if uiContains(c, "QR") {
				t.Errorf("the kit offers a QR choice: %q", c)
			}
			kitCut(t, ctx, frame, e)
		}
		c, ok := pumpUntil(frame, "Cards 2 to 6 of this kit are cut.", 64)
		if !ok {
			t.Fatalf("no end screen; got %q", c)
		}
		if uiContains(c, "ms1") {
			t.Errorf("the kit run shows the ms1 reminder: %q", c)
		}
		if len(*calls) != 5 {
			t.Fatalf("%d layouts for 5 plates", len(*calls))
		}
		for i, call := range *calls {
			if want := fmt.Sprintf("CARD %d/6", i+2); call.title != want || call.footer != "PLATE 1/1" {
				t.Errorf("plate %d marked %q / %q, want %q / %q", i+2, call.title, call.footer, want, "PLATE 1/1")
			}
		}
		click(&ctx.Router, Button3)
		if c, ok = pumpUntil(frame, "Start at:", 32); !ok {
			t.Fatalf("the list did not return; got %q", c)
		}
		_ = drawer
	})

	// Back at plate 1 returns to the list with no other screen; the highlight
	// is on the card the run stopped in.
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, true, false)
		p, _ := kitRunPlatform()
		ctx := NewContext(p)
		ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
		frame, drawer, quit := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()
		pumpUntil(frame, "Start at:", 32)
		kitSelectCard(t, ctx, frame, drawer, 2)
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 2 of 6 | Plate 1 of 2", 64); !ok {
			t.Fatalf("one-string-per-plate seam: no 2-plate card 2; got %q", c)
		}
		click(&ctx.Router, Button1) // Back at p = 1
		c, ok := pumpUntil(frame, "Start at:", 32)
		if !ok || uiContains(c, "Stopped before") {
			t.Fatalf("Back at plate 1 must return straight to the list; got %q", c)
		}
		click(&ctx.Router, Button3) // OK: the highlight is on card 2 again
		if c, ok = pumpUntil(frame, "Card 2 of 6 | Plate 1 of 2", 64); !ok {
			t.Fatalf("the highlight did not stay on the stopped card; got %q", c)
		}
	})
}

// Back at plate 2 shows the stop screen (needs one plate cut first).
func TestKitDoorStopsAtPlateTwo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, true, false)
		p, e := kitRunPlatform()
		ctx := NewContext(p)
		ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
		frame, drawer, quit := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()
		pumpUntil(frame, "Start at:", 32)
		kitSelectCard(t, ctx, frame, drawer, 2)
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 2 of 6 | Plate 1 of 2", 64); !ok {
			t.Fatalf("no plate 1; got %q", c)
		}
		kitCut(t, ctx, frame, e)
		if c, ok := pumpUntil(frame, "Card 2 of 6 | Plate 2 of 2", 64); !ok {
			t.Fatalf("no plate 2; got %q", c)
		}
		click(&ctx.Router, Button1)
		c, ok := pumpUntil(frame, "Stopped before", 32)
		if !ok {
			t.Fatalf("no stop screen; got %q", c)
		}
		kitSentences(t, c, "Engrave Kit",
			"Stopped before plate 2 of 2 of card 2 of 6. To finish this card, choose card 2 in the list again: it starts again at its plate 1.")
	})
}

func TestKitDoorPlateDoesNotFit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, false, true)
		p, _ := kitRunPlatform()
		ctx := NewContext(p)
		ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
		frame, _, quit := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()
		pumpUntil(frame, "Start at:", 32)
		click(&ctx.Router, Button3)
		c, ok := pumpUntil(frame, "does not fit on a plate", 64)
		if !ok {
			t.Fatalf("no does-not-fit screen; got %q", c)
		}
		kitSentences(t, c, "Engrave Kit",
			"Card 1 of 6, plate 1 of 1 does not fit on a plate, so this kit cannot be engraved on this machine.")
		click(&ctx.Router, Button3)
		if c, ok = pumpUntil(frame, "Start at:", 32); !ok {
			t.Fatalf("not back at the list; got %q", c)
		}
	})
}

// S1 test 32 / D4 (d): the lead's M equals the plates the door cuts, for every
// eligible kit vector. Cuts are counted at the layout seam, which the real run
// calls once per plate it is about to offer, and each plate is engraved.
func TestKitLeadPlatesEqualCuts(t *testing.T) {
	want := map[string]int{"kit-2of3": 6, "kit-single": 1, "kit-two-copies": 4, "kit-two-copies-mk1": 4}
	for name, m := range want {
		name, m := name, m
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := kitSeams(t, false, false)
				p, e := kitRunPlatform()
				ctx := NewContext(p)
				ctx.sysw = sessionWith(kitVectorRecords(t, name)...)
				frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
				defer quit()
				lead := fmt.Sprintf("(%d %s, %d %s)?", m, map[bool]string{true: "card", false: "cards"}[m == 1], m, map[bool]string{true: "plate", false: "plates"}[m == 1])
				_ = lead
				content, ok := pumpUntil(frame, "Engrave kit now", 32)
				if !ok {
					t.Fatalf("no lead; got %q", content)
				}
				var cards, plates int
				if _, err := fmt.Sscanf(extractLeadCounts(content), "%d,%d", &cards, &plates); err != nil {
					t.Fatalf("cannot read the lead counts from %q: %v", content, err)
				}
				click(&ctx.Router, Button3) // ENGRAVE KIT
				pumpUntil(frame, "Start at:", 32)
				click(&ctx.Router, Button3) // card 1
				for i := 0; i < plates; i++ {
					if c, ok := pumpUntil(frame, "Choose engraving", 64); !ok {
						t.Fatalf("plate %d: no choice screen; got %q", i+1, c)
					}
					kitCut(t, ctx, frame, e)
				}
				if c, ok := pumpUntil(frame, "of this kit", 64); !ok {
					t.Fatalf("the run did not end after the lead's %d plates; got %q", plates, c)
				}
				if len(*calls) != plates {
					t.Errorf("lead says %d plates, the run cut %d", plates, len(*calls))
				}
				if plates != m {
					t.Errorf("measured %d plates for %s, plan measured %d", plates, name, m)
				}
			})
		})
	}
}

// extractLeadCounts turns "...(6 cards, 6 plates)?" into "6,6" (the frame text
// has no spaces).
func extractLeadCounts(content string) string {
	txt := strings.ReplaceAll(content, " ", "")
	i := strings.Index(txt, "Engravekitnow(")
	if i < 0 {
		return ""
	}
	s := txt[i+len("Engravekitnow("):]
	s = strings.NewReplacer("cards,", ",", "card,", ",", "plates)?", "", "plate)?", "").Replace(s)
	if j := strings.IndexAny(s, "?\n"); j >= 0 {
		s = s[:j]
	}
	return s
}

var _ = time.Second
