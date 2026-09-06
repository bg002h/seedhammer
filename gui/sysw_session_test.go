package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/codex32"
	"seedhammer.com/hashlock"
	"seedhammer.com/sysw"
)

// ─── H6 Task 11 (spec §9, §8.8) ──────────────────────────────────────────────

// h6MS1Shaped is the shortest string hashlock.IsMS1Shaped answers true for:
// the `ms1` prefix, bech32 characters and minMS1Len = 48 of them.
//
// SHORT DELIBERATELY. The tests below type it one key at a time on the real
// keyboard, so a 74-character plate string would cost 74 taps to prove the same
// predicate. The GROUPED case, which is the one the predicate exists for, uses
// a REAL plate string -- see TestHashlockMS1WarningFiresOnAGroupedPlate.
const h6MS1Shaped = "ms1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

func TestH6MS1ShapedFixtureIsWhatItClaims(t *testing.T) {
	if len(h6MS1Shaped) != 48 {
		t.Fatalf("the fixture is %d characters; minMS1Len is 48", len(h6MS1Shaped))
	}
	if !hashlock.IsMS1Shaped(h6MS1Shaped) {
		t.Fatal("the fixture is not ms1-shaped, so every test using it proves nothing")
	}
	if hashlock.IsMS1Shaped(h6MS1Shaped[:47]) {
		t.Fatal("one character short is still ms1-shaped: the predicate is not the host's")
	}
}

// TestSyswWarnMS1ShapedFiresOnceAndIsReArmedByAnEdit is §9's "once per
// composition, re-armed by an edit", at the helper both programs call.
//
// MUTATION: drop the `accepted` bookkeeping -> the warning fires on every OK,
// and an operator who deliberately cuts an ms1 string as text is asked again at
// every pass through the entry screen.
// MUTATION: keep `accepted` but never compare it to the CURRENT text -> the
// edited row below is silent, and the operator is not warned about the string
// they actually typed.
func TestSyswWarnMS1ShapedFiresOnceAndIsReArmedByAnEdit(t *testing.T) {
	type run struct {
		text  string
		drawn bool
		out   bool
	}
	var accepted string
	do := func(t *testing.T, text string, confirm bool) run {
		t.Helper()
		r := run{text: text}
		synctest.Test(t, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			done := false
			frame, quit := runUI(ctx, func() {
				r.out = syswWarnMS1Shaped(ctx, &descriptorTheme, "Engrave Text", text, &accepted)
				done = true
			})
			defer quit()
			c, ok := pumpUntil(frame, "looks like an ms1 string", 8)
			r.drawn = ok
			if ok {
				if confirm {
					press(&ctx.Router, Button3)
					frame()
					time.Sleep(confirmDelay)
					frame()
				} else {
					click(&ctx.Router, Button1)
				}
			}
			for i := 0; i < 16 && !done; i++ {
				if _, more := frame(); !more {
					break
				}
			}
			if !done {
				t.Fatalf("syswWarnMS1Shaped never returned.\nLast frame: %q", c)
			}
		})
		return r
	}

	if r := do(t, "not an ms1 string at all", false); r.drawn || !r.out {
		t.Errorf("a plain string drew the warning (drawn=%v, out=%v)", r.drawn, r.out)
	}
	if r := do(t, h6MS1Shaped, false); !r.drawn || r.out {
		t.Errorf("declining did not keep the operator on the screen (drawn=%v, out=%v)", r.drawn, r.out)
	}
	if accepted != "" {
		t.Error("a declined warning was recorded as accepted")
	}
	if r := do(t, h6MS1Shaped, true); !r.drawn || !r.out {
		t.Errorf("accepting did not continue (drawn=%v, out=%v)", r.drawn, r.out)
	}
	if r := do(t, h6MS1Shaped, false); r.drawn || !r.out {
		t.Errorf("the warning fired a SECOND time for the same text (drawn=%v)", r.drawn)
	}
	// An EDIT re-arms it. One character is enough, and it is still ms1-shaped.
	edited := h6MS1Shaped[:len(h6MS1Shaped)-1] + "p"
	if !hashlock.IsMS1Shaped(edited) {
		t.Fatal("the edited fixture is no longer ms1-shaped")
	}
	if r := do(t, edited, false); !r.drawn {
		t.Error("editing the text did not re-arm §9's warning")
	}
}

