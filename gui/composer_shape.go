package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/md"
)

// The shape half of the composer (SPEC §7b): wrapper, then an ordered list of
// spend paths the operator edits until it validates.
//
// THE UI'S BOUNDS AND THE CODEC'S REFUSALS ARE BOTH KEPT, and that is not
// redundancy. The picker does not offer an illegal value (§4e: "REFUSE at the
// picker"), which is the kinder half; md.ValidatePathList is the AUTHORITY,
// runs before the shape is left, and its answer is what §8m renders. A bound
// written only in the UI is a bound that can drift from the codec's, and the
// codec is the one that decides what gets engraved.
//
// BACK PRESERVES EVERYTHING (2026-08-19 operator directive, the same rule
// gui/multisig_build.go:291-299 states): Back inside a path editor returns to
// the list with the list intact; Back at the list leaves the shape, and the
// caller decides what that means.

// composerRefusalBody maps a compose sentinel to its §8m line.
//
// ONLY THE FIVE §8 NAMES. An error with no §8m line returns ok=false and the
// caller shows the codec's own message instead of borrowing another
// refusal's words -- a refusal that says the wrong true thing is worse than
// one that says an unpolished true thing.
func composerRefusalBody(err error) (string, bool) {
	switch {
	// An EMPTY list and a list with no keyed path are one sentence to an
	// operator: "Every wallet needs at least one path with a key." Tapping
	// Done on an empty list is a plausible first action, and without this arm
	// it printed md.ErrComposeNoPaths' text -- `md: compose: a wallet needs at
	// least one spend path` -- an internal prefix on an operator screen, which
	// §11 forbids.
	case errors.Is(err, md.ErrComposeNoPaths), errors.Is(err, md.ErrComposeNoKeyedPath):
		return composerCopyRefuseNoKeyedPath(), true
	case errors.Is(err, md.ErrComposeLockOnlyPath):
		return composerCopyRefuseLockOnly(), true
	case errors.Is(err, md.ErrComposeKeylessUnderTr):
		return composerCopyRefuseKeylessTr(), true
	case errors.Is(err, md.ErrComposeLegacyWrapperShape):
		return composerCopyRefuseLegacyShape(), true
	case errors.Is(err, md.ErrComposeTooManySlots):
		return composerCopyRefuseSlotCap(), true
	// §8v IS the body for this condition, and without the arm composerShowRefusal
	// drew the codec's own text -- "md: compose: two slots declare the same
	// origin without two distinct fingerprints ... slots @0 and @1" -- an
	// internal `md:` prefix on an operator screen, which §11 forbids (review
	// r0 M-5). Unreachable today, because composerInvariantViolation refuses at
	// the mapping review before composerArtifactsFor runs; an unmapped sentinel
	// is one refactor away from being reachable with no body.
	case errors.Is(err, md.ErrComposeIndistinguishableSlots):
		return composerCopySameOriginFewFingerprints(), true
	}
	return "", false
}

// composerShowRefusal renders §8m for a compose error, or the codec's own
// message when §8 has no line for it.
func composerShowRefusal(ctx *Context, th *Colors, title string, err error) {
	if body, ok := composerRefusalBody(err); ok {
		showError(ctx, th, title, body)
		return
	}
	showError(ctx, th, title, err.Error())
}

