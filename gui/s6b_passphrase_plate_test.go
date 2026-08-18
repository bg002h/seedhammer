package gui

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/passphrase"
	"seedhammer.com/sysw"
)

// ─── S6b GATE 2.4b/2.4c: the engraved policy id equals the mk1's own stub,
// computed from the POST-templateizeBundle md1, on BOTH forms ────────────

// s6bPolicyIDHex is the exact expression gui/singlesig.go's passphrase-plate
// offer uses to format S6b spec 2.4's "wallet policy id" for
// engravePassphraseFlowPreloaded's policyID parameter: md.FormAwareStubChunks
// on the FINAL b.MD1, formatted as 8 uppercase hex digits. A test helper
// rather than a call into gui/singlesig.go because the policy id is computed
// inline there, not behind its own named function -- this mirrors the exact
// expression so a divergence between the two is exactly the defect GATE
// 2.4b/2.4c exists to catch.
func s6bPolicyIDHex(t *testing.T, mdChunks []string) string {
	t.Helper()
	stub, err := md.FormAwareStubChunks(mdChunks)
	if err != nil {
		t.Fatalf("md.FormAwareStubChunks: %v", err)
	}
	return fmt.Sprintf("%X", stub[:])
}

// TestPolicyIDMatchesTheMK1StubOnBothForms is GATES 2.4b and 2.4c together:
// 2.4b is the VALUE EQUALITY (not a label check) between the engraved policy
// id and the policy_id_stub the mk1 this run cut actually carries; 2.4c is
// that both sides are computed from the POST-templateizeBundle b.MD1. Both
// forms (full-policy and template-only) are exercised because they select
// FormAwareStub's two different branches (spec 2.4: "Not
// md.WalletPolicyIDStub ... Template-only md1 is a reachable choice").
func TestPolicyIDMatchesTheMK1StubOnBothForms(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	full, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}
	tmpl, err := templateizeBundle(full)
	if err != nil {
		t.Fatalf("templateizeBundle: %v", err)
	}

	check := func(t *testing.T, name string, mdChunks, mk1 []string) {
		t.Helper()
		policyID := s6bPolicyIDHex(t, mdChunks)
		card, err := mk.Decode(mk1)
		if err != nil {
			t.Fatalf("%s: mk.Decode: %v", name, err)
		}
		if len(card.Stubs) != 1 {
			t.Fatalf("%s: mk1 carries %d stubs, want 1", name, len(card.Stubs))
		}
		wantHex := fmt.Sprintf("%X", card.Stubs[0][:])
		if policyID != wantHex {
			t.Errorf("%s: engraved policy id %q != the mk1's own stub %q", name, policyID, wantHex)
		}
	}
	check(t, "full policy", full.MD1, full.MK1)
	check(t, "template-only", tmpl.MD1, tmpl.MK1)

	// The two forms' policy ids must actually DIFFER -- otherwise this test
	// could pass by accident even if the "computed from b.MD1" wiring were
	// broken in a way that happened to reuse one value for both (e.g. a
	// caller that captured the pre-template stub and never updated it,
	// spec 2.4's own named hazard: "A value captured near :107 would be the
	// pre-template one").
	if s6bPolicyIDHex(t, full.MD1) == s6bPolicyIDHex(t, tmpl.MD1) {
		t.Fatal("test is void: full-policy and template-only policy ids are equal")
	}
}

// ─── S6b GATE 2.6: the offer appears only when passphrase != "", and the
// ~31s bare-seed KDF does NOT run when no passphrase plate is engraved ────

