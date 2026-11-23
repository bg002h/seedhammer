//go:build tinygo && rp && !refugium

package main

import (
	"machine"

	"seedhammer.com/driver/otp"
)

// The OTP writer that the `command: lock-boot` debug command used to call,
// moved out of platform_sh2.go unchanged so the Refugium build links none of it
// (Refugium plan F7 §4.1). Nothing calls it: the default build dropped that
// command too (Brian 2026-11-23, P19; design/FIX_lockboot_default_remove.md),
// and gui/refugium_build_test.go's TestDefaultBuildCarriesNoLockBootCommand
// keeps it that way: the redirect URL literal and the OTP write primitives it
// reaches are asserted absent from both ELFs (scripts/refugium-elf-check.sh),
// which cannot see a write whose primitives TinyGo inlined. It is kept, not deleted, to keep the
// upstream diff small. Its twin, lockboot_refugium.go, refuses. signKeyHash
// stays in platform_sh2.go because isSecureBootEnabled still reads it.

func (p *Platform) LockBoot() error {
	if err := writeOTPValues(signKeyHash, boardVersion()); err != nil {
		return err
	}
	if err := otp.EnableSecureBoot(); err != nil {
		return err
	}
	machine.CPUReset()
	panic("reboot failed")
}
