package gui

import (
	"strings"
	"testing"
)

// D3's first half from REAL mk1 STRINGS, not hand-built Card structs.
//
// Everything above builds `mk.Card` values directly, which proves the seating
// logic and nothing about the path an operator's card actually takes:
// scan -> chunk-assemble -> mk.Decode -> seat. This closes that gap, and the
// cards below are minted by the `mk` CLI from the conformance vector's own
// xpubs, stubbed on the template's id.
//
// THEY CARRY AN EXPLICIT STUB because `mk encode --from-md1` CANNOT read a
// current md1: it refuses with "wire-format version mismatch: got 9, expected
// 4" -- `mk` vendors an older md-codec. That is F-127's class, still live, and
// it means the one command that derives a stub from a card cannot be used with
// cards this constellation produces today.

func TestSeatingFromRealMk1Strings(t *testing.T) {
	template := []string{
		"md1fd2ztqq9q2tvyyy5jmpprj5qqczf2q9wxasas9633yg",
	}
	keyCardStrings := [][]string{
		{"mk1qps997pqqsqu3l58e4eutks2q5zg3vs7rnefw94m5rru59s2su80aw2q4wgdpapgfl4pkhsdyytkwl5z8lphut2hvvpp58hnsjj45r23mkyz", "mk1qps997pp806lhaeh6reknylagmwyjycf8044xtt9flsdlkvt6f6cthyl9xq5hej2njucewfzexjue"},
		{"mk1qpnwagzqqsqu3l58e4eutks2lcztpqyqsqygpqyqsqygrqyqsqyg9qyqsqyqfz9jrcld706hn9svfgll7zvw5qnkxgea7dhh8v5jgjcp9w4f", "mk1qpnwagzp68w6hzragnj3g5qrl85zeape8wq0vdczfyy55tqsd5576trsa3p40nfpd7hsyjyf7vlx6hk2j6ckr4wf0m3ekgdx7w6lf6vp27h4", "mk1qpnwagzzh29asc49q7gmzkpymx"},
	}

	var cards []bundleCard
	for _, cs := range keyCardStrings {
		cards = append(cards, bundleCard{kind: cardMK1, strings: cs})
	}
	decoded, err := walletPolicyKeyCards(cards)
	if err != nil {
		t.Fatalf("a real mk1 card set did not decode: %v", err)
	}
	if len(decoded) != len(keyCardStrings) {
		t.Fatalf("decoded %d cards, want %d", len(decoded), len(keyCardStrings))
	}

	lines, err := walletPolicyConsentLines(template, decoded)
	if err != nil {
		t.Fatalf("consent refused real cards: %v", err)
	}
	joined := strings.Join(lines, "\n")
	want := vectorAddress(t, "keyed_tr_with_leaf", "0", 0)
	if !strings.Contains(joined, want) {
		t.Fatalf("real mk1 cards did not seat to the Rust-derived address %s:\n%s", want, joined)
	}
	// And the screen must not still be claiming it has nothing.
	if strings.Contains(joined, "no addresses") {
		t.Fatalf("the screen says it has no addresses while showing them:\n%s", joined)
	}
}

// A6 — ONE CARD FILLS TWO SLOTS, and it must, because the policy is legitimate.
//
// `wsh(multi(2, @0/48'/0'/0'/2'/<0;1>/*, @1/48'/0'/0'/2'/<2;3>/*))`: one master,
// one origin, TWO slots that differ only in their multipath branch. The branches
// derive different children at every index, so this is a genuine two-key script
// and not the duplicate F-218 refuses — F-218's check is `(xpub, use-site)`
// precisely so this shape survives it.
//
// It is also the case a naive "one card, one slot" rule would break: the single
// gathered card has to seat itself twice, and refusing that would make a legal
// wallet unprovable.
func TestOneCardFillsTwoSlotsAtDifferentUseSites(t *testing.T) {
	template := []string{
		"md1fl6djqqpq2tvyyy4qqxppsg2qknq2zcqvk7fyv32kdh4x",
	}
	cardStrings := []string{
		"mk1qpsrpvpqqsq7k3qrv3eutks2q5zg3vs7rnefw94m5rru59s2su80aw2q4wgdpapgfl4pkhsdyytkwl5z8lphut2hvvpp5u7459ydp5mne0my",
		"mk1qpsrpvpp806lhaeh6reknylagmwyjycf8044xtt9flsdlkvt6f6cthyl9xqv9j48zyp9thcewjz0a",
	}
	const want = "bc1qsa6qqvkypr9v8ve5z54t8yjtn99ws48w8umyyzyhxy8n6p4msemslqyh88"

	decoded, err := walletPolicyKeyCards([]bundleCard{{kind: cardMK1, strings: cardStrings}})
	if err != nil {
		t.Fatalf("the card did not decode: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected ONE card, got %d — the point is that one card fills two slots", len(decoded))
	}
	lines, err := walletPolicyConsentLines(template, decoded)
	if err != nil {
		t.Fatalf("one card for two slots was refused: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("the seated wallet is not the one the host derives (%s):\n%s", want, joined)
	}
}
