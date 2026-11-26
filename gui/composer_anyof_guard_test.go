package gui

import (
	"testing"

	"seedhammer.com/md"
)

// ─── seedhammer I-1: the composer's three shape readers refuse an any-of ─────
//
// composerConsentLinesFor, composerSelfCheck and composerUnspendableFires read
// md.Branch as a CONJUNCTION (every key and lock listed is required). The
// composer cannot build a thresh (md-codec compose has no Tag::Thresh), so an
// any-of entry reaching one of them is a builder defect, and each treats it
// exactly as an incomplete shape (DESIGN_thresh_summary §3, "Other device
// readers of Branches"). Without the guard a reader that ignores AnyN repeats
// the defect I-1 fixed.

// anyOfCaseChunks is the card of a vendored case whose one entry is an any-of.
func anyOfCaseChunks(t *testing.T, name string) []string {
	t.Helper()
	for _, c := range summaryCases(t) {
		if c.Name != name {
			continue
		}
		shape, err := md.PolicyShapeChunks(c.MD1)
		if err != nil {
			t.Fatal(err)
		}
		if !shape.Complete || len(shape.Branches) != 1 || shape.Branches[0].AnyN == 0 {
			t.Fatalf("INCONCLUSIVE: %s is not one any-of entry: %+v", name, shape)
		}
		return c.MD1
	}
	t.Fatalf("no vendored case %s", name)
	return nil
}

func TestComposerConsentRefusesAnAnyOfEntry(t *testing.T) {
	chunks := anyOfCaseChunks(t, "thresh_2of3_keys")
	lines, err := composerConsentLinesFor(chunks, nil, 0, false)
	if err == nil || err.Error() != "md: this device cannot describe the policy it just built" {
		t.Fatalf("consent over an any-of entry: lines %q, err %v; want the incomplete-shape refusal", lines, err)
	}
}

func TestComposerSelfCheckRefusesAnAnyOfEntry(t *testing.T) {
	chunks := anyOfCaseChunks(t, "thresh_2of3_keys")
	err := composerSelfCheck(&composerState{}, chunks)
	if err == nil || err.Error() != "self-check: the decoded policy cannot be described" {
		t.Fatalf("self-check over an any-of entry: %v; want the incomplete-shape refusal", err)
	}
}

// TestComposerUnspendableRefusesAnAnyOfEntry feeds the predicate's judgement a
// shape Liana's model would ACCEPT if the any-of were read as a conjunction:
// one unlocked key path, and a "locked" path whose lock is in fact optional
// (any 2 of 2 keys and older(52560)). Read that way the kind-1 choice fires;
// the any-of must instead be treated as an unbuildable shape.
func TestComposerUnspendableRefusesAnAnyOfEntry(t *testing.T) {
	shape := md.PolicyShape{
		Complete: true,
		KeyPath:  md.KeyPathLianaUnspendable,
		Branches: []md.Branch{
			{Keys: 1},
			{AnyK: 2, AnyN: 3, Keys: 2, Timelock: true,
				Locks: []md.Lock{{Kind: md.LockOlderBlocks, Value: 52560}}},
		},
	}
	asConj := shape
	asConj.Branches = []md.Branch{shape.Branches[0], shape.Branches[1]}
	asConj.Branches[1].AnyK, asConj.Branches[1].AnyN = 0, 0
	if fires, _ := composerUnspendableJudge(asConj, nil); !fires {
		t.Fatal("INCONCLUSIVE: the conjunction reading does not fire, so this test proves nothing")
	}
	fires, drop := composerUnspendableJudge(shape, nil)
	if fires || drop != (composerUnspendableDrop{unbuilt: true}) {
		t.Fatalf("any-of shape: fires %v, drop %+v; want the incomplete-shape drop", fires, drop)
	}
}
