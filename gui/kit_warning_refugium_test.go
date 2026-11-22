//go:build refugium

package gui

import (
	"strings"
	"testing"

	"seedhammer.com/gui/op"
)

const refugiumSecretLine = "Refugium build: secrets travel only in the Sealed Payload, never in this one. Unload this payload unless you mean to engrave its secret now."

// warningsOf is the Payload Warnings screen's lines for a session.
func warningsOf(recs ...string) []string { return syswLoadWarnings(sessionWith(recs...)) }

// TestRefugiumSecretWarningLine is S1 test 31 (section 14f).
func TestRefugiumSecretWarningLine(t *testing.T) {
	seed := chainPayloadNamed(t, "chain-codex32").Records
	// Plaintext with a secret: the line directly after F1's.
	lines := warningsOf(seed...)
	if len(lines) != 2 || lines[0] != "A SECRET is stored unencrypted in flash." || lines[1] != refugiumSecretLine {
		t.Errorf("plaintext secret: warnings %q", lines)
	}
	// An unconfirmed md1/mk1 alone is secret only by [mdmk-decode]: no line.
	alone := kitVectorRecords(t, "kit-mk1-alone")
	for _, l := range warningsOf(alone[2:3]...) {
		if l == refugiumSecretLine {
			t.Errorf("an unconfirmed md1/mk1 alone raised the Refugium line: %q", l)
		}
	}
	// A confirmed public kit has no secret: no warnings at all.
	if l := warningsOf(kitVectorRecords(t, "kit-single")...); len(l) != 0 {
		t.Errorf("a public kit warns: %q", l)
	}
	// An unconfirmed md1/mk1 beside a secret: F1 twice, the secret line right
	// after the SECRET line, never before it.
	mixed := warningsOf(append(append([]string{}, alone[2:3]...), seed...)...)
	i := indexOfLine(mixed, "A SECRET is stored unencrypted in flash.")
	if i < 0 || i+1 >= len(mixed) || mixed[i+1] != refugiumSecretLine {
		t.Errorf("the line is not directly after F1's SECRET line: %q", mixed)
	}
}

func indexOfLine(lines []string, want string) int {
	for i, l := range lines {
		if strings.EqualFold(l, want) {
			return i
		}
	}
	return -1
}

// loadSealedForWarnings drives the real load flow over a sealed vector to the
// Payload Warnings screen and returns its frame and the harness.
func loadSealedForWarnings(t *testing.T, vec string, words []string) (content string, ctx *Context, frame func() (string, bool), drawer func() *op.Drawer, quit func()) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, vec)
	ctx = NewContext(p)
	frame, drawer, quit = runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	if c, ok := pumpUntil(frame, "1:", 64); !ok {
		t.Fatalf("%s: no passphrase prompt; got %q", vec, c)
	}
	for _, w := range words {
		runes(&ctx.Router, w)
		click(&ctx.Router, Button3)
		if _, more := frame(); !more {
			t.Fatal("passphrase entry ended early")
		}
	}
	tapNavSlot(t, ctx, drawer(), Button2) // done
	if c, ok := pumpUntil(frame, "words - unlock", 32); !ok {
		_ = c
	}
	click(&ctx.Router, Down)
	tapNavSlot(t, ctx, drawer(), Button3) // UNLOCK
	if vec == "S-E" {
		if c, ok := pumpUntil(frame, "Compare this against", 64); !ok {
			t.Fatalf("%s: no digest; got %q", vec, c)
		}
		tapNavSlot(t, ctx, drawer(), Button3)
	}
	c, ok := pumpUntil(frame, "Payload Warnings", 64)
	if !ok {
		t.Fatalf("%s: no Payload Warnings; got %q", vec, c)
	}
	return c, ctx, frame, drawer, quit
}

func TestRefugiumSecretWarningLineSealed(t *testing.T) {
	// S-D: sealed, 12-word passphrase, a secret, no flag: the line is the
	// screen's only line.
	words := make([]string, 12)
	for i := range words {
		words[i] = "abandon"
	}
	words[11] = "about"
	c, ctx, frame, drawer, quit := loadSealedForWarnings(t, "S-D", words)
	defer quit()
	if !uiContains(c, refugiumSecretLine) || uiContains(c, "unencrypted") || uiContains(c, "word-count floor") {
		t.Errorf("S-D: want the line alone; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if k, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		t.Errorf("S-D: KEEP / UNLOAD does not follow the line; got %q", k)
	}

	// S-E: F2's line, then the Refugium line last.
	c, _, _, _, quit2 := loadSealedForWarnings(t, "S-E", []string{"abandon", "about"})
	defer quit2()
	o := orderOf(c, "The passphrase is below the word-count floor.", refugiumSecretLine)
	if !(o[0] >= 0 && o[1] > o[0]) {
		t.Errorf("S-E: F2's line then the Refugium line expected; got %q (%v)", c, o)
	}
}
