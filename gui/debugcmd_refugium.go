//go:build refugium

package gui

import "log"

// debugResult is what StartScreen.Flow does after a `command: ` tag. The
// values match debugcmd_default.go's so StartScreen.Flow is the same code in
// both builds.
type debugResult int

const (
	debugPassThrough debugResult = iota
	debugStay
	debugReturn
)

// handleDebugCommand is the Refugium build's debug-command arm: it recognises
// NO command. It logs that one arrived and keeps the start screen up, so a
// command tag neither engraves the QA plate nor reaches Platform.LockBoot
// (Refugium plan F7 §4.1). With NFC off (§4.2) no tag reaches the start screen
// in this build at all; the arm is replaced anyway, so the commands are absent
// from the binary rather than merely unreachable.
//
// The command text is not logged: this build has no reason to echo what a tag
// carried.
func handleDebugCommand(ctx *Context, m *StartScreen, cmd debugCommand) (debugResult, startScreenAction) {
	log.Printf("debug commands are not part of this build")
	return debugStay, startScreenAction{}
}
