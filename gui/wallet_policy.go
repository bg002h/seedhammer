package gui

import (
	"errors"
	"fmt"
	"strings"

	"seedhammer.com/mk"

	"seedhammer.com/address"
	"seedhammer.com/md"
	"seedhammer.com/nonstandard"
	"seedhammer.com/sysw"
)

// ─── The Wallet Policy program (plan D5) ─────────────────────────────────────
//
// Engrave a wallet policy that came from OUTSIDE this device, with PROOF of
// which wallet it is.
//
// The transport is not new — this reuses bundleGatherFlowResume and
// bundleEngrave verbatim, so a card enters through the same offer(), the same
// deduplication and the same validation it would in Engrave Bundle. What is new
// is the consent surface between them.
//
// WHY A CONSENT SCREEN WAS THE MISSING PIECE. Engrave Bundle's review reads
// "N cards verified" plus a per-card label: it says the chunks reassembled, and
// nothing about which wallet is about to be committed to steel. Plan D2 makes
// proof concrete — derived addresses plus a named wallet id — and says the
// derivation belongs ON the consent path rather than beside it. So the
// addresses are lines on the consent screen, not a side trip: an operator who
// taps straight through still passes them.

// walletPolicyFlow is the walletPolicy program front door.
func walletPolicyFlow(ctx *Context, th *Colors) {
	const title = "Wallet Policy"
	// The payload's cards are offered ONCE, before gathering, through the SAME
	// offer() a scanned card enters by — bundleFlow does it this way and for the
	// reason stated there: a separate insertion path would be a second way for a
	// card to join the set, and only one of them would have the checks.
	//
	// EVERY md1/mk1 RECORD, not the first (F-76): a card is a chunk SET, and one
	// record of it completes nothing. Measured here as `md1 descriptors: 0` for
	// a payload holding a whole six-chunk card.
	if bodies, ok := syswOfferCards(ctx, th, sysw.ClassMDMK, "Cards from where?"); ok {
		ctx.syswBundleSeeds = bodies
	} else if body, ok := syswOfferAlt(ctx, th, sysw.ClassDescriptor, "Input",
		"Wallet policy from where?", syswAltScan); ok {
		// S2's SECOND offer at the SAME door, and it is a second offer rather
		// than a widened first one for the reason newInputFlow states at its own
		// pair (gui.go:2761-2767): syswOffer takes ONE class. So this screen is
		// reached only when the payload holds a Descriptor record AND either
		// holds no md1 card or the operator declined it.
		//
		// A `Descriptor` record IS an outside wallet policy -- which is this
		// program's whole subject -- so it belongs here and not behind a new
		// menu. What it is NOT is a card: it carries no chunks, nothing to
		// gather, deduplicate or reassemble, and no cosigner plates to cut
		// alongside. So it routes to the descriptor screen and this call
		// RETURNS; the md1-card path below is untouched.
		//
		// `SCAN CARDS`, not `ENTER IT` (F-437). This screen shipped offering to
		// let the operator "enter" a wallet policy, and DECLINING it falls
		// through to the md1 card gather below -- an NFC wait, on a device with
		// no keyboard and no camera. The choice now names the route it actually
		// takes.
		//
		// WHAT CLASSIFICATION ACTUALLY PROVED, and it is not what an earlier
		// version of this comment claimed. It proved something about
		// `strings.TrimSpace(body)`, because that is the string
		// classifyConstellation hands the arm -- while `take` returns `r.body`
		// unmodified (gui/sysw_session.go:123). The shipped corpus row
		// `whitespace/leading-space-bip380` is the standing counterexample: it
		// is `host_admits: true` and single-line, so the seam suite REQUIRES it
		// to classify, and its raw bytes do not re-parse. So the trim is applied
		// here too, and the two sides are now the same string by construction
		// rather than by assertion.
		//
		// The half that was always true is the one that matters: the record is
		// §4.7-ADMITTED. DescriptorScreen encodes on the way to a plate, and
		// admission is what keeps a §4.2 zero-Script descriptor -- the titled
		// zero-key BlueWallet shape -- out of Descriptor.encode's panicking
		// default arm. Parsing proves only that a descriptor exists; admission
		// proves it is one of §4.7's seven forms with every conjunct holding.
		//
		// The error return stays. It is no longer guarding an impossibility, and
		// even when it was, a silent nil dereference would have been a worse
		// answer than leaving the program.
		desc, err := nonstandard.OutputDescriptor([]byte(strings.TrimSpace(body)))
		if err != nil {
			showError(ctx, th, title, "Couldn't read the wallet policy from the payload.")
			return
		}
		descriptorFlow(ctx, th, desc)
		return
	}
	var gathered []bundleCard
	for {
		// Resume with what was already scanned: Back from consent returns to the
		// gather WITH THE CARDS STILL ON IT (the "going back should lose nothing"
		// directive). Back at the gather itself leaves the program.
		cards, ok := bundleGatherFlowResume(ctx, th, title, gathered)
		if !ok {
			return
		}
		gathered = cards
		// device-csid-warning Contract 3: notice modal at gather set completion.
		showBundleCSIDMismatchNotices(ctx, th, title, cards)
		md1, ok := walletPolicyMd1(cards)
		if !ok {
			showError(ctx, th, title, "Supply exactly one wallet policy (md1) card.")
			continue
		}
		keyCards, err := walletPolicyKeyCards(cards)
		if err != nil {
			showError(ctx, th, title, "Couldn't read one of the key cards.")
			continue
		}
		lines, err := walletPolicyConsentLines(md1, keyCards)
		if err != nil {
			showError(ctx, th, title, err.Error())
			continue
		}
		if !confirmReviewScreen(ctx, th, title, lines) {
			continue // Back → re-gather, cards intact.
		}
		if !bundleReviewFlow(ctx, th, cards) {
			continue
		}
		bundleEngrave(ctx, th, title, cards, "", "")
		return
	}
}

