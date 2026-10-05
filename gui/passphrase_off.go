package gui

import "seedhammer.com/gui/op"

// BIP-39 passphrases are off in the Refugium build, LOUDLY (Refugium plan F7
// §4.3, decision D4, R0 I-7). Every place that would ask "Add a BIP-39
// passphrase?" shows this notice instead, the operator acknowledges it, and
// the flow continues exactly as its no-passphrase branch. The default build
// asks the question as it always has.

// passphraseOffNotice is the notice's text.
const passphraseOffNotice = "This build takes no BIP-39 passphrase. The seed is used without one."

// askBIP39Passphrase is every "Add a BIP-39 passphrase?" question's Choose.
// Its result reads exactly as cs.Choose's: index 0 is Skip, index 1 is Add,
// and ok false is the question's Back.
//
// Under the profile it draws the notice titled as the question was. The
// notice keeps the question's two exits (review M-1):
//
//   - acknowledging it (Button3) answers (0, true): Skip, so the flow takes
//     its no-passphrase branch;
//   - Back on it (Button1) answers (0, false), exactly as Back on the
//     question did, so each site steps back as it always has (the seed
//     picker, the script, "not this seed");
//   - a session that ends on it (ctx.Done) is not an acknowledgement either:
//     (0, false).
//
// At all eight sites ok false means "no passphrase" or "step back", never
// "take one".
func askBIP39Passphrase(ctx *Context, th *Colors, cs *ChoiceScreen) (int, bool) {
	if refugiumProfile {
		return 0, passphraseOffNoticeFlow(ctx, th, cs.Title)
	}
	return cs.Choose(ctx, th)
}

// passphraseOffNoticeFlow draws the notice and reports whether it was
// acknowledged (true) rather than backed out of or ended by ctx.Done (false).
// It is showModal with the two dismissals told apart: ErrorScreen.Layout
// treats Back and OK alike, so Back is taken here first.
func passphraseOffNoticeFlow(ctx *Context, th *Colors, title string) bool {
	s := &ErrorScreen{Title: title, Body: passphraseOffNotice}
	s.back.Button = Button1
	for !ctx.Done {
		if s.back.Clicked(ctx) {
			return false
		}
		dims := ctx.Platform.DisplaySize()
		d, dismissed := s.Layout(ctx, th, dims)
		if dismissed {
			// Defensive: Done flips only in ctx.Frame, so it is still false
			// here; a wipe on the notice leaves through the loop condition.
			return !ctx.Done
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// slip39PassphraseRefusal is what the Refugium build says when SLIP-39 shares
// carry a passphrase. Skipping it would SILENTLY recover a different valid
// seed (slip39_polish.go), so the question stays and "yes" ends the recovery.
const slip39PassphraseRefusal = "This build cannot take a SLIP-39 passphrase. Recover these shares on another build."
