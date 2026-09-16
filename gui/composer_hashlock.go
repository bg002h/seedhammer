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
	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// The phrase route of `Which hash?` (SPEC_hashlock_H2_device §4): phrase screen ->
// method pick (+ its modal) -> derivation -> hold-to-confirm. One loop, so every
// inner Back moves WITHIN the route with the phrase intact, and only Back at the
// phrase screen returns to `Which hash?` (§4.6).
//
// THE PREIMAGE NO LONGER DIES WITH THIS FUNCTION. This record used to read "the
// preimage lives on the stack here and is dropped when this function returns
// (L7, L15)"; H6 §2.2 HOLDS it, with the phrase and the method, in
// composerState.hashlockHeld for the life of the composition, so §5.3 can offer
// a plate for it at Done. composerHoldHashlockMaterial
// (gui/composer_state.go:346) is the next production statement after the
// confirm modal is accepted, and composerScrubHashlockHeld
// (gui/composer_state.go:363) runs from composerFlowExit's ONE defer. L7 and
// L15 are superseded on store/show/engrave and unchanged on `source`.

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

// `kind` is the hash kind chosen on the screen BEFORE the phrase screen
// (SPEC_hashlock_kinds §7.1). It reaches the digest through hashlockLockOf and
// the confirm and reconcile bodies through h.Kind(), so every screen on this
// route names the same kind the script will commit to.
func hashlockPhraseRoute(ctx *Context, th *Colors, st *composerState, idx int, payload []*md.HashLock, kind md.HashKind) hashlockOutcome {
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
			h := hashlockLockOf(kind, &x)
			body := composerCopyHashlockConfirm(hashlockFirst8Last8(h), m.String(), len(phrase),
				hashlockRelationLine(payload, h), hashlockOtherPathLine(st, idx, h), h.Kind())
			if composerConfirmScreen(ctx, th, "Hash lock", composerConfirmBody(body)) {
				st.list.Paths[idx].Hash = h
				composerNotePhraseDigest(st, h)
				// H6 §2.2: the phrase TYPED HERE is held for this composition,
				// so §5.3 can offer a preimage plate for it at Done. L7's
				// "never stores" is the verb H6 reverses; the flow-exit defer
				// is what keeps the reversal bounded to one composition.
				composerHoldHashlockMaterial(st, h, hashlockMaterial{
					phrase: phrase, method: m, preimage: x, provenance: hashlockFromPhrase,
				})
				// The reconciliation line, on its own screen and reachable for
				// EVERY policy that has a phrase-set hash (r0 adversarial I-1 =
				// fidelity I-2 = journey I-3). Spec §4.5's drop-order step 2
				// moved it into the phrase-route §8h at Done, but §8h is guarded
				// by composerEveryPathHashed (composer_state.go at the fork
				// baseline c4a64fc), which is false the moment ONE path is keyed
				// -- so on the ordinary mixed wallet the line was drawn nowhere
				// at all. §4.5's own statement
				// of what the line is for ("converts a divergence discovered at
				// spend time into a five-minute check") is met here instead, at
				// the one moment every phrase-set hash passes through.
				showError(ctx, th, "Hash lock",
					composerCopyHashlockReconcile(hashlockFirst8Last8(h), m.String(), len(phrase), h.Kind()))
				return hashlockAssigned
			}
			// Back on the confirm -> method pick, nothing assigned
		}
	}
}

