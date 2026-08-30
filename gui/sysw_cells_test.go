package gui

import (
	"testing"

	"seedhammer.com/codex32"
	"seedhammer.com/sysw"
)

// A BCH-valid ms1 share, 74 characters — under `MaxEngraveableMs1Len`, so
// sysw.Classify calls it ClassCodex32Secret. Borrowed from backup_test.go rather
// than invented: a hand-written one carries no checksum and classifies as
// nothing.
const cellMs1 = "ms10leetsllhdmn9m42vcsamx24zrxgs3qrl7ahwvhw4fnzrhve25gvezzyq0pgjxpzx0ysaam"

func sessionHolding(records ...string) *syswSession {
	s := &syswSession{}
	s.load(&sysw.Payload{Public: records}, [32]byte{7}, false, true, true, true)
	return s
}

// Plan stage 13a. §3.3.2 admits ClassCodex32Secret to Backup Wallet, and this is
// the ONE program with a carrier for it: the typed menu's `M*1 STRING` row
// already returns the codex32.String that engraveObjectFlow's case expects.
//
// The record must ARRIVE, not merely be offered — a route that showed the offer
// and then fell through to the keyboard would draw exactly the same screen.
func TestBackupWalletTakesACodex32SecretFromThePayload(t *testing.T) {
	if got := sysw.Classify(cellMs1); got != sysw.ClassCodex32Secret {
		t.Fatalf("INCONCLUSIVE: the fixture classifies as %v, not "+
			"ClassCodex32Secret, so this test cannot exercise the cell", got)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding(cellMs1)

	var obj any
	var ok bool
	frame, drawer, quit := runUITouch(ctx, func() { obj, ok = newInputFlow(ctx, &descriptorTheme) })
	defer quit()

	content, found := pumpUntil(frame, "FROM PAYLOAD", 32)
	if !found {
		t.Fatalf("Backup Wallet did not offer the payload's codex32 secret; got %q", content)
	}
	// FROM PAYLOAD is index 0 now (operator ruling 2026-08-12), so no Down.
	tapNavSlot(t, ctx, drawer(), Button3) // choose
	for i := 0; i < 32; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	if !ok {
		t.Fatal("the offer was accepted and nothing came back")
	}
	s, isString := obj.(codex32.String)
	if !isString {
		t.Fatalf("newInputFlow returned %T, not the codex32.String engraveObjectFlow "+
			"routes — the typed M*1 row's own object", obj)
	}
	if s.String() != cellMs1 {
		t.Errorf("the returned share is %q, want the payload's %q", s.String(), cellMs1)
	}
}

// Plan stage 13b. §3.3.2 admits ClassPassphrase to the four SEAM programs, for
// their optional-passphrase step — the cell the spec recorded as "admitted, no
// consumption path".
//
// Taking it must SKIP the keyboard: a flow that offered the payload and then
// asked for typing anyway would have served the cell in appearance only.
func TestTheSeamPassphraseComesFromThePayloadWithoutTyping(t *testing.T) {
	// `pass:` is a RESERVED prefix and its body is lowercase hex (§5.3.1) --
	// "abandon about", which contains the space EPD §6.4 forbids raw.
	const (
		record = "pass:6162616e646f6e2061626f7574"
		pass   = "abandon about"
	)
	if got := sysw.Classify(record); got != sysw.ClassPassphrase {
		t.Fatalf("INCONCLUSIVE: the fixture classifies as %v, not ClassPassphrase", got)
	}
	if raw, err := sysw.DecodeBody(record); err != nil || string(raw) != pass {
		t.Fatalf("INCONCLUSIVE: the fixture decodes to (%q, %v), want %q", raw, err, pass)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding(record)

	var got string
	var ok bool
	frame, drawer, quit := runUITouch(ctx, func() {
		got, ok = syswPassphraseFlow(ctx, &descriptorTheme)
	})
	defer quit()

	content, found := pumpUntil(frame, "Password from where?", 32)
	if !found {
		t.Fatalf("the seam did not offer the payload's password; got %q", content)
	}
	// FROM PAYLOAD is index 0 now (operator ruling 2026-08-12), so no Down.
	tapNavSlot(t, ctx, drawer(), Button3)
	var after []string
	for i := 0; i < 32; i++ {
		c, more := frame()
		if !more {
			break
		}
		after = append(after, c)
	}
	for _, c := range after {
		if uiContains(c, "Enter Passphrase") {
			t.Errorf("the keyboard was shown after the payload supplied the "+
				"password; got %q", c)
		}
	}
	if !ok || got != pass {
		t.Errorf("syswPassphraseFlow = (%q, %v), want (%q, true)", got, ok, pass)
	}
}

// DECLINING the offer falls through to the keyboard, which is what makes
// syswOffer's shape strictly additive: a machine with no payload, and an
// operator who says no, both get exactly what they got before.
func TestDecliningTheSeamPassphraseOfferReachesTheKeyboard(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding("pass:6162616e646f6e2061626f7574")

	frame, drawer, quit := runUITouch(ctx, func() {
		syswPassphraseFlow(ctx, &descriptorTheme)
	})
	defer quit()
	content, found := pumpUntil(frame, "Password from where?", 32)
	if !found {
		t.Fatalf("no offer; got %q", content)
	}
	click(&ctx.Router, Down)              // index 0 is FROM PAYLOAD now
	tapNavSlot(t, ctx, drawer(), Button3) // so ENTER IT is index 1
	if content, found = pumpUntil(frame, "Enter Passphrase", 32); !found {
		t.Fatalf("declining the offer did not reach the keyboard; got %q", content)
	}
}

// Plan stage 13c. §3.3.2 admits ClassMDMK to Engrave Multisig for the
// supplied-md1 path, and the payload card must enter through the SAME offer()
// a scanned card takes — so it is deduplicated, chunk-assembled and validated
// identically.
//
// The needle is the gather screen's own TALLY, which counts cards the gatherer
// accepted. A card set on ctx.syswBundleSeeds and dropped would leave the tally
// at zero while every structural assertion still passed.
//
// S1 SPLIT THE TWO FLOWS' MECHANISM, so this test carries both shapes:
//
//   - SUPPLIED policy still SHOWS the picker, because it still has two answers
//     to pick between: the payload, or the reader. (F-76 later changed WHAT the
//     payload arm hands over — every md1/mk1 card rather than the first record
//     — and the lead was reworded from "First card from where?" to "Cards from
//     where?" to match. The picker itself is what this row is about.)
//   - BUILT policy takes the WHOLE cosigner set from the payload — that is S1's
//     deliverable — so there is nothing to pick between and the picker is gone.
//     The seam is not: buildCosignerSource is the one place that answers "where
//     does a cosigner key come from", with payload as phase 1's only answer.
func TestMultisigTakesItsFirstCardFromThePayload(t *testing.T) {
	// A complete, non-chunked md1: it decodes on its own, so the gatherer counts
	// it immediately rather than waiting for chunks that will never be scanned.
	const md1 = "md1yqpqqxqq8xtwhw4xwn4qh"
	if got := sysw.Classify(md1); got != sysw.ClassMDMK {
		t.Fatalf("INCONCLUSIVE: the fixture classifies as %v, not ClassMDMK", got)
	}
	// The built path refuses before the gather unless the payload can fill its
	// open slots, so its payload carries one cosigner card alongside the md1
	// (n defaults to 2 through the pickers, so exactly one slot is open).
	buildRecords := append([]string{md1}, cosignerCardRecords(t, 1)...)

	for _, tc := range []struct {
		name    string
		records []string
		picker  bool // does this flow still show the source picker?
		want    string
		run     func(ctx *Context)
	}{
		{"supplied policy", []string{md1}, true, "md1 descriptors: 1",
			func(ctx *Context) { supplyMultisigPolicyFlow(ctx, &descriptorTheme) }},
		{"built policy", buildRecords, false, "mk1 keys: 1",
			func(ctx *Context) { buildMultisigPolicyFlow(ctx, &descriptorTheme) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(tc.records...)

			frame, drawer, quit := runUITouch(ctx, func() { tc.run(ctx) })
			defer quit()

			// The built-policy path opens with buildParamPickFlow's pickers, so
			// the gather is reached by accepting each default in turn. Every
			// screen before it is a ChoiceScreen whose default is choice 0, so
			// one Button3 per screen walks them.
			var content string
			var found bool
			needle := "Cards from where?"
			if !tc.picker {
				needle = tc.want
			}
			for i := 0; i < 12 && !found; i++ {
				if content, found = pumpUntil(frame, needle, 8); found {
					break
				}
				click(&ctx.Router, Button3)
			}
			if !found {
				t.Fatalf("never reached %q; got %q", needle, content)
			}
			if tc.picker {
				// FROM PAYLOAD is index 0 (operator ruling 2026-08-12), so no Down.
				tapNavSlot(t, ctx, drawer(), Button3) // choose
				if content, found = pumpUntil(frame, tc.want, 32); !found {
					t.Fatalf("the payload card never reached the gatherer's tally; got %q", content)
				}
			} else if uiContains(content, "Cards from where?") {
				t.Errorf("the built path still draws a source picker; S1 leaves it "+
					"exactly one answer, and a one-option Input screen is a tap that "+
					"teaches nothing: %q", content)
			}
			if len(ctx.syswBundleSeeds) != 0 {
				t.Errorf("cards were left on ctx.syswBundleSeeds after the gather "+
					"consumed them: %q", ctx.syswBundleSeeds)
			}
		})
	}
}
