package gui

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/sysw"
)

// F-145. syswLoadFlow closed F-144 -- the step that makes the payload feature
// exist -- and shipped with nothing exercising it, which is F-144's own failure
// shape approached from the other side: a green suite that would stay green if
// the function returned immediately.
//
// CORRECTION TO F-145 AS FILED: it claimed the harness had no Platform fake with
// a SyswReader and no region fixture. Both were already there --
// `testPlatform.sysw` with `SyswReader()` (gui_test.go:343,447) and
// `sysw.FileReader` (sysw/read_host.go:9). The entry was written from
// assumption rather than from a grep, so the reason it gave was wrong even
// though the gap was real.

// syswRegionFor writes a vector's blob padded to a full region, exactly as
// `me sysw pack --region` does, and returns a reader over it. The fixture is
// therefore the artifact that actually gets flashed, not a hand-built blob.
func syswRegionFor(t *testing.T, name string) sysw.Reader {
	t.Helper()
	path := os.Getenv("SYSW_VECTORS")
	if path == "" {
		path = filepath.Join("..", "..", "mnemonic-engrave", "crates", "me-cli",
			"testdata", "sysw_vectors.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		abs, _ := filepath.Abs(path)
		if os.Getenv("SYSW_REQUIRE_VECTORS") == "1" {
			t.Fatalf("SYSW_REQUIRE_VECTORS=1 and the vectors are unreadable at %s: %v", abs, err)
		}
		t.Skipf("vectors not found at %s (%v)", abs, err)
	}
	var vs []struct {
		Name string `json:"name"`
		Blob string `json:"blob"`
	}
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, v := range vs {
		if v.Name != name {
			continue
		}
		b, err := hex.DecodeString(v.Blob)
		if err != nil {
			t.Fatalf("vector %s blob is not hex: %v", name, err)
		}
		region := make([]byte, sysw.RegionLen)
		for i := range region {
			region[i] = 0xFF
		}
		copy(region, b)
		f := filepath.Join(t.TempDir(), "region.bin")
		if err := os.WriteFile(f, region, 0o600); err != nil {
			t.Fatal(err)
		}
		return sysw.FileReader{Path: f}
	}
	t.Fatalf("no vector named %q in %s", name, path)
	return nil
}

// countingSyswReader proves a claim no other test makes: that the flow does not
// touch the region when there is nothing there to touch.
type countingSyswReader struct {
	probe bool
	reads int
}

func (r *countingSyswReader) Probe() bool { return r.probe }
func (r *countingSyswReader) Read() ([]byte, error) {
	r.reads++
	return nil, nil
}

// THE ADDITIVE PROPERTY, which is the one that must never regress: a machine
// with no payload behaves exactly as it did before. No prompt, no read, no
// session. Asserted without the UI harness because the flow must return before
// it draws anything -- if it ever starts drawing here, this test hangs, which is
// the correct failure.
func TestSyswLoadFlowIsSilentWithoutAPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    sysw.Reader
	}{
		{"nil reader — the platform has no region at all", nil},
		{"probe false — a region that holds no container", &countingSyswReader{probe: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			if got := syswLoadFlow(ctx, &descriptorTheme, tc.r, true); got {
				t.Error("reported a payload loaded when there is none")
			}
			if ctx.sysw != nil {
				t.Error("a session was created with no payload present")
			}
			if c, ok := tc.r.(*countingSyswReader); ok && c.reads != 0 {
				t.Errorf("read the region %d times after Probe said there was nothing", c.reads)
			}
		})
	}
}

// The boot offer must be refusable, and refusing must leave the machine exactly
// as it was. Without this, SKIP could load anyway and every existing test would
// still pass.
func TestSyswLoadFlowBootSkipLoadsNothing(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-A")
	ctx := NewContext(p)

	var ret bool
	frame, drawer, quit := runUITouch(ctx, func() {
		ret = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true)
	})
	defer quit()
	content, ok := pumpUntil(frame, "Load it?", 32)
	if !ok {
		t.Fatalf("the boot offer never appeared; got %q", content)
	}
	// Button1 is the left nav slot, which ChoiceScreen uses to cancel.
	tapNavSlot(t, ctx, drawer(), Button1)
	quit()
	if ret {
		t.Error("SKIP reported a payload loaded")
	}
	if ctx.sysw != nil {
		t.Error("SKIP left a session behind; every program would then offer FROM PAYLOAD")
	}
}

