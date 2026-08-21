package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/address"
	"seedhammer.com/md"
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
	// A payload card is offered ONCE, before gathering, through the SAME offer()
	// a scanned card enters by — bundleFlow does it this way and for the reason
	// stated there: a separate insertion path would be a second way for a card
	// to join the set, and only one of them would have the checks.
	if body, ok := syswOffer(ctx, th, sysw.ClassMDMK, "First card from where?"); ok {
		ctx.syswBundleSeeds = []string{body}
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
		md1, ok := walletPolicyMd1(cards)
		if !ok {
			showError(ctx, th, title, "Supply exactly one wallet policy (md1) card.")
			continue
		}
		lines, err := walletPolicyConsentLines(md1)
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
func walletPolicyConsentLines(md1 []string) ([]string, error) {
	tpl, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		return nil, errors.New("Couldn't decode the supplied wallet policy.")
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
		// KNOWN STALE, DELIBERATELY KEPT: both shapes it names have moved since
		// it was written (F-215) — `tr(sortedmulti_a)` now round-trips through
		// the toolkit, and `sortedmulti` under a combinator is refused at encode
		// so it cannot reach a card at all. Loosening an admission rule is
		// risk-set work and belongs in its own cycle, applied to BOTH paths at
		// once; a new program quietly admitting more than the shipped one is the
		// worse of the two errors.
		if err := md.TemplateEngraveShapeGuardChunks(md1); err != nil {
			return nil, errors.New("This template can't be restored with the shipped tools.")
		}
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
