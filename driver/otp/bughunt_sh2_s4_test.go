package otp

import (
	"bytes"
	"testing"
)

// TestBughuntSh213WhiteLabelSpaceUTF16Neighbour is bug #13: when
// WriteWhiteLabelString scans the existing table for free OTP rows it doubles
// the character count of a UTF-16 entry by switching on the NEW string's index
// (idx) instead of the scanned entry's index (i). A new string written at index
// 8 therefore never accounts for an existing UTF-16 product string at index 5
// and is placed on top of it. OTP is write-once.
//
// RP2350 datasheet, USB_WHITE_LABEL_ADDR register: "the top bit of the first
// (LSB) byte if set to indicate that each character in the string is two
// bytes (Unicode) versus one byte if unset" and "Unicode _STRDEFs are only
// supported for USB_DEVICE_PRODUCT_STRDEF, USB_DEVICE_SERIAL_NUMBER_STRDEF and
// USB_DEVICE_MANUFACTURER_STRDEF". Entry 5 is INDEX_USB_DEVICE_PRODUCT_STRDEF.
func TestBughuntSh213WhiteLabelSpaceUTF16Neighbour(t *testing.T) {
	resetOTP()
	const tbl = FirstUserRow
	if err := WriteWhiteLabelAddr(tbl); err != nil {
		t.Fatal(err)
	}
	// An existing UTF-16 product string (as another tool could have written
	// it): 10 characters = 20 bytes = 10 rows, stored at table offset 16.
	const (
		chars  = 10
		offset = 16
	)
	product := bytes.Repeat([]byte{0x41, 0x00}, chars)
	if err := writeECCRow(tbl+INDEX_USB_DEVICE_PRODUCT_STRDEF, offset<<8|0x80|chars); err != nil {
		t.Fatal(err)
	}
	if err := writeECC(product, tbl+offset); err != nil {
		t.Fatal(err)
	}
	if err := writeOrRow(USB_BOOT_FLAGS, 3, 0b1<<INDEX_USB_DEVICE_PRODUCT_STRDEF|WHITE_LABEL_ADDR_VALID); err != nil {
		t.Fatal(err)
	}

	// Now write a different, ASCII string at index 8.
	if err := WriteWhiteLabelString(INDEX_VOLUME_LABEL_STRDEF, "SHII"); err != nil {
		t.Fatalf("WriteWhiteLabelString: %v", err)
	}
	entry, err := readECCRow(tbl + INDEX_VOLUME_LABEL_STRDEF)
	if err != nil {
		t.Fatal(err)
	}
	// The product string occupies table rows offset..offset+10-1, so the new
	// string must start at offset+10 or later.
	if got, want := entry>>8, uint16(offset+chars); got < want {
		t.Errorf("new string placed at table row %d, want >= %d (overlaps the UTF-16 product string at rows %d..%d)",
			got, want, offset, offset+chars-1)
	}
	// The existing product string must be untouched.
	got := make([]byte, len(product))
	if err := readECC(got, tbl+offset); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, product) {
		t.Errorf("UTF-16 product string overwritten: % x, want % x", got, product)
	}
}

// TestBughuntSh214WhiteLabelValidBit is bug #14: ReadWhiteLabelString tests
// `flags|(1<<idx) == 0`, which is never true, so the per-entry valid bit of
// USB_BOOT_FLAGS is never consulted and an entry that is not marked valid is
// read as if it were.
//
// RP2350 datasheet, USB_WHITE_LABEL_ADDR register: the table's rows "are also
// ECC and marked valid by the corresponding valid bit in USB_BOOT_FLAGS (ECC)"
// and "the corresponding valid bit enables replacing the default value".
// The function's own `return "", nil` arm is its "not set" answer.
func TestBughuntSh214WhiteLabelValidBit(t *testing.T) {
	resetOTP()
	const tbl = FirstUserRow
	if err := WriteWhiteLabelAddr(tbl); err != nil {
		t.Fatal(err)
	}
	// An entry and its string are present, but only the table-level
	// WHITE_LABEL_ADDR_VALID bit is set: bit INDEX_VOLUME_LABEL_STRDEF is not.
	const s = "SHII"
	if err := writeECCRow(tbl+INDEX_VOLUME_LABEL_STRDEF, 16<<8|uint16(len(s))); err != nil {
		t.Fatal(err)
	}
	if err := writeECC([]byte(s), tbl+16); err != nil {
		t.Fatal(err)
	}
	if err := writeOrRow(USB_BOOT_FLAGS, 3, WHITE_LABEL_ADDR_VALID); err != nil {
		t.Fatal(err)
	}
	got, err := ReadWhiteLabelString(INDEX_VOLUME_LABEL_STRDEF)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("ReadWhiteLabelString = %q for an entry whose valid bit is clear, want \"\"", got)
	}
}
