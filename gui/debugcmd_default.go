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
// replace it with debugcmd_refugium.go, which carries neither command. The
// behaviour is pinned by gui/debugcmd_characterization_test.go:
//
//   - FOREVERLAURA! returns the QA program;
//   - lock-boot calls Platform.LockBoot once and stays on the start screen,
//     reporting a failure in the status line;
//   - anything else is logged and passed through.
func handleDebugCommand(ctx *Context, m *StartScreen, cmd debugCommand) (debugResult, startScreenAction) {
	switch cmd.Command {
	case "FOREVERLAURA!":
		return debugReturn, startScreenAction{prog: qaProgram}
	case "lock-boot":
		m.Status = scanIdle
		if err := ctx.Platform.LockBoot(); err != nil {
			log.Printf("lock-boot: %v", err)
			m.Status = scanFailed
		}
		return debugStay, startScreenAction{}
	default:
		log.Printf("unknown debug command: %q", cmd.Command)
		return debugPassThrough, startScreenAction{}
	}
}
