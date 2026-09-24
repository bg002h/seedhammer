package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/md"
)

// SPEC_liana_unspendable_internal_key §0b: the KEY PATH choice screen.
//
// WHERE IT SITS, and why there. Between composerShapeFlow and
// composerTemplateChunksFor, in composerFlow's forward leg: the kind changes
// the tree, hence the template chunks, hence the Template-ID and the mk1 stub
// the stub screen tells the operator to COPY ONTO STEEL and mint cosigner
// cards from. Placed any later, those would already have been computed for the
// other wallet.
//
// HOW IT GETS A SHAPE BEFORE THE CHUNKS EXIST. The predicate needs a decoded
// PolicyShape, and the only producer of one is md.PolicyShapeChunks, which
// needs chunks. That is not circular: composing is PURE
// (md.ComposeWithUnspendable(...).Chunks()), so this file composes once for
// itself to evaluate the predicate, and the flow composes again afterwards for
// the stub screen. Nothing is shared between the two calls.
//
// THE RESET IS THE PREDICATE, NOT AN EDIT DETECTOR. composerShapeSignature is
// blind to exactly the edits that flip it (a lock, a hash, a key count that
// makes a path bare), so this does not try to notice edits. It re-evaluates
// the predicate on every forward pass, and a kind the predicate no longer
// admits is dropped -- LOUDLY, because conjunct 2 is a usefulness judgement,
// not a representability one, and a silent change of wallet is the defect
// this screen exists to prevent.

// composerUnspendableDrop names why §0b's predicate does not fire, so a
// dropped kind-1 choice can say which fact moved.
type composerUnspendableDrop struct {
	notTr   bool   // the wrapper has no taproot key path at all
	keyPath int    // > 0: the operator's path number that became the real internal key
	class   string // != "": Liana's first refusal class for the kind-1 composition
	unbuilt bool   // the kind-1 composition itself failed; not a Liana verdict (R0 n1)
}

// composerUnspendableFires is §0b's FIRING PREDICATE, both conjuncts:
//
//	fire <=> the tr internal key is NUMS today (no path became a real key)
//	     AND composerLianaOutsideModelClass returns "" with class 2 skipped
//
// CONJUNCT 2 IS EVALUATED ON THE KIND-1 COMPOSITION, which is what "class 2
// skipped for the new kind" means in code: class 2 fires on KeyPathNUMS, and a
// kind-1 shape reports KeyPathLianaUnspendable, so the classifier skips it by
// construction -- and, by the same construction, does NOT count it as an
// unlocked path (only KeyPathSpendable is counted). SPEC §7 states those as two
// separate rulings; TestComposerLianaClassRulings pins each.
//
// Any composition error is "does not fire": the flow's own compose, a few
// lines later, reports the refusal properly.
func composerUnspendableFires(st *composerState) (bool, composerUnspendableDrop) {
	if st.list.Wrapper != md.ComposeTr {
		return false, composerUnspendableDrop{notTr: true}
	}
	c, err := md.ComposeWithUnspendable(st.list, composerDeclaredOrigins(st), md.UnspendableLiana)
	if err != nil {
		return false, composerUnspendableDrop{unbuilt: true}
	}
	if p, real := c.InternalKeyPath(); real {
		return false, composerUnspendableDrop{keyPath: p + 1}
	}
	chunks, err := c.Chunks()
	if err != nil {
		return false, composerUnspendableDrop{unbuilt: true}
	}
	shape, err := md.PolicyShapeChunks(chunks)
	if err != nil || !shape.Complete {
		return false, composerUnspendableDrop{unbuilt: true}
	}
	if class := composerLianaOutsideModelClass(md.ScriptTr, shape); class != "" {
		return false, composerUnspendableDrop{class: class}
	}
	return true, composerUnspendableDrop{}
}

// composerUnspendableRows is the choice screen's rows, index-aligned with
// composerUnspendableKinds. Row 0 is NUMS: the zero-value trap (§0b DEFAULT
// ROW) is that a default of "Liana" would silently change the wallet for an
// operator who pressed through, so the first-entry default is the wallet
// every earlier firmware built.
//
// THE LIANA ROW'S CLAIM IS CONDITIONAL ON THE SEATING (F-671). On first entry
// nothing is seated; on re-entry the seating is kept, and a seed at two slots
// of one path is a wallet Liana refuses -- composerLianaRefusesSeating, the
// rule the mapping review's §8g warning uses, decides which row is drawn.
func composerUnspendableRows(st *composerState) []string {
	liana := composerCopyUnspendableRowLiana()
	if composerLianaRefusesSeating(st) {
		liana = composerCopyUnspendableRowLianaSameSeed()
	}
	return []string{composerCopyUnspendableRowNUMS(), liana}
}

