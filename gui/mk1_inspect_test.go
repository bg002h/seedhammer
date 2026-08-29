package gui

import (
	"strings"
	"testing"

	"seedhammer.com/mk"
)

// V1 (2-chunk) and V3 (different key set) strings.
const (
	v1c0 = "mk1qpzg69pqqsq3zg3ngj4thnxaq5zg3vs7zqsrqqdt4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4vp3kx98j76m4mjlwphf"
	v1c1 = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	v3c1 = "mk1qpx3t8pprlnqdqf52q7jwgcnxgnuseav37nvs0zn06dyfs79hk7uk8lrxlyw57x7v7rzx74tlflqh"
)

func TestMK1Gatherer(t *testing.T) {
	g := &mk1Gatherer{}
	if st := g.offer(v1c1); st != gatherAdded { // out-of-order: index 1 first
		t.Fatalf("offer c1: status %v", st)
	}
	if g.complete() {
		t.Fatal("complete after 1 of 2")
	}
	if st := g.offer(v1c1); st != gatherDup {
		t.Fatalf("offer dup: status %v", st)
	}
	if st := g.offer(v3c1); st != gatherForeign { // different chunk_set_id
		t.Fatalf("offer foreign: status %v", st)
	}
	if st := g.offer("not an mk1 chunk"); st != gatherIgnored {
		t.Fatalf("offer garbage: status %v", st)
	}
	if st := g.offer(v1c0); st != gatherAdded {
		t.Fatalf("offer c0: status %v", st)
	}
	if !g.complete() {
		t.Fatal("not complete after 2 of 2")
	}
	card, err := mk.Decode(g.collected())
	if err != nil {
		t.Fatalf("Decode(collected): %v", err)
	}
	if card.Path != "m/48h/0h/0h/2h" {
		t.Fatalf("path = %q", card.Path)
	}
}

func TestHasMKPrefix(t *testing.T) {
	if !hasMKPrefix("mk1qpzg69p...") || !hasMKPrefix("MK1QPZG...") {
		t.Fatal("mk1 prefix not detected")
	}
	if hasMKPrefix("md1qabc...") {
		t.Fatal("md1 misdetected as mk1")
	}
}

func TestMK1DisplayFlowPaging(t *testing.T) {
	ctx := NewContext(newPlatform())
	card := mk.Card{
		Network:     "mainnet",
		Path:        "m/48h/0h/0h/2h",
		Fingerprint: "aabbccdd",
		Stubs:       make([][4]byte, 1),
		Xpub:        "xpub6Den8YwXbKQvkwukmx7Uukicw4qDgMEPuuUkhMp3Rn557YSN2uVQnCMQNSfgDtennU9nES3Wbbmz1LAPBydhNpED8NU4mf1SFF41hM7vFrc",
	}
	frame, quit := runUI(ctx, func() { mk1DisplayFlow(ctx, &descriptorTheme, card) })
	defer quit()
	var all strings.Builder
	for i := 0; i < 16; i++ {
		content, ok := frame()
		if !ok {
			break
		}
		all.WriteString(content)
		click(&ctx.Router, Button3) // page forward
	}
	got := all.String()
	if !uiContains(got, "m/48h/0h/0h/2h") {
		t.Errorf("path not shown; got %q", got)
	}
	if !uiContains(got, "aabbccdd") {
		t.Errorf("fingerprint not shown")
	}
	// Invariant 2.10: paging reaches the xpub tail, gap-free.
	if !uiContains(got, "1hM7vFrc") {
		t.Errorf("xpub tail not reached via paging")
	}
}

func TestMK1DisplayFlowBackExits(t *testing.T) {
	ctx := NewContext(newPlatform())
	card := mk.Card{Network: "mainnet", Path: "m", Stubs: make([][4]byte, 1), Xpub: "xpub6x"}
	frame, quit := runUI(ctx, func() { mk1DisplayFlow(ctx, &descriptorTheme, card) })
	defer quit()
	frame()
	click(&ctx.Router, Button1) // Back
	if _, ok := frame(); ok {
		t.Fatal("mk1DisplayFlow did not exit on Back")
	}
}

func TestMK1GatherFlowBackNoReader(t *testing.T) {
	// testPlatform.NFCReader() == nil, so a multi-chunk set can't complete;
	// only Back exits. Verifies the no-reader render path + progress.
	ctx := NewContext(newPlatform())
	var card mk.Card
	var ok bool
	frame, quit := runUI(ctx, func() { card, ok = mk1GatherFlow(ctx, &descriptorTheme, v1c0) })
	defer quit()
	content, _ := frame()
	if !uiContains(content, "1 of 2") {
		t.Errorf("progress not shown; got %q", content)
	}
	click(&ctx.Router, Button1) // Back
	if _, fok := frame(); fok {
		t.Fatal("mk1GatherFlow did not exit on Back")
	}
	// mk.Card has a slice field → not comparable; check fields.
	if ok || card.Xpub != "" || card.Path != "" || len(card.Stubs) != 0 {
		t.Fatalf("Back should yield (zero, false); got ok=%v card=%+v", ok, card)
	}
}

func TestMdmkFlowMK1ShowsInspect(t *testing.T) {
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText(v1c0)) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("mdmkFlow produced no frame")
	}
	if !uiContains(content, "Inspect key") {
		t.Errorf("mk1 chooser missing Inspect key; got %q", content)
	}
}