// TestPassphrasePlateOfferGate is GATE 2.6, in its three reachable states.
// Uses singleSigBareSeedFPHook -- fired immediately before the lazy
// deriveAccountXpub(mnemonic, "", ...) call -- as the observable proxy for
// "did the ~31s KDF run": on THIS machine that derivation completes in
// milliseconds (the 31s figure is the real SeedHammer II hardware's cost,
// not this test runner's), so the hook counts invocations rather than timing
// them.
func TestPassphrasePlateOfferGate(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	full, masterFP, _, _, err := deriveSingleSigBundle(m, "hunter2", &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}

	run := func(t *testing.T, passphrase string, drive func(ctx *Context, frame func() (string, bool))) int {
		t.Helper()
		var hookCount int
		singleSigBareSeedFPHook = func() { hookCount++ }
		defer func() { singleSigBareSeedFPHook = nil }()

		ctx := NewContext(newPlatform())
		done := false
		frame, quit := runUI(ctx, func() {
			singleSigPassphrasePlateOffer(ctx, &descriptorTheme, m, passphrase, masterFP, path, full)
			done = true
		})
		defer quit()
		if drive != nil {
			drive(ctx, frame)
		}
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("singleSigPassphrasePlateOffer never returned")
		}
		return hookCount
	}

	t.Run("no passphrase: no offer shown, no derivation", func(t *testing.T) {
		// No drive func: if the offer's ChoiceScreen were shown regardless
		// of passphrase, the loop above would spin without a press and
		// "never returned" would fire -- proving the offer's absence, not
		// merely its outcome.
		if n := run(t, "", nil); n != 0 {
			t.Errorf("the bare-seed derivation ran %d times with no passphrase, want 0", n)
		}
	})

	t.Run("passphrase entered, offer declined: no derivation", func(t *testing.T) {
		n := run(t, "hunter2", func(ctx *Context, frame func() (string, bool)) {
			if c, ok := pumpUntil(frame, "Passphrase Plate", 32); !ok {
				t.Fatalf("the offer was not shown with a passphrase entered; got %q", c)
			}
			click(&ctx.Router, Button3) // "Skip" is choice 0, the default
		})
		if n != 0 {
			t.Errorf("the bare-seed derivation ran %d times after declining, want 0", n)
		}
	})

	t.Run("passphrase entered, offer accepted: derivation runs exactly once", func(t *testing.T) {
		var hookCount int
		singleSigBareSeedFPHook = func() { hookCount++ }
		defer func() { singleSigBareSeedFPHook = nil }()

		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			singleSigPassphrasePlateOffer(ctx, &descriptorTheme, m, "hunter2", masterFP, path, full)
		})
		defer quit()
		if c, ok := pumpUntil(frame, "Passphrase Plate", 32); !ok {
			t.Fatalf("the offer was not shown; got %q", c)
		}
		click(&ctx.Router, Down) // Skip -> Engrave
		frame()
		click(&ctx.Router, Button3) // choose Engrave
		for i := 0; i < 16; i++ {
			frame()
		}
		if hookCount != 1 {
			t.Errorf("the bare-seed derivation ran %d times after accepting, want exactly 1", hookCount)
		}
	})
}

