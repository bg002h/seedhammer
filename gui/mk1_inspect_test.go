package gui

import (
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