// walletPolicyMd1 picks the ONE wallet-policy md1 out of the gathered set.
//
// Deliberately NOT extractSuppliedMd1, and the difference is mk1. That function
// refuses any key card, because Engrave Multisig's supply is "one policy, and
// the seed supplies the rest". Here key cards are legitimate cargo: an operator
// engraving someone else's wallet policy may hold its key plates too, and
// bundleEngrave cuts every card in the set. A secret card is still refused —
// ms1 is refused upstream at classify, and this is the defensive second line.
func walletPolicyMd1(cards []bundleCard) ([]string, bool) {
	var md1 []string
	count := 0
	for _, c := range cards {
		switch c.kind {
		case cardMD1:
			count++
			md1 = c.strings
		case cardMS1:
			return nil, false
		}
	}
	if count != 1 {
		return nil, false // 0 (nothing to prove) or ≥2 (which one is the wallet?).
	}
	return md1, true
}

// walletPolicyConsentLines is the proof (plan D2), in the order an operator
// checks it: which wallet, what shape, where it pays.
func walletPolicyConsentLines(md1 []string, keyCards []mk.Card) ([]string, error) {
	tpl, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		return nil, errors.New("Couldn't decode the supplied wallet policy.")
	}

	// D3's first half: a KEYLESS template plus gathered mk1 key cards is a
	// wallet this device can prove, once each card is seated at the slot whose
	// declared origin it matches. Skipping the gather is still valid and still
	// reaches consent — without address proof — which is D3's second half and
	// shipped first.
	//
	// Only when the template carries no keys of its own. A full-policy card
	// already has them, and seating over the top would let a stray key card
	// silently replace one the policy declared.
	if len(keyCards) > 0 && templateIsKeyless(keys) {
		seated, err := seatKeyCards(md1, keyCards)
		if err != nil {
			return nil, seatRefusalMessage(err)
		}
		keys = seated
	}

	// THE ID, NAMED. The kind comes from the codec rather than from a guess
	// here, so this cannot label a template id as a policy id: they are both 16
	// hex bytes, they differ for the same wallet, and an operator comparing the
	// wrong one against a coordinator reads the mismatch as a corrupted backup.
	id, kind, err := md.FormAwareIdChunks(md1)
	if err != nil {
		return nil, errors.New("Couldn't compute this policy's wallet id.")
	}
	lines := []string{fmt.Sprintf("%s: %x", kind, id)}

	if kind == md.WalletIdTemplate {
		// A keyless template is engravable on purpose (D4) — but only if the
		// shipped off-device toolkit can reconstruct it, or the plate is an
		// unrecoverable backup. Same guard the template-engrave opt-in runs, so
		// the two paths refuse the same shapes.
		//
		// NO LONGER STALE (F-215, closed 2026-08-21). When this program was
		// written the guard still refused `tr(sortedmulti_a)` on a premise that
		// had already died with the S0 pin lift, and this comment said so while
		// calling it anyway — a new program quietly admitting more than the
		// shipped one being the worse of the two errors. The guard has since
		// been narrowed on measurement, for both paths at once, so calling it
		// now refuses only what is genuinely unrecoverable: `sortedmulti` under
		// a combinator, which our own encoder rejects outright, leaving this as
		// defence against a card from some other producer.
		if err := md.TemplateEngraveShapeGuardChunks(md1); err != nil {
			return nil, errors.New("This template can't be restored with the shipped tools.")
		}
	}

	// F-218: REFUSE a policy that seats one key twice, before consent.
	//
	// It reads as k-of-n and one holder can satisfy two of the seats. The build
	// flow has refused this since it shipped — but only where the DEVICE
	// assembled the policy; a card that arrived already built, which is every
	// card this program sees, never reached that check.
	//
	// Refused rather than warned about, matching the build flow and the host.
	// A warning on a consent screen is a thing to tap past.
	if a, b, dup := md.DuplicateKeySlots(keys); dup {
		return nil, fmt.Errorf(
			"Slots @%d and @%d hold the same key. This policy names %d cosigners "+
				"but one of them holds two seats.", a, b, tpl.N)
	}

	lines = append(lines, md1Summary(tpl)...)
	lines = append(lines, walletPolicyAddressLines(md1, tpl, keys)...)
	return lines, nil
}

