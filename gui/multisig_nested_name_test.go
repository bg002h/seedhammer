package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S3: nested segwit must be NAMEABLE (SPEC §4.4, plan §3 S3) ──────────────
//
// The defect is that TWO of the three sh-rooted names are EQUAL, so a test that
// only checked "P2SH-P2WSH" would have passed against the broken code the moment
// the string existed anywhere. What has to be asserted is PAIRWISE DISTINCTNESS,
// plus each exact string, because either half alone is satisfiable by a wrong
// implementation:
//
//   - distinctness alone is satisfied by "A"/"B"/"C";
//   - the exact strings alone are satisfied by a table that never runs
//     (a switch arm that cannot be reached still type-checks).
//
// The three cases are the three rows of SPEC §4.4.

// scriptNameCase is one row of SPEC §4.4's table, with the template shaped
// exactly as md.Decode would hand it over (md/md.go:1212-1225: InnerWsh is
// meaningful only when Root==ScriptSh; InnerWpkh only when Root==ScriptSh &&
// Policy==PolicySingle).
type scriptNameCase struct {
	label string
	tpl   md.Template
	want  string
}

func shScriptNameCases() []scriptNameCase {
	return []scriptNameCase{
		{
			label: "sh(wsh(sortedmulti)) nested segwit",
			tpl:   md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true, InnerWsh: true},
			want:  "P2SH-P2WSH",
		},
		{
			label: "sh(wpkh) nested single-key",
			tpl:   md.Template{N: 1, Root: md.ScriptSh, Policy: md.PolicySingle, Renderable: true, InnerWpkh: true},
			want:  "P2SH-P2WPKH",
		},
		{
			label: "sh(sortedmulti) bare legacy",
			tpl:   md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true},
			want:  "P2SH",
		},
	}
}

// TestScriptNameDistinguishesNestedFromLegacy is SPEC §4.4: scriptName takes the
// whole Template and the three sh-rooted names are PAIRWISE DISTINCT.
func TestScriptNameDistinguishesNestedFromLegacy(t *testing.T) {
	cases := shScriptNameCases()

	for _, c := range cases {
		if got := scriptName(c.tpl); got != c.want {
			t.Errorf("scriptName(%s) = %q, want %q", c.label, got, c.want)
		}
	}

	// PAIRWISE DISTINCT. Reported as a pair, because the whole defect is that a
	// pair collided and a single-value assertion cannot see that.
	for i := range cases {
		for j := i + 1; j < len(cases); j++ {
			a, b := scriptName(cases[i].tpl), scriptName(cases[j].tpl)
			if a == b {
				t.Errorf("%s and %s BOTH render %q: two wallets that hash to different "+
					"addresses are indistinguishable on the restore doc (SPEC 2.2 D-3)",
					cases[i].label, cases[j].label, a)
			}
		}
	}

	// The non-sh roots must not have moved: they share the function.
	for _, c := range []struct {
		tpl  md.Template
		want string
	}{
		{md.Template{Root: md.ScriptWsh, Policy: md.PolicySortedMulti, Renderable: true}, "P2WSH"},
		{md.Template{Root: md.ScriptPkh, Policy: md.PolicySingle, Renderable: true}, "P2PKH"},
		{md.Template{Root: md.ScriptTr, Policy: md.PolicySingle, Renderable: true}, "P2TR"},
		{md.Template{Root: md.ScriptWpkh, Policy: md.PolicySingle, Renderable: true}, "P2WPKH"},
	} {
		if got := scriptName(c.tpl); got != c.want {
			t.Errorf("scriptName(root %v) = %q, want %q (unrelated root regressed)", c.tpl.Root, got, c.want)
		}
	}
}

// buildAssembledMd1 assembles a REAL full-policy 2-of-3 md1 through the shipped
// producer (assembleBuildPolicy -> md.EncodeMultisig), the way
// buildMultisigPolicyFlow does at gui/multisig_build.go:175. Nothing here is
// hand-built: the restore doc must be exercised over bytes the device would
// actually engrave.
func buildAssembledMd1(t *testing.T, script md.MultisigScript) []string {
	t.Helper()
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks(fixture): %v", err)
	}
	selfXpub, selfFP, err := deriveAccountXpub(abandonAboutMnemonic(), "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub: %v", err)
	}
	card0 := mk.Card{Network: "mainnet", Path: "m/48h/0h/0h/2h", Xpub: xpubFromExpandedKey(t, keys[0]), Stubs: [][4]byte{{0, 0, 0, 0}}}
	card2 := mk.Card{Network: "mainnet", Path: "m/48h/0h/0h/2h", Xpub: xpubFromExpandedKey(t, keys[2]), Stubs: [][4]byte{{0, 0, 0, 0}}}
	out, _, _, err := assembleBuildPolicy(
		buildPolicyParams{Script: script, N: 3, K: 2, SelfSlots: []int{1}, IncludeFp: false},
		selfKeyAt(1, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{card0, card2})
	if err != nil {
		t.Fatalf("assembleBuildPolicy(%v): %v", script, err)
	}
	return out
}

// restoreDocBlob runs the restore doc's PRODUCTION line builder
// (gui/multisig_restore.go:22 multisigRestoreLines, whose desc4Display at :50 is
// the scriptName call site) over an assembled md1 and returns what the screen
// would read.
func restoreDocBlob(t *testing.T, script md.MultisigScript) string {
	t.Helper()
	tpl, keys, err := md.ExpandWalletPolicyChunks(buildAssembledMd1(t, script))
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks(assembled): %v", err)
	}
	lines, _, err := multisigRestoreLines(tpl, keys)
	if err != nil {
		t.Fatalf("multisigRestoreLines: %v", err)
	}
	// The screen draws one line per element with no separator that carries
	// meaning, and chunkString splits mid-word, so the blob is joined WITHOUT a
	// separator — exactly how cmd/emu's shScreen() + squash() read it. Joining on
	// "\n" would let an assertion pass on a name split across two lines that the
	// operator would never see whole.
	return strings.Join(lines, "")
}

