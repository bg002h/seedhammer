package gui

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── Task 6: single-sig template-engrave transform ───────────────────────────

// TestTemplateizeBundle: templateizeBundle strips the md1 to a keyless template
// and re-mints the mk1 stub on the template's WDT-Id, so the resulting bundle
// VERIFIES against itself (the device's own readback, C2) and the md1 is keyless.
func TestTemplateizeBundle(t *testing.T) {
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

	// md1 still decodes (keyless template).
	if _, err := md.Reassemble(tmpl.MD1); err != nil {
		t.Fatalf("Reassemble template md1: %v", err)
	}

	// ms1 leg is unchanged (still the secret seed backup).
	if tmpl.MS1 != full.MS1 {
		t.Errorf("templateize changed ms1 (should be untouched)")
	}

	// The template bundle VERIFIES against itself (own readback, C2).
	if err := bundle.Verify(tmpl, tmpl); err != nil {
		t.Fatalf("template bundle own-readback verify failed: %v (want PASS)", err)
	}

	// The mk1 stub now roots on the WDT-Id (form-aware), and the md1 is keyless:
	// the full md1's stub (WalletPolicyId) must DIFFER from the template stub.
	fullStub, err := md.FormAwareStubChunks(full.MD1)
	if err != nil {
		t.Fatal(err)
	}
	tmplStub, err := md.FormAwareStubChunks(tmpl.MD1)
	if err != nil {
		t.Fatal(err)
	}
	if fullStub == tmplStub {
		t.Error("template stub equals full-policy stub; the strip did not change the id space")
	}
	// The re-minted mk1 carries the template stub.
	card, err := mk.Decode(tmpl.MK1)
	if err != nil {
		t.Fatalf("mk.Decode template mk1: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != tmplStub {
		t.Fatalf("template mk1 stubs = %v, want [%x]", card.Stubs, tmplStub)
	}
	// xpub/path/fingerprint preserved from the full card.
	fullCard, _ := mk.Decode(full.MK1)
	if card.Xpub != fullCard.Xpub || card.Path != fullCard.Path || card.Fingerprint != fullCard.Fingerprint {
		t.Error("templateize altered the mk1 xpub/path/fingerprint (only the stub should change)")
	}
}

// TestEngraveSingleSigFlowTemplate: selecting "Template-only md1" shows the
// loud warning + estimate strings, then engraves the template bundle (full mode:
// 3 cards). Asserts the load-bearing consent strings render (S4/S6).
func TestEngraveSingleSigFlowTemplate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
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
		click(&ctx.Router, Button3) // Skip passphrase
		if c, ok := pumpUntil(frame, "Engrave Mode", 64); !ok {
			t.Fatalf("did not reach the full/watch-only choice; got %q", c)
		}
		click(&ctx.Router, Button3) // Full
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		// Wallet policy: Template-only md1 (choice 1).
		click(&ctx.Router, Down)
		frame()
		click(&ctx.Router, Button3)
		// The loud warning + estimate appears.
		if c, ok := pumpUntil(frame, "TEMPLATE-ONLY md1", 64); !ok {
			t.Fatalf("template warning not shown; got %q", c)
		}
		// The recovery estimate is later on the paginated review screen — page
		// forward (Button2) until the sortedmulti estimate line renders.
		found := false
		for i := 0; i < 6 && !found; i++ {
			click(&ctx.Router, Button2) // next page
			if c, ok := pumpUntil(frame, "sortedmulti", 16); ok {
				found = true
			} else {
				_ = c
			}
		}
		if !found {
			t.Fatal("recovery estimate (sortedmulti line) not shown across the review pages")
		}
		// Confirm "I understand" → engrave (full = 3 cards).
		click(&ctx.Router, Button3)
		// S6a/F-202: the pre-engrave plate census stands between the template
		// consent and the first plate. pumpUntil never presses and
		// confirmReviewScreen loops until it is pressed, so this walk parks here
		// for its whole budget without this press.
		if c, ok := pumpUntil(frame, "Plates To Cut", 64); !ok {
			t.Fatalf("the plate census was not shown before the engrave; got %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 3", 64); !ok {
			t.Fatalf("template engrave did not reach the 3-card engrave; got %q", c)
		}
	})
}

