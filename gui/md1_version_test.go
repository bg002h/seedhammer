package gui

import (
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

// md1AtVersion rewrites an md1 string's 4-bit wire version and re-checksums
// it: a BCH-valid card at a version no encoder emits. (Test-local twin of
// md/wire_version8_test.go's withVersion; gui cannot import md's test code.)
func md1AtVersion(t *testing.T, s string, v uint8) string {
	t.Helper()
	syms, err := codex32.MDDataSymbols(s)
	if err != nil {
		t.Fatal(err)
	}
	syms = append([]byte(nil), syms...)
	if syms[0]&1 == 1 {
		syms[0] = v<<1 | 1
	} else {
		syms[0] = syms[0]&0b10000 | v&0b1111
	}
	return codex32.AssembleMD1(syms)
}

// SPEC §6a / §8.9 stage-3 row, the gather half. A v12 chunk is a well-formed
// md1 this firmware does not read: it must NOT be "Not an md1 descriptor
// chunk.", and the message must name 12. A v8 chunk -- the plate stage 3
// exists to read -- is ADDED, not refused.
func TestMD1GathererNamesAnUnsupportedVersion(t *testing.T) {
	v8 := loadVectorChunks(t, "keyed_tr_liana_kofn_recovery")[0]
	g := &md1Gatherer{}
	if st := g.offer(md1AtVersion(t, v8, 12)); st != gatherUnsupportedVersion || g.refusedVersion != 12 {
		t.Fatalf("v12 chunk: status %v version %d, want gatherUnsupportedVersion naming 12", st, g.refusedVersion)
	}
	if msg := md1VersionMessage(g.refusedVersion); !strings.Contains(msg, "12") || strings.Contains(msg, "Not an md1") {
		t.Fatalf("message %q must name the version and must not deny the card is md1", msg)
	}
	if st := (&md1Gatherer{}).offer(v8); st != gatherAdded {
		t.Fatalf("v8 chunk: status %v, want gatherAdded -- this firmware reads version 8", st)
	}
	if st := (&md1Gatherer{}).offer("not an md1 chunk"); st != gatherIgnored {
		t.Fatalf("garbage: status %v, want gatherIgnored (unchanged)", st)
	}
}

// The same refusal through the two routes an operator actually takes from the
// inspect chooser: a chunked v12 card (gather flow, first chunk) and a single
// v12 card (md.Decode's default arm). Both must show the ONE phrasing.
func TestMdmkFlowNamesAnUnsupportedMD1Version(t *testing.T) {
	for _, c := range []struct{ name, card string }{
		{"chunked", md1AtVersion(t, loadVectorChunks(t, "keyed_tr_liana_kofn_recovery")[0], 12)},
		{"single", md1AtVersion(t, loadVectorChunks(t, "liana_taproot")[0], 12)},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newPlatform()
			p.engraver = newEngraver()
			ctx := NewContext(p)
			frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText(c.card)) })
			defer quit()
			var all strings.Builder
			for i := 0; i < 6; i++ {
				s, ok := frame()
				if !ok {
					break
				}
				all.WriteString(s + "\n")
				if i == 0 {
					chooseInspect(&ctx.Router)
				}
			}
			got := all.String()
			if !uiContains(got, md1VersionMessage(12)) {
				t.Errorf("no %q on screen; got %q", md1VersionMessage(12), got)
			}
			for _, lie := range []string{"Not an md1 descriptor chunk", "Can't decode this descriptor", "Captured 0 of 0"} {
				if uiContains(got, lie) {
					t.Errorf("screen says %q about a well-formed md1 at version 12", lie)
				}
			}
		})
	}
}

// And the bundle channel, which classifies the same strings on its own.
func TestBundleNamesAnUnsupportedMD1Version(t *testing.T) {
	for _, c := range []struct{ name, card string }{
		{"chunked", md1AtVersion(t, loadVectorChunks(t, "keyed_tr_liana_kofn_recovery")[0], 12)},
		{"single", md1AtVersion(t, loadVectorChunks(t, "liana_taproot")[0], 12)},
	} {
		g := &bundleGatherer{}
		st := g.offer(mdmkText(c.card))
		if st != bundleUnsupportedMD1Version || g.refusedMD1Version != 12 {
			t.Errorf("%s: status %v version %d, want bundleUnsupportedMD1Version naming 12", c.name, st, g.refusedMD1Version)
		}
		s := &bundleGatherScreen{g: g}
		if msg := s.feedback(st); msg != md1VersionMessage(12) {
			t.Errorf("%s: feedback %q, want %q", c.name, msg, md1VersionMessage(12))
		}
	}
}

// The positive twins: version 8 is READ on every route that refuses 12, so the
// refusal is the accepted SET and not "anything but 4". A single v8 card
// reaches the display (no version message, no "Can't decode"), and a complete
// v8 chunk set becomes a bundle card.
func TestVersion8CardsAreReadOnEveryRoute(t *testing.T) {
	single := loadVectorChunks(t, "liana_taproot")[0]
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText(single)) })
	var all strings.Builder
	for i := 0; i < 6; i++ {
		s, ok := frame()
		if !ok {
			break
		}
		all.WriteString(s + "\n")
		if i == 0 {
			chooseInspect(&ctx.Router)
		}
	}
	quit()
	for _, lie := range []string{"cannot read md1 version", "Can't decode this descriptor"} {
		if uiContains(all.String(), lie) {
			t.Errorf("v8 single card: screen says %q", lie)
		}
	}
	// R0 m3: a POSITIVE too. Absences alone pass on a flow that draws nothing;
	// the display must actually show the card's three keys.
	if !uiContains(all.String(), "Keys: 3") {
		t.Errorf("v8 single card: the display never showed %q; got %q", "Keys: 3", all.String())
	}

	g := &bundleGatherer{}
	var last bundleOfferStatus
	for _, c := range loadVectorChunks(t, "keyed_tr_liana_kofn_recovery") {
		last = g.offer(mdmkText(c))
	}
	if last != bundleCardComplete || len(g.cards) != 1 {
		t.Fatalf("v8 chunk set: last status %v with %d cards, want bundleCardComplete and one card", last, len(g.cards))
	}
}