// TestRestoreDocNamesNestedSegwit is the surface that matters: the operator reads
// the restore doc years later, alone (SPEC §4.4, "the restore document is the one
// that matters most"; plan §3 S3 test 2).
//
// It drives gui/multisig_restore.go's PRODUCTION path over a REAL assembled md1
// rather than calling desc4Display on a hand-built Template, because the two are
// not the same question. MEASURED 2026-08-15 while writing this test: a
// full-policy build reaches multisigRestoreLines' expandOK branch, which renders
// the descriptor and NEVER calls desc4Display — so a test that only exercised
// desc4Display would go green while the screen the walk reads said nothing about
// the script type at all.
func TestRestoreDocNamesNestedSegwit(t *testing.T) {
	nested := restoreDocBlob(t, md.MultisigShWsh)
	legacy := restoreDocBlob(t, md.MultisigSh)
	native := restoreDocBlob(t, md.MultisigWsh)

	if !strings.Contains(nested, "P2SH-P2WSH") {
		t.Errorf("the sh(wsh) restore doc never names P2SH-P2WSH:\n%s", nested)
	}
	// "P2SH-P2WSH" CONTAINS "P2SH", so the legacy arm cannot be checked by
	// absence of "P2SH". Check that the nested name is absent and the bare one
	// present, which is the only pair of conditions that separates them.
	if strings.Contains(legacy, "P2SH-P2WSH") {
		t.Errorf("the bare sh(sortedmulti) restore doc claims nested segwit:\n%s", legacy)
	}
	if !strings.Contains(legacy, "P2SH 2-of-3 multisig (sorted)") {
		t.Errorf("the bare sh(sortedmulti) restore doc never names P2SH:\n%s", legacy)
	}
	if !strings.Contains(native, "P2WSH 2-of-3 multisig (sorted)") {
		t.Errorf("the wsh restore doc never names P2WSH:\n%s", native)
	}

	// The addresses must still be there and must still DIFFER — a type line that
	// arrived by dropping the descriptor/address content would be a regression,
	// not a fix.
	for name, blob := range map[string]string{"nested": nested, "legacy": legacy, "native": native} {
		if !strings.Contains(blob, "First receive:") || !strings.Contains(blob, "First change:") {
			t.Errorf("%s restore doc lost its addresses:\n%s", name, blob)
		}
		if strings.Contains(blob, "xprv") {
			t.Errorf("%s restore doc leaked an xprv", name)
		}
	}
	if nested == legacy {
		t.Error("the nested and legacy restore docs are byte-identical: two wallets " +
			"that hash to different addresses read the same on steel (SPEC 2.2 D-3)")
	}

	// desc4Display itself (gui/multisig_restore.go:50), which is the named call
	// site, on the shapes that reach it through the display-only branch.
	cases := shScriptNameCases()
	for i := range cases {
		for j := i + 1; j < len(cases); j++ {
			a, b := desc4Display(cases[i].tpl), desc4Display(cases[j].tpl)
			if a == b {
				t.Errorf("desc4Display: %s and %s BOTH read %q", cases[i].label, cases[j].label, a)
			}
		}
	}
}

// TestRestoreDocNestedNameIsActuallyDrawn is the F-151/F-179 counter-measure for
// the one screen S3 changed.
//
// EVERY text assertion in this package proves a string was SUBMITTED for drawing
// and never that it was drawn: ExtractText walks the op tree, so a line the panel
// renders as nothing still "appears" (gui/raster_test.go:11). The same is true of
// the emulator's shScreen(), which is the same extraction, so S3's walk is blind
// to it too. A name that is on the restore doc only in the op tree is a name the
// operator does not have.
//
// So this rasterises. The floor is buildWalkRasterFloor rather than
// assertFrameHasBody's 4000: the restore doc draws THREE nav buttons (Back, page,
// Done), and gui/multisig_build_walk_test.go's calibration measured a title-only
// frame with three nav buttons at 5482 px — above 4000, so the weaker floor would
// pass a completely blank body on this screen.
func TestRestoreDocNestedNameIsActuallyDrawn(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	tpl, keys, err := md.ExpandWalletPolicyChunks(buildAssembledMd1(t, md.MultisigShWsh))
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks(assembled): %v", err)
	}
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		// The status is the production placeholder (S6a step 4), not "": this test
		// rasterises the document the operator is shown, and a document with an
		// empty first line is not one any call site produces.
		multisigRestoreDocFlow(ctx, &descriptorTheme, tpl, keys, verifyStatusNotFullyCheckedLine, nil)
	})
	defer quit()
	content, ok := pumpUntil(frame, "P2SH-P2WSH", 64)
	if !ok {
		t.Fatalf("the restore doc never showed the nested-segwit name; last frame %q", content)
	}
	t.Logf("restore doc (sh(wsh)) ink = %d px, floor %d", ink(), buildWalkRasterFloor)
	if ink() < buildWalkRasterFloor {
		t.Errorf("the restore doc drew only %d ink pixels (floor %d): the name is in "+
			"the op tree but not on the glass, which is the shape a text assertion "+
			"cannot see", ink(), buildWalkRasterFloor)
	}
}
