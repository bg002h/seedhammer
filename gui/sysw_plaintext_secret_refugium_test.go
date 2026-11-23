//go:build refugium

package gui

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/seal"
	"seedhammer.com/sysw"
)

// FIX_sec-sh-sysw-plaintext-refuse (rows kitarch-a-s1-sysw-plaintext-secret,
// kitarch-b-s1-plaintext-secret-admitted, kitarch-c-a1-plaintext-secret-not-refused):
// the Refugium build refuses by default, whole and before the digest, a
// systemwide payload holding a secret outside a sealed section: a record whose
// CLASS is secret; the operator may load it anyway on an explicit override
// screen (Q3 re-ruled 2026-11-22).

const plaintextSecretRefusalFmt = "Record %d of this payload is a secret, and it is not sealed. " +
	"This build takes secrets only in the Sealed Payload, so nothing was loaded. " +
	"Pack the payload again without it, or load it anyway on the next screen."

// The override screen (Q3 re-ruled 2026-11-22, "Refuse, with override").
const plaintextSecretOverrideLead = "Load it anyway? Every program can then use its secret. " +
	"It stays unencrypted in flash until you wipe it from the host."

const plaintextSecretTestMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// plaintextSecretLoad drives the real load flow over a plaintext region of
// `records` and returns the first screen that is the refusal modal, the digest
// comparison, or the boot prompt's successor, plus the harness.
func plaintextSecretLoad(t *testing.T, boot bool, records ...string) (first string, ctx *Context, ret *bool, frame func() (string, bool), quit func()) {
	t.Helper()
	ctx = NewContext(kitPlatform(plaintextRegion(t, records...)))
	ret = new(bool)
	frame, _, quit = runUITouch(ctx, func() {
		*ret = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), boot)
	})
	if boot {
		if c, ok := pumpUntil(frame, "Load it?", 32); !ok {
			quit()
			t.Fatalf("no boot prompt; got %q", c)
		}
		click(&ctx.Router, Button3) // LOAD (index 0)
	}
	for i := 0; i < 64; i++ {
		c, more := frame()
		if !more {
			break
		}
		if uiContains(c, "Compare this against") || uiContains(c, "nothing was loaded") ||
			uiContains(c, "Nothing was loaded") {
			return c, ctx, ret, frame, quit
		}
	}
	quit()
	t.Fatal("neither a refusal nor the digest comparison was drawn")
	return
}

// TestKitarchPlaintextSecretRefusedBeforeDigest is RED row R1: every secret
// class but pass: (which keeps its own refusal), at position 2 behind a public
// text record, on both load routes.
func TestKitarchPlaintextSecretRefusedBeforeDigest(t *testing.T) {
	preimage := composerTestPreimageRecord(t, composerTestPreimageX())
	for _, tc := range []struct {
		name   string
		secret string
		class  sysw.Class
	}{
		{"mnemonic", plaintextSecretTestMnemonic, sysw.ClassMnemonic},
		{"codex32 secret", refugiumMS1Vec, sysw.ClassCodex32Secret},
		{"preimage", preimage, sysw.ClassPreimage},
		{"phrase", composerTestPhraseRecord(sysw.HashlockHardened, hashlockAnchorPhrase), sysw.ClassPhrase},
	} {
		if got := sysw.Classify(tc.secret); got != tc.class || !got.IsSecret() {
			t.Fatalf("INCONCLUSIVE: %s fixture classifies %v, want secret %v", tc.name, got, tc.class)
		}
		for _, boot := range []bool{false, true} {
			c, ctx, ret, frame, quit := plaintextSecretLoad(t, boot, "text:48656c6c6f", tc.secret)
			want := fmt.Sprintf(plaintextSecretRefusalFmt, 2)
			if !uiContains(c, want) || uiContains(c, "Compare this against") {
				t.Errorf("%s boot=%v: want the refusal before any digest; got %q", tc.name, boot, c)
			}
			click(&ctx.Router, Button3) // dismiss the refusal; the override screen follows
			if c2, ok := pumpUntil(frame, plaintextSecretOverrideLead, 64); !ok ||
				!uiContains(c2, "DO NOT LOAD") || !uiContains(c2, "LOAD ANYWAY") {
				t.Errorf("%s boot=%v: no override screen after the refusal; got %q", tc.name, boot, c2)
			}
			click(&ctx.Router, Button3) // the DEFAULT choice, DO NOT LOAD: the flow must now return on its own
			ended := false
			for i := 0; i < 64; i++ {
				c2, more := frame()
				if !more {
					ended = true
					break
				}
				if uiContains(c2, "Compare this against") || uiContains(c2, "Keep this payload loaded?") {
					t.Errorf("%s boot=%v: the flow went on past the default DO NOT LOAD: %q", tc.name, boot, c2)
					break
				}
			}
			if !ended {
				t.Errorf("%s boot=%v: the load flow did not return after DO NOT LOAD", tc.name, boot)
			}
			quit()
			if ctx.sysw != nil || *ret {
				t.Errorf("%s boot=%v: a session was left (sysw=%v, ret=%v)", tc.name, boot, ctx.sysw != nil, *ret)
			}
		}
	}
}

// TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord is RED row R2: the
// position is the FIRST secret record's, 1-based over the public section.
func TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord(t *testing.T) {
	c, _, _, _, quit := plaintextSecretLoad(t, false, plaintextSecretTestMnemonic, refugiumMS1Vec)
	defer quit()
	if !uiContains(c, fmt.Sprintf(plaintextSecretRefusalFmt, 1)) {
		t.Errorf("want Record 1 named; got %q", c)
	}
}

// TestKitarchPassRecordRefusalKeepsPrecedence is guard row G1: a payload with
// a pass: record and another secret shows the pass: refusal, unchanged.
func TestKitarchPassRecordRefusalKeepsPrecedence(t *testing.T) {
	c, _, _, _, quit := plaintextSecretLoad(t, false, plaintextSecretTestMnemonic, "pass:636f7272")
	defer quit()
	if !uiContains(c, syswPassphraseRecordRefusal(2)) || uiContains(c, "is a secret, and it is not sealed") {
		t.Errorf("want the pass: refusal alone; got %q", c)
	}
}

// TestKitarchPlaintextWithoutSecretClassStillLoads is guard row G2: a public
// plaintext payload, and an unconfirmed md1 alone (secret only by
// [mdmk-decode], not by class), reach the digest comparison as before.
func TestKitarchPlaintextWithoutSecretClassStillLoads(t *testing.T) {
	for _, recs := range [][]string{
		{"text:48656c6c6f"},
		kitVectorRecords(t, "kit-mk1-alone")[2:3],
	} {
		c, _, _, _, quit := plaintextSecretLoad(t, false, recs...)
		if !uiContains(c, "Compare this against") {
			t.Errorf("%q: want the digest comparison; got %q", recs, c)
		}
		quit()
	}
	// R3: the unconfirmed md1 alone keeps its F1 warning and then the
	// KEEP / UNLOAD offer (TestF1OffersUnloadWhereItOnceOfferedErase is skipped
	// under this tag, so this pins it at flow level).
	ctx := NewContext(kitPlatform(plaintextRegion(t, kitVectorRecords(t, "kit-mk1-alone")[2:3]...)))
	frame, drawer, quit := runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	defer quit()
	if c, ok := pumpUntil(frame, "Compare this against", 64); !ok {
		t.Fatalf("no digest comparison; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // confirm the digest
	if c, ok := pumpUntil(frame, "Payload Warnings", 64); !ok {
		t.Fatalf("an unconfirmed md1 lost its F1 warning; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // acknowledge the warnings
	if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		t.Fatalf("the KEEP / UNLOAD offer did not follow F1; got %q", c)
	}
}

// TestKitarchSecretInTheCleartextSectionOfASealedPayload: a sealed header does
// not seal a record in its public section; only p.Secret is sealed.
func TestKitarchSecretInTheCleartextSectionOfASealedPayload(t *testing.T) {
	for _, tc := range []struct {
		name  string
		p     sysw.Payload
		n     int
		found bool
	}{
		{"secret in public of a sealed payload", sysw.Payload{Public: []string{"text:41", plaintextSecretTestMnemonic}, Secret: []string{refugiumMS1Vec}}, 2, true},
		{"secret only in the sealed section", sysw.Payload{Public: []string{"text:41"}, Secret: []string{refugiumMS1Vec}}, 0, false},
	} {
		n, found := syswFirstSecretRecord(&tc.p)
		if n != tc.n || found != tc.found {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", tc.name, n, found, tc.n, tc.found)
		}
	}
}

// sealedRegionWithPublic hand-seals a systemwide container whose PUBLIC
// (cleartext, authenticated only) section is `pub` and whose sealed section is
// `sec`, under the passphrase "abandon about". `me sysw pack` never writes a
// secret class into the public section; any other writer can.
func sealedRegionWithPublic(t *testing.T, pub, sec []string) sysw.Reader {
	t.Helper()
	pb := []byte(strings.Join(pub, "\n"))
	pt := []byte(strings.Join(sec, "\n"))
	h := sysw.Header{Iterations: sysw.MinIterations, PubLen: uint32(len(pb)), CtLen: uint32(len(pt))}
	enc := h.Encode()
	blob := append(enc[:], pb...)
	key := seal.DeriveKey(seal.NormalisePassphrase("abandon about"), h.Salt[:], int(h.Iterations))
	b, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		t.Fatal(err)
	}
	blob = g.Seal(blob, h.IV[:], pt, append([]byte(nil), blob...))
	region := make([]byte, sysw.RegionLen)
	for i := range region {
		region[i] = 0xFF
	}
	copy(region, blob)
	f := filepath.Join(t.TempDir(), "region.bin")
	if err := os.WriteFile(f, region, 0o600); err != nil {
		t.Fatal(err)
	}
	return sysw.FileReader{Path: f}
}

// TestKitarchSealedPayloadWithASecretInItsPublicSectionIsRefused: the header
// is sealed, but record 2 sits in the cleartext section.
func TestKitarchSealedPayloadWithASecretInItsPublicSectionIsRefused(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = sealedRegionWithPublic(t, []string{"text:48656c6c6f", plaintextSecretTestMnemonic}, []string{"text:41"})
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	defer quit()
	if c, ok := pumpUntil(frame, "1:", 64); !ok {
		t.Fatalf("no passphrase prompt; got %q", c)
	}
	for _, w := range []string{"abandon", "about"} {
		runes(&ctx.Router, w)
		click(&ctx.Router, Button3)
		if _, more := frame(); !more {
			t.Fatal("passphrase entry ended early")
		}
	}
	tapNavSlot(t, ctx, drawer(), Button2) // done
	pumpUntil(frame, "words - unlock", 32)
	click(&ctx.Router, Down)
	tapNavSlot(t, ctx, drawer(), Button3) // UNLOCK
	for i := 0; i < 64; i++ {
		c, more := frame()
		if !more {
			break
		}
		if uiContains(c, "Compare this against") || uiContains(c, "Payload Warnings") {
			t.Fatalf("a secret in the cleartext section of a sealed payload was not refused; got %q", c)
		}
		if uiContains(c, fmt.Sprintf(plaintextSecretRefusalFmt, 2)) {
			// Q3 override: LOAD ANYWAY on a sealed payload's public-section
			// secret loads it, with the 14f line alone (no F1: the header is
			// sealed) and then KEEP / UNLOAD.
			click(&ctx.Router, Button3)
			if c, ok := pumpUntil(frame, plaintextSecretOverrideLead, 64); !ok {
				t.Fatalf("no override screen; got %q", c)
			}
			click(&ctx.Router, Down)
			click(&ctx.Router, Button3) // LOAD ANYWAY
			if c, ok := pumpUntil(frame, "Compare this against", 64); !ok {
				t.Fatalf("LOAD ANYWAY did not reach the digest; got %q", c)
			}
			click(&ctx.Router, Button3)
			c, ok := pumpUntil(frame, "Payload Warnings", 64)
			if !ok || !uiContains(c, "Refugium build: secrets travel only in the Sealed Payload") ||
				uiContains(c, "A SECRET is stored unencrypted in flash.") {
				t.Fatalf("want the 14f line alone after LOAD ANYWAY; got %q", c)
			}
			click(&ctx.Router, Button3)
			if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
				t.Fatalf("no KEEP / UNLOAD after LOAD ANYWAY; got %q", c)
			}
			return
		}
	}
	t.Fatal("no refusal drawn")
}

// overrideToOutcome drives a refused plaintext load through the override
// screen with `keys` and reports what the flow then did.
func overrideToOutcome(t *testing.T, ctx *Context, keys ...Button) (frame func() (string, bool), ret *bool, quit func()) {
	t.Helper()
	ret = new(bool)
	frame, _, quit = runUITouch(ctx, func() {
		*ret = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	if c, ok := pumpUntil(frame, fmt.Sprintf(plaintextSecretRefusalFmt, 2), 64); !ok {
		quit()
		t.Fatalf("no refusal drawn; got %q", c)
	}
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, plaintextSecretOverrideLead, 64); !ok {
		quit()
		t.Fatalf("no override screen; got %q", c)
	}
	click(&ctx.Router, keys...)
	return frame, ret, quit
}

// TestKitarchPlaintextSecretLoadAnywayLoadsAndWarns is RED row R8 (Q3
// override): LOAD ANYWAY goes on to the digest, F1 and the 14f line, and
// KEEP / UNLOAD; KEEP leaves the payload loaded. A second load of the same
// payload is refused again: the override is per load, not remembered.
func TestKitarchPlaintextSecretLoadAnywayLoadsAndWarns(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, "text:48656c6c6f", plaintextSecretTestMnemonic)))
	frame, ret, quit := overrideToOutcome(t, ctx, Down, Button3) // LOAD ANYWAY
	if c, ok := pumpUntil(frame, "Compare this against", 64); !ok {
		quit()
		t.Fatalf("LOAD ANYWAY did not reach the digest comparison; got %q", c)
	}
	click(&ctx.Router, Button3) // the digest matched
	c, ok := pumpUntil(frame, "Payload Warnings", 64)
	if !ok || !uiContains(c, "A SECRET is stored unencrypted in flash.") ||
		!uiContains(c, "Refugium build: secrets travel only in the Sealed Payload") {
		quit()
		t.Fatalf("want F1 then the 14f line after LOAD ANYWAY; got %q", c)
	}
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		quit()
		t.Fatalf("no KEEP / UNLOAD after LOAD ANYWAY; got %q", c)
	}
	click(&ctx.Router, Button3) // KEEP
	for i := 0; i < 64; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	quit()
	if !*ret || ctx.sysw == nil || !ctx.sysw.loaded {
		t.Fatalf("LOAD ANYWAY then KEEP left nothing loaded (ret=%v)", *ret)
	}
	// Per load only: LOAD AGAIN meets the refusal and the override again.
	ctx.Done = false
	_, _, quit2 := overrideToOutcome(t, ctx, Button1)
	quit2()
}

