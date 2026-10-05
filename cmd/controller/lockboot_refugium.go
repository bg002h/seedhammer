//go:build tinygo && rp && refugium

package main

import "errors"

// LockBoot is refused in the Refugium build, which links no OTP writer at all
// (Refugium plan F7 §4.1): not the white-label strings, not the boot key, not
// the secure-boot enable. The default build's writer is lockboot_default.go.
// Nothing in this build calls it -- gui's debug-command arm recognises no
// command here (gui/debugcmd_refugium.go) -- and it fails closed if anything
// ever does.
func (p *Platform) LockBoot() error {
	return errors.New("this build cannot lock boot")
}
