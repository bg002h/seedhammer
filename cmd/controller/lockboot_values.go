//go:build !refugium

package main

import (
	"encoding/hex"

	"seedhammer.com/driver/otp"
)

const (
	// White label information.
	otpVolumeLabel  = "SHII"
	otpRedirectURL  = "https://seedhammer.com/doc/?d=SHII"
	otpRedirectName = "SeedHammer II Manual"
	otpModel        = "SeedHammer II"
	otpBoardID      = "SHII"
	otpVendor       = "SH"
)

// The OTP writes behind writeOTPValues, as variables so a host test can
// replace them. Func literals, not method values: gui's Refugium positive
// control needs a call expression naming each OTP writer in the default set.
var (
	otpWriteWhiteLabelAddr   = func(row uint16) error { return otp.WriteWhiteLabelAddr(row) }
	otpWriteWhiteLabelString = func(idx uint8, s string) error { return otp.WriteWhiteLabelString(idx, s) }
	otpAddBootKey            = func(hash []byte) (int, error) { return otp.AddBootKey(hash) }
)

// writeOTPValues write the white label information and our signing
// key to OTP memory.
func writeOTPValues(keyHash, version string) error {
	khash, err := hex.DecodeString(keyHash)
	if err != nil {
		panic(err)
	}
	if err := otpWriteWhiteLabelAddr(otp.FirstUserRow); err != nil {
		return err
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
		{otp.INDEX_SCSI_INQUIRY_VERSION_STRDEF, version},
	}
	for _, inf := range infos {
		if err := otpWriteWhiteLabelString(inf.Index, inf.Value); err != nil {
			return err
		}
	}
	_, err = otpAddBootKey(khash)
	return err
}