// composerConfirmScreen is the unskippable confirm-to-proceed surface: the
// same ConfirmWarningScreen shape multisigBuildExperimentalWarning uses
// (gui/multisig_build.go:854-871), hold-to-confirm, Back declines.
func composerConfirmScreen(ctx *Context, th *Colors, title, body string) bool {
	warn := &ConfirmWarningScreen{Title: title, Body: body, Icon: assets.IconHammer}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, res := warn.Layout(ctx, th, dims)
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

// composerShapeGuard asks §8j before an edit that CAN move slot numbering.
//
// IT HAS ITS REAL BODY FROM THE START and is never a stub. In Part A nothing
// has been seated, so it returns true without drawing -- which is §7d's own
// rule ("With no slot yet assigned there is nothing to discard and §8j does
// not fire"), not a placeholder standing in for it.
//
// It runs BEFORE the edit, so it cannot know what the operator will change.
// The confirm is therefore asked on ENTRY to an editor that can renumber, and
// composerApplyShapeEdit's signature comparison afterwards decides whether
// anything is actually discarded -- so answering "continue" and then touching
// only a lock keeps the seats, which is §7d's rule for a lock edit.
func composerShapeGuard(ctx *Context, th *Colors, st *composerState) bool {
	if !composerAnySlotAssigned(st) {
		// Nothing is at stake, so nothing is asked. A warning that fires when
		// nothing is at stake is one the operator learns to tap through.
		return true
	}
	return composerConfirmScreen(ctx, th, "Edit the shape",
		composerConfirmBody(composerCopyEditClearsKeys()))
}

// composerWrapperPick is §4a. The legacy wrappers are offered because C7's
// migration needs them, and §4e then holds them to ONE unlocked, unhashed
// key set with n >= 2.
func composerWrapperPick(ctx *Context, th *Colors) (md.ComposeWrapper, bool) {
	choices := []string{"Taproot (tr)", "Segwit (wsh)", "Nested (sh-wsh)", "Legacy (sh)"}
	wrappers := []md.ComposeWrapper{md.ComposeTr, md.ComposeWsh, md.ComposeShWsh, md.ComposeSh}
	cs := &ChoiceScreen{Title: "New policy", Lead: "Which script?", Choices: choices}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return md.ComposeTr, false
	}
	return wrappers[sel], true
}

// composerCountPick offers 1..max on a paged list, so a 9-row picker cannot
// overflow the panel the way an unpaged ChoiceScreen would.
func composerCountPick(ctx *Context, th *Colors, title, lead string, min, max int) (int, bool) {
	if max < min {
		return 0, false
	}
	rows := make([]string, 0, max-min+1)
	for v := min; v <= max; v++ {
		rows = append(rows, fmt.Sprintf("%d", v))
	}
	sel, ok := composerPickScreen(ctx, th, title, lead, rows)
	if !ok {
		return 0, false
	}
	return min + sel, true
}

// composerKeysEdit asks for n then k, within the picker's bounds, and offers
// the sorted choice ONLY where §5 makes sorted legal.
func composerKeysEdit(ctx *Context, th *Colors, st *composerState, idx int) bool {
	max := composerMaxKeysForPath(st, idx)
	if max == 0 {
		// The 33rd slot, refused where the operator asked for it.
		showError(ctx, th, "Keys", composerCopyRefuseSlotCap())
		return false
	}
	min := 1
	if st.list.Wrapper == md.ComposeSh || st.list.Wrapper == md.ComposeShWsh {
		// §4a: n = 1 is refused at the picker under the legacy wrappers.
		min = 2
		if max < 2 {
			showError(ctx, th, "Keys", composerCopyRefuseLegacyShape())
			return false
		}
	}
	n, ok := composerCountPick(ctx, th, "Keys", fmt.Sprintf("Path %d: how many keys?", idx+1), min, max)
	if !ok {
		return false
	}
	k, ok := composerCountPick(ctx, th, "Threshold", fmt.Sprintf("Path %d: how many must sign?", idx+1), 1, n)
	if !ok {
		return false
	}
	// SORTED IS LEFT TRUE HERE AND THE QUESTION IS ASKED LATER.
	//
	// It used to be asked right here, and that was a decision the device
	// reversed in silence: §5's key-set rule lowers a SOLE unlocked, unhashed
	// path to sortedmulti and ANY other multi-key path to `multi`, so adding a
	// second path turned an answered-sorted path into an order-dependent one
	// with nothing said. The operator was left holding the opposite of §8b's
	// warning from a screen this flow had just shown them.
	//
	// So the question moves to the transition out of the shape
	// (composerKeyOrderStep), where `sole` is FINAL and cannot be reversed
	// afterwards. Restating the outcome on a row instead would describe the
	// reversal after the fact; asking once, at the end, removes it.
	st.list.Paths[idx].Keys = &md.KeySet{K: uint8(k), N: uint8(n), Sorted: true}
	return true
}

