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
# THE -run FILTER IS DERIVED FROM THE SOURCE, NOT MAINTAINED BY HAND.
#
# It used to be a hand-written allowlist, and the comment here warned that a
# tagged test missing from it "still compiles, still passes `go vet -tags
# oraclelive` on CI, and never executes anywhere". The warning was correct and
# nothing enforced it: review I-1 renamed the two S5.0 live tests out from under
# the filter, mutated the encoder so every @i took another cosigner's key, and
# this script still printed "live checks: PASS (exit 0)" -- because
# `go test -run <matches nothing>` exits 0.
#
# So the list is now built by grepping `func Test` out of every //go:build
# oraclelive file, and the run is checked for VACUITY afterwards: the number of
# tests that actually ran must equal the number discovered. A filter that
# silences a check, and a run that executes nothing, both fail loudly now.
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
  # ANCHORED, and checked for vacuity — this is I-1's exact mechanism on the mint
  # path (re-review Minor-1). Unanchored, a rename leaves `-run` matching nothing;
  # `go test` then exits 0 on "no tests to run" and this branch printed
  # "mint: OK (exit 0)" having written no golden at all. The golden is what makes
  # the byte-identity gate trustworthy everywhere else, so a silent no-op mint is
  # the worst outcome here.
  mint_out=$(CGO_ENABLED=0 go test -tags oraclelive -count=1 -v \
    -run '^TestAssembledMd1MatchesThePrimaryByteForByte$' ./gui/ -update 2>&1)
  rc=$?
  printf '%s\n' "$mint_out"
  if ! printf '%s\n' "$mint_out" | grep -q '^=== RUN   TestAssembledMd1MatchesThePrimaryByteForByte$'; then
    echo
    echo "::error::the mint test never executed, so no golden was written"
    rc=1
  fi
  echo
  [ $rc -eq 0 ] && echo "mint: OK (exit 0)" || echo "mint: FAILED (exit $rc) — nothing was written"
  exit $rc
fi

echo "== live oracle checks (-tags oraclelive) =="
echo "   pinned oracles expected at ~/.cargo/bin; see oracle/pins.json"
echo "   primary checkouts expected BESIDE this repo: descriptor-mnemonic,"
echo "   mnemonic-key, mnemonic-secret (drift) and mnemonic-engrave (sysw vectors)"
echo

# Discover every test behind the tag, from the tree rather than from memory.
# Match the CONSTRAINT, not a prefix: `//go:build linux && oraclelive` is a
# tagged file too, and a literal-substring grep cannot see it. Re-review I-2
# planted exactly that with an unconditional t.Fatal and this script printed
# PASS while it never ran. \boraclelive\b also rejects `oraclelivex`.
# (It would match a negated `//go:build !oraclelive`; none exists, and one
# would show up as a discovered-but-never-run name below rather than silently.)
tagged_files=$(grep -rlE '^//go:build\b.*\boraclelive\b' --include='*.go' ./oracle/ ./gui/ ./sysw/)
if [ -z "$tagged_files" ]; then
  echo "::error::no //go:build oraclelive files found; this script would run nothing"
  exit 1
fi
tagged_tests=$(grep -h '^func Test' $tagged_files | sed 's/^func \(Test[A-Za-z0-9_]*\).*/\1/' | grep -v '^TestMain$' | sort -u)
want=$(printf '%s\n' "$tagged_tests" | grep -c .)
if [ "$want" -eq 0 ]; then
  echo "::error::tagged files exist but declare no tests; this script would run nothing"
  exit 1
fi
filter=$(printf '%s\n' "$tagged_tests" | paste -sd'|' -)
echo "   discovered $want tagged test(s) from source"
echo

out=$(CGO_ENABLED=0 go test -tags oraclelive -count=1 -v -run "^($filter)\$" ./oracle/ ./gui/ ./sysw/ "$@" 2>&1)
rc=$?
printf '%s\n' "$out"

# VACUITY CHECK, by SET DIFFERENCE rather than by count.
#
# `go test -run <matches nothing>` exits 0, so a green exit code alone proves
# nothing about whether anything ran. Counting `=== RUN` LINES does not fix that
# either, and re-review I-1 proved it worse than useless: Go emits a RUN line per
# SUBTEST, so one subtest's line pays for a top-level test that never ran. It was
# driven to "PASS (exit 0)" with the md1-vs-primary byte-identity proof missing
# from the output entirely. Counting also went spuriously RED whenever any tagged
# test grew a subtest.
#
# So compare NAMES. The $-anchored sed drops `TestFoo/sub` because it contains a
# '/', and naming the absentees beats a count at no cost.
ran_names=$(printf '%s\n' "$out" | sed -n 's/^=== RUN   \(Test[A-Za-z0-9_]*\)$/\1/p' | sort -u)
missing=$(comm -23 <(printf '%s\n' "$tagged_tests") <(printf '%s\n' "$ran_names"))
if [ -n "$missing" ]; then
  echo
  echo "::error::discovered $want tagged test(s); these never executed:"
  printf '         %s\n' $missing
  echo "         a live check that does not run is the defect this script exists to prevent"
  rc=1
fi

echo
if [ $rc -eq 0 ]; then
  echo "live checks: PASS (exit 0)"
else
  echo "live checks: FAIL (exit $rc)"
fi
exit $rc
