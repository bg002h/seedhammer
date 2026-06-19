package gui

import (
	"fmt"
	"image"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/font/constant"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	slip39words "seedhammer.com/slip39"
)

// showModal displays a dismissible title+body modal (Button3 dismisses) over a
// blank background; returns when dismissed or ctx.Done. (Generalizes
// showCodex32Error with a title parameter.) Callers should use showError for
// failures and showNotice for non-error notices.
func showModal(ctx *Context, th *Colors, title, msg string) {
	errScr := &ErrorScreen{Title: title, Body: msg}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, dismissed := errScr.Layout(ctx, th, dims)
		if dismissed {
			return
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
}

// showError displays a dismissible error modal for a failure/refusal.
func showError(ctx *Context, th *Colors, title, msg string) {
	showModal(ctx, th, title, msg)
}

// showNotice displays a dismissible modal for a SUCCESS or informational notice
// (e.g. "Verify OK"), so a non-error result is not surfaced via showError. The
// presentation is identical today; the distinct entry point keeps the call sites
// honest and lets the success/notice styling diverge later without churn.
func showNotice(ctx *Context, th *Colors, title, msg string) {
	showModal(ctx, th, title, msg)
}

// slip39LengthPick asks how many words are on the user's physical share and
// returns the chosen word count ∈ {20,23,27,30,33}. The 20- and 33-word counts
// (the only ones mainstream wallets emit) are presented prominently; the three
// intermediate 160/192/224-bit counts follow. Returns 0 if the user backs out.
// (inputSLIP39Flow fills a pre-sized slice, so the length must be known at
// allocation; the user can simply count the words on their share — SPEC §5.2.)
func slip39LengthPick(ctx *Context, th *Colors) int {
	counts := []int{20, 33, 23, 27, 30}
	cs := &ChoiceScreen{
		Title: "SLIP-39 Share",
		Lead:  "Words on your share?",
		Choices: []string{
			"20 (128-bit)",
			"33 (256-bit)",
			"23 (160-bit)",
			"27 (192-bit)",
			"30 (224-bit)",
		},
	}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return 0
	}
	return counts[sel]
}

// slip39ConfirmAction is the result of the pre-engrave SLIP-39 confirm screen.
type slip39ConfirmAction int

const (
	slip39Back    slip39ConfirmAction = iota // Button1
	slip39Engrave                            // Button3 / Center
	slip39Recover                            // Button2 — only when part of a multi-share set
)

// recoverable reports whether a share can be combined with siblings: it must be
// part of a set with a threshold ≥ 2 layer (member or group), so the Recover
// path always exercises a digest gate. A lone 1-of-1 share takes the verbatim
// Engrave path only.
func recoverableSLIP39(s slip39words.Share) bool {
	return s.MemberThreshold > 1 || s.GroupThreshold > 1
}