// composerKeyOrderStep asks §5's key-order question ONCE, at the transition
// out of the shape, and only where §5 makes a sorted form legal at all.
//
// Called after ValidatePathList has accepted the list, so `sole` is final: no
// later edit can turn the answer into its opposite without coming back through
// here. §8b fires only on the decline, which is §5a's rule -- never on a
// lowering-forced `multi`, where the operator declined nothing.
func composerKeyOrderStep(ctx *Context, th *Colors, st *composerState) bool {
	if !composerSortedIsLegal(st.list, 0) {
		return true
	}
	cs := &ChoiceScreen{
		Title:   "Key order",
		Lead:    "Sorted keys, or your order?",
		Choices: []string{"Sorted (usual)", "Keep my order"},
	}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return false
	}
	if sel == 0 {
		st.list.Paths[0].Keys.Sorted = true
		return true
	}
	if !composerConfirmScreen(ctx, th, "EXPERIMENTAL",
		composerConfirmBody(composerCopyUnsortedKeys())) {
		return false
	}
	st.list.Paths[0].Keys.Sorted = false
	return true
}

// composerAddPath appends a path and runs the §8a confirm when the operator
// makes it key-less.
func composerAddPath(ctx *Context, th *Colors, st *composerState) {
	if len(st.list.Paths) >= md.ComposeMaxPaths {
		// Unreachable from the list, which stops offering the row at the cap.
		// Kept as a bound rather than a screen: a refusal with no §8 home is
		// copy no gate covers (§11).
		return
	}
	idx := len(st.list.Paths)
	st.list.Paths = append(st.list.Paths, md.SpendPath{})
	cs := &ChoiceScreen{
		Title:   fmt.Sprintf("Path %d", idx+1),
		Lead:    "What can spend on this path?",
		Choices: []string{"Keys", "A hash, no keys"},
	}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		st.list.Paths = st.list.Paths[:idx]
		return
	}
	if sel == 0 {
		if !composerKeysEdit(ctx, th, st, idx) {
			st.list.Paths = st.list.Paths[:idx]
		}
		return
	}
	// A key-less path is wsh-only and EXPERIMENTAL (§4b, C16). Under tr it is
	// refused with §8m line 3 rather than confirmed.
	if st.list.Wrapper == md.ComposeTr {
		st.list.Paths = st.list.Paths[:idx]
		showError(ctx, th, fmt.Sprintf("Path %d", idx+1), composerCopyRefuseKeylessTr())
		return
	}
	// §8a FIRES ON EVERY KEY-LESS PATH THAT IS CREATED, with no memo.
	//
	// It used to be memoised as keylessConfirmed[idx], and an index is not an
	// identity: "Remove path" splices the slice and leaves the map, so adding
	// path 1 key-less, removing it and adding another put the new path back at
	// index 0 with the confirm already recorded -- an unskippable
	// confirm-to-proceed that could be skipped (C16). A path is created
	// exactly once, here, so firing on creation IS "once per key-less path"
	// (§8a) and needs no state to be wrong about.
	if !composerConfirmScreen(ctx, th, "EXPERIMENTAL",
		composerConfirmBody(composerCopyKeylessPath())) {
		st.list.Paths = st.list.Paths[:idx]
		return
	}
	if !composerHashEdit(ctx, th, st, idx) {
		st.list.Paths = st.list.Paths[:idx]
		return
	}
	// A path that ends with NEITHER keys nor a hash is a cancel, not a path.
	// The key-less route's last pick-list row is "No hash lock", which clears
	// the digest and returns true, and the empty path it left read
	// "Path N: hash only" on the row and was refused at Done with the
	// lock-only body -- naming a lock nobody set.
	if st.list.Paths[idx].Keys == nil && st.list.Paths[idx].Hash == nil {
		st.list.Paths = st.list.Paths[:idx]
	}
}

