package gui

// deriveXpubFlow is the new top-level program (parallel to backupWallet) that
// turns a hand-typed BIP-39 seed into a PUBLIC account xpub engraved as an mk1
// key card. The full flow (seed entry, two-stage path picker, derive, stub-0
// warning, mk1 encode, multi-plate engrave) is implemented in Task 4.
//
// SECURITY: this flow NEVER calls engraveSeed / backup.EngraveSeed and NEVER
// engraves the seed/mnemonic/passphrase. The only engraved output is the public
// xpub wrapped in mk1.
func deriveXpubFlow(ctx *Context, th *Colors) {
	// Placeholder until Task 4; the program is navigable and titled now.
}