// TestKitarchPlaintextSecretOverrideBackLoadsNothing is RED row R9: Back on
// the override screen is DO NOT LOAD.
func TestKitarchPlaintextSecretOverrideBackLoadsNothing(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, "text:48656c6c6f", plaintextSecretTestMnemonic)))
	frame, ret, quit := overrideToOutcome(t, ctx, Button1)
	ended := false
	for i := 0; i < 64; i++ {
		c, more := frame()
		if !more {
			ended = true
			break
		}
		if uiContains(c, "Compare this against") {
			t.Errorf("Back on the override screen went on to the digest: %q", c)
			break
		}
	}
	quit()
	if !ended || *ret || ctx.sysw != nil {
		t.Errorf("Back on the override screen: ended=%v ret=%v sysw=%v", ended, *ret, ctx.sysw != nil)
	}
}

// TestKitarchLoadAgainDeclineLeavesNothingLoaded is RED row R1c: LOAD AGAIN
// re-reads the same region (it cannot change while the GUI runs), so after
// LOAD ANYWAY + KEEP the loaded session holds this very secret. DO NOT LOAD,
// or Back, on the re-asked override must leave nothing loaded, as the modal
// says ("so nothing was loaded") and as declining the digest does.
func TestKitarchLoadAgainDeclineLeavesNothingLoaded(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  Button
	}{{"DO NOT LOAD", Button3}, {"Back", Button1}} {
		ctx := NewContext(kitPlatform(plaintextRegion(t, "text:48656c6c6f", plaintextSecretTestMnemonic)))
		frame, ret, quit := overrideToOutcome(t, ctx, Down, Button3) // LOAD ANYWAY
		for _, s := range []string{"Compare this against", "Payload Warnings", "Keep this payload loaded?"} {
			if c, ok := pumpUntil(frame, s, 64); !ok {
				quit()
				t.Fatalf("%s: setup: no %q; got %q", tc.name, s, c)
			}
			click(&ctx.Router, Button3) // the digest matched, acknowledge, KEEP
		}
		for i := 0; i < 64; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		quit()
		if !*ret || ctx.sysw == nil || !ctx.sysw.loaded {
			t.Fatalf("%s: INCONCLUSIVE: LOAD ANYWAY + KEEP left nothing loaded (ret=%v)", tc.name, *ret)
		}
		ctx.Done = false
		frame2, ret2, quit2 := overrideToOutcome(t, ctx, tc.key) // LOAD AGAIN, then decline
		ended := false
		for i := 0; i < 64; i++ {
			if _, more := frame2(); !more {
				ended = true
				break
			}
		}
		quit2()
		if !ended || *ret2 || ctx.sysw != nil {
			t.Errorf("%s on LOAD AGAIN left a session (ended=%v, ret=%v, sysw=%v); the modal said nothing was loaded",
				tc.name, ended, *ret2, ctx.sysw != nil)
		}
	}
}

// TestKitarchPlaintextSecretOverrideLabelOrder is RED row R9b: row 0 (the
// default, what Back selects) is labelled DO NOT LOAD, LOAD ANYWAY is below it.
func TestKitarchPlaintextSecretOverrideLabelOrder(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, "text:48656c6c6f", plaintextSecretTestMnemonic)))
	frame, _, quit := overrideToOutcome(t, ctx) // no key: stay on the override screen
	defer quit()
	c, ok := pumpUntil(frame, plaintextSecretOverrideLead, 64)
	if !ok {
		t.Fatalf("override screen not redrawn; got %q", c)
	}
	flat := strings.ReplaceAll(strings.ToLower(c), " ", "")
	a, b := strings.Index(flat, "donotload"), strings.Index(flat, "loadanyway")
	if a < 0 || b < 0 || a > b {
		t.Errorf("want DO NOT LOAD before LOAD ANYWAY (row 0 first); indexes %d, %d in %q", a, b, c)
	}
}
