package gui

import (
	"strings"
	"testing"

	"seedhammer.com/mt"
)

// G-P3.9 — ONE CONDITION, ONE BEHAVIOUR, WHICHEVER DOOR IT CAME IN.
//
// A complete mt1 set that does not confirm reached the device two ways and was
// treated two ways:
//
//	payload  -> offered, engraveable, legend replaced   (ruling 2026-08-25b)
//	NFC      -> "Set complete but does not confirm as one transaction. Dropped."
//
// The NFC arm threw away every string the operator had just scanned, one at a
// time, off a tag -- and it contradicted the ruling outright while the payload
// arm three functions up obeyed it.

// The SMUGGLED set: 32 bytes of entropy as a complete, BCH-valid 1-chunk mt1
// set. It reassembles and is not a transaction -- the C3 channel.
const mtSmuggled = "mt1pm6kmqqqqqq4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w4sqxxtg7uwrnug7"

// The 113-byte signature-stripped "even" transaction as a complete 3-chunk
// set. It reassembles, PARSES, and BINDS to its set id -- and cannot be
// broadcast. Only the signature predicate can tell.
var mtStrippedSet = []string{
	"mt1p9h8jqqzqqqqgqqqqqp0jx6jfd0wrjf5y4se6nmvwwl2qmus7ml5c0jv2ux4sevg74rhgdqqhgq73ru3s5kep",
	"mt1p9h8jqqzqqpqqqqqq8allll7qjqfdxqqqqqqqqpvqq5c80qm4p46822m9ldragav0u3eqqvcwzfhcyyza74xq",
	"mt1p9h8jqqzqqzf64nagde9yqsqqqqzcqpgagsjlpfn434f7ajck5y2ykawz8jjqh4ucqqqqqq774jy98z3xll2",
}

// gatherCandidateFor feeds a set to the NFC gather's OWN decision function --
// txGather.offer, the code a scanned string runs through -- one string at a
// time, in the order they came off the tag.
func gatherCandidateFor(t *testing.T, set []string) txCandidate {
	t.Helper()
	g := newTxGather()
	for i, s := range set {
		c, done := g.offer(s)
		if done {
			if i != len(set)-1 {
				t.Fatalf("the gather decided at string %d of %d", i+1, len(set))
			}
			return c
		}
	}
	t.Fatalf("the gather never decided after %d strings: %q", len(set), g.msg)
	return txCandidate{}
}

func payloadCandidateFor(t *testing.T, set []string) txCandidate {
	t.Helper()
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(set...)
	cands, _ := payloadTransactions(ctx)
	if len(cands) != 1 {
		t.Fatalf("payload path produced %d candidates for a %d-string set", len(cands), len(set))
	}
	return cands[0]
}

// THE PARITY ASSERTION. For each of the three ways a set fails, the two
// delivery paths must produce the same verdict, the same legend and the same
// strings -- differing only in `src`, which is what F3 exists to name.
func TestBothDeliveryPathsTreatABrokenSetTheSameWay(t *testing.T) {
	// COMPLETE sets only, and that scoping is itself a finding rather than a
	// convenience. An INCOMPLETE set is not a divergence: the payload is
	// finite, so the payload path knows the set will never grow, while the
	// gather's operator can always present another tag -- "String 3 of 6.
	// 3 to go." is the right answer there, not a substituted legend.
	//
	// What that leaves OPEN, recorded rather than invented: an operator who
	// holds only 3 of 6 tags has no way to engrave the three from the gather.
	// The payload path offers exactly that (ruling 2026-08-25). Closing it
	// means a second button inside a live scanning loop, which changes what
	// Back means -- an operator-shaped decision, not a mechanical one.
	cases := []struct {
		name string
		set  []string
	}{
		{"does not decode", []string{mtSmuggled}},
		{"unsigned inputs", mtStrippedSet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Premise: the set really is complete/incomplete as claimed, and
			// mt.Decode really refuses it. A parity test over a set that
			// CONFIRMS proves nothing.
			if _, err := mt.Decode(tc.set); err == nil {
				t.Fatalf("premise broken: this set confirms, so there is no divergence to test")
			}
			payload := payloadCandidateFor(t, tc.set)
			nfc := gatherCandidateFor(t, tc.set)

			if payload.confirmed != nfc.confirmed {
				t.Errorf("confirmed: payload=%v nfc=%v", payload.confirmed, nfc.confirmed)
			}
			if payload.subst != nfc.subst {
				t.Errorf("legend substitution differs:\n  payload %q\n  nfc     %q",
					payload.subst, nfc.subst)
			}
			if len(payload.strs) != len(nfc.strs) {
				t.Errorf("strings: payload=%d nfc=%d -- the NFC path DROPPED them",
					len(payload.strs), len(nfc.strs))
			}
			if payload.csid != nfc.csid {
				t.Errorf("csid: %05x vs %05x", payload.csid, nfc.csid)
			}
			// The one legitimate difference, and it is named on the screen.
			if payload.src == nfc.src {
				t.Error("src must differ: F3 requires the review screen to name the source")
			}
		})
	}
}

// The THREE substitutions are three sentences, and the unsigned one is not
// "DOES NOT DECODE" -- those bytes decode perfectly, which is the hazard.
func TestTheSubstitutionNamesWhatActuallyWentWrong(t *testing.T) {
	incomplete := payloadCandidateFor(t, txEven[:3]).subst
	broken := payloadCandidateFor(t, []string{mtSmuggled}).subst
	unsigned := payloadCandidateFor(t, mtStrippedSet).subst

	if !strings.Contains(incomplete, "MISSING STRINGS") {
		t.Errorf("incomplete: %q", incomplete)
	}
	if !strings.Contains(broken, "DOES NOT DECODE") {
		t.Errorf("non-decoding: %q", broken)
	}
	if strings.Contains(unsigned, "DOES NOT DECODE") {
		t.Errorf("an unsigned set decodes fine; telling the operator to re-encode "+
			"a correctly-encoded payload sends them nowhere: %q", unsigned)
	}
	if !strings.Contains(unsigned, "UNSIGNED") {
		t.Errorf("unsigned: %q", unsigned)
	}
	for i, a := range []string{incomplete, broken, unsigned} {
		for j, b := range []string{incomplete, broken, unsigned} {
			if i < j && a == b {
				t.Errorf("substitutions %d and %d are the same sentence: %q", i, j, a)
			}
		}
	}
}
