package sysw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"
)

// THE HOST/DEVICE codex32 SEAM, as a gate rather than a comment.
//
// host-admits => device-admits. The host (`me`) packs only the constellation
// ms1 PROFILE; this device displays any BCH-valid codex32 it can engrave. The
// host being the NARROWER side is safe. The other direction is not: a host that
// admitted what this classifier refuses would pack a record into a payload the
// device cannot read -- an engraved backup that will not load.
//
// This half asserts the DEVICE column of testdata/codex32_seam_vectors.json
// against Classify. mnemonic-engrave's crates/me-cli/tests/codex32_seam.rs
// reads a BYTE-IDENTICAL copy of the same file and asserts the HOST column.
// Neither implementation is ever compared to the other -- both are compared to
// the file, which is why it has to be the same file. seamVectorsSHA256 below is
// pinned identically in the Rust test, so the two copies cannot drift without
// one of the two suites going red.
//
// The file is authored in the Rust primary (Rust-primary rule) and vendored
// here; its own header carries the per-row provenance and the re-pin recipe.
const seamVectorsSHA256 = "2c2fbb3fa4d38c8858b9de4769d876d275478956c76ca491005c70d9f6bd541b"

func TestCodex32SeamDeviceAdmitsEverythingTheHostDoes(t *testing.T) {
	const path = "testdata/codex32_seam_vectors.json"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != seamVectorsSHA256 {
		t.Fatalf("%s hashes to %s, not the pinned %s -- the vendored copy and the primary "+
			"have drifted, or a row changed without re-pinning BOTH literals",
			path, hex.EncodeToString(sum[:]), seamVectorsSHA256)
	}
	var doc struct {
		Vectors []struct {
			Name         string `json:"name"`
			String       string `json:"string"`
			Chars        int    `json:"chars"`
			HostAdmits   bool   `json:"host_admits"`
			DeviceAdmits bool   `json:"device_admits"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var both, deviceOnly, neither int
	for _, v := range doc.Vectors {
		// A mistyped vector must fail loudly, not quietly stop testing.
		if n := utf8.RuneCountInString(v.String); n != v.Chars {
			t.Errorf("%s: declared %d characters, is %d", v.Name, v.Chars, n)
		}
		// THE SAFE DIRECTION. This is the assertion the file exists for.
		if v.HostAdmits && !v.DeviceAdmits {
			t.Errorf("%s: the HOST admits what the DEVICE refuses", v.Name)
		}
		if got := Classify(v.String) == ClassCodex32Secret; got != v.DeviceAdmits {
			t.Errorf("%s: device admits = %v, want %v (Classify = %v)",
				v.Name, got, v.DeviceAdmits, Classify(v.String))
		}
		switch {
		case v.HostAdmits && v.DeviceAdmits:
			both++
		case v.DeviceAdmits:
			deviceOnly++
		default:
			neither++
		}
	}
	// All three shapes, or the set goes vacuous: with no yes/yes row a mutant
	// that refuses everything passes, with no no/no row one that admits
	// everything passes, and with no device-only row the seam is untested.
	if both == 0 || deviceOnly == 0 || neither == 0 {
		t.Errorf("%d rows: %d both / %d device-only / %d neither -- the set must keep all three",
			len(doc.Vectors), both, deviceOnly, neither)
	}
}
