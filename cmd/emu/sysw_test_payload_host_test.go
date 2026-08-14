package main

// UNTAGGED on purpose. sysw_test_payload.go is //go:build js, so nothing here
// can reference syswTestPayload or syswTestDigest as SYMBOLS -- the host build
// does not compile them. It reads the .bin and the .go SOURCE off disk
// instead, which is the only way a `go test` on this machine can check either.
//
// That indirection is the point rather than a workaround: the blob and the
// digest constant beside it are two halves of one `me sysw pack` run, and a
// journey document quotes the digest on the host side of the air gap and
// photographs it on the device side. If they drift, the document is wrong in
// the specific way no reader can detect -- it still reads as consistent.

import (
	"os"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// TestSyswTestPayloadMatchesItsDigest recomputes the digest from the embedded
// bytes with the firmware's own code and requires the pinned constant to equal
// it.
//
// It also proves the blob is a container the FIRMWARE can actually open, not
// merely one `me` could write: Open and PublicDataHash here are the same calls
// gui makes at load. A blob that parsed on the host and not on the device
// would make Load Payload unwalkable in exactly the build added to walk it.
func TestSyswTestPayloadMatchesItsDigest(t *testing.T) {
	blob, err := os.ReadFile("sysw_test_payload.bin")
	if err != nil {
		t.Fatalf("reading the embedded blob: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("the firmware cannot open its own test payload: %v", err)
	}
	got := sysw.FormatHash(sysw.PublicDataHash(p.Public, false))

	src, err := os.ReadFile("sysw_test_payload.go")
	if err != nil {
		t.Fatalf("reading the source that pins the digest: %v", err)
	}
	const marker = `const syswTestDigest = "`
	i := strings.Index(string(src), marker)
	if i < 0 {
		t.Fatalf("syswTestDigest is gone from sysw_test_payload.go, so this test "+
			"is checking nothing; looked for %q", marker)
	}
	rest := string(src)[i+len(marker):]
	want := rest[:strings.Index(rest, `"`)]

	if got != want {
		t.Errorf("digest drift: the blob hashes to %q but syswTestDigest pins %q.\n"+
			"These are two halves of one `me sysw pack` run. Whichever was "+
			"regenerated, the other must be updated with it -- a journey document "+
			"quotes this value on the host side and photographs it on the device.",
			got, want)
	}
}

// TestSyswTestPayloadCarriesThreeClasses pins WHY this blob has three records.
//
// A single-record payload would load, show a digest, and demonstrate nothing:
// the property worth walking is that ONE payload feeds SEVERAL programs, each
// picker defaulting to FROM PAYLOAD for the class it wants. Shrinking the blob
// to one record is a silent, plausible-looking edit that would leave every
// other test green and quietly delete the reason the file exists.
func TestSyswTestPayloadCarriesThreeClasses(t *testing.T) {
	blob, err := os.ReadFile("sysw_test_payload.bin")
	if err != nil {
		t.Fatalf("reading the embedded blob: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if len(p.Secret) != 0 {
		t.Errorf("the demo payload is meant to be UNSEALED, but it carries %d "+
			"encrypted records", len(p.Secret))
	}
	seen := map[sysw.Class]bool{}
	for _, r := range p.Public {
		seen[sysw.Classify(r)] = true
	}
	for _, want := range []struct {
		c    sysw.Class
		name string
	}{
		{sysw.ClassMnemonic, "ClassMnemonic"},
		{sysw.ClassPassphrase, "ClassPassphrase"},
		{sysw.ClassFreeText, "ClassFreeText"},
	} {
		if !seen[want.c] {
			t.Errorf("the demo payload no longer carries a %s record, so it can no "+
				"longer show one payload feeding several programs", want.name)
		}
	}
}

// TestSyswTestPayloadIsConfinedToJSOnlyFiles was REMOVED 2026-08-14. It matched a hand-maintained names[] list
// and an allowed-files list; embed_confinement_test.go now derives both from
// the tree, so the next payload blob is protected without anyone remembering
// to add it. That test also flagged any file merely QUOTING the names in a
// comment, which is what a literal match cannot distinguish.