// confirmSLIP39Flow shows a pre-engrave review of a parsed SLIP-39 share. For a
// lone 1-of-1 share it offers Back/Engrave; for a share in a multi-share set it
// also offers Recover (reconstruct the seed from enough shares). Button2 is
// ALWAYS drained every frame — even when Recover is not offered — so an
// unconsumed event cannot block the router queue head in a direct-call
// (non-runUI) context. (Cycle-B R0-C1 EventRouter footgun.)
func confirmSLIP39Flow(ctx *Context, th *Colors, s slip39words.Share) slip39ConfirmAction {
	recover := recoverableSLIP39(s)
	lines := []string{
		fmt.Sprintf("id %d", s.Identifier),
		fmt.Sprintf("member %d of %d", s.MemberIndex+1, s.MemberThreshold),
	}
	if s.GroupCount > 1 {
		lines = append(lines, fmt.Sprintf("group %d of %d", s.GroupIndex+1, s.GroupCount))
	}
	lines = append(lines, fmt.Sprintf("%d words", len(s.Mnemonic)))
	if recover {
		lines = append(lines, "Engrave this share, or Recover the seed")
	}

	backBtn := &Clickable{Button: Button1}
	recoverBtn := &Clickable{Button: Button2}
	engraveBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return slip39Back
		}
		// Always drain Button2 — even for a lone share, where Recover is not
		// offered — so an unconsumed event cannot block the router queue head
		// in a direct-call (non-runUI) context. Act on it only when offered.
		recoverClicked := recoverBtn.Clicked(ctx)
		if recover && recoverClicked {
			return slip39Recover
		}
		if engraveBtn.Clicked(ctx) {
			return slip39Engrave
		}
		dims := ctx.Platform.DisplaySize()
		navBtns := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}
		if recover {
			navBtns = append(navBtns, NavButton{Clickable: recoverBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		navBtns = append(navBtns, NavButton{Clickable: engraveBtn, Style: StylePrimary, Icon: assets.IconHammer})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm SLIP-39 Share")

		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(leadingSize)
		body := make([]op.Op, 0, len(lines))
		y := content.Min.Y + 8
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return slip39Back
}

// groupSatisfied reports whether a group's collected shares exactly fill its
// member threshold (each group's first share carries its MemberThreshold; the
// collection loop never over-fills a group, so == is the satisfaction test).
func groupSatisfied(gs []slip39words.Share) bool {
	return len(gs) > 0 && len(gs) == gs[0].MemberThreshold
}

// countSatisfied returns the number of groups in the roster that have reached
// their member threshold.
func countSatisfied(byGroup map[int][]slip39words.Share) int {
	n := 0
	for _, gs := range byGroup {
		if groupSatisfied(gs) {
			n++
		}
	}
	return n
}

// selectForCombine returns the flattened members of EXACTLY groupThreshold
// satisfied groups (a group is satisfied when it holds exactly its
// MemberThreshold members), dropping any partial/extra group lingering in the
// roster (e.g. a lone share from the wrong pile). ok=false if fewer than
// groupThreshold groups are satisfied. Combine requires exactly the satisfied
// groups' members — feeding it a flat slice with a stray partial group makes it
// return errInsufficientShares on a genuinely-sufficient pile (plan-R0 I1).
func selectForCombine(byGroup map[int][]slip39words.Share, groupThreshold int) ([]slip39words.Share, bool) {
	gids := make([]int, 0, len(byGroup))
	for g := range byGroup {
		gids = append(gids, g)
	}
	sort.Ints(gids)
	var out []slip39words.Share
	picked := 0
	for _, g := range gids {
		if picked == groupThreshold {
			break
		}
		gs := byGroup[g]
		if !groupSatisfied(gs) {
			continue // prune partial/extra groups
		}
		out = append(out, gs...)
		picked++
	}
	if picked < groupThreshold {
		return nil, false
	}
	return out, true
}

// showSLIP39Message renders a single titled message frame (no buttons) until
// ctx.Done — used for the brief "Recovering…" indicator shown before the
// blocking Combine call. (The firmware has no active watchdog — only a BOOTSEL
// reboot-vector stage — so a blocking Combine at e=0/1, ~0.5–1.9s, is safe;
// SPEC §5.6.)
func showSLIP39Message(ctx *Context, th *Colors, title, msg string) {
	dims := ctx.Platform.DisplaySize()
	titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	content, _ = content.CutBottom(leadingSize)
	lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, msg)
	body := lbl.Offset(image.Pt((dims.X-sz.X)/2, content.Min.Y+8))
	ctx.Frame(op.Layer(titleOp, body, op.Color(&ctx.B, th.Background)))
}

// recoverSLIP39Flow collects enough SLIP-39 shares to satisfy the group
// threshold (each represented group at its member threshold), optionally takes
// a SLIP-39 passphrase, reconstructs the master secret, and returns it as a
// BIP-39 mnemonic. Returns (nil, false) if the user backs out or recovery
// fails. The collection roster shows live group satisfaction; subsequent shares
// inherit the first share's word length. SPEC §5.3.
func recoverSLIP39Flow(ctx *Context, th *Colors, first slip39words.Share) (bip39.Mnemonic, bool) {
	GT := first.GroupThreshold
	L := len(first.Mnemonic)
	byGroup := map[int][]slip39words.Share{first.GroupIndex: {first}}

	// Collection loop: prompt until enough groups are satisfied.
	for countSatisfied(byGroup) < GT {
		done := countSatisfied(byGroup)
		title := fmt.Sprintf("Share · %d/%d groups", done, GT)
		m := emptySLIP39Mnemonic(L)
		if ok := inputSLIP39Flow(ctx, th, m, 0, title); !ok {
			return nil, false // Back exits recovery (the dead-end / cancel path)
		}
		cand, err := buildSLIP39Share(m)
		if err != nil {
			showError(ctx, th, "Invalid share", slip39words.Describe(err))
			continue
		}
		// Eager cross-share consistency against everything collected so far.
		if err := slip39words.ConsistentShares(append(allShares(byGroup), cand)); err != nil {
			showError(ctx, th, "Invalid share", slip39words.Describe(err))
			continue
		}
		// Reject a share for an already-satisfied group (would over-fill it;
		// Combine needs exactly memberThreshold members per group — I1).
		if groupSatisfied(byGroup[cand.GroupIndex]) {
			showError(ctx, th, "Invalid share", "that group is already complete")
			continue
		}
		byGroup[cand.GroupIndex] = append(byGroup[cand.GroupIndex], cand)
	}

	sel, ok := selectForCombine(byGroup, GT)
	if !ok { // defensive — the loop guarantees GT satisfied groups
		showError(ctx, th, "Recovery failed", "not enough shares")
		return nil, false
	}

	// High iteration-exponent gate (§5.6): warn-and-confirm on a long wait
	// (NOT a hard cap — that would break recoverability of real high-e backups).
	if first.IterationExp >= 4 {
		confirm := &ConfirmWarningScreen{
			Title: "Slow Recovery",
			Body:  fmt.Sprintf("This backup uses a high iteration exponent (%d) and may take a long time to recover.\n\nHold button to continue.", first.IterationExp),
			Icon:  assets.IconInfo,
		}
		if !holdToConfirm(ctx, th, confirm) {
			return nil, false
		}
	}

	// Optional SLIP-39 (EMS-decryption) passphrase — distinct from a BIP-39
	// 25th-word passphrase. Defaults to Skip; a wrong one silently recovers a
	// different valid seed (SLIP-39 plausible deniability) — surfaced, not
	// claimed verified (§5.5).
	pass := ""
	ppChoice := &ChoiceScreen{
		Title:   "SLIP-39 Passphrase",
		Lead:    "SLIP-39 passphrase? (NOT a BIP-39 passphrase) A wrong passphrase silently recovers a different seed.",
		Choices: []string{"Skip", "Enter passphrase"},
	}
	if psel, ok := ppChoice.Choose(ctx, th); ok && psel == 1 {
		p, ok := passphraseFlow(ctx, th)
		if !ok {
			return nil, false
		}
		pass = p
	}

	// Brief progress frame before the blocking decrypt.
	showSLIP39Message(ctx, th, "Recovering", "Reconstructing the seed…")

	secret, err := slip39words.Combine(sel, []byte(pass))
	if err != nil {
		showError(ctx, th, "Recovery failed", slip39words.Describe(err))
		return nil, false
	}
	m := bip39.New(secret)
	wipeBytes(secret)
	return m, true
}

// buildSLIP39Share joins a completed input mnemonic into a share string and
// parses it.
func buildSLIP39Share(m slip39words.Mnemonic) (slip39words.Share, error) {
	words := make([]string, len(m))
	for i, w := range m {
		words[i] = slip39words.LabelFor(w)
	}
	return slip39words.ParseShare(joinWords(words))
}

func joinWords(words []string) string {
	out := ""
	for i, w := range words {
		if i > 0 {
			out += " "
		}
		out += w
	}
	return out
}

// allShares flattens the roster into a single slice (for eager consistency
// checking).
func allShares(byGroup map[int][]slip39words.Share) []slip39words.Share {
	var out []slip39words.Share
	for _, gs := range byGroup {
		out = append(out, gs...)
	}
	return out
}

// wipeBytes best-effort zeroes a secret-bearing slice (TinyGo GC may copy/
// retain — defense-in-depth, not a guarantee; SPEC §4.8).
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// holdToConfirm drives a ConfirmWarningScreen to its terminal result, returning
// true on a held confirm and false on cancel/Done.
func holdToConfirm(ctx *Context, th *Colors, confirm *ConfirmWarningScreen) bool {
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, res := confirm.Layout(ctx, th, dims)
		switch res {
		case ConfirmNo:
			return false
		case ConfirmYes:
			return true
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// engraveSLIP39 confirms a SLIP-39 share. A lone share is engraved verbatim; a
// share in a multi-share set may instead be Recovered into the master seed,
// which is then engraved as the native BIP-39 plate via backupWalletFlow.
// Always returns true (recognized/handled) — Back, a fit failure, and
// engrave-complete all return true, never falling to the caller's
// scanUnknownFormat ("Unknown format").
func engraveSLIP39(ctx *Context, th *Colors, scan slip39words.Share) bool {
	for {
		switch confirmSLIP39Flow(ctx, th, scan) {
		case slip39Back:
			return true
		case slip39Engrave:
			engraveSLIP39Verbatim(ctx, th, scan)
			return true
		case slip39Recover:
			m, ok := recoverSLIP39Flow(ctx, th, scan)
			if !ok {
				continue // back to the original share's confirm
			}
			if !engraveRecoveredSLIP39(ctx, th, scan, m) {
				continue // Back at the fork, or declined the fingerprint check
			}
			return true
		}
	}
}

// engraveRecoveredSLIP39 forks on how the recovered backup was made (SPEC §3
// interpretation ambiguity). The BIP-39 arm hands off to the §5.4 always-on
// master-fingerprint check then backupWalletFlow (the native BIP-39 seed-plate
// path); the verbatim arm re-engraves the user's first share words verbatim via
// engraveSLIP39Verbatim with NO BIP-39 fingerprint (it is convention-specific
// and would mislead on a non-BIP-39 wallet). Returns false if the user backs
// out of the fork (caller continues to the original confirm) or declines the
// fingerprint check.
func engraveRecoveredSLIP39(ctx *Context, th *Colors, scan slip39words.Share, m bip39.Mnemonic) bool {
	// §3 — the device cannot tell a BIP-39/toolkit backup from a Trezor/other
	// SLIP-39 backup; the bytes are valid under both. Ask the user, framed by
	// how the backup was made. Fresh ChoiceScreen per call (like backupWalletFlow).
	choice := &ChoiceScreen{
		Title: "Recovered Seed",
		// The Lead IS width-wrapped (widget.Labelw) — put the explanation here.
		Lead: "How was this backup made? A BIP-39 phrase / this toolkit recovers as a " +
			"seed. A Trezor or other SLIP-39 wallet should engrave its shares verbatim.",
		// The choice buttons are SINGLE-LINE (widget.Label, NOT wrapped), so keep
		// them short; detail lives in the Lead above.
		Choices: []string{
			"BIP-39 seed",    // sel == 0 (default)
			"Engrave shares", // sel == 1
		},
	}
	sel, ok := choice.Choose(ctx, th)
	if !ok {
		return false // Back → caller continues to the original confirm
	}
	if sel == 1 {
		// Not a constellation backup: engrave the share verbatim (convention-
		// agnostic, restorable). NO BIP-39 fingerprint here — it would be a
		// misleading "verification" of a number unrelated to a non-BIP-39 wallet.
		engraveSLIP39Verbatim(ctx, th, scan)
		return true
	}
	// BIP-39 arm: §5.4 — always-on recovered master-fingerprint check (match
	// backupWalletFlow's %.8X format). Framed as a check-against-records, NOT a
	// verification claim.
	mfp, err := masterFingerprintFor(m, &chaincfg.MainNetParams, "")
	if err != nil {
		showError(ctx, th, "Recovery failed", "could not derive the fingerprint")
		return false
	}
	if !confirmSLIP39Fingerprint(ctx, th, mfp) {
		return false // Back at the fingerprint check
	}
	backupWalletFlow(ctx, th, m)
	return true
}

// confirmSLIP39Fingerprint shows the recovered seed's master fingerprint and
// waits for the user to confirm it against their records (Engrave/Center) or go
// Back (Button1). Button2 is drained unconditionally (no-hang).
func confirmSLIP39Fingerprint(ctx *Context, th *Colors, mfp uint32) bool {
	lines := []string{
		fmt.Sprintf("Fingerprint %.8X", mfp),
		"Confirm this matches your wallet records before engraving.",
	}
	backBtn := &Clickable{Button: Button1}
	drainBtn := &Clickable{Button: Button2}
	okBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		drainBtn.Clicked(ctx) // drain Button2 (no-hang)
		if okBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Recovered Fingerprint")
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(leadingSize)
		body := make([]op.Op, 0, len(lines))
		y := content.Min.Y + 8
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}

// engraveSLIP39Verbatim engraves a single share's words verbatim onto a plate.
func engraveSLIP39Verbatim(ctx *Context, th *Colors, scan slip39words.Share) {
	seedDesc := backup.Seed{
		Mnemonic:     scan.Mnemonic, // canonical uppercase words; verbatim
		ShortestWord: slip39words.ShortestWord,
		LongestWord:  slip39words.LongestWord,
		Title:        fmt.Sprintf("%d #%d/%d", scan.Identifier, scan.MemberIndex+1, scan.MemberThreshold), // max "32767 #16/16" = 12 <= MaxTitleLen 18
		Font:         constant.Font,
	}
	params := ctx.Platform.EngraverParams()
	seedSide, err := backup.EngraveSeed(params, seedDesc)
	if err != nil {
		showError(ctx, th, "Too large", "Share doesn't fit a plate.")
		return
	}
	plate, err := toPlate(seedSide, params)
	if err != nil {
		showError(ctx, th, "Too large", "Share doesn't fit a plate.")
		return
	}
	for {
		if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
			return
		}
	}
}