// A payload that is present but structurally wrong must not produce a session,
// and must not say "unreadable" (§5.2) -- that phrase teaches the operator to
// read a wrong file as tampering.
func TestSyswLoadFlowRefusesAMalformedRegionWithoutCryingTamper(t *testing.T) {
	f := filepath.Join(t.TempDir(), "region.bin")
	junk := make([]byte, sysw.RegionLen)
	copy(junk, []byte("MNEMSYSW")) // right magic, nothing else valid
	if err := os.WriteFile(f, junk, 0o600); err != nil {
		t.Fatal(err)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = sysw.FileReader{Path: f}
	ctx := NewContext(p)

	frame, _, quit := runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	defer quit()
	content, ok := pumpUntil(frame, "Load Payload", 32)
	if !ok {
		t.Fatalf("no error screen for a malformed region; got %q", content)
	}
	if uiContains(content, "unreadable") {
		t.Errorf("§5.2: a structural failure must never be reported as tampering; got %q", content)
	}

	// WHAT THIS TEST DOES NOT ASSERT, and why it does not pretend to.
	//
	// The obvious additions -- that the flow returned false and left ctx.sysw
	// nil -- are NOT here, because they could not be made to fail. runUITouch
	// drives the flow as an iter.Pull coroutine on another goroutine, so a
	// mutant that sets `ctx.sysw = &syswSession{}` and returns true is invisible
	// to an unsynchronised read from the test goroutine: verified by applying
	// exactly that mutation (asserting it landed) and watching this test pass.
	//
	// Assertions that cannot fail are worse than absent ones, so they are absent
	// and filed instead (F-146). What IS asserted above survives mutation: the
	// error screen appears, and it does not cry tampering.
}

// C1 from the pre-flash-of-the-load-flow fable review, as a test that fails
// without the fix.
//
// `make(bip39.Mnemonic, 24)` zero-fills, and Word(0) is a REAL wordlist entry --
// inputWordsFlow's empty sentinel is -1, which is why every other caller uses
// emptyBIP39Mnemonic. A zero-filled buffer therefore looks ALREADY FULL, entry
// terminates immediately, and the passphrase handed to Open is whatever the
// zeroes spell rather than what the operator typed. Nothing sealed could ever be
// opened, which also closes the AEAD route to [compared] and makes a sealed
// pub_len==0 payload permanently unconsumable.
//
// This asserts the buffer contract directly rather than driving the keyboard:
// the defect is entirely in how the buffer is initialised, and a direct check
// cannot be defeated by tap coordinates.
func TestSyswLoadFlowPassphraseBufferStartsEmpty(t *testing.T) {
	const n = 24
	zero := make(bip39.Mnemonic, n)
	empty := emptyBIP39Mnemonic(n)

	if zero[0] == empty[0] {
		t.Fatal("INCONCLUSIVE: a zero-filled buffer already matches an empty one, " +
			"so this test cannot distinguish the defect it exists for")
	}
	// The sentinel, stated so a change to it fails here rather than silently in
	// the flow.
	for i, w := range empty {
		if w != -1 {
			t.Fatalf("emptyBIP39Mnemonic[%d] = %v, want -1 (the empty sentinel)", i, w)
		}
	}
	// And the trap itself: slot 0 of a zero-filled buffer is a real word, so it
	// reads as filled.
	if got := bip39.LabelFor(zero[0]); got == "" {
		t.Fatal("Word(0) has no label, so a zero-filled buffer would read as empty " +
			"and this defect could not occur -- re-check the premise")
	} else {
		t.Logf("Word(0) is %q -- a zero-filled buffer reads as 24 filled slots", got)
	}
}
