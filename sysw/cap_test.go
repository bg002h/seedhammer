package sysw

import (
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

// rustPrimaryWire returns the source of the Rust primary's sysw/wire.rs, and
// whether it was found.
//
// TWO LAYOUTS ARE TRIED, because this repo lives in both: the constellation's
// side-by-side checkout (../../mnemonic-engrave) and the submodule layout the
// `me` repo uses (third_party/seedhammer, so ../../../ is the `me` root).
// Nothing else is guessed.
func rustPrimaryWire() (src string, path string, tried []string, ok bool) {
	candidates := []string{
		"../../mnemonic-engrave/crates/me-cli/src/sysw/wire.rs", // side-by-side
		"../../../crates/me-cli/src/sysw/wire.rs",               // third_party/seedhammer
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			return string(b), p, candidates, true
		}
	}
	return "", "", candidates, false
}

// **THE PORT'S CAP MUST EQUAL THE PRIMARY'S**, or the device refuses containers
// the host emits -- and it refuses them saying "malformed container", which is
// the wrong sentence for a payload that is exactly right.
//
// The constant and the geometry are asserted UNCONDITIONALLY. Only the
// cross-repo half depends on the primary being on disk, so a checkout without
// the sibling still runs a gate rather than skipping one: a gate that can
// silently not run is the default failure mode in this tree.
func TestTheSectionCapMatchesTheRustPrimary(t *testing.T) {
	if MaxSectionLen != 32734 {
		t.Errorf("this port says %d; the primary's formula gives 32734", MaxSectionLen)
	}
	// The geometry the no-wrap argument in boundBlob rests on, at RUNTIME as
	// well as at compile time, so the reason survives a refactor that deletes
	// the array-length assertion in wire.go.
	if HeaderLen+2*MaxSectionLen+TagLen > RegionLen {
		t.Error("two maxed sections plus header plus tag no longer fit the region")
	}
	// ...and the ugly number is the largest one with that property: 32,768
	// overruns the region by 34 bytes, which is why the cap is not a round
	// power of two.
	if HeaderLen+2*32768+TagLen <= RegionLen {
		t.Error("32,768 now fits, so the formula is not what it claims to be")
	}

	src, found, tried, ok := rustPrimaryWire()
	if !ok {
		t.Logf("the Rust primary is not beside this checkout (tried %s); "+
			"the cross-repo half of this gate did not run", strings.Join(tried, ", "))
		return
	}
	// The value is read out of the Rust source as a FORMULA rather than
	// retyped, because a retyped constant is precisely how a port silently
	// forks: both sides would then be free to be wrong together.
	const want = "pub const MAX_SECTION_LEN: usize = (REGION_LEN - HEADER_LEN - TAG_LEN) / 2;"
	if !strings.Contains(src, want) {
		t.Errorf("the Rust primary no longer defines MAX_SECTION_LEN by that formula; "+
			"this port's %d may have drifted", MaxSectionLen)
	}
	// And the SIBLING container's cap is FROZEN at 8191, on both sides. This
	// repo's own seal package is checked by seal/wire_test.go; what is checked
	// here is the PRIMARY's, because a raise applied to the wrong constant over
	// there would pass every assertion above.
	sealSrc, err := os.ReadFile(strings.Replace(found, "/sysw/wire.rs", "/seal/wire.rs", 1))
	if err != nil {
		t.Fatalf("the primary's seal/wire.rs sits beside sysw/wire.rs and did not read: %v", err)
	}
	if !strings.Contains(string(sealSrc), "pub const MAX_SECTION_LEN: u32 = 8191;") {
		t.Error("the primary's seal cap moved; it is FROZEN and must not follow sysw")
	}
	if MaxSectionLen == 8191 {
		t.Error("this port's sysw cap is seal's -- the raise landed on the wrong constant")
	}
}

// A container carrying a section LARGER than the old 8191 cap must LOAD. This
// is the case the raise exists for, and it is the case that silently failed
// before: the host would emit it and the device would call it malformed.
func TestASectionPastTheOldCapIsAccepted(t *testing.T) {
	buf := make([]byte, HeaderLen)
	copy(buf, MAGIC[:])
	buf[8] = Version
	binary.BigEndian.PutUint32(buf[44:48], 20000)
	if _, err := ParseHeader(buf); err != nil {
		t.Fatalf("a 20,000-byte section was refused: %v", err)
	}

	// THE NEAREST HOSTILE INPUT: one byte past the NEW cap is still refused,
	// and exactly at the cap is still accepted.
	binary.BigEndian.PutUint32(buf[44:48], MaxSectionLen)
	if _, err := ParseHeader(buf); err != nil {
		t.Errorf("a section exactly at the cap must be legal: %v", err)
	}
	binary.BigEndian.PutUint32(buf[44:48], MaxSectionLen+1)
	if _, err := ParseHeader(buf); !errors.Is(err, ErrSectionTooLong) {
		t.Errorf("a section one byte past the cap: want ErrSectionTooLong, got %v", err)
	}
}