// TestHashlockMS1WarningFiresOnAGroupedPlate is H2 §2 rule 3's own argument,
// and the reason the predicate is hashlock.IsMS1Shaped and not
// codex32.IsPreimage.
//
// MUTATION: use codex32.IsPreimage instead -> a GROUPED plate is what
// `ms hashlock`'s card prints and therefore what an operator retypes, and
// IsPreimage answers FALSE for it: this row fails and the warning is silent for
// the shape it exists for.
func TestHashlockMS1WarningFiresOnAGroupedPlate(t *testing.T) {
	x := hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
	plate := composerTestPreimageRecord(t, x)
	grouped := groupBy(plate, 10)
	if !hashlock.IsMS1Shaped(grouped) {
		t.Fatalf("a grouped plate string is not ms1-shaped: %q", grouped)
	}
	// The narrower predicate answers false for exactly this string, which is
	// what makes the choice load-bearing rather than stylistic.
	if _, err := codex32.New(grouped); err == nil {
		t.Errorf("codex32 parsed the grouped string %q, so the two predicates do not "+
			"differ here and this test proves nothing", grouped)
	}
}

// TestSyswNoticeHashlockPhraseFiresOnItsCondition is §8.8.
//
// MUTATION: drop the notice -> the operator sees the ordinary keyboard with no
// explanation, which is today's behaviour and the finding.
// MUTATION: draw the notice when the payload also holds a `pass:` record -> the
// both-present row fails; there the offer DOES draw and there is nothing to say.
// TestPasswordProgramActuallyDrawsTheHashlockPhraseNotice is §8.8's JOIN, and
// it is a separate test from the one below on purpose.
//
// The test below drives syswNoticeHashlockPhrase DIRECTLY, so it proves the
// notice draws on its condition and says nothing at all about whether any
// program reaches it -- delete the call from engravePassphraseFlowFrom and it
// stays green, which is §8.8 shipped as an inert function and today's silence
// unchanged. That is the "plans list components and omit the call that joins
// them" class, and the composer's own join guard cannot catch it either:
// syswNoticeHashlockPhrase HAS a production caller, so the guard is satisfied
// by the very line this asserts.
//
// It is an AST assertion because nothing at the screen layer can distinguish
// "the offer drew nothing and the notice drew nothing" from "the offer drew
// nothing and there is no notice" without building the exact payload the
// arm needs, which the row below already does for the helper.
//
// MUTATION: delete the syswNoticeHashlockPhrase call from
// engravePassphraseFlowFrom -> "draws no §8.8 notice".
func TestPasswordProgramActuallyDrawsTheHashlockPhraseNotice(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "passphrase_flow.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "engravePassphraseFlowFrom" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "syswNoticeHashlockPhrase" {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Error("engravePassphraseFlowFrom draws no §8.8 notice: the operator whose " +
			"payload holds a hashlock phrase still meets the ordinary keyboard with " +
			"no offer, no mention and no reason, which is the silence §8.8 exists to " +
			"replace and the move that routes them around ruling L2's guard")
	}
}

func TestSyswNoticeHashlockPhraseFiresOnItsCondition(t *testing.T) {
	phrase := composerTestPhraseRecord(sysw.HashlockSHA256, hashlockAnchorPhrase)
	pass := composerRecord("pass:", "hunter2")
	for _, tc := range []struct {
		name   string
		secret []string
		want   bool
	}{
		{"a hashlock phrase and no passphrase", []string{phrase}, true},
		{"both classes present", []string{phrase, pass}, false},
		{"a passphrase alone", []string{pass}, false},
		{"neither", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				ctx.sysw = composerSessionWith(nil, tc.secret)
				done := false
				frame, quit := runUI(ctx, func() {
					syswNoticeHashlockPhrase(ctx, &descriptorTheme)
					done = true
				})
				defer quit()
				c, drawn := pumpUntil(frame, "not a BIP-39 passphrase", 8)
				if drawn != tc.want {
					t.Errorf("the notice drew = %v, want %v.\nFrame: %q", drawn, tc.want, c)
				}
				if drawn {
					if !uiContains(c, "opens a different wallet") {
						t.Errorf("the notice does not state the stake ruling L2 names.\nFrame: %q", c)
					}
					if !uiContains(c, "Wallet Policy program") {
						t.Errorf("the notice does not name where a hashlock phrase IS used.\nFrame: %q", c)
					}
					click(&ctx.Router, Button3)
				}
				for i := 0; i < 16 && !done; i++ {
					if _, more := frame(); !more {
						break
					}
				}
			})
		})
	}
	// The notice never echoes the phrase.
	if strings.Contains(composerCopyHashlockPhraseNotPassphrase(), hashlockAnchorPhrase) {
		t.Error("§8.8's notice echoes the phrase it is about")
	}
}
