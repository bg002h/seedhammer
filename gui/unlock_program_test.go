package gui

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/seal"
)

// §10.1 — the unlockPayload menu entry is CONDITIONAL: present when the payload
// region holds a blob, invisible when it does not.
//
// unlockProgramTitles is the pager in order WITH a payload present: the eight
// unconditional programs, then the conditional ninth. Without a payload the
// carousel must stop at "BIP-85" and never reach the last entry.
var unlockProgramTitles = []string{
	"Backup Wallet",
	"BIP-39 Password",
	"Engrave Text",
	"Account Xpub",
	"Engrave Bundle",
	"Engrave Single-Sig",
	"Engrave Multisig",
	"BIP-85",
	"Sealed Payload",
}

// sealVectorBlob returns the named SPEC §11.4 vector's raw blob, read from the
// seal package's own GENERATED vectors file rather than retyped here.
// seal/testdata/README.md: the file is produced by the normative Rust
// implementation and never hand-edited, and "retyped constants are how a port
// silently forks".
func sealVectorBlob(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "seal", "testdata", "vectors.json"))
	if err != nil {
		t.Fatalf("read seal vectors: %v", err)
	}
	var f struct {
		Vectors []struct {
			Name    string `json:"name"`
			BlobHex string `json:"blob_hex"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse seal vectors: %v", err)
	}
	for _, v := range f.Vectors {
		if v.Name != name {
			continue
		}
		b, err := hex.DecodeString(v.BlobHex)
		if err != nil {
			t.Fatalf("vector %s: blob_hex: %v", name, err)
		}
		return b
	}
	t.Fatalf("no vector named %q in seal/testdata/vectors.json", name)
	return nil
}

// payloadReaderFor writes a vector's blob to a temp file and hands back a
// seal.Reader over it. seal.FileReader is the host stand-in for the XIP read
// (seal/read_host.go) — which is exactly the seam Platform.PayloadReader exists
// to keep out of gui: no build-tagged type is named here.
func payloadReaderFor(t *testing.T, name string) seal.Reader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, sealVectorBlob(t, name), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return seal.FileReader{Path: path}
}

// pagerDots measures how many dots layoutMainPager actually drew, by inverting
// its own geometry against the dot asset's size.
//
// The count is asserted against LITERAL 8 and 9 by the callers, never against
// int(bip85Derive)+1: an assertion whose expected side is derived from the same
// constant the code uses pins nothing.
func pagerDots(t *testing.T, ctx *Context, lastNav program) int {
	t.Helper()
	_, sz := layoutMainPager(&ctx.B, &descriptorTheme, backupWallet, lastNav)
	const space = 4
	dot := assets.CircleFilled.Bounds().Size().X
	n := (sz.X + space) / (dot + space)
	if (dot+space)*n-space != sz.X {
		t.Fatalf("pager is %dpx wide, which is not a whole number of %dpx dots", sz.X, dot)
	}
	return n
}

// TestUnlockPayloadInvisibleWithoutAPayload — §10.1's negative path. The
// carousel must be an EIGHT-program lap, and the ninth entry must not appear on
// any frame of it.
func TestUnlockPayloadInvisibleWithoutAPayload(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	if p.PayloadReader() != nil {
		t.Fatal("the default test platform must have no payload reader")
	}
	frame, drawer, quit := runUITouch(ctx, func() { uiFlow(ctx, "test") })
	defer quit()

	content, ok := frame()
	if !ok {
		t.Fatal("uiFlow produced no frame")
	}
	if !uiContains(content, unlockProgramTitles[0]) {
		t.Fatalf("initial program is not %q; got %q", unlockProgramTitles[0], content)
	}
	if uiContains(content, "Sealed Payload") {
		t.Fatalf("the Sealed Payload entry is visible with no payload present: %q", content)
	}
	_, right := arrowPoints(ctx)
	// A full lap of EIGHT: with no payload the eighth tap must wrap back to the
	// first program, not step onto a ninth.
	const lap = 8
	for i := 1; i <= lap; i++ {
		want := unlockProgramTitles[i%lap]
		tap(&ctx.Router, drawer(), right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after tap #%d", i)
		}
		if !uiContains(content, want) {
			t.Fatalf("tap #%d: expected program %q, got %q", i, want, content)
		}
		if uiContains(content, "Sealed Payload") {
			t.Fatalf("tap #%d reached the Sealed Payload entry with no payload present: %q", i, content)
		}
	}
	if got := pagerDots(t, ctx, bip85Derive); got != 8 {
		t.Errorf("the no-payload pager draws %d dots, want 8", got)
	}
}

// TestUnlockPayloadVisibleWithAPayload — §10.1's positive path. The lap is NINE
// programs, the ninth is the new entry, and the other eight are all still
// reachable in order (the regression the const-to-runtime bound can cause).
func TestUnlockPayloadVisibleWithAPayload(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.payload = payloadReaderFor(t, "E")
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() { uiFlow(ctx, "test") })
	defer quit()

	content, ok := frame()
	if !ok {
		t.Fatal("uiFlow produced no frame")
	}
	if !uiContains(content, unlockProgramTitles[0]) {
		t.Fatalf("initial program is not %q; got %q", unlockProgramTitles[0], content)
	}
	_, right := arrowPoints(ctx)
	const lap = 9
	seenUnlock := false
	for i := 1; i <= lap; i++ {
		want := unlockProgramTitles[i%lap]
		tap(&ctx.Router, drawer(), right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after tap #%d", i)
		}
		if !uiContains(content, want) {
			t.Fatalf("tap #%d: expected program %q, got %q", i, want, content)
		}
		if i == lap-1 {
			if !uiContains(content, "Sealed Payload") {
				t.Fatalf("the ninth program is not Sealed Payload; got %q", content)
			}
			seenUnlock = true
		}
	}
	if !seenUnlock {
		t.Fatal("the Sealed Payload entry was never reached")
	}
	if got := pagerDots(t, ctx, unlockPayload); got != 9 {
		t.Errorf("the payload-present pager draws %d dots, want 9", got)
	}
}

// TestUnlockPayloadWrapsBothDirections. The prev wrap and the next wrap are
// SEPARATE sites and a fix to one does not fix the other, so both are driven in
// both states. The next wrap is covered by the lap tests above; this pins prev.
func TestUnlockPayloadWrapsBothDirections(t *testing.T) {
	for _, tc := range []struct {
		name     string
		vector   string
		wantLast string
	}{
		{"no payload", "", "BIP-85"},
		{"payload present", "E", "Sealed Payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			if tc.vector != "" {
				p.payload = payloadReaderFor(t, tc.vector)
			}
			ctx := NewContext(p)
			frame, drawer, quit := runUITouch(ctx, func() { uiFlow(ctx, "test") })
			defer quit()
			if _, ok := frame(); !ok {
				t.Fatal("uiFlow produced no frame")
			}
			left, _ := arrowPoints(ctx)
			tap(&ctx.Router, drawer(), left)
			content, ok := frame()
			if !ok {
				t.Fatal("no frame after the left tap")
			}
			if !uiContains(content, tc.wantLast) {
				t.Fatalf("Left from the first program did not wrap to %q; got %q", tc.wantLast, content)
			}
			// And the wrap target is the LAST program: one more Left must not
			// land on it again.
			tap(&ctx.Router, drawer(), left)
			content, ok = frame()
			if !ok {
				t.Fatal("no frame after the second left tap")
			}
			if uiContains(content, tc.wantLast) {
				t.Fatalf("two Lefts stayed on %q; the wrap bound is off by one: %q", tc.wantLast, content)
			}
		})
	}
}
