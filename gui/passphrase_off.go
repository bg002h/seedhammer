package gui

// BIP-39 passphrases are off in the Refugium build, LOUDLY (Refugium plan F7
// §4.3, decision D4, R0 I-7). Every place that would ask "Add a BIP-39
// passphrase?" shows this notice instead, the operator acknowledges it, and
// the flow continues exactly as its no-passphrase branch. The default build
// asks the question as it always has.

// passphraseOffNotice is the notice's text.
const passphraseOffNotice = "This build takes no BIP-39 passphrase. The seed is used without one."

// askBIP39Passphrase is every "Add a BIP-39 passphrase?" question's Choose.
// Its result reads exactly as cs.Choose's: index 0 is Skip, index 1 is Add.
//
// Under the profile it draws the notice titled as the question was, and
// answers (0, true) -- Skip -- whatever button dismissed it: the notice offers
// no alternative, so any acknowledgement continues the no-passphrase branch.
func askBIP39Passphrase(ctx *Context, th *Colors, cs *ChoiceScreen) (int, bool) {
	if refugiumProfile {
		showNotice(ctx, th, cs.Title, passphraseOffNotice)
		return 0, true
	}
	return cs.Choose(ctx, th)
}

// slip39PassphraseRefusal is what the Refugium build says when SLIP-39 shares
// carry a passphrase. Skipping it would SILENTLY recover a different valid
// seed (slip39_polish.go), so the question stays and "yes" ends the recovery.
const slip39PassphraseRefusal = "This build cannot take a SLIP-39 passphrase. Recover these shares on another build."