// hashlockPayloadRoute is §5.1's DERIVE-ONLY sibling of hashlockPhraseRoute,
// for a phrase: record the payload delivered.
//
// A DIFFERENT FUNCTION, not a flag on the phrase route, because
// hashlockPhraseRoute calls hashlockMethodPick and a payload phrase must not:
// the method is the RECORD's, so offering a pick is J4-1's mistake -- a phrase
// derived under a method its record does not name produces a digest nothing
// else in the payload agrees with.
//
// THE RECONCILIATION SCREEN IS CONDITIONAL HERE, and the condition is whether
// the payload says what the digest should be (F-496, operator ruling
// 2026-09-06). §10.2 originally scoped composerCopyHashlockReconcile to the
// phrase route and called the scoping "true by construction" -- one call site,
// so no runtime guard and no test for one. That argument rested on the claim
// that "run ms hashlock with this phrase" is a no-op for a phrase the host
// already has, and it is a no-op only in the case the argument had in mind:
// the payload ALSO carries a `hash:` record equal to this digest, so the
// comparison has already been made and the confirm modal's relation line has
// already shown it. When the payload carries the phrase and NO matching hash,
// nothing has compared the device's derivation to the host's, the relation line
// says so, and this screen is the only check on offer -- the operator has the
// phrase in the payload and can run the host command against it. So the screen
// is drawn exactly then, and payloadStatesDigest is the guard that decides.
//
// THERE IS NO PHRASE SCREEN AND NO METHOD PICK, so the only Back before the
// confirm is the derivation countdown's, which returns to `Which hash?` with
// nothing assigned -- the same contract the phrase route's own phrase screen
// has (§4.6).
func hashlockPayloadRoute(ctx *Context, th *Colors, st *composerState, idx int, rec sysw.PhraseRecord, payload []*md.HashLock) hashlockOutcome {
	phrase := []byte(rec.Phrase)
	m := hashlockMethodOf(rec.Method)
	x, ok := hashlockDeriveFlow(ctx, th, phrase, m)
	if !ok {
		return hashlockBackToWhichHash
	}
	h := hashlockLockOf(md.KindSha256, &x)
	body := composerCopyHashlockConfirm(hashlockFirst8Last8(h), m.String(), len(phrase),
		hashlockRelationLine(payload, h), hashlockOtherPathLine(st, idx, h), h.Kind())
	if !composerConfirmScreen(ctx, th, "Hash lock", composerConfirmBody(body)) {
		return hashlockBackToWhichHash
	}
	st.list.Paths[idx].Hash = h
	// H6 §2.2: the material is HELD, so §5.3 can offer a plate for it at Done.
	// composerNotePhraseDigest is deliberately NOT called: that map is H5's
	// record of a phrase TYPED HERE, which drives the §8h banner that says the
	// preimage "is not on this device" -- false for a payload phrase, whose
	// provenance is recorded in the material instead.
	composerHoldHashlockMaterial(st, h, hashlockMaterial{
		phrase: phrase, method: m, preimage: x, provenance: hashlockFromPayload,
	})
	// F-496. Drawn only when the payload states no matching digest; see the
	// header. hashlockPreimageRecordRoute deliberately does NOT get this: a
	// preimage record carries X directly, so there is no derivation to
	// reconcile and no phrase for the host command to take.
	if !payloadStatesDigest(payload, h) {
		showError(ctx, th, "Hash lock",
			composerCopyHashlockReconcile(hashlockFirst8Last8(h), m.String(), len(phrase), h.Kind()))
	}
	return hashlockAssigned
}

// payloadStatesDigest reports whether the payload carries a `hash:` record
// equal to h -- the same question hashlockRelationLine asks in order to word
// the confirm modal's relation line, asked here as a plain fact so the two
// screens cannot disagree about it (F-496).
func payloadStatesDigest(payload []*md.HashLock, h *md.HashLock) bool {
	for _, d := range payload {
		if d.Equal(h) {
			return true
		}
	}
	return false
}

// hashlockPreimageRecordRoute is the same shape WITHOUT the KDF: a preimage
// plate record carries X directly, so there is no countdown, no phrase and no
// method.
//
// IT DOES NOT REUSE composerCopyHashlockConfirm. That body is phrase-shaped --
// it prints `method: <m>   chars: <n>` and tells the operator to write down the
// phrase and the method -- and for a preimage record every one of those is
// false: there is no phrase, no method, and `chars: 0` would be a measurement
// of nothing on the screen that gates funds.
func hashlockPreimageRecordRoute(ctx *Context, th *Colors, st *composerState, idx int, p hashlockPayloadPreimage, payload []*md.HashLock) hashlockOutcome {
	body := composerCopyHashlockPreimageConfirm(hashlockFirst8Last8(p.digest),
		hashlockRelationLine(payload, p.digest), hashlockOtherPathLine(st, idx, p.digest), p.digest.Kind())
	if !composerConfirmScreen(ctx, th, "Hash lock", composerConfirmBody(body)) {
		return hashlockBackToWhichHash
	}
	st.list.Paths[idx].Hash = p.digest
	composerHoldHashlockMaterial(st, p.digest, hashlockMaterial{
		preimage: p.preimage, provenance: hashlockFromPayload,
	})
	return hashlockAssigned
}

