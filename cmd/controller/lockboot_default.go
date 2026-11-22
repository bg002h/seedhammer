//go:build tinygo && rp && !refugium

package main

import (
	"machine"

	"seedhammer.com/driver/otp"
)

// The OTP writer behind the `command: lock-boot` debug command
// (gui/debugcmd_default.go), moved out of platform_sh2.go unchanged so the
// Refugium build links none of it (Refugium plan F7 §4.1). Its twin,
// lockboot_refugium.go, refuses. signKeyHash stays in platform_sh2.go because
// isSecureBootEnabled still reads it.

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
