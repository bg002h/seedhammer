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
# THE PACKAGES ARE DERIVED FROM THE SOURCE TOO. The search used to be limited
# to a hand list (./oracle/ ./gui/ ./sysw/), and the same hand list was given
# to `go test`, so cmd/emu's tagged test was neither discovered nor run while
# this script printed PASS (bughunt-1119 bh-sh-I11). Now every directory that
# `./...` would visit is searched, each package that holds a tagged file is run
# with its own filter, and the vacuity check compares PACKAGE AND NAME.
# oracle/live_runner_test.go (untagged, so in every `go test ./...`) checks
# all of this against the tree with a stub `go`.
#
# The tagged files are compiled on every push by `go test -tags oraclelive
# -run '^$'` over a package list in .github/workflows/test.yml, so they cannot
# rot uncompiled behind their own tag. That list is kept by hand.
#
# ABSENCE IS FATAL IN HERE. You ran this on purpose; a missing oracle or a
# missing primary checkout is a failure, not a reason to report success.

set -uo pipefail
set -f  # no pathname expansion: the path lists below are split unquoted, and a [ in a path is a glob
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
  # Here-string, not `printf | grep -q` (F-695): the RUN line is first, so on
  # long output grep exits early and a SIGPIPE'd printf fails the pipeline.
  if ! grep -q '^=== RUN   TestAssembledMd1MatchesThePrimaryByteForByte$' <<<"$mint_out"; then
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
#
# The search covers the directories `./...` visits, by Go's own rules: not
# testdata, vendor, or a name starting with "." or "_" (that also keeps out a
# worktree or cache nested in a hidden directory). find, not `grep -r
# --exclude-dir='.*'`: GNU grep applies that pattern to the "." it starts from
# and searches nothing.
# -Z and tr: a newline inside a path becomes a space, so the whitespace refusal below sees it rather than the loops splitting it.
tagged_files=$(find . -type d \( -name testdata -o -name vendor -o -name '.?*' -o -name '_*' \) -prune \
  -o -type f -name '*.go' -print0 | sort -z | xargs -0 -r grep -lZE '^//go:build\b.*\boraclelive\b' | tr '\n\0' ' \n')
if [ -z "$tagged_files" ]; then
  echo "::error::no //go:build oraclelive files found; this script would run nothing"
  exit 1
fi
# Go cannot load a package whose path holds whitespace, and the loops below split
# on it: refuse rather than drop the file and report PASS.
if grep -q '[[:space:]]' <<<"$tagged_files"; then
  echo "::error::a tagged file's path holds whitespace; go cannot load it and this script cannot run it:"
  grep '[[:space:]]' <<<"$tagged_files" | sed 's/^/         /'
  exit 1
fi
# One line per (package, test): "./dir/ TestName".
tagged_tests=$(for f in $tagged_files; do
  pkg="${f%/*}/"
  grep -h '^func Test' "$f" | sed 's/^func \(Test[A-Za-z0-9_]*\).*/\1/' | grep -v '^TestMain$' | sed "s|^|$pkg |"
done | sort -u)
want=$(printf '%s\n' "$tagged_tests" | grep -c .)
if [ "$want" -eq 0 ]; then
  echo "::error::tagged files exist but declare no tests; this script would run nothing"
  exit 1
fi
packages=$(printf '%s\n' "$tagged_tests" | cut -d' ' -f1 | sort -u)
npkg=$(printf '%s\n' "$packages" | grep -c .)
echo "   discovered $want tagged test(s) in $npkg package(s) from source"
echo

# One `go test` per package, each with its own names, so a name is only ever
# credited to the package that declares it.
rc=0
ran=
for pkg in $packages; do
  filter=$(printf '%s\n' "$tagged_tests" | sed -n "s|^$pkg ||p" | paste -sd'|' -)
  out=$(CGO_ENABLED=0 go test -tags oraclelive -count=1 -v -run "^($filter)\$" "$pkg" "$@" 2>&1)
  prc=$?
  printf '%s\n' "$out"
  [ $rc -eq 0 ] && rc=$prc
  ran+=$(printf '%s\n' "$out" | sed -n "s|^=== RUN   \(Test[A-Za-z0-9_]*\)\$|$pkg \1|p")$'\n'
done

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
# So compare PACKAGE AND NAME. The $-anchored sed drops `TestFoo/sub` because it
# contains a '/', and naming the absentees beats a count at no cost.
ran_names=$(printf '%s' "$ran" | grep . | sort -u)
missing=$(comm -23 <(printf '%s\n' "$tagged_tests") <(printf '%s\n' "$ran_names"))
if [ -n "$missing" ]; then
  echo
  echo "::error::discovered $want tagged test(s); these never executed:"
  printf '%s\n' "$missing" | sed 's/^/         /'
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
