package main

import "testing"

func TestMapCharSpaceMark(t *testing.T) {
	got, ok := mapChar("space_mark")
	if !ok {
		t.Fatal("space_mark not recognised")
	}
	if got != 0x1F {
		t.Errorf("space_mark = %#x, want 0x1F", got)
	}
	// 0x1F must be below 0x20 so a validated ASCII passphrase can never
	// contain it, and so it sorts FIRST in an ascending alphabet.
	if got >= 0x20 {
		t.Errorf("space_mark %#x must be a control codepoint", got)
	}
}
