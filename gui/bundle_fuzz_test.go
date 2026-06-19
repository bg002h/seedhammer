package gui

import (
	"testing"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// FuzzBundleGatherer feeds arbitrary sequences of {ms1, single-mk1, single-md1,
// chunked-mk1/md1 chunks, garbage} into a fresh bundleGatherer and asserts the
// safety invariants hold for ANY sequence (Task 5):
//
//   - no panic;
//   - only verified-complete cards ever land in g.cards — every chunked card
//     re-decodes via mk.Decode/md.DecodeChunks, every standalone md1 via
//     md.Decode (I-1);
//   - an ms1 secret is NEVER added (R0-C2 / security spine);
//   - a single (non-chunked) mk1 is NEVER added (R0-C1).
//
// The fuzz bytes select objects from a fixed pool of real fixtures + garbage; an
// op byte choosing the pool entry exercises the classification + accumulation
// without needing an NFC reader.
func FuzzBundleGatherer(f *testing.F) {
	// Build the object pool once (fixtures are deterministic).
	mkA := mk1CardA(f)
	mkB := mk1CardB(f)
	mdA := md1CardA(f)
	mdB := md1CardB(f)
	ms1 := ms1Object(f)
	singleMK := singleMK1Fixture(f)
	singleMD := singleMD1(f)

	var pool []any
	add := func(o any) { pool = append(pool, o) }
	for _, s := range mkA {
		add(mdmkText(s))
	}
	for _, s := range mkB {
		add(mdmkText(s))
	}
	for _, s := range mdA {
		add(mdmkText(s))
	}
	for _, s := range mdB {
		add(mdmkText(s))
	}
	add(ms1)                               // ms1 secret — must never be added
	add(mdmkText(singleMK))                // single mk1 — must never be added
	add(mdmkText(singleMD))                // single md1 — a standalone card
	add(addressText("bc1qexampleaddress")) // a non-md/mk object
	add(mdmkText("garbage-not-a-card"))    // unparseable mdmkText
	add(nil)                               // a nil scan object

	// Seed the corpus with a few hand-built sequences.
	f.Add([]byte{0, 1, 2, 3, 4, 5})                                  // an interleaved prefix
	f.Add([]byte{byte(len(pool) - 5)})                               // the ms1 entry alone
	f.Add([]byte{0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 2, 3, 4, 5, 6}) // many chunks

	f.Fuzz(func(t *testing.T, ops []byte) {
		g := &bundleGatherer{}
		for _, op := range ops {
			obj := pool[int(op)%len(pool)]
			if st := g.offer(obj); st == bundleRefusedMs1 {
				// An ms1 secret must NEVER mutate the bundle (R0-C2, security spine).
				continue
			}
		}
		// Every card in the bundle is verified-complete + integral (I-1), and is
		// never an ms1 / single-mk1.
		for i, c := range g.cards {
			switch c.kind {
			case cardMK1:
				if len(c.strings) < 2 {
					t.Fatalf("card %d: mk1 with %d strings (single mk1 must be refused)", i, len(c.strings))
				}
				if _, err := mk.Decode(c.strings); err != nil {
					t.Fatalf("card %d: added mk1 fails integrity: %v", i, err)
				}
			case cardMD1:
				// An md1 card with a single string can be EITHER a standalone
				// (non-chunked) descriptor OR a single-chunk chunked set (e.g.
				// wsh_multi_chunked). Either way it must pass its integrity gate via
				// the matching decoder (I-1): try single-string md.Decode first, then
				// the chunked md.DecodeChunks path.
				_, singleErr := md.Decode(c.strings[0])
				if len(c.strings) == 1 && singleErr == nil {
					break
				}
				if _, err := md.DecodeChunks(c.strings); err != nil {
					t.Fatalf("card %d: added md1 card fails integrity (single=%v chunked=%v)", i, singleErr, err)
				}
			default:
				t.Fatalf("card %d: unknown kind %v", i, c.kind)
			}
		}
	})
}
