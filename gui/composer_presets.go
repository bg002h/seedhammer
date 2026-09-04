package gui

import "seedhammer.com/md"

// The six named archetypes of SPEC_wallet_policy_composer.md §4d, as
// md.PathList shapes the operator can start from instead of a blank list.
//
// NOTHING NORMATIVE IS DECIDED HERE (CLAUDE.md's Rust-primary rule). Every
// shape below is transcribed from the primary's own exported vector, produced
// by `md compose --preset <name>` on descriptor-mnemonic main at 1dc8d409
// (F-453 / plan S0b), and gui/composer_presets_test.go compares the chunks
// md.Compose builds from each entry against the vendored
// md/testdata/vectors/keyed_compose_preset_<archetype>.phrase.txt. If they
// ever differ, the VECTOR wins and this table changes.
//
// THE PARAMETERS ARE THE VECTORS' PARAMETERS, VERBATIM, and that is S0b's
// ruling rather than a test corpus's accident: the smallest legal shape of
// each archetype is the honest starter on a device whose only editor is a
// scroll wheel, and a wider tier is one edit away while a shape the operator
// did not intend is not. The invocation each entry reproduces is named on it.
//
// THE TWO WRAPPERS SHARE ONE TABLE, because the origins are already
// wrapper-derived: md.DefaultOrigin(w, account) is m/48'/0'/<account>'/<script
// type>' and ComposeWrapper.ScriptType() supplies 2 for wsh and 3 for tr,
// which is exactly the difference between the five wsh templates'
// 48'/0'/<i>'/2' and the tr template's 48'/0'/<i>'/3'. So an archetype is
// built once, at the wrapper it is asked for, and no origin is hardcoded.

// composerPreset is one offered archetype: the label the operator picks and
// the path list it seeds the shape flow with.
type composerPreset struct {
	name string
	list md.PathList
}

// composerPresetKeys is k-of-n over fresh slots, sorted -- the form every
// preset uses. §5's lowering decides whether a position emits sortedmulti or
// a bare multi; asking for sorted here and being LOWERED to multi is not the
// unsorted choice §8b warns about, which is why every entry below sets it.
func composerPresetKeys(k, n uint8) *md.KeySet { return &md.KeySet{K: k, N: n, Sorted: true} }

func composerPresetOlder(blocks uint32) *md.Lock {
	return &md.Lock{Kind: md.LockOlderBlocks, Value: blocks}
}

func composerPresetAfter(height uint32) *md.Lock {
	return &md.Lock{Kind: md.LockAfterHeight, Value: height}
}

// composerPresetDigest is the digest keyed_compose_preset_hashlock_gated
// carries. It is READ OFF THE VECTOR, never typed from memory: a hashlock
// whose preimage nobody holds is a path that can never be spent, so the one
// value here that an operator cannot check by eye is the one pinned hardest.
func composerPresetDigest() *[32]byte {
	var d [32]byte
	for i := range d {
		d[i] = 0xa8
	}
	return &d
}

