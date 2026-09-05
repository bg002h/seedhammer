package gui

import (
	"encoding/hex"
	"fmt"
	"image"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/hashlock"
)

// The phrase route of `Which hash?` (SPEC_hashlock_H2_device §4): phrase screen ->
// method pick (+ its modal) -> derivation -> hold-to-confirm. One loop, so every
// inner Back moves WITHIN the route with the phrase intact, and only Back at the
// phrase screen returns to `Which hash?` (§4.6). The preimage lives on the stack
// here and is dropped when this function returns (L7, L15).

type hashlockOutcome int

const (
	hashlockAssigned hashlockOutcome = iota
	hashlockBackToWhichHash
)

type hashlockMethod int

const (
	hashlockHardened hashlockMethod = iota
	hashlockSHA256
)

func (m hashlockMethod) String() string {
	if m == hashlockSHA256 {
		return "sha256"
	}
	return "hardened"
}

func hashlockPhraseRoute(ctx *Context, th *Colors, st *composerState, idx int, payload [][32]byte) hashlockOutcome {
	var phrase []byte
	for {
		p, ok := hashlockPhraseFlow(ctx, th, phrase)
		if !ok {
			return hashlockBackToWhichHash // phrase dropped
		}
		phrase = p
	pick:
		for {
			m, ok := hashlockMethodPick(ctx, th)
			if !ok {
				break pick // Back at the method pick -> phrase screen, phrase intact
			}
			if !hashlockMethodWarning(ctx, th, phrase, m) {
				continue // declined -> method pick, phrase intact
			}
			x, ok := hashlockDeriveFlow(ctx, th, phrase, m)
			if !ok {
				continue // Back during derivation -> method pick
			}
			h := hashlock.Digest(&x)
			body := composerCopyHashlockConfirm(hashlockFirst8Last8(h), m.String(), len(phrase),
				hashlockRelationLine(payload, h), hashlockOtherPathLine(st, idx, h))
			if composerConfirmScreen(ctx, th, "Hash lock", composerConfirmBody(body)) {
				d := h
				st.list.Paths[idx].Hash = &d
				st.hashByPhrase = true
				// The reconciliation line, on its own screen and reachable for
				// EVERY policy that has a phrase-set hash (r0 adversarial I-1 =
				// fidelity I-2 = journey I-3). Spec §4.5's drop-order step 2
				// moved it into the phrase-route §8h at Done, but §8h is guarded
				// by composerEveryPathHashed (composer_state.go:239 at the fork
				// baseline c4a64fc), which is false the moment ONE path is keyed
				// -- so on the ordinary mixed wallet the line was drawn nowhere
				// at all. §4.5's own statement
				// of what the line is for ("converts a divergence discovered at
				// spend time into a five-minute check") is met here instead, at
				// the one moment every phrase-set hash passes through.
				showError(ctx, th, "Hash lock", composerCopyHashlockReconcile())
				return hashlockAssigned
			}
			// Back on the confirm -> method pick, nothing assigned
		}
	}
}

// hashlockRelationLine is §4.5's relation line: which payload `hash:` record
// this digest equals, or that none does. "" when the payload holds none.
//
// match starts at -1 so the "no record matches" arm is reachable at all.
// MUTATION: `match := 0` -> TestHashlockConfirmRelationLine's no-match case
// reports `matches hash 1 in the payload`.
func hashlockRelationLine(payload [][32]byte, h [32]byte) string {
	if len(payload) == 0 {
		return ""
	}
	match := -1
	for i, d := range payload {
		if d == h {
			match = i
			break
		}
	}
	return composerCopyHashlockRelation(match)
}

