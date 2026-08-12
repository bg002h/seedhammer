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
	click(&ctx.Router, Down)              // ENTER IT -> FROM PAYLOAD
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
	click(&ctx.Router, Down)
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
	tapNavSlot(t, ctx, drawer(), Button3) // choice 0 == ENTER IT
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
// accepted. A card set on ctx.syswBundleSeed and dropped would leave the tally
// at zero while every structural assertion still passed.
func TestMultisigTakesItsFirstCardFromThePayload(t *testing.T) {
	// A complete, non-chunked md1: it decodes on its own, so the gatherer counts
	// it immediately rather than waiting for chunks that will never be scanned.
	const md1 = "md1yqpqqxqq8xtwhw4xwn4qh"
	if got := sysw.Classify(md1); got != sysw.ClassMDMK {
		t.Fatalf("INCONCLUSIVE: the fixture classifies as %v, not ClassMDMK", got)
	}
	for _, tc := range []struct {
		name string
		run  func(ctx *Context)
	}{
		{"supplied policy", func(ctx *Context) { supplyMultisigPolicyFlow(ctx, &descriptorTheme) }},
		{"built policy", func(ctx *Context) { buildMultisigPolicyFlow(ctx, &descriptorTheme) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(md1)

			frame, drawer, quit := runUITouch(ctx, func() { tc.run(ctx) })
			defer quit()

			// The built-policy path opens with buildParamPickFlow's pickers, so
			// the offer is reached by accepting each default in turn. Every
			// screen before the offer is a ChoiceScreen whose default is choice
			// 0, so one Button3 per screen walks them.
			var content string
			var found bool
			for i := 0; i < 12 && !found; i++ {
				if content, found = pumpUntil(frame, "First card from where?", 8); found {
					break
				}
				click(&ctx.Router, Button3)
			}
			if !found {
				t.Fatalf("no payload offer before the gather; got %q", content)
			}
			click(&ctx.Router, Down)              // ENTER IT -> FROM PAYLOAD
			tapNavSlot(t, ctx, drawer(), Button3) // choose
			if content, found = pumpUntil(frame, "md1 descriptors: 1", 32); !found {
				t.Fatalf("the payload card never reached the gatherer's tally; got %q", content)
			}
			if ctx.syswBundleSeed != "" {
				t.Errorf("the card was left on ctx.syswBundleSeed after the gather "+
					"consumed it: %q", ctx.syswBundleSeed)
			}
		})
	}
}
