//go:build tinygo && rp && !refugium

package main

import (
	"encoding/hex"
	"fmt"
	"machine"

	"seedhammer.com/driver/otp"
)

// The OTP writer behind the `command: lock-boot` debug command
// (gui/debugcmd_default.go), moved out of platform_sh2.go unchanged so the
// Refugium build links none of it (Refugium plan F7 §4.1). Its twin,
// lockboot_refugium.go, refuses. signKeyHash stays in platform_sh2.go because
// isSecureBootEnabled still reads it.

const (
	// White label information.
	otpVolumeLabel  = "SHII"
	otpRedirectURL  = "https://seedhammer.com/doc/?d=SHII"
	otpRedirectName = "SeedHammer II Manual"
	otpModel        = "SeedHammer II"
	otpBoardID      = "SHII"
	otpVendor       = "SH"
)

func (p *Platform) LockBoot() error {
	if err := writeOTPValues(); err != nil {
		return err
	}
	if err := otp.EnableSecureBoot(); err != nil {
		return err
	}
	machine.CPUReset()
	panic("reboot failed")
}

// writeOTPValues write the white label information and our signing
// key to OTP memory.
func writeOTPValues() error {
	khash, err := hex.DecodeString(signKeyHash)
	if err != nil {
		panic(err)
	}
	if err := otp.WriteWhiteLabelAddr(otp.FirstUserRow); err != nil {
		fmt.Printf("label addr err: %v", err)
	}
	infos := []struct {
		Index uint8
		Value string
	}{
		{otp.INDEX_VOLUME_LABEL_STRDEF, otpVolumeLabel},
		{otp.INDEX_INDEX_HTM_REDIRECT_URL_STRDEF, otpRedirectURL},
		{otp.INDEX_INDEX_HTM_REDIRECT_NAME_STRDEF, otpRedirectName},
		{otp.INDEX_INFO_UF2_TXT_MODEL_STRDEF, otpModel},
		{otp.INDEX_INFO_UF2_TXT_BOARD_ID_STRDEF, otpBoardID},
		{otp.INDEX_SCSI_INQUIRY_PRODUCT_STRDEF, otpBoardID},
		{otp.INDEX_SCSI_INQUIRY_VENDOR_STRDEF, otpVendor},
		{otp.INDEX_SCSI_INQUIRY_VERSION_STRDEF, boardVersion()},
	}
	for _, inf := range infos {
		if err := otp.WriteWhiteLabelString(inf.Index, inf.Value); err != nil {
			return err
		}
	}
	_, err = otp.AddBootKey(khash)
	return err
}