// A THREE-chunk mk1 set, in ChunkIndex order 0,1,2 (chunk_set_id 702021,
// m/48h/0h/1h/2h). Verified with mk.ParseHeader rather than assumed from the
// order the generator printed them in.
//
// Three rather than two on purpose: with two chunks a random readback order
// agrees with index order half the time, so a map-iteration regression would
// slip past a single trial far too often. With three it is one in six.
//
// Test material, public by construction: this is card A@1 of cmd/emu's cosigner
// payload, whose master is BIP-39's own published `abandon…about` vector.
// Regenerate with `go run ./cmd/buildpayloadcards`. Never put funds behind it.
var mk1ThreeChunks = []string{
	"mk1qp4dj9zqqsq4kj90x4eutks2lcztpqyqsqygpqyqsqygrqyqsqyg9qyqsqyqfz9jrcld706hn9svfgll7zvw5qnkxgea7y6pqcgj2njpw0xx",
	"mk1qp4dj9zp68w6hzragnj3g5qrl85zeape8wq0vdczfyy55tqsd5576trsa3p40nfpd7hsyjyf7vlx6hk2j6ckr4wf0m36k7q0920s9wqfx6hj",
	"mk1qp4dj9zzv308jhm5uzl5tlxr6z",
}

// F-162, and the mk1 twin of TestMD1GathererCollectedIndexOrder (T-H2):
// collected() must return chunks in ChunkIndex order whatever order they
// ARRIVED in. The gatherer keys its map BY ChunkIndex and then, before the fix,
// ranged that map — and Go randomises map iteration, so the engrave plan built
// from it put a card's plates in a different sequence on different runs.
//
// Found by running the emulator walk three times and getting three different
// plate orders. The identical defect was fixed for md1 at 3a23dbb; this line
// was never touched.
//
// Asserts the CONTRACT (the string at slot i declares ChunkIndex i) rather than
// equality against a canonical slice, so the test cannot drift from the fixture.
func TestMK1GathererCollectedIndexOrder(t *testing.T) {
	orders := [][]int{
		{2, 0, 1},
		{1, 2, 0},
		{0, 1, 2},
		{2, 1, 0},
	}
	for _, order := range orders {
		// Repeat: Go's map iteration is randomised per run, so one trial could
		// coincidentally agree. Ten makes a regression observable.
		for trial := 0; trial < 10; trial++ {
			g := &mk1Gatherer{}
			for _, i := range order {
				if st := g.offer(mk1ThreeChunks[i]); st != gatherAdded {
					t.Fatalf("order %v: offer chunk %d status %v", order, i, st)
				}
			}
			if !g.complete() {
				t.Fatalf("order %v: not complete after %d chunks", order, len(order))
			}
			got := g.collected()
			if len(got) != len(mk1ThreeChunks) {
				t.Fatalf("order %v: collected %d, want %d", order, len(got), len(mk1ThreeChunks))
			}
			for i, s := range got {
				h, err := mk.ParseHeader(s)
				if err != nil {
					t.Fatalf("order %v trial %d: collected()[%d] does not parse: %v", order, trial, i, err)
				}
				if int(h.ChunkIndex) != i {
					t.Fatalf("order %v trial %d: collected()[%d] declares ChunkIndex %d — "+
						"not index order, so the engrave plan is nondeterministic",
						order, trial, i, h.ChunkIndex)
				}
			}
		}
	}
}

// The same property through the PRODUCTION path that F-162 actually broke: a
// bundle gathered out of order must lay its plates out in index order. This is
// the one that binds the engrave plan, since bundlePlatePlan walks
// bundleCard.strings — documented as "verbatim chunk strings in index order".
//
// TestBundlePlanVerbatim looks like it covers this and does not: it compares the
// plan against c.strings, so it stays self-consistent however c.strings is
// ordered.
func TestBundleGatherOutOfOrderPlatesInIndexOrder(t *testing.T) {
	for trial := 0; trial < 10; trial++ {
		g := &bundleGatherer{}
		// Arrive last-chunk-first; the queue makes this ordinary in practice.
		for i := len(mk1ThreeChunks) - 1; i >= 0; i-- {
			if st := g.offer(mdmkText(mk1ThreeChunks[i])); st == bundleDropped {
				t.Fatalf("trial %d: chunk %d dropped", trial, i)
			}
		}
		if len(g.cards) != 1 {
			t.Fatalf("trial %d: %d cards, want 1", trial, len(g.cards))
		}
		plan := bundlePlatePlan(engraverParams, g.cards)
		// F-423 packs strings onto plates, so the assertion is on the ORDER of
		// the strings the plan carries, not on a plate count: plate P's strings
		// come before plate P+1's, and read end to end they are chunk 0, 1, 2.
		var got []string
		for _, p := range plan {
			got = append(got, p.strs...)
		}
		if len(got) != len(mk1ThreeChunks) {
			t.Fatalf("trial %d: the plan carries %d strings, want %d", trial, len(got), len(mk1ThreeChunks))
		}
		for i, s := range got {
			h, err := mk.ParseHeader(s)
			if err != nil {
				t.Fatalf("trial %d: string %d does not parse: %v", trial, i, err)
			}
			if int(h.ChunkIndex) != i {
				t.Fatalf("trial %d: string %d of the plan carries ChunkIndex %d — "+
					"the plates are cut in the order the plan lists them",
					trial, i, h.ChunkIndex)
			}
		}
	}
}