// composerPathEdit is one path's own menu.
func composerPathEdit(ctx *Context, th *Colors, st *composerState, idx int) {
	for !ctx.Done {
		choices := []string{"Keys", "Time lock", "Hash lock", "Remove path"}
		if idx > 0 {
			// §5 makes LISTED ORDER decide the or_i/or_d nesting, the leaf
			// depth on the taproot spine and which path becomes the internal
			// key -- so an order mistake is a different wallet, and without a
			// move the only repair was remove-and-re-add, which §8j pays for
			// with every seat.
			choices = append(choices, "Move up")
		}
		cs := &ChoiceScreen{
			Title:   fmt.Sprintf("Path %d", idx+1),
			Lead:    composerPathLine(st.list.Paths[idx], idx),
			Choices: choices,
		}
		sel, ok := cs.Choose(ctx, th)
		if !ok {
			return
		}
		switch sel {
		case 0:
			// THE GUARD IS ON THIS ARM AND ON REMOVE UNCONDITIONALLY, and on
			// the lock and hash arms only where the codec says the edit can
			// renumber. §7d: "A lock or hash edit moves no slot, keeps
			// assignments", and §7g classifies it DEFAULT -- true under wsh,
			// FALSE under tr, where the lock picks the internal key. Asking
			// §8j before every lock editor told an operator who wanted to
			// change a lock that every key would be cleared -- false for the
			// edit they intended -- and declining it left the lock uneditable
			// at all; asking it before none let a tr lock edit move slot @0 in
			// silence (verification C-1/I-1).
			if !composerShapeGuard(ctx, th, st) {
				continue
			}
			// SNAPSHOT AND RESTORE, not clear. A decline at any screen inside
			// the key editor used to leave Keys == nil on a path that already
			// had a key set, which then read "hash only" on the row and was
			// refused at Done with a body about a lock nobody set.
			before := st.list.Paths[idx].Keys
			composerApplyShapeEdit(st, func() {
				if !composerKeysEdit(ctx, th, st, idx) {
					st.list.Paths[idx].Keys = before
				}
			})
		case 1:
			// §8j IS ASKED HERE ONLY WHERE IT IS TRUE. Under wsh a lock moves
			// no slot and the confirm must not fire (§7g calls this edit
			// DEFAULT); under tr the lock decides which path supplies the
			// internal key, so clearing one can hand slot @0 to another path.
			// composerEditCanRenumber asks the codec which case this is.
			if composerEditCanRenumber(st.list, idx, composerFieldLock) && !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerApplyShapeEdit(st, func() {
				composerLockEdit(ctx, th, st, idx)
			})
		case 2:
			if composerEditCanRenumber(st.list, idx, composerFieldHash) && !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerApplyShapeEdit(st, func() {
				composerHashEdit(ctx, th, st, idx)
			})
		case 3:
			if !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerApplyShapeEdit(st, func() {
				st.list.Paths = append(st.list.Paths[:idx], st.list.Paths[idx+1:]...)
				// Post-impl interruption M-1: removing the last hashed path is
				// the other event after which no phrase-set hash can remain.
				composerHashByPhraseSync(st)
			})
			return
		case 4:
			if !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerMoveUp(st, idx)
			return
		}
	}
}