var composerUnspendableKinds = []md.UnspendableKind{md.UnspendableNums, md.UnspendableLiana}

// composerUnspendableStep is §0b's screen and its reset. true = go on to the
// stub screen; false = Back, which returns to the path list (composerFlow's
// `continue`), the same place Back from the stub screen goes.
//
// THE DEFAULT ROW IS SEEDED ONCE PER ENTRY, FROM st.unspendable -- never from
// a constant. composerPickScreenFrom reads `initial` once, so a re-entry after
// choosing Liana opens on the Liana row, and pressing through keeps it (§0b:
// "a picker that opens on row zero proposes a setting").
func composerUnspendableStep(ctx *Context, th *Colors, st *composerState) bool {
	fire, drop := composerUnspendableFires(st)
	if !fire {
		if st.unspendable != md.UnspendableNums {
			st.unspendable = md.UnspendableNums
			showError(ctx, th, "Key path", composerCopyLianaKeyDropped(composerUnspendableDropCause(drop)))
		}
		return true
	}
	initial := 0
	for i, k := range composerUnspendableKinds {
		if k == st.unspendable {
			initial = i
		}
	}
	sel, ok := composerPickScreenFrom(ctx, th, "Key path", composerCopyUnspendableLead(), composerUnspendableRows(st), initial)
	if !ok {
		return false
	}
	st.unspendable = composerUnspendableKinds[sel]
	return true
}

// composerUnspendableDropCause is the one sentence naming which fact moved.
func composerUnspendableDropCause(d composerUnspendableDrop) string {
	switch {
	case d.notTr:
		return "Only a Taproot policy has a key path to choose."
	case d.keyPath > 0:
		return fmt.Sprintf("Path %d is one key with no lock, so it became the key path, and there is no unspendable key to choose.", d.keyPath)
	case d.unbuilt:
		// A compose or describe failure is not something Liana said, so it
		// must not read as a verdict. If it were ever reached, the operator
		// would meet THIS modal first and composerTemplateChunksFor's refusal,
		// with the real reason, right after it: composerFlow runs the step
		// before it builds the chunks (F-449 stage 4 R1 N2). It is unreachable
		// today only because md.ValidatePathList gates Done and the kind moves
		// nothing but the internal key.
		return "This device could not build the policy with the Liana key."
	default:
		return "Liana would not import this policy (" + d.class + ")."
	}
}

// composerCompose is the ONE place the composer lowers its path list, so the
// operator's key-path choice reaches every artifact: the template the stub
// screen shows, the keyed policy, and the cards minted from them. A second
// call site that went straight to md.ComposeWith would silently build the NUMS
// wallet (TestComposerComposesOnlyThroughOneSite).
func composerCompose(st *composerState, declared []*md.SlotOrigin) (md.Composed, error) {
	c, err := md.ComposeWithUnspendable(st.list, declared, st.unspendable)
	if err != nil {
		return md.Composed{}, err
	}
	// A LIANA CHOICE THAT BUILT SOMETHING ELSE IS REFUSED, not carried on (SPEC
	// §6 row 3; md-cli warns on the same signal, Composed::unspendable_request_
	// unmet). composerUnspendableStep resets the kind whenever conjunct 1 fails,
	// so this cannot fire through the flow today -- and it runs HERE, before
	// the stub screen shows an id, rather than only at consent, where the
	// self-check's biconditional is the later belt (R0 m1).
	// The error's text IS the §8y body: composerShowRefusal draws err.Error()
	// for an error composerRefusalBody does not map.
	if c.UnspendableRequestUnmet() {
		return md.Composed{}, errors.New(composerCopyLianaUnmet())
	}
	// SPEC §6's mint refusals (F-654), on the mint path and NOT in md's
	// encodePayload, which Reassemble also runs over cards this device reads.
	// The predicate keeps a kind-1 choice off every shape these refuse today;
	// this is the belt for the day it does not.
	if err := c.ValidateUnspendableShape(); err != nil {
		return md.Composed{}, err
	}
	return c, nil
}
