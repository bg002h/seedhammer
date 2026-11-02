//go:build !refugium

package main

import (
	"errors"
	"testing"
)

// TestBughuntSh239LockBootStopsOnLabelAddrError: a failed white-label address
// write stops writeOTPValues before any string write or boot key write.
func TestBughuntSh239LockBootStopsOnLabelAddrError(t *testing.T) {
	addrErr := errors.New("otp: unsupported modification")
	strings, keys := 0, 0
	oldA, oldS, oldK := otpWriteWhiteLabelAddr, otpWriteWhiteLabelString, otpAddBootKey
	t.Cleanup(func() { otpWriteWhiteLabelAddr, otpWriteWhiteLabelString, otpAddBootKey = oldA, oldS, oldK })
	otpWriteWhiteLabelAddr = func(uint16) error { return addrErr }
	otpWriteWhiteLabelString = func(uint8, string) error { strings++; return nil }
	otpAddBootKey = func([]byte) (int, error) { keys++; return 0, nil }
	err := writeOTPValues("00", "v")
	if !errors.Is(err, addrErr) {
		t.Errorf("writeOTPValues = %v, want %v", err, addrErr)
	}
	if strings != 0 || keys != 0 {
		t.Errorf("after a failed label address write: %d string writes, %d boot key writes; want 0, 0", strings, keys)
	}
}
