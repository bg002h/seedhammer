package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestSingleSigPickDefaultBIP84: the first screen offers BIP-84 (native segwit)
// as the one-tap default; confirming it (Button3) yields purpose 84 /
// ScriptWpkh / m/84'/0'/0'.
func TestSingleSigPickDefaultBIP84(t *testing.T) {
	ctx := NewContext(newPlatform())
	var gotPurpose int
	var gotScript md.ScriptKind
	var gotOK bool
	done := false
	frame, quit := runUI(ctx, func() {
		gotPurpose, gotScript, gotOK = singleSigPickFlow(ctx, &descriptorTheme)
		done = true
	})
	defer quit()
	frame() // first (default) screen
	// Default entry is the first one (BIP-84); confirm with Button3.
	click(&ctx.Router, Button3)
	for i := 0; i < 8 && !done; i++ {
		frame()
	}
	if !gotOK {
		t.Fatal("picker did not resolve the default")
	}
	if gotPurpose != 84 || gotScript != md.ScriptWpkh {
		t.Fatalf("default = purpose %d / script %v, want 84 / ScriptWpkh", gotPurpose, gotScript)
	}
	if got := singleSigPath(gotPurpose).String(); got != "m/84h/0h/0h" {
		t.Fatalf("default path = %q, want m/84h/0h/0h", got)
	}
}

// TestSingleSigPickAdvanced: the "Advanced…" entry on the first screen opens a
// second ChoiceScreen of the three non-default single-sig types; each resolves
// to the correct purpose + ScriptKind + path.
func TestSingleSigPickAdvanced(t *testing.T) {
	cases := []struct {
		name       string
		advDowns   int // Downs in the Advanced submenu to reach the entry
		wantPurp   int
		wantScript md.ScriptKind
		wantPath   string
	}{
		{"bip44 pkh", 0, 44, md.ScriptPkh, "m/44h/0h/0h"},
		{"bip49 sh-wpkh", 1, 49, md.ScriptShWpkh, "m/49h/0h/0h"},
		{"bip86 tr", 2, 86, md.ScriptTr, "m/86h/0h/0h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			var gotPurpose int
			var gotScript md.ScriptKind
			var gotOK bool
			done := false
			frame, quit := runUI(ctx, func() {
				gotPurpose, gotScript, gotOK = singleSigPickFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			frame() // default screen
			// Move down to the "Advanced…" entry (last entry on the first screen)
			// and confirm it to open the submenu.
			chooseEntry(frame, &ctx.Router, advancedEntryIndex())
			// In the submenu, move down to the target entry and confirm.
			chooseEntry(frame, &ctx.Router, c.advDowns)
			for i := 0; i < 8 && !done; i++ {
				frame()
			}
			if !gotOK {
				t.Fatalf("picker did not resolve %s", c.name)
			}
			if gotPurpose != c.wantPurp || gotScript != c.wantScript {
				t.Fatalf("%s = purpose %d / script %v, want %d / %v", c.name, gotPurpose, gotScript, c.wantPurp, c.wantScript)
			}
			if got := singleSigPath(gotPurpose).String(); got != c.wantPath {
				t.Fatalf("%s path = %q, want %q", c.name, got, c.wantPath)
			}
		})
	}
}

// TestSingleSigPickBackFromAdvanced: Back (Button1) from the Advanced submenu
// returns to the default screen; the operator can then pick the default.
func TestSingleSigPickBackFromAdvanced(t *testing.T) {
	ctx := NewContext(newPlatform())
	var gotPurpose int
	var gotScript md.ScriptKind
	var gotOK bool
	done := false
	frame, quit := runUI(ctx, func() {
		gotPurpose, gotScript, gotOK = singleSigPickFlow(ctx, &descriptorTheme)
		done = true
	})
	defer quit()
	frame()
	// Open Advanced.
	chooseEntry(frame, &ctx.Router, advancedEntryIndex())
	// Back from the submenu → back to the default screen.
	click(&ctx.Router, Button1)
	frame()
	// Now pick the default (BIP-84): the selection resets to entry 0 on re-show,
	// so a single Button3 confirms the default.
	click(&ctx.Router, Button3)
	for i := 0; i < 8 && !done; i++ {
		frame()
	}
	if !gotOK {
		t.Fatal("picker did not resolve after Back from Advanced")
	}
	if gotPurpose != 84 || gotScript != md.ScriptWpkh {
		t.Fatalf("after Back from Advanced, default = purpose %d / script %v, want 84 / ScriptWpkh", gotPurpose, gotScript)
	}
}

// TestSingleSigPickBackFromDefault: Back (Button1) from the default screen →
// ok=false (the operator cancelled the picker).
func TestSingleSigPickBackFromDefault(t *testing.T) {
	ctx := NewContext(newPlatform())
	var gotOK bool
	done := false
	frame, quit := runUI(ctx, func() {
		_, _, gotOK = singleSigPickFlow(ctx, &descriptorTheme)
		done = true
	})
	defer quit()
	frame()
	click(&ctx.Router, Button1) // Back from the default screen.
	for i := 0; i < 8 && !done; i++ {
		frame()
	}
	if gotOK {
		t.Fatal("Back from the default screen should yield ok=false")
	}
}