// composerPresets returns the archetypes offered under w (§4d).
//
// All six under wsh and tr. Under sh and sh(wsh), plain k-of-n ALONE, because
// §4e admits nothing else there -- offering a shape the wrapper refuses turns
// a menu into a refusal the operator meets only after choosing.
func composerPresets(w md.ComposeWrapper) []composerPreset {
	// plain-multisig, `md compose --preset plain-multisig,2of3`:
	//   wsh(sortedmulti(2,@0,@1,@2))
	plain := composerPreset{"plain-multisig", md.PathList{Wrapper: w, Paths: []md.SpendPath{
		{Keys: composerPresetKeys(2, 3)},
	}}}
	if w == md.ComposeSh || w == md.ComposeShWsh {
		return []composerPreset{plain}
	}
	return []composerPreset{
		plain,
		// simple-timelocked-inheritance,
		// `--preset simple-timelocked-inheritance,older=26280`:
		//   wsh(or_i(pkh(@0),and_v(v:pkh(@1),older(26280))))
		{"simple-timelocked-inheritance", md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: composerPresetKeys(1, 1)},
			{Keys: composerPresetKeys(1, 1), Lock: composerPresetOlder(26280)},
		}}},
		// kofn-recovery, `--preset kofn-recovery,2of3,older=26280`:
		//   tr(NUMS,{multi_a(2,@0,@1,@2),and_v(v:pk(@3),older(26280))})
		{"kofn-recovery", md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: composerPresetKeys(2, 3)},
			{Keys: composerPresetKeys(1, 1), Lock: composerPresetOlder(26280)},
		}}},
		// tiered-recovery, `--preset tiered-recovery,2of2,1of2,older=26280`:
		//   wsh(or_d(multi(2,@0,@1),and_v(v:multi(1,@2,@3),older(26280))))
		{"tiered-recovery", md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: composerPresetKeys(2, 2)},
			{Keys: composerPresetKeys(1, 2), Lock: composerPresetOlder(26280)},
		}}},
		// hashlock-gated,
		// `--preset hashlock-gated,older=26280,sha256=a8..a8`:
		//   wsh(or_i(and_v(v:pkh(@0),sha256(a8..)),and_v(v:pkh(@1),older(26280))))
		{"hashlock-gated", md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: composerPresetKeys(1, 1), Hash: composerPresetDigest()},
			{Keys: composerPresetKeys(1, 1), Lock: composerPresetOlder(26280)},
		}}},
		// decaying-multisig, `--preset decaying-multisig,2of2,1of1,
		// older1=13140,older2=26280,after=1000000`:
		//   wsh(or_i(and_v(v:multi(2,@0,@1),older(13140)),
		//        or_i(and_v(v:pkh(@2),older(26280)),
		//             and_v(v:pkh(@3),after(1000000)))))
		// older1 locks the FIRST (primary) tier: the primary tier does not
		// spend immediately, which is the whole of "decaying".
		{"decaying-multisig", md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: composerPresetKeys(2, 2), Lock: composerPresetOlder(13140)},
			{Keys: composerPresetKeys(1, 1), Lock: composerPresetOlder(26280)},
			{Keys: composerPresetKeys(1, 1), Lock: composerPresetAfter(1_000_000)},
		}}},
	}
}

// composerPresetPick offers the archetypes legal under w. §7b's step is
// "Wrapper -> preset or blank -> paths", so this sits between
// composerWrapperPick and composerShapeFlow.
//
// S4 walk W-1 (2026-09-02, on the device): the blank route used to be the
// Back key alone. An operator who wanted their own shape saw six presets and
// no way forward, and the one key that worked was the one they expected to go
// backwards. So the blank route is now a ROW, and the first one: the default
// selection commits to nothing. Back here now means back, to the wrapper
// choice, and the caller does that.
//
// `replace` IS WHAT THE ROW MEANS, NOT WHAT THE LIST HOLDS, and the
// distinction is W-6's: this screen is reached a second time, by Back out of
// the path list, and there the blank row means "I am building my own paths,
// leave them alone". Returning a blank list for it would make the DEFAULT row
// of a screen the operator reached by pressing BACK the one that silently
// discards their composition. On the first pass the current list is empty, so
// "keep" and "blank" are the same thing and the row reads the same way.
const composerPresetBlankRow = "Build my own paths"

func composerPresetPick(ctx *Context, th *Colors, w md.ComposeWrapper) (list md.PathList, replace, ok bool) {
	presets := composerPresets(w)
	choices := make([]string, 0, len(presets)+1)
	choices = append(choices, composerPresetBlankRow)
	for _, p := range presets {
		choices = append(choices, p.name)
	}
	cs := &ChoiceScreen{Title: "New policy", Lead: "Start from?", Choices: choices}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return md.PathList{Wrapper: w}, false, false
	}
	if sel == 0 {
		return md.PathList{Wrapper: w}, false, true
	}
	return presets[sel-1].list, true, true
}
