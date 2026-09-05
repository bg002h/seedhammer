//go:build tinygo

package gui

// setComposerStateHook and clearComposerStateHook do nothing on the machine,
// and composerStateHook and ComposerPathHashes do not exist here.
//
// See composer_state_hook.go for why the firmware carries none of them: what
// they would hand over is the composition the operator is building, and the
// only consumer for it lives in a browser. A variable the image does not
// contain cannot be assigned by accident.
//
// WHAT IT COSTS, MEASURED: nothing. Built at the production settings
// (-target pico-plus2 -stack-size 16kb -gc precise -opt 2 -scheduler tasks) on
// the H5 gate tree, against the SAME tree with this file, composerFlow's
// setComposerStateHook call and composerFlowExit's clearComposerStateHook call
// deleted -- so the number is the hook's own share and not a delta inherited
// from frame_hook's measurement, which is a different call in a different place
// and was measured on a different day:
//
//	with the hook, one defer                1,599,208 B flash / 62,856 B RAM
//	the hook deleted from the tinygo view   1,599,224 B flash / 62,856 B RAM
//
// THE HOOK'S SHARE IS -16 B, WHICH IS ZERO PLUS LAYOUT NOISE, and saying so is
// the honest form of the claim. The image WITHOUT the hook is 16 bytes LARGER;
// nothing here can cost negative flash, so what the pair measures is that the
// hook contributes nothing the compiler does not reclaim elsewhere, to within
// the granularity of a whole-image build. The r0 fold is what showed this:
// before it, on a tree differing only in four operator-facing string literals,
// the same pair measured 1,599,164 and 1,599,164 -- an exact 0. A delta that
// moves from 0 to -16 when unrelated copy changes shift the layout is not a
// structural zero, and a spec that asserts "0 bytes" of it is asserting the
// noise. It is asserted as "no measurable cost", not as an exact 0.
//
// AND THE ZERO IS NOT AN ARTEFACT OF A BUILD THAT IGNORED THE EDIT. Giving this
// stub a body the compiler cannot drop -- `println("hook")` inside
// setComposerStateHook, exactly that -- moves the image to 1,599,368 B, +160 B.
// Edits here reach the image; this one costs nothing.
//
// WHAT IT COST BEFORE THE SHAPE WAS FIXED: 96 B. composerFlow first cleared the
// hook through a SECOND `defer clearComposerStateHook()` beside the seed
// scrub's own defer, and that measured 1,599,304 B -- TinyGo elides the empty
// call and not the defer record around it. Folding both into the one deferred
// composerFlowExit call the flow already had is what makes it free. Measured,
// because a guess about the compiler in plate_hook_tinygo.go's first version
// was wrong, and this file exists to say what the number IS.
//
// The numbers are recorded in IMPLEMENTATION_PLAN_hashlock_H5_device_polish.md
// Task 5 as well, with the fork baseline they are a delta against.
func setComposerStateHook(*composerState) {}

func clearComposerStateHook() {}
