package oracle

// The ONE opt-out for the whole class "a gate that checks this Go firmware
// against the pinned Rust primary".
//
// # Why there is exactly one, and why it only points outwards (C-1..C-4)
//
// Five sites in this tree used to SKIP when the pinned oracles — or the sibling
// repo's vectors — were absent: oracle/expect_test.go, oracle/oracle_test.go,
// gui/multisig_build_oracle_test.go, sysw/conformance_test.go and
// gui/sysw_load_test.go. All five were therefore skipped on the machine whose
// verdict decides whether a merge lands, and the suite still printed `ok` and
// exit 0. Measured, not predicted: CI run 31898063163 on 4b8488e concluded
// SUCCESS with every one of them silent.
//
// The sysw pair shipped the RIGHT mechanism — SYSW_REQUIRE_VECTORS=1 turning
// the skip into a t.Fatalf — and it failed anyway, because nothing ever set it.
// An escalation nobody sets is a skip with paperwork. So the rule this file
// encodes is:
//
//	ENFORCEMENT IS NEVER SPELLED BY AN ENVIRONMENT VARIABLE.
//
// An environment variable may only ever RELAX a gate, never arm one, and the
// layer that actually carries the guarantee — the committed expectations in
// oracle/gaterecords/*.expect.json, gui/testdata/*.expect.json and
// sysw/testdata/sysw_vectors.json — has NO skip path at all, needs no toolchain,
// and is what CI executes on every push. Nothing that can be opted out of is
// load-bearing.
//
// What the opt-out buys is the contributor case that was real all along: a
// developer without the Rust toolchain gets the full byte-identity comparison
// from the vendored layer, and states — by typing this variable, or by a
// reviewed line in a workflow file that says what still enforces the property
// there — that the LIVE re-derivation cannot run on their machine.

import (
	"fmt"
	"os"
)

// OraclesOptionalEnv names the opt-out. Set it to "1" to turn the live-oracle
// arms of the gates from a failure into a skip.
const OraclesOptionalEnv = "SH_ORACLES_OPTIONAL"

// OraclesOptional reports whether the operator has declared that this machine
// has no pinned oracles.
func OraclesOptional() bool { return os.Getenv(OraclesOptionalEnv) == "1" }

// MissingOracleMessage is the text every site in the class prints when a pinned
// oracle is absent. It names BOTH remedies and what is still enforcing the
// property regardless, because a refusal that does not say how to proceed is
// how a fail-closed default gets reverted to a skip by the next person.
func MissingOracleMessage(name, dir string) string {
	return fmt.Sprintf("the pinned oracle %q is not installed at %s, so the LIVE "+
		"re-derivation cannot run.\n"+
		"  Remedy 1: install the pinned oracles (see oracle/pins.json for the "+
		"repo and commit of each) — this is what a maintainer minting a record does.\n"+
		"  Remedy 2: export %s=1 to declare that this machine has no Rust "+
		"toolchain.\n"+
		"Either way the committed expectations still compare byte for byte on this "+
		"machine, with no toolchain and no skip path — they are the gate; this is "+
		"the freshness check on top of it.",
		name, dir, OraclesOptionalEnv)
}