// TestPassphrasePlateOfferReachableFromTheOrchestrator closes the "can an
// operator actually reach this" gap the tests above cannot: they call
// singleSigPassphrasePlateOffer directly. This drives the REAL
// engraveSingleSigFlow, with a passphrase, through actually-completed
// plates, and confirms the passphrase-plate offer is the very next screen
// after the verify offer -- proving the insertion point and the argument
// wiring (mnemonic, passphrase, masterFP, path, b), not merely the factored
// function in isolation.
//
// Watch-only mode (2 plates: mk1+md1, no ms1), not full (3): GATE 2.6 does
// not condition the offer on `full` (only on passphrase != ""), so the
// cheaper mode proves the same wiring at a lower real-engrave cost.
// engraveOnePlate + runUITouchRaster are reused from
// gui/multisig_build_walk_test.go and gui/raster_test.go rather than
// reimplemented: that file's own comment records why the raster harness
// matters here -- under plain runUI the same plate took 12x the frames.
func TestPassphrasePlateOfferReachableFromTheOrchestrator(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		frame, _, _, quit := runUITouchRaster(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84
		if c, ok := pumpUntil(frame, "passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Down) // Skip -> Add passphrase
		frame()
		click(&ctx.Router, Button3)
		// syswPassphraseFlow offers the systemwide payload first, then falls
		// to passphraseFlowTitled's plain keyboard (gui/gui.go) -- a
		// DIFFERENT screen from engravePassphraseFlow's own
		// passphraseEntryFlow (no live counter), which is why this matches
		// its title rather than "/100".
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase entry; got %q", c)
		}
		runes(&ctx.Router, "hunter2")
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave Mode", 64); !ok {
			t.Fatalf("did not reach the full/watch-only choice; got %q", c)
		}
		click(&ctx.Router, Down) // Watch-only
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		click(&ctx.Router, Button3) // Full policy md1
		if c, ok := pumpUntil(frame, "Plates To Cut", 64); !ok {
			t.Fatalf("the plate census was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 2", 64); !ok {
			t.Fatalf("watch-only mode did not reach engrave with 2 cards; got %q", c)
		}
		plates := 0
		for plates < 8 {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			engraveOnePlate(t, ctx, frame, e)
			plates++
		}
		if plates == 0 {
			t.Fatal("no plate was cut, so this walk never reached the post-engrave surfaces")
		}
		// Watch-only mode carries no cardMS1 (the secret leg is never
		// engraved), so bundleEngrave shows the end-of-bundle "hand-engrave
		// your ms1" reminder modal -- dismiss it before the verify offer.
		if c, ok := pumpUntil(frame, "hand-engrave your ms1", 32); ok {
			click(&ctx.Router, Button3)
			frame()
		} else {
			t.Logf("no ms1 reminder shown; last frame %q", c)
		}
		if c, ok := pumpUntil(frame, "Verify Bundle", 96); !ok {
			t.Fatalf("did not reach the verify offer after engraving %d plate(s); got %q", plates, c)
		}
		click(&ctx.Router, Down) // Skip verify
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Passphrase Plate", 96); !ok {
			t.Fatalf("the passphrase-plate offer was not reached after skipping verify; got %q", c)
		}
	})
}

// ─── S6b GATE 2.4a: every device-side mk.Card site sets Fingerprint ──────

