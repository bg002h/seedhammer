package gui

// The engrave form choice (SPEC §7f, C10, C13).
//
// THE "THREE SECRET FORMS" ARE TWO CODE PATHS. engraveSeed (gui/gui.go
// :839-861) builds ONE backup.Seed carrying the words AND a SeedQR and cuts
// them on one plate; backup.SeedString (backup/backup.go:26-31) is the
// string-only plate engraveCodex32 cuts for ms1. A words-only or a QR-only
// plate for a mnemonic does not exist anywhere in this tree. So the picker
// offers the two the device HAS, labelled for what is actually on the plate,
// and F-455 owns the split -- which is a new backup layout with its own
// sizing and its own goldens, not a ChoiceScreen over two existing functions.

type composerForm int

const (
	// composerFormConcrete is §7f's form A: the policy itself, as text or QR
	// plates or keyed md1 strings. Offered only when EVERY slot is seated.
	composerFormConcrete composerForm = iota
	// composerFormTemplateAndCards is form B: keyless md1 WITH fingerprints,
	// plus one mk1 card per seated slot.
	composerFormTemplateAndCards
	// composerFormTemplateOnly is the collapsed case: a key-less composition
	// has no form A and no cards.
	composerFormTemplateOnly
)

// THE SECRET FORM PICKER IS GONE, and its absence is the fix.
//
// It offered "Words and SeedQR" and "ms1 string" and NOTHING CONSUMED THE
// ANSWER: neither engraveSeed nor engraveCodex32 was ever called. §7f names
// three secret forms, this device has two plate designs (F-455), and only one
// of them -- ms1 -- is a bundle card the plate planner carries (cardMS1,
// gui/multisig_engrave.go:36). A backup.Seed words-plus-SeedQR plate needs its
// own plate pass, so composerSecretCards cuts ms1 and the words plate is filed
// with F-455. A picker whose answer nothing reads is worse than one choice
// honestly stated.

// composerFormsFor is §7f's offer, per seating state.
func composerFormsFor(st *composerState) []composerForm {
	seated := 0
	for _, a := range st.assigned {
		if a.src >= 0 {
			seated++
		}
	}
	switch {
	case seated == 0:
		// A key-less composition has no form A and no cards: the choice
		// collapses to template only, and the screen says so.
		return []composerForm{composerFormTemplateOnly}
	case seated < len(st.assigned):
		// PARTIALLY seated (§8p's fallback): no form A either. Its form B is
		// the key-less template, whose unseated slots take §4f's lowest-free
		// accounts, plus one card per SEATED slot carrying the TEMPLATE stub
		// only -- the policy id does not exist until every slot is seated.
		return []composerForm{composerFormTemplateAndCards}
	default:
		return []composerForm{composerFormConcrete, composerFormTemplateAndCards}
	}
}

func composerFormLabel(f composerForm) string {
	switch f {
	case composerFormConcrete:
		return "The policy itself"
	case composerFormTemplateAndCards:
		return "Template plus key cards"
	}
	return "Template only (no keys)"
}

// composerFormPick offers what §7f allows for this seating state, and says
// plainly when there is nothing to choose between.
func composerFormPick(ctx *Context, th *Colors, st *composerState) (composerForm, bool) {
	forms := composerFormsFor(st)
	if len(forms) == 1 {
		lead := "No slot is seated, so there is a template and nothing else."
		if forms[0] == composerFormTemplateAndCards {
			lead = "Some slots are unseated, so this policy has no id yet. " +
				"The template and the cards for the seated slots are what this cuts."
		}
		showError(ctx, th, "What to engrave", lead)
		return forms[0], true
	}
	choices := make([]string, len(forms))
	for i, f := range forms {
		choices[i] = composerFormLabel(f)
	}
	cs := &ChoiceScreen{Title: "What to engrave", Lead: "Which form?", Choices: choices}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return composerFormTemplateOnly, false
	}
	return forms[sel], true
}

// composerEngraveModePick is §7f's Full versus Watch-only, reusing Multisig
// Build's own labels so the two programs cannot describe one decision in two
// ways. buildFullModeLabel names what "Full" LEAVES OUT when a passphrase was
// used: a passphrase is a required spending factor and is never engraved.
func composerEngraveModePick(ctx *Context, th *Colors, st *composerState) (bool, bool) {
	cs := &ChoiceScreen{
		Title:   "Engrave Mode",
		Lead:    "What to engrave?",
		Choices: []string{buildFullModeLabel(st.reg.usesPassphrase()), "Watch-only (keys)"},
	}
	sel, ok := cs.Choose(ctx, th)
	return sel == 0, ok
}