// addrProofPerChain is how many addresses per chain the consent screen shows.
//
// Plan D6: receive AND change, FEWER of each — change is where a policy
// mismatch silently loses funds, so proving both chains derive beats proving one
// chain five times.
const addrProofPerChain = 2

// walletPolicyAddressLines derives the consent screen's address proof, or says
// plainly why there is none.
//
// IT NEVER RETURNS EMPTY. An absent address block is indistinguishable from a
// screen that simply has no addresses on it, and "I did not see any addresses"
// is exactly the observation that should stop an operator. So every path here
// produces a line, including the two that produce no address.
func walletPolicyAddressLines(md1 []string, tpl md.Template, keys []md.ExpandedKey) []string {
	at, ok := policyAddressAt(md1, tpl, keys)
	if !ok {
		if len(keys) == 0 {
			return []string{"", "Keyless template - no addresses.", "Verify off-device."}
		}
		for _, k := range keys {
			if !k.XpubPresent {
				return []string{"", "Template has no keys - no addresses.", "Verify off-device."}
			}
		}
		// Keys are present and it still cannot derive: an unsupported shape
		// (F-214). Say which of the two it is — "no addresses" for a policy that
		// HAS keys means something different, and an operator who reads it as
		// "keyless" would conclude the card was stripped.
		return []string{"", "This device can't derive", "addresses for this policy."}
	}
	lines := []string{""}
	for _, chain := range []struct {
		label  string
		change bool
	}{{"Receive", false}, {"Change", true}} {
		for i := 0; i < addrProofPerChain; i++ {
			a, err := at(uint32(i), chain.change)
			if err != nil {
				return []string{"", "Address derivation failed.", "Do not engrave this card."}
			}
			lines = append(lines, fmt.Sprintf("%s %d:", chain.label, i), a)
		}
	}
	return lines
}

// policyAddressAt resolves a decoded policy to an address deriver, over BOTH
// routes — the flat *bip380.Descriptor one and the complex one.
//
// gatheredDescriptorFlow makes this choice too, but as control flow rather than
// a value, because it also decides which SCREEN to show. Here only the deriver
// is wanted. The routing order is identical in both, which is what keeps the
// consent screen's addresses and the inspect screen's addresses the same
// addresses.
func policyAddressAt(md1 []string, tpl md.Template, keys []md.ExpandedKey) (func(uint32, bool) (string, error), bool) {
	if desc, status := expandedToDescriptor(tpl, keys); status == expandOK {
		return func(i uint32, change bool) (string, error) {
			if change {
				return address.Change(desc, i)
			}
			return address.Receive(desc, i)
		}, true
	}
	return complexAddressSource(md1, keys)
}

// templateIsKeyless reports whether NO slot carries an xpub.
//
// All-or-nothing on purpose. A partially-keyed card is not a template waiting
// for cards; it is a shape this device already refuses to derive from, and
// seating into the gaps would produce a wallet that is half what was engraved
// and half what happened to be scanned.
func templateIsKeyless(keys []md.ExpandedKey) bool {
	for _, k := range keys {
		if k.XpubPresent {
			return false
		}
	}
	return len(keys) > 0
}

// walletPolicyKeyCards decodes the mk1 key cards out of the gathered set.
func walletPolicyKeyCards(cards []bundleCard) ([]mk.Card, error) {
	var out []mk.Card
	for _, c := range cards {
		if c.kind != cardMK1 {
			continue
		}
		card, err := mk.Decode(c.strings)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}

// seatRefusalMessage turns a typed seating refusal into the sentence an
// operator can act on.
//
// ONE SENTENCE PER CASE, because "your key cards were refused" is the worst
// version of a correct refusal: it is accurate, actionable by nobody, and
// indistinguishable from a broken device. Each of these names what the device
// saw and what the operator can do about it.
func seatRefusalMessage(err error) error {
	switch {
	case errors.Is(err, errSeatNotThisPolicy):
		// The likeliest cause by far, and it is not operator error: a key card
		// minted for the FULL policy carries the POLICY id stub, while a keyless
		// template's stub is the TEMPLATE id. Same wallet, two different stubs.
		return errors.New(
			"A key card doesn't belong to this policy. Note that key cards made " +
				"for the full-policy card carry a different stub than a " +
				"template-only card expects.")
	case errors.Is(err, errSeatNoSlot):
		return errors.New(
			"A key card's derivation path matches no slot in this policy. " +
				"Check it belongs to this wallet.")
	case errors.Is(err, errSeatSlotUnfilled):
		return errors.New(
			"Some slots have no key card yet. Gather one card per slot, or " +
				"skip and continue without address proof.")
	case errors.Is(err, errSeatSlotContested):
		// The undecidable case, and the one where guessing would be invisible.
		// It happens when the template declares no fingerprints and two slots
		// share a derivation path — a stripped template cannot tell them apart.
		return errors.New(
			"Two different key cards claim the same slot, and this template " +
				"can't tell them apart. It declares no fingerprints.")
	}
	return errors.New("Couldn't match the key cards to this policy.")
}