// mkCardLiteralsSetFingerprint scans src for every `mk.Card{...}` composite
// literal (via simple brace counting -- the literals in question are single
// nested struct/slice literals, no string containing an unbalanced brace) and
// reports, per occurrence, whether its body contains "Fingerprint:".
func mkCardLiteralsSetFingerprint(src string) []bool {
	var out []bool
	const needle = "mk.Card{"
	for i := 0; i < len(src); {
		j := strings.Index(src[i:], needle)
		if j < 0 {
			break
		}
		start := i + j + len(needle)
		depth := 1
		end := start
		for end < len(src) && depth > 0 {
			switch src[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
			end++
		}
		out = append(out, strings.Contains(src[start:end], "Fingerprint:"))
		i = end
	}
	return out
}

// TestMkCardLiteralScanCatchesAMissingField proves the scanner used by
// TestDeviceMkCardSitesSetFingerprint actually detects an absent
// Fingerprint field, on a synthetic fixture -- so that test's green result
// means what it claims rather than a scanner that always returns true.
func TestMkCardLiteralScanCatchesAMissingField(t *testing.T) {
	src := `mk.Card{Network: "mainnet", Path: p, Stubs: [][4]byte{stub}, Xpub: xpub}`
	got := mkCardLiteralsSetFingerprint(src)
	if len(got) != 1 || got[0] {
		t.Fatalf("mkCardLiteralsSetFingerprint(%q) = %v, want [false]", src, got)
	}
}

// TestDeviceMkCardSitesSetFingerprint is GATE 2.4a (spec S6b §2.4/§7): a
// REGRESSION PIN, not new behaviour -- the spec records this as already true
// ("all three device-side mk.Card sites set it") and S6b's own mechanism
// (§2.4's "key-id" = the master fingerprint, already on the plate) depends
// on it staying true. Green throughout this phase; not red-then-green.
//
// Source-fact assertion, per file: a missing Fingerprint field on any of
// these three sites would mint an mk1 that authenticates no key, silently.
func TestDeviceMkCardSitesSetFingerprint(t *testing.T) {
	for _, file := range []string{
		"singlesig_derive.go",
		"multisig_derive.go",
		"derive_xpub.go",
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		got := mkCardLiteralsSetFingerprint(string(src))
		if len(got) == 0 {
			t.Fatalf("%s: no mk.Card{...} composite literal found -- the file changed shape, update this scan", file)
		}
		for i, ok := range got {
			if !ok {
				t.Errorf("%s: mk.Card literal #%d does not set Fingerprint", file, i)
			}
		}
	}
}

// ─── S6b GATE 2.2: a new syswSource value, with an explicit case in
// syswSourceName ─────────────────────────────────────────────────────────

// TestSrcDerivedRendersItsOwnName is GATE 2.2 (spec S6b §2.2, R-C.1/R-D):
// srcDerived -- "carried from this session's own derivation" -- must render
// its OWN string, not fall through syswSourceName's `default:` arm to "the
// keyboard". That arm is a trap of the F-198 class: a new syswSource value
// added without an explicit `case` compiles clean and fails no test on its
// own -- it just becomes a printed falsehood. Asserting the RENDERED STRING,
// not merely that the value is distinct from srcTyped, is what a missing
// case cannot survive.
func TestSrcDerivedRendersItsOwnName(t *testing.T) {
	got := syswSourceName(srcDerived)
	if got == "the keyboard" {
		t.Fatalf("srcDerived fell through to the default arm and prints %q -- add its case to syswSourceName", got)
	}
	// Pin the exact string so a future edit that changes the wording (without
	// removing the case) is a visible diff here, not a silent drift.
	const want = "this session's own derivation"
	if got != want {
		t.Errorf("syswSourceName(srcDerived) = %q, want %q", got, want)
	}
}

// TestSrcDerivedIsDistinctFromEveryExistingSource is a cheap structural
// guard: srcDerived must be its own enum value, not an alias for
// srcTyped/srcNFC/srcPayload (which would make the case above accidentally
// pass by aliasing an existing, already-correct case).
func TestSrcDerivedIsDistinctFromEveryExistingSource(t *testing.T) {
	for _, other := range []syswSource{srcTyped, srcNFC, srcPayload} {
		if srcDerived == other {
			t.Fatalf("srcDerived == %v -- it must be a distinct enum value", other)
		}
	}
}

// TestSrcDerivedAcceptanceScreenRuns is R-C.3/R-D, at the screen that
// actually draws: "the acceptance screen RUNS" for the preloaded path,
// naming the source, exactly as it does for srcNFC/srcPayload. srcTyped is
// the ONLY value that skips this screen (syswSourceAccept's own short
// circuit); srcDerived is not srcTyped, so flagSource fires
// (gui/sysw_admit.go's syswFlags: `src != srcTyped`) and the screen shows
// "Source: this session's own derivation".
func TestSrcDerivedAcceptanceScreenRuns(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, _, quit := runUITouch(ctx, func() {
		syswSourceAccept(ctx, &descriptorTheme, "BIP-39 Password", sysw.ClassPassphrase, srcDerived)
	})
	defer quit()
	content, ok := pumpUntil(frame, "Source:", 32)
	if !ok {
		t.Fatalf("srcDerived never showed the acceptance screen; got %q", content)
	}
	if !uiContains(content, "this session's own derivation") {
		t.Errorf("acceptance screen does not name srcDerived's source; got %q", content)
	}
	// F4 (NFC-no-integrity) and the two payload-plaintext flags must NOT fire
	// for this source: it is neither srcNFC nor srcPayload.
	if uiContains(content, "no integrity check") {
		t.Errorf("srcDerived raised F4 (NFC-no-integrity), which only applies to srcNFC; got %q", content)
	}
}

// ─── S6b GATE 2.1: the preloaded path presents no fingerprint-entry step,
// and Back from the QR step lands on a real prior step ───────────────────

// TestPreloadedFlowElidesFingerprintSteps is GATE 2.1 (spec S6b §2.1.2). It
// proves both halves normatively required there:
//
//  1. No fingerprint-entry screen ("Seed FP" / "Expected Comb FP") is ever
//     reached between the passphrase entry step and the QR step.
//  2. Back from the QR step lands on a REAL prior step -- the passphrase
//     entry screen, with the preloaded content still there -- not a
//     silent bounce straight back to QR. A "leave the fingerprint cases
//     present but short-circuited" implementation would satisfy (1) (the
//     short-circuited cases execute with no screen, so they cost no visible
//     frame going forward) but FAIL (2): Back would land on the (still
//     present) combined-fingerprint case, which instantly re-executes and
//     falls through BACK to QR within the same frame -- so the assertion
//     that actually catches that class of bug is "the screen after Back is
//     not QR again", not merely "no fingerprint screen was seen".
func TestPreloadedFlowElidesFingerprintSteps(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() {
		engravePassphraseFlowPreloaded(h.ctx, &descriptorTheme, []byte("correct horse"), "A1B2C3D4", "5E6F7A8B", "1A2B3C4D")
	})
	h.mustReach("Source:")
	if !uiContains(h.content, "this session's own derivation") {
		t.Fatalf("acceptance screen does not name srcDerived; got %q", h.content)
	}
	h.tapNav(Button3) // accept

	// Entry screen, pre-filled with the preloaded passphrase. The readout is
	// MASKED by default (no reveal toggle pressed), so the counter is the
	// observable proof of preload: "correct horse" is 13 characters, and
	// "13/100" appears on no other screen in this flow.
	h.mustReach("13/100")
	h.tapWidget("ok")

	// (1) The very next screen is QR -- never a fingerprint-entry screen.
	h.mustReach("QR Code")
	if uiHas(h.content, "Seed FP") || uiHas(h.content, "Expected Comb FP") {
		t.Fatalf("a fingerprint-entry screen appeared on the preloaded path; got %q", h.content)
	}

	// (2) Back from QR must NOT land back on QR (the short-circuit bug) and
	// must land on the real entry screen with its content preserved.
	h.tapNav(Button1)
	if uiHas(h.content, "QR Code") {
		t.Fatalf("Back from the QR step bounced straight back to QR -- the fingerprint steps are present-but-skipped, not elided (spec 2.1.2); got %q", h.content)
	}
	if !uiHas(h.content, "13/100") {
		t.Fatalf("Back from QR did not land on the real entry screen with the passphrase preserved; got %q", h.content)
	}
}

