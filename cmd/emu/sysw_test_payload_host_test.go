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
	"path/filepath"
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

// TestSyswTestPayloadIsConfinedToJSOnlyFiles is the sealed blob's confinement
// argument applied to this one. The reasoning is identical and lives in
// confinement_test.go; only the identifiers differ.
//
// It is a SEPARATE test rather than four more entries in `guarded` because the
// two blobs fail differently: the sealed one leaks a pre-known PASSPHRASE, and
// this one leaks a pre-known PAYLOAD -- a device booting with somebody else's
// records already loaded, which is the thing the boot offer exists to let an
// operator refuse.
func TestSyswTestPayloadIsConfinedToJSOnlyFiles(t *testing.T) {
	root := repoRoot(t)
	names := []string{"syswTestPayload", "syswTestDigest", "sysw_test_payload.bin"}

	// The js-only files that are ALLOWED to name these, relative to the module
	// root. Anything else naming them is the failure.
	allowed := map[string]bool{
		filepath.Join("cmd", "emu", "sysw_test_payload.go"):           true,
		filepath.Join("cmd", "emu", "sysw_test_payload_host_test.go"): true,
		filepath.Join("cmd", "emu", "platform.go"):                    true,
	}

	checked := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if allowed[rel] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		for _, n := range names {
			if strings.Contains(string(b), n) {
				t.Errorf("%s names %q, which must not escape cmd/emu's browser build: "+
					"a shipped SeedHammer II must never boot with a payload already "+
					"in its region", rel, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	// Without this the test passes vacuously if the walk is ever misrooted.
	if checked < 50 {
		t.Fatalf("INCONCLUSIVE: only %d .go files scanned, so this test is not "+
			"looking at the module it thinks it is", checked)
	}
}
