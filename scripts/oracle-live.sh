#!/usr/bin/env bash
#
# The LIVE oracle checks — the ones that shell out to the pinned Rust primary.
#
#   ./scripts/oracle-live.sh            check
#   ./scripts/oracle-live.sh -update    re-mint the S2 md1 golden
#
# WHY THIS IS A COMMAND AND NOT PART OF `go test ./...`
#
# Operator directive, 2026-08-15: "Don't skip jobs unless I ask." These checks
# need md/mk/ms installed at ~/.cargo/bin at the exact commits and binary hashes
# oracle/pins.json records. That cannot be arranged on the machine whose verdict
# gates a merge — the pins bind binaries built on the maintainer's machine, so a
# CI `cargo install` yields a different binary, a different hash, and a hard
# resolution failure. A test that cannot run there has three honest shapes:
#
#   SKIP when absent        reports ok having checked nothing. This is the defect
#                           C-1..C-4 were filed about, measured on CI run
#                           31898063163: the whole class silent, workflow green.
#   FAIL when absent        turns the required check permanently red for a reason
#                           that is not a defect.
#   NOT EXIST unless asked  this script, via the `oraclelive` build tag.
#
# An environment-variable opt-out was written and then removed: it is a skip with
# extra steps. This repo already proved the mirror image fails — sysw shipped
# SYSW_REQUIRE_VECTORS=1 -> t.Fatalf, exactly the right mechanism, and nothing
# ever set it.
#
# WHAT THIS SCRIPT DOES *NOT* COVER — stated, because a gate that hides its own
# blind spot is worse than no gate.
#
# Nothing here is load-bearing for correctness. The byte-identity guarantee is
# carried by three things that run everywhere, with no toolchain and no skip:
#
#   cmd/gaterecord   refuses to mint a gate record whose census is not what it
#                    JUST derived live, and writes the derivation out beside it.
#                    So a committed expectation cannot exist except as the output
#                    of a live run. THIS is where live derivation is mandatory.
#   go test ./...    compares every record against that committed expectation,
#                    and every expectation's recorded oracle identity against
#                    oracle/pins.json.
#
# What this script adds is FRESHNESS and DRIFT, which are two questions:
#
#   FRESHNESS  do the INSTALLED binaries still reproduce the committed bytes,
#              and are they still the ones pins.json names?
#   DRIFT      has the PRIMARY moved under a pin that did not? Nothing else in
#              this repo can ask that — every other check compares a pin against
#              something derived from the same pin, so a pin can be perfectly
#              honest and years stale at once. TestPinsAreCurrentWithTheirPrimaries
#              reads the sibling checkouts and answers it (added 2026-08-15;
#              before that, this file and live_test.go both CLAIMED drift
#              detection and neither did any).
#
# Both are maintainer's questions and they are asked here, by hand, deliberately.
# This script also holds the only code path that can mint the S2 md1 golden
# (-update), which is what makes the golden trustworthy elsewhere.
#
# EVERY TAGGED TEST MUST BE NAMED IN THE -run FILTER BELOW. That filter is an
# ALLOWLIST: a test added behind the tag and not added there still compiles, and
# still passes `go vet -tags oraclelive` on CI, and never executes anywhere. A
# check that exists and never runs is the exact defect this deliverable was filed
# about.
#
# The tagged files are type-checked on every push by
# `go vet -tags oraclelive ./oracle/ ./gui/ ./sysw/` in .github/workflows/
# test.yml, so they cannot rot uncompiled behind their own tag.
#
# ABSENCE IS FATAL IN HERE. You ran this on purpose; a missing oracle or a
# missing primary checkout is a failure, not a reason to report success.

set -uo pipefail
cd "$(dirname "$0")/.."

# -update is dispatched to ./gui/ ALONE, and deliberately not forwarded to the
# whole set: only the gui test declares an `update` flag, so `go test -update`
# across ./oracle/ and ./sysw/ dies with "flag provided but not defined" — a
# failure that says nothing about any oracle. Measured, not guessed: that is
# exactly what the first version of this script did.
if [ "${1:-}" = "-update" ]; then
  echo "== re-minting the S2 md1 golden from the pinned primary =="
  echo "   this is the ONLY code path that writes gui/testdata/s2_md1_golden.expect.json,"
  echo "   which is what makes that file trustworthy where no toolchain exists"
  echo
  CGO_ENABLED=0 go test -tags oraclelive -count=1 -v \
    -run TestAssembledMd1MatchesThePrimaryByteForByte ./gui/ -update
  rc=$?
  echo
  [ $rc -eq 0 ] && echo "mint: OK (exit 0)" || echo "mint: FAILED (exit $rc) — nothing was written"
  exit $rc
fi

echo "== live oracle checks (-tags oraclelive) =="
echo "   pinned oracles expected at ~/.cargo/bin; see oracle/pins.json"
echo "   primary checkouts expected BESIDE this repo: descriptor-mnemonic,"
echo "   mnemonic-key, mnemonic-secret (drift) and mnemonic-engrave (sysw vectors)"
echo

CGO_ENABLED=0 go test -tags oraclelive -count=1 -v \
  -run 'TestLiveDerivationReproducesEveryCommittedExpectation|TestRealPinsResolveTheInstalledOracles|TestPinsAreCurrentWithTheirPrimaries|TestBuiltPolicyDerivationMatchesTheS2Golden|TestAssembledMd1MatchesThePrimaryByteForByte|TestVendoredVectorsAreInSyncWithThePrimary' \
  ./oracle/ ./gui/ ./sysw/ "$@"
rc=$?

echo
if [ $rc -eq 0 ]; then
  echo "live checks: PASS (exit 0)"
else
  echo "live checks: FAIL (exit $rc)"
fi
exit $rc