// TestMultisigBuildTemplateBinding (C2, build path): assemble a 2-of-3
// wsh-sortedmulti on-device, strip it to a keyless template, derive the
// operator's self leg over the TEMPLATE md1 (as buildMultisigPolicyFlow does),
// and confirm the self mk1 binds to the template's WDT-Id (NOT the full
// WalletPolicyId). This is the multisig build-path own-readback.
func TestMultisigBuildTemplateBinding(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	self := abandonAboutMnemonic()
	selfXpub, selfMasterFP, err := deriveAccountXpub(self, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub(self): %v", err)
	}
	card0 := mk.Card{Network: "mainnet", Path: "m/48h/0h/0h/2h", Xpub: xpubFromExpandedKey(t, keys[0]), Stubs: [][4]byte{{0, 0, 0, 0}}}
	card2 := mk.Card{Network: "mainnet", Path: "m/48h/0h/0h/2h", Xpub: xpubFromExpandedKey(t, keys[2]), Stubs: [][4]byte{{0, 0, 0, 0}}}
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{1}, IncludeFp: false}
	assembled, fullStub, _, err := assembleBuildPolicy(p, selfKeyAt(1, selfXpub, selfMasterFP, multisigSharedOrigin()), []mk.Card{card0, card2})
	if err != nil {
		t.Fatalf("assembleBuildPolicy: %v", err)
	}

	// Strip to the keyless template, then derive the self leg over it (the build
	// flow passes the stripped md1 to deriveMultisigLeg).
	tmplMd1, err := md.StripToTemplate(assembled)
	if err != nil {
		t.Fatalf("StripToTemplate: %v", err)
	}
	b, err := deriveMultisigLeg(self, "", &chaincfg.MainNetParams, multisigSharedOrigin(), tmplMd1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(template): %v", err)
	}

	tmplStub, err := md.FormAwareStubChunks(tmplMd1)
	if err != nil {
		t.Fatal(err)
	}
	if tmplStub == fullStub {
		t.Error("template stub equals the full WalletPolicyId stub; strip did not change id space")
	}
	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != tmplStub {
		t.Fatalf("self mk1 stubs = %v, want [%x] (WDT-Id, C2)", card.Stubs, tmplStub)
	}
	// The template md1 is keyless.
	dt, err := md.DecodeChunks(tmplMd1)
	if err != nil {
		t.Fatalf("DecodeChunks(template): %v", err)
	}
	if dt.Policy != md.PolicySortedMulti || dt.N != 3 {
		t.Errorf("template classify Policy=%d N=%d, want sortedmulti N=3", dt.Policy, dt.N)
	}
}

// TestTemplateConsentLines: a classifiable sortedmulti shows the full k-of-N +
// slot count; a PolicyComplex depth-≥2 taptree shows the honest-minimal consent
// + the EXPERIMENTAL >13.1.0/PR #953 warning (C3/S5).
func TestTemplateConsentLines(t *testing.T) {
	// Classifiable: wsh-sortedmulti 2-of-2.
	wsh := mustTemplateMD1(t, "../md/testdata/template/wsh_sortedmulti.tmpl.md1.txt")
	tmplWsh, err := md.DecodeChunks(wsh)
	if err != nil {
		t.Fatal(err)
	}
	stub, _ := md.FormAwareStubChunks(wsh)
	lines := templateConsentLines(tmplWsh, stub, 0, md.PolicyShape{})
	if !containsLineSubstr(lines, "sortedmulti") {
		t.Errorf("classifiable consent missing k-of-N label: %v", lines)
	}
	if containsLineSubstr(lines, "EXPERIMENTAL") {
		t.Errorf("non-taproot consent must NOT show the experimental gate: %v", lines)
	}

	// Complex depth-≥2: tr4.
	tr4 := mustTemplateMD1(t, "../md/testdata/template/tr4_depth2.tmpl.md1.txt")
	tmplTr4, err := md.DecodeChunks(tr4)
	if err != nil {
		t.Fatal(err)
	}
	if tmplTr4.Renderable {
		t.Fatal("tr4 must be PolicyComplex / non-renderable")
	}
	depth, _ := md.TapTreeDepthChunks(tr4)
	stub4, _ := md.FormAwareStubChunks(tr4)
	shape4, err := md.PolicyShapeChunks(tr4)
	if err != nil {
		t.Fatalf("PolicyShapeChunks: %v", err)
	}
	lines4 := templateConsentLines(tmplTr4, stub4, depth, shape4)
	if !containsLineSubstr(lines4, "COMPLEX POLICY") {
		t.Errorf("complex consent missing honest-minimal header: %v", lines4)
	}
	if !containsLineSubstr(lines4, "Key slots") {
		t.Errorf("complex consent missing slot count: %v", lines4)
	}
	// THE DEPTH-2 CAVEAT IS A MINIMUM VERSION NOW, not an impossibility claim
	// (2026-08-20, with the miniscript pin move). It must still APPEAR -- an
	// operator's backup outlives the build that made it, and an older md or
	// toolkit genuinely cannot reconstruct this taptree.
	if !containsLineSubstr(lines4, "depth >= 2") {
		t.Errorf("depth-2 consent lost its taproot caveat entirely: %v", lines4)
	}
	if !containsLineSubstr(lines4, "953") {
		t.Errorf("depth-2 caveat no longer names the upstream fix: %v", lines4)
	}
	if !containsLineSubstr(lines4, "md 0.13+") || !containsLineSubstr(lines4, "toolkit 0.97+") {
		t.Errorf("depth-2 caveat must name the MINIMUM versions that can recover: %v", lines4)
	}
	// And it must no longer say the thing that stopped being true. A warning
	// naming a fixed blocker teaches the operator to discount the next one.
	for _, stale := range []string{"UNRELEASED", "until that ships", "shipped toolkit CANNOT"} {
		if containsLineSubstr(lines4, stale) {
			t.Errorf("depth-2 caveat still carries the superseded claim %q: %v", stale, lines4)
		}
	}
}