// TestPreloadedFlowSourceIsAlwaysDerived: engravePassphraseFlowPreloaded has
// no src parameter -- it exists ONLY for the preloaded path, so there is no
// provenance to select (spec 2.1's own framing: "the preloaded entry"). This
// is covered structurally by TestSrcDerivedAcceptanceScreenRuns showing
// srcDerived's own screen; this test additionally confirms the flow shows
// NO OTHER source's acceptance wording (an NFC tag / the systemwide
// payload), which would indicate the wrong constant reached
// syswSourceAccept.
func TestPreloadedFlowSourceIsAlwaysDerived(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() {
		engravePassphraseFlowPreloaded(h.ctx, &descriptorTheme, []byte("x"), "A1B2C3D4", "5E6F7A8B", "")
	})
	h.mustReach("Source:")
	for _, wrong := range []string{"an NFC tag", "the systemwide payload", "the keyboard"} {
		if uiHas(h.content, wrong) {
			t.Errorf("acceptance screen names %q, want \"this session's own derivation\"; got %q", wrong, h.content)
		}
	}
}

// TestPreloadedConfirmScreenNamesDerivedProvenance closes the invariant
// ppConfirmWarning's own doc records: "the fingerprint clause deliberately
// echoes the plate's own footer ... so the screen and the steel say the same
// thing." S6b's plate footer becomes provenance-selected (spec 2.3); this
// proves the pre-engrave CONFIRM SCREEN was updated with it, not left
// claiming "typed, not verified" for fingerprints the device just derived --
// not a named P3 gate, but the same R-D truthfulness defect the plate
// footer fix exists to close, on a different screen the spec text did not
// separately enumerate. See ppConfirmWarningDerived's doc for the finding.
func TestPreloadedConfirmScreenNamesDerivedProvenance(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() {
		engravePassphraseFlowPreloaded(h.ctx, &descriptorTheme, []byte("hunter2"), "A1B2C3D4", "5E6F7A8B", "1A2B3C4D")
	})
	h.mustReach("Source:")
	h.tapNav(Button3) // accept
	h.mustReach("7/100")
	h.tapWidget("ok") // accept the preloaded passphrase as-is
	h.mustReach("QR Code")
	h.tapNav(Button3) // accept the QR default (off)
	h.mustReach("Confirm")
	if uiContains(h.content, "typed, not verified") {
		t.Fatalf("the preloaded confirm screen claims the fingerprints are typed; got %q", h.content)
	}
	if !uiContains(h.content, "derived by this device") {
		t.Fatalf("the preloaded confirm screen does not claim the fingerprints were derived; got %q", h.content)
	}
}

