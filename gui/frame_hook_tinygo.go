//go:build tinygo

package gui

import "seedhammer.com/gui/op"

// notifyFrame does nothing on the machine, and FrameAware does not exist here.
//
// See frame_hook.go for why the firmware carries neither: the op it would pass
// is the operator's words as drawn text, and the only consumer for it lives in
// a browser. An interface the image does not contain cannot be implemented by
// accident.
//
// WHAT IT COSTS, MEASURED: nothing. Not one byte. Built at the production
// settings (-target pico-plus2 -stack-size 16kb -gc precise -opt 2
// -scheduler tasks) with VERSION pinned to "measure" so the stamp could not
// differ:
//
//	10286e4 (before the hook)                     sha256 b3e622a8...
//	the hook, with the run_flow.go call           sha256 b3e622a8...
//	the hook, with ONLY that call deleted         sha256 b3e622a8...
//
// All three are byte-identical at 2,683,904 bytes. TinyGo eliminates the call
// completely.
//
// THAT IS THE OPPOSITE OF plate_hook_tinygo.go's RESULT, which is why this is
// written down rather than assumed in either direction. Its per-job call costs
// 486,697 bytes; this per-frame one costs zero. Both numbers were measured on
// the same toolchain, and plate_hook's was RE-measured while writing this --
// deleting its notifyPlate call at this commit still moves the image
// (116affd0... against b3e622a8...), so that claim has not gone stale, the two
// hooks simply differ. WHY they differ is not established here, and no
// explanation is offered for it: the difference is the measurement, and a
// plausible story about argument materialisation is exactly the kind of
// compiler claim that was wrong the first time.
//
// The zero is real and not an artefact of a build that ignored the edit. The
// control for that: giving this stub a body the compiler cannot drop moves the
// image by 274,110 bytes (268221876703dcf4...). Edits reach the image; this one
// costs nothing.
func notifyFrame(Platform, op.Op) {}
