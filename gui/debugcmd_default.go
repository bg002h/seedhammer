//go:build !refugium

package gui

import "log"

// debugResult is what StartScreen.Flow does after a `command: ` tag.
type debugResult int

const (
	// debugPassThrough hands the command on as the scanned object, which
	// engraveObjectFlow reports as an unknown format.
	debugPassThrough debugResult = iota
	// debugStay keeps the start screen up.
	debugStay
	// debugReturn returns the accompanying action from StartScreen.Flow.
	debugReturn
)

// handleDebugCommand is the start screen's debug-command arm, moved out of
// StartScreen.Flow unchanged (Refugium plan F7 §4.1) so the Refugium build can
// replace it with debugcmd_refugium.go, which carries no command. The
// behaviour is pinned by gui/debugcmd_characterization_test.go:
//
//   - FOREVERLAURA! returns the QA program;
//   - anything else is logged and passed through, lock-boot included: this
//     build carries no lock-boot command and nothing here calls
//     Platform.LockBoot (Brian 2026-11-23, P19; design/FIX_lockboot_default_remove.md).
func handleDebugCommand(ctx *Context, m *StartScreen, cmd debugCommand) (debugResult, startScreenAction) {
	switch cmd.Command {
	case "FOREVERLAURA!":
		return debugReturn, startScreenAction{prog: qaProgram}
	default:
		log.Printf("unknown debug command: %q", cmd.Command)
		return debugPassThrough, startScreenAction{}
	}
}