// ─── S6b whole-diff review C1: an edit at the preloaded entry step must
// never reach the plate ──────────────────────────────────────────────────

// TestPreloadedEntryEditIsRefusedNotEngraved closes C1
// (design/agent-reports/s6b-whole-diff-review.md): engravePassphraseFlowPreloaded's
// entry step hands the preloaded passphrase to the same fully EDITABLE
// keyboard the typed path uses -- editing is its whole function -- and
// nothing downstream re-checks the operator's answer against what the
// wallet was actually derived with: every claim on the plate (seedFP,
// combinedFP, policyID, the DERIVED footer) is for the ORIGINAL
// derivation, unconditionally. This drives a real edit -- backspace one
// character, type a different one, exactly the "fix a typo that wasn't
// there" gesture the finding describes -- and proves, via
// passphrasePlateHook (the only place the flow's own actual argument to
// ppBuildPlate is visible), what would really reach the plate.
//
// Before the fix this reaches ppBuildPlate with the EDITED bytes, under
// the ORIGINAL wallet's fingerprints, stamped DERIVED -- the wallet's
// TRUE passphrase recorded nowhere. After the fix, the edit is refused
// with a truthful message, the buffer is reloaded with the true
// passphrase (R-C: the operator does not have to retype it), and only
// THAT reaches ppBuildPlate.
func TestPreloadedEntryEditIsRefusedNotEngraved(t *testing.T) {
	var gotSecret []byte
	var hookCalls int
	passphrasePlateHook = func(secret []byte, seedFP, combinedFP string, qr bool) {
		gotSecret = bytes.Clone(secret)
		hookCalls++
	}
	t.Cleanup(func() { passphrasePlateHook = nil })

	body := []byte("hunter2")
	h := newPPHarness(t)
	h.start(func() {
		engravePassphraseFlowPreloaded(h.ctx, &descriptorTheme, body, "A1B2C3D4", "5E6F7A8B", "1A2B3C4D")
	})
	h.mustReach("Source:")
	h.tapNav(Button3) // accept srcDerived
	h.mustReach("7/100")

	// The operator "corrects" a character that was never wrong:
	// "hunter2" -> "hunter3".
	h.backspace(1)
	h.typeRune('3')
	h.mustReach("7/100")
	h.tapWidget("ok")

	// The edit must be caught HERE, with a truthful explanation -- not
	// silently carried forward to the QR step.
	h.mustReach("changed")
	if uiContains(h.content, "QR Code") {
		t.Fatalf("the edited passphrase reached the QR step instead of being refused; got %q", h.content)
	}
	h.tapNav(Button3) // dismiss the refusal

	// The buffer must be back to the TRUE passphrase, not left holding the
	// edit.
	h.mustReach("7/100")
	h.tapWidget("ok") // accept the restored, correct passphrase this time
	h.mustReach("QR Code")
	h.tapNav(Button3) // QR default (off)
	h.mustReach("Confirm")
	h.tapWidget("ok")
	for i := 0; i < 8 && hookCalls == 0; i++ {
		h.step()
	}
	if hookCalls == 0 {
		t.Fatal("ppBuildPlate was never called")
	}
	if !bytes.Equal(gotSecret, body) {
		t.Fatalf("the plate would carry %q under this wallet's OWN fingerprints, stamped DERIVED -- want the passphrase it was actually derived with, %q", gotSecret, body)
	}
}

