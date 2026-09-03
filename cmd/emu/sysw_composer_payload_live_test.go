//go:build oraclelive

package main

// THE COMPOSER PAYLOAD'S CROSS-IMPLEMENTATION DIGEST CHECK: does `me sysw
// show` print the digest this blob's pin claims?
//
// Behind a build tag rather than skipping, exactly as
// gui/chain_fixture_live_test.go and sysw/vendored_vectors_live_test.go are and
// for the same reason: it needs the `me` CLI, which no CI runner has, and a
// test that answers "I could not tell" by reporting success is the default
// failure mode in this tree. Operator directive, 2026-08-15: "Don't skip jobs
// unless I ask."
//
// What runs everywhere with no skip path and no toolchain: the digest
// RECOMPUTATION by the firmware's own sysw.Open + sysw.PublicDataHash, and the
// record inventory, both in sysw_composer_payload_host_test.go. Those enforce
// AGREEMENT between the blob and the pin. This one asks the different question
// -- do the two IMPLEMENTATIONS agree, so that the sixteen hex digits an
// operator compares across the air gap are the same sixteen on both sides:
//
//	ME=/path/to/me go test -tags oraclelive ./cmd/emu/ -run TestSyswComposerPayloadDigestAgreesWithMe
//
// ABSENCE IS FATAL HERE. You asked for the audit; not having `me` means the
// audit cannot be performed, which is a failure and not a pass.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSyswComposerPayloadDigestAgreesWithMe(t *testing.T) {
	me := os.Getenv("ME")
	if me == "" {
		me = "me"
	}
	// RESOLVED AND REPORTED. A bare name can be a shell alias or a stale
	// install, and this test's whole claim is about which binary printed the
	// digest -- `me` 0.7.0 does not even know the composer record classes.
	path, err := exec.LookPath(me)
	if err != nil {
		t.Fatalf("the `me` CLI is not on PATH as %q (%v).\n"+
			"This audit was requested explicitly (-tags oraclelive) and cannot be "+
			"performed without the other implementation to compare against, so its "+
			"absence is a failure. Set ME=/path/to/me, or run the suite without the "+
			"tag -- the pin is still enforced there by "+
			"TestSyswComposerPayloadMatchesItsDigest.", me, err)
	}
	ver, err := exec.Command(path, "--version").Output()
	if err != nil {
		t.Fatalf("%s --version: %v", path, err)
	}
	t.Logf("auditing against %s (%s)", path, strings.TrimSpace(string(ver)))

	// COMBINED, and that is a measurement rather than caution: `me` 0.8.0
	// prints `sealed:`, `pub_len:`, `identity:` and the record inventory on
	// STDOUT and the `digest:` line on STDERR. Reading .Output() alone found no
	// digest and this audit failed reporting "comparing nothing" -- which is
	// the honest failure, but the stream split is the fact, so both are read.
	out, err := exec.Command(path, "sysw", "show", "sysw_composer_payload.bin").CombinedOutput()
	if err != nil {
		t.Fatalf("%s sysw show sysw_composer_payload.bin: %v\n%s", path, err, out)
	}
	var got string
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "digest:"); ok {
			got = strings.TrimSpace(rest)
		}
	}
	if got == "" {
		t.Fatalf("no `digest:` line in `%s sysw show`'s output, so this audit is "+
			"comparing nothing. It printed:\n%s", path, out)
	}

	if want := composerDigestPin(t); got != want {
		t.Errorf("the two implementations disagree: `me sysw show` prints %q, "+
			"syswComposerDigest pins %q.\nThe operator compares this value across the "+
			"air gap, so a disagreement means one of the two screens is lying about "+
			"which payload is in flash.", got, want)
	}
}