// hashlockOtherPathLine warns when ANOTHER path of this same policy already
// carries a DIFFERENT hash (r0 journey I-1): "One phrase per policy" is advice,
// md.ValidatePathList (md/compose.go:299-334) has no clause about two paths'
// Hash values, and nothing else on the route compares them. Two phrases is a
// legal composition; it is a backup burden the operator must choose knowingly.
//
// It reads *p.Hash directly rather than st.hashByPhrase, so it is unaffected by
// that flag's own staleness, and it skips idx because the path being edited may
// already hold the hash it is about to replace.
func hashlockOtherPathLine(st *composerState, idx int, h [32]byte) string {
	for i, p := range st.list.Paths {
		if i == idx || p.Hash == nil {
			continue
		}
		if *p.Hash != h {
			return composerCopyHashlockOtherPath()
		}
	}
	return ""
}

func hashlockFirst8Last8(h [32]byte) string {
	s := hex.EncodeToString(h[:])
	return s[:8] + ".." + s[len(s)-8:]
}

// hashlockPhraseFlow is the phrase screen (§4.2): the four-page printable-ASCII
// keyboard, a lead, an unclamped n/100 counter, and the §2 rule on OK. initial
// restores what was typed before a Back from the method pick. NOT
// passphraseEntryFlow (its title, pass-proof trigger and over-length message are
// the passphrase's -- r2 M-4), and NOTHING normalises the bytes.
func hashlockPhraseFlow(ctx *Context, th *Colors, initial []byte) ([]byte, bool) {
	kbd := NewPassphraseKeyboard(ctx)
	kbd.Fragment = string(initial)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		if okBtn.Clicked(ctx) {
			phrase := []byte(kbd.Fragment)
			if err := hashlock.ValidatePhrase(phrase); err != nil {
				showError(ctx, th, "Hashlock phrase", composerCopyHashlockRefusal(err))
				continue
			}
			return phrase, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		leadOp, leadSz := widget.Labelw(&ctx.B, ctx.Styles.lead, dims.X-2*8, th.Text,
			composerCopyHashlockPhraseLead())
		leadBand, content := content.CutTop(leadSz.Y)
		leadOp = leadOp.Offset(leadBand.N(leadSz))
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text,
			"%d/%d", len(kbd.Fragment), hashlock.PhraseMaxChars)
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Hashlock phrase")
		ctx.Frame(op.Layer(kbdOp, leadOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return nil, false
}

func hashlockMethodPick(ctx *Context, th *Colors) (hashlockMethod, bool) {
	sel, ok := composerPickScreen(ctx, th, "Hashlock method", "Which method?",
		[]string{"Hardened (about 10 s)", "SHA-256"})
	if !ok {
		return 0, false
	}
	if sel == 1 {
		return hashlockSHA256, true
	}
	return hashlockHardened, true
}

// hashlockMethodWarning shows the §4.3 modal when its condition holds; both are
// confirm-to-proceed (L12). Returns false when declined.
func hashlockMethodWarning(ctx *Context, th *Colors, phrase []byte, m hashlockMethod) bool {
	switch m {
	case hashlockSHA256:
		return composerConfirmScreen(ctx, th, "SHA-256", composerConfirmBody(composerCopyHashlockSHA256Warning()))
	case hashlockHardened:
		if len(phrase) < 20 {
			return composerConfirmScreen(ctx, th, "Hardened", composerConfirmBody(composerCopyHashlockHardenedWarning()))
		}
	}
	return true
}

// hashlockDerivingLead is §4.4's lead: the zero state until the first slice has
// actually been timed, then the estimate. A pure function on the unlockKDFLead
// model (gui/unlock_kdf.go), so the zero state can be asserted without a screen.
//
// The guard is `done <= 0`, not `done > 0` -- and that distinction is the whole
// point of hoisting the zero-state frame below (r0 adversarial I-4): every call
// DeriveHardened makes arrives with done >= 501 (seal.NewDeriver sets done = 1
// and the loop calls progress only after a Step(500) returns false), so a lead
// chosen inside the callback alone can NEVER be the zero state, and §4.4's
// "Deriving. This takes about 10 seconds." would be dead copy.
//
// MUTATION: return the estimate unconditionally -> TestHashlockDerivingLead's
// zero-state rows fail, and the drawn-frame assertion in
// TestHashlockDeriveKeepsAwakeUnderTheScreensaver stops finding the lead.
func hashlockDerivingLead(done, total int, elapsed time.Duration) string {
	if done <= 0 || elapsed <= 0 || total <= 0 {
		return composerCopyHashlockDerivingLead()
	}
	left := time.Duration(float64(elapsed) * float64(total-done) / float64(done))
	return fmt.Sprintf("About %d seconds left.", int(left.Seconds()+0.5))
}

// hashlockDeriveFlow derives X. SHA-256 is instant. Hardened runs on a countdown
// screen driven by hashlock.DeriveHardened (the 14-byte salt as a slice --
// NEVER unlockDerive/seal.Header, §3); Back abandons with nothing assigned.
func hashlockDeriveFlow(ctx *Context, th *Colors, phrase []byte, m hashlockMethod) ([32]byte, bool) {
	if m == hashlockSHA256 {
		return hashlock.PreimageSHA256(phrase), true
	}
	backBtn := &Clickable{Button: Button1}
	start := time.Now()
	abandoned := false
	frame := func(done, total int) {
		dims := ctx.Platform.DisplaySize()
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Deriving")
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		pctOp, pctSz := widget.Label(&ctx.B, ctx.Styles.progress, th.Text,
			fmt.Sprintf("%d%%", pct))
		leadOp, leadSz := widget.Labelw(&ctx.B, ctx.Styles.lead, dims.X-2*8, th.Text,
			hashlockDerivingLead(done, total, time.Since(start)))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconDiscard},
		}...)
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		pctOp = pctOp.Offset(content.N(pctSz).Add(image.Pt(0, 24)))
		leadOp = leadOp.Offset(content.Center(leadSz))
		// BEFORE ctx.Frame, and the order is load-bearing -- the same fix, for
		// the same reason, as unlockDerive's (gui/unlock_kdf.go:334-335, F-93).
		// ctx.Frame IS the yield, and Run reads the deadline for the frame it
		// has just been handed before its own ctx.Reset(), so a WakeupAt placed
		// AFTER Frame governs the NEXT frame and frame 1 inherits Run's own
		// ctx.WakeupAt(idleWakeup) -- three minutes. Without KeepAwake, Run
		// refreshes a.idle.start only on `effectiveInput(evts, &a.pressed) ||
		// (ctx.keepAwake && !armed)` (run_flow.go:349-350) and a derivation
		// produces no events, so once idleTimeout (3 min,
		// gui/gui.go:3584) is crossed the screensaver takes the screen and its
		// branch `continue`s without breaking (run_flow.go:401-406) -- ctx.Frame
		// never returns and the derivation stops until a touch.
		//
		// Hardened is 100,000 iterations at a measured 9,715 it/s = 10.3 s on
		// the SH2, so the crossing needs an operator who walks away mid-screen;
		// the parked KDF then never resumes, and Back is the only way out of a
		// screen that says "About N seconds left". r0 adversarial C-1.
		ctx.KeepAwake()
		ctx.WakeupAt(time.Now())
		ctx.Frame(op.Layer(pctOp, leadOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	// §4.4's zero-state frame, drawn BEFORE the first Step so the zero-state
	// lead is reachable at all (r0 adversarial I-4). It also registers backBtn
	// with the router one frame earlier, so a Back pressed on the very first
	// frame is seen by the next callback.
	frame(0, hashlock.Iterations)
	x, ok := hashlock.DeriveHardened(phrase, func(done, total int) bool {
		if ctx.Done {
			return false
		}
		if backBtn.Clicked(ctx) {
			abandoned = true
			return false
		}
		frame(done, total)
		return true
	})
	if !ok || abandoned {
		return x, false
	}
	return x, true
}