// hashlockRelationLine is §4.5's relation line: which payload `hash:` record
// this digest equals, or that none does. "" when the payload holds none.
//
// match starts at -1 so the "no record matches" arm is reachable at all.
// MUTATION: `match := 0` -> TestHashlockConfirmRelationLine's no-match case
// reports `matches hash 1 in the payload`.
func hashlockRelationLine(payload []*md.HashLock, h *md.HashLock) string {
	if len(payload) == 0 {
		return ""
	}
	match := -1
	for i, d := range payload {
		if d.Equal(h) {
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
// It reads *p.Hash directly rather than the phrase set, so it is unaffected by
// that set's own history, and it skips idx because the path being edited may
// already hold the hash it is about to replace.
func hashlockOtherPathLine(st *composerState, idx int, h *md.HashLock) string {
	for i, p := range st.list.Paths {
		if i == idx || p.Hash == nil {
			continue
		}
		if !p.Hash.Equal(h) {
			return composerCopyHashlockOtherPath()
		}
	}
	return ""
}

// hashlockLockOf is THE ONE PLACE this package turns a preimage into the lock a
// path carries: hashlock.DigestOf picks the kind's hash, md.NewHashLock checks
// it against the kind's own width. No caller writes a hash or a width by hand,
// which is what SPEC_hashlock_kinds §5 asks for -- four correct digest
// functions behind one mis-wired call site is the same lost-funds outcome with
// a different cause.
//
// EVERY CALL SITE PASSES md.KindSha256 TODAY, and that is a statement about the
// screens rather than about this function: `Which hash?` offers no kind, so
// sha256 is what every route has always meant. The parameter is here so the
// kind arrives from the flow when a flow finally has one, instead of this
// function having to grow a second name.
//
// IT PANICS RATHER THAN RETURNING ok, because false is unreachable by
// construction: DigestOf and DigestLen switch on the same kind and agree by
// construction, so the only way here is a fifth kind added to one switch and
// not the other. Silently seating a wrong-width digest composes a wallet nobody
// can spend; composerHashEdit's own default arm panics on the same grounds.
func hashlockLockOf(k md.HashKind, x *[32]byte) *md.HashLock {
	h, ok := md.NewHashLock(k, hashlock.DigestOf(k, x))
	if !ok {
		panic("gui: hashlockLockOf: " + k.Token() + " digest is not that kind's width")
	}
	return h
}

// hashlockFirst8Last8 is the elided form every screen and every locator row
// draws: the first eight and last eight hex characters of the digest.
//
// AT THE KIND'S OWN WIDTH (md.HashLock.Digest), never the stored array: the
// digest lives in a fixed [32]byte for the alloc gate, so a 20-byte kind
// carries twelve bytes of padding, and hexing the array would print eight
// zeroes as the operator's last eight.
func hashlockFirst8Last8(h *md.HashLock) string {
	s := hex.EncodeToString(h.Digest())
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
				// The COPY is ours and the refusal abandons it, so zero it
				// before looping: a rejected phrase is usually retyped, and
				// without this every attempt leaves another unreferenced copy
				// behind for the GC to move around at its leisure. The keyboard
				// fragment itself is an immutable Go string and cannot be
				// wiped -- that is F-483's accepted residue (H6 brainstorm
				// decision 9, RAM retention until Done), and this is the part
				// of it that WAS avoidable.
				clear(phrase)
				showError(ctx, th, "Hashlock phrase", composerCopyHashlockRefusal(err))
				continue
			}
			return phrase, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		// No CutBottom here: the keyboard's readout needs every pixel below the
		// lead and counter bands (post-impl I-2 / F-481 -- an 8 px cut left the
		// readout budget one line short and the show key dead).
		leadOp, leadSz := hashlockPhraseLead(ctx, th, dims, content.Min.Y)
		_, content = content.CutTop(leadSz.Y)
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

// hashlockPhraseLead lays out the phrase screen's lead INSIDE the composer's
// text band, positioned at y = top (H5 §3, F-484).
//
// IT USED TO WRAP AT `dims.X - 2*8` AND CENTRE ON THE WHOLE PANEL, which is
// exactly the layout W-3 removed from composerPageLines: the lead's band
// overlaps the Back button's row, so 152 px of its ink was drawn inside that
// button's rectangle. No glyph or chip was lost -- the ink was in the button's
// empty margin -- but the margin is what keeps a glyph from sitting flush
// against a control it is not part of, and that margin was spent.
//
// A SEPARATE FUNCTION SO THE GATE MEASURES WHAT PRODUCTION DRAWS. The geometry
// test rasterises the op this returns; a test that re-derived the layout would
// pass on its own arithmetic rather than on the screen's
// (composer_paged_geometry_test.go's own split makes the same point).
func hashlockPhraseLead(ctx *Context, th *Colors, dims image.Point, top int) (op.Op, image.Point) {
	left, width := composerTextBand(dims)
	lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.lead, width, th.Text,
		composerCopyHashlockPhraseLead())
	return lbl.Offset(image.Pt(left+(width-sz.X)/2, top)), sz
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