func containsLineSubstr(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func mustTemplateMD1(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Fields(string(raw))
}

// TestComplexConsentShowsAStructuralSummary is Stage 2's screen contract.
//
// Before this, a complex policy got script family + slot count + template-id and
// "Cannot fully display on-device." — correct, and nearly contentless. The
// summary adds spend-path structure walked from the decoded tree.
func TestComplexConsentShowsAStructuralSummary(t *testing.T) {
	// tr(@0,{{pk(@1),pk(@2)},pk(@3)}) — a spendable key-path and three leaves.
	shape := md.PolicyShape{
		Complete: true,
		KeyPath:  md.KeyPathSpendable,
		TapDepth: 2,
		Branches: []md.Branch{
			{Keys: 1, Depth: 2},
			{Keys: 1, Depth: 2},
			{K: 2, N: 3, Keys: 3, Depth: 1, Timelock: true},
		},
	}
	lines := templateConsentLines(md.Template{N: 4}, [4]byte{0xde, 0xad, 0xbe, 0xef}, 2, shape)
	joined := strings.Join(lines, "\n")

	// THE KEY-PATH LINE IS THE ONE THAT MUST NOT BE MISSING: a spendable
	// internal key moves funds without satisfying any leaf.
	if !strings.Contains(joined, "A KEY CAN SPEND ALONE") {
		t.Errorf("the spendable key-path is not stated:\n%s", joined)
	}
	if !strings.Contains(joined, "Spend paths: 3") {
		t.Errorf("the spend-path count is missing:\n%s", joined)
	}
	if !strings.Contains(joined, "tree depth 2") {
		t.Errorf("the taptree depth is missing:\n%s", joined)
	}
	if !strings.Contains(joined, "2-of-3 +timelock") {
		t.Errorf("the threshold branch and its timelock are missing:\n%s", joined)
	}
	// A branch that is NOT a plain threshold must not be described as one.
	if strings.Contains(joined, "1-of-1") {
		t.Errorf("a non-threshold branch was rendered as 1-of-1:\n%s", joined)
	}
	// The summary REPLACES the contentless line when it can be given.
	if strings.Contains(joined, "Cannot fully display on-device.") {
		t.Errorf("a complete summary still claimed it cannot display:\n%s", joined)
	}
	// And the verify instruction survives regardless — a summary is not a
	// substitute for checking against the coordinator.
	if !strings.Contains(joined, "VERIFY against your coordinator") {
		t.Errorf("the verify instruction was dropped:\n%s", joined)
	}
}

// TestIncompleteShapeShowsNoSummaryAtAll is the honesty contract at the screen.
//
// If the walk could not classify every node, the operator must get the old
// honest-minimal wording and NOT a partial spend-path list — believing you have
// seen the whole policy is the failure mode SPEC §4.2/C3 names.
func TestIncompleteShapeShowsNoSummaryAtAll(t *testing.T) {
	partial := md.PolicyShape{
		Complete: false,
		KeyPath:  md.KeyPathSpendable,
		Branches: []md.Branch{{K: 2, N: 3, Keys: 3}},
	}
	lines := templateConsentLines(md.Template{N: 3}, [4]byte{1, 2, 3, 4}, 1, partial)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "Cannot fully display on-device.") {
		t.Errorf("an incomplete walk lost the honest-minimal wording:\n%s", joined)
	}
	for _, leaked := range []string{"Spend path", "2-of-3", "A KEY CAN SPEND ALONE"} {
		if strings.Contains(joined, leaked) {
			t.Errorf("an INCOMPLETE summary leaked %q to the screen:\n%s", leaked, joined)
		}
	}
}
