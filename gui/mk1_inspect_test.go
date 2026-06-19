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
	if card.Path != "m/48'/0'/0'/2'" {
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
		Path:        "m/48'/0'/0'/2'",
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
	if !uiContains(got, "m/48'/0'/0'/2'") {
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

func TestMdmkFlowMD1NoInspect(t *testing.T) {
	// §2.5/§2.9: an md1 string keeps the engrave-only flow (no Inspect).
	// validateMdmk only QR-encodes + lays out (no BCH re-check), so an
	// md1-prefixed literal exercises the isMK==false branch directly.
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText("md1qqqqqqpqqzgr3hq2v")) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("mdmkFlow(md1) produced no frame")
	}
	if uiContains(content, "Inspect key") {
		t.Errorf("md1 chooser must NOT offer Inspect; got %q", content)
	}
}