// composerShapeFlow runs the path list until it validates, then returns true.
//
// BACK RETURNS FALSE AND THE CALLER GOES BACK ONE STEP, TO THE WRAPPER, WITH
// THE LIST INTACT. It used to make composerFlow return, dropping the wrapper,
// every path, every lock, every digest and every confirm already given --
// against §7b's own rule ("going back should lose nothing") and the standing
// 2026-08-19 directive. Nothing here clears st.list, so the state the caller
// re-enters with is the state the operator left.
//
// THE DISCARD RULE HAS ONE PLACE TO LIVE, and it is composerApplyShapeEdit:
// composerPathEdit's Keys and Remove arms, composerAddPath, the wrapper row,
// the Back leg's composerStartStep, and -- since the S4 walk W-7 verification
// -- the Lock and Hash arms too. The MOVE arm is the one exception and stays
// one: composerMoveUp discards unconditionally, because a swap of two paths
// with equal key counts leaves the signature identical (see its own comment,
// measured). §7d used to say a lock or a hash
// edit moves no slot, and that is true under wsh and FALSE under tr, where the
// internal key is the first bare single: those two arms therefore ask §8j
// exactly when composerEditCanRenumber says the edit can move the codec's
// mapping, and apply through composerApplyShapeEdit either way.
func composerShapeFlow(ctx *Context, th *Colors, st *composerState) bool {
	for !ctx.Done {
		rows := make([]string, 0, len(st.list.Paths)+3)
		for i, p := range st.list.Paths {
			rows = append(rows, composerPathLine(p, i))
		}
		if len(st.list.Paths) < md.ComposeMaxPaths {
			// §4e rules the path cap "at the picker (the picker does not offer
			// the value)". Refusing it afterwards needed an ad-hoc string that
			// is in no §8 table, so neither the glyph gate nor the modal-fits
			// gate covered it; not offering the row is what §4e asks for and
			// needs no copy at all.
			rows = append(rows, "Add a spend path")
		}
		// §7g's row "shape | edits the wrapper ... after a slot was assigned"
		// needs an affordance to be reachable at all, and §12 item 4 names a
		// wrapper change after seating as one of its vectors. Without this row
		// composerShapeSignature's wrapper term was unreachable from the flow.
		rows = append(rows, "Change the script")
		rows = append(rows, "Done")
		lead := composerSlotsKeysLine(st)
		sel, ok := composerPickScreen(ctx, th, "Spend paths", lead, rows)
		if !ok {
			return false
		}
		// THE ROWS ARE DISPATCHED BY NAME, not by an arithmetic offset: the
		// "Add a spend path" row disappears at the cap (§4e), so an index
		// computed from len(st.list.Paths) would silently address the wrong
		// action on a full list.
		switch {
		case sel < len(st.list.Paths):
			composerPathEdit(ctx, th, st, sel)
		case rows[sel] == "Add a spend path":
			if !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerApplyShapeEdit(st, func() { composerAddPath(ctx, th, st) })
		case rows[sel] == "Change the script":
			if !composerShapeGuard(ctx, th, st) {
				continue
			}
			composerApplyShapeEdit(st, func() {
				w, ok := composerWrapperPick(ctx, th)
				if !ok {
					return
				}
				st.list.Wrapper = w
			})
		default:
			if _, err := md.ValidatePathList(st.list); err != nil {
				composerShowRefusal(ctx, th, "Spend paths", err)
				continue
			}
			if !composerKeyOrderStep(ctx, th, st) {
				continue
			}
			if composerEveryPathHashed(st.list) {
				showError(ctx, th, "Spend paths", composerCopyHashEveryPathFor(st))
			}
			return true
		}
	}
	return false
}

// composerMoveUp swaps a path with the one above it and ALWAYS discards the
// seats, reporting that it did.
//
// It does not go through composerApplyShapeEdit, and that is the fix rather
// than an inconsistency (review r0 I-1). Reordering two paths with EQUAL key
// counts leaves the signature identical -- still true now that it carries the
// codec's own mapping, and measured rather than assumed: `w1/1,1,|0.0/1.0/`
// before and after a swap, because §5 numbers slots by first appearance in
// LISTED order and a swap moves both paths at once, so every slot index keeps
// its ordinal position. The signature therefore discarded nothing, after
// composerShapeGuard had already drawn §8j: "Slot numbers change with the
// shape. Every key you seated will be cleared." The retained assignments then
// denoted different spend paths -- the family's keys behind the timelock, the
// recovery keys spending immediately -- with no screen saying so, and
// composerSelfCheck agreed because st.list had moved with them.
//
// Discarding unconditionally is what §8j already promised. Move up is the one
// edit whose numbering effect the signature cannot see, because it changes the
// ORDER of paths and not their shape.
func composerMoveUp(st *composerState, idx int) bool {
	if idx <= 0 || idx >= len(st.list.Paths) {
		return false
	}
	st.list.Paths[idx-1], st.list.Paths[idx] = st.list.Paths[idx], st.list.Paths[idx-1]
	composerDiscardAssignments(st)
	return true
}