// ─── S6b whole-diff review C2: an over-length wallet passphrase never
// reaches the preloaded plate builder, truncated or otherwise ───────────

// TestPassphrasePlateOfferRefusesOverLengthPassphrase closes C2
// (design/agent-reports/s6b-whole-diff-review.md): the wallet-derivation
// passphrase itself is unbounded -- passphraseFlowTitled (gui.go) and the
// payload branch of syswPassphraseFlowTitled (sysw_source.go) accept any
// length, because any length is fine for DERIVATION.
// engravePassphraseFlowPreloaded's buffer is exactly passphrase.MaxLen
// bytes, and its copy() truncates SILENTLY -- so, pre-fix, a >100-char
// wallet passphrase would derive the real wallet, then engrave a
// TRUNCATED passphrase under the FULL passphrase's fingerprints, stamped
// DERIVED. This proves the offer refuses BEFORE any of that: neither the
// ~31s bare-seed KDF nor engravePassphraseFlowPreloaded's own
// buffer/plate builder is ever reached, and the operator sees a truthful
// reason instead of a plate offer it cannot honestly fulfil.
func TestPassphrasePlateOfferRefusesOverLengthPassphrase(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	long := strings.Repeat("a", passphrase.MaxLen+1)
	full, masterFP, _, _, err := deriveSingleSigBundle(m, long, &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}

	var bareSeedCalls, secretCalls int
	singleSigBareSeedFPHook = func() { bareSeedCalls++ }
	passphraseSecretHook = func(secret []byte) { secretCalls++ }
	t.Cleanup(func() {
		singleSigBareSeedFPHook = nil
		passphraseSecretHook = nil
	})

	ctx := NewContext(newPlatform())
	var result passphrasePlateResult
	done := false
	frame, quit := runUI(ctx, func() {
		result = singleSigPassphrasePlateOffer(ctx, &descriptorTheme, m, long, masterFP, path, full)
		done = true
	})
	defer quit()
	// The refusal must reach the operator WITHOUT the "Engrave a
	// passphrase plate?" offer ever being shown -- there is nothing a
	// Skip/Engrave choice could mean for a passphrase that cannot fit a
	// plate at all.
	if c, ok := pumpUntil(frame, "100 characters", 32); !ok {
		t.Fatalf("no refusal reached the operator; got %q", c)
	}
	click(&ctx.Router, Button3) // dismiss
	for i := 0; i < 16 && !done; i++ {
		frame()
	}
	if !done {
		t.Fatal("singleSigPassphrasePlateOffer never returned")
	}
	if result != passphrasePlateNotCut {
		t.Fatalf("result = %v, want passphrasePlateNotCut", result)
	}
	if bareSeedCalls != 0 {
		t.Errorf("the ~31s bare-seed derivation ran %d times for a passphrase that can never reach a plate, want 0", bareSeedCalls)
	}
	if secretCalls != 0 {
		t.Errorf("engravePassphraseFlowPreloaded's own truncating buffer was allocated %d times -- the over-length passphrase reached it, want 0", secretCalls)
	}
}
