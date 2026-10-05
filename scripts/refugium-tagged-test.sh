#!/usr/bin/env bash
# refugium-tagged-test.sh -- run ./gui's suite under -tags refugium, provably.
#
#   scripts/refugium-tagged-test.sh [SHARDS]      (default: nproc)
#
# Refugium plan F7 §4.5 (R0 M-3, M-4). Two ways a tagged run lies:
#
#   1. a -run pattern that matches nothing reports `ok` (the vacuous run);
#   2. a shard set built from a hand-written "everything else" regex silently
#      drops every test added after it.
#
# So the list is ENUMERATED from `go test -tags refugium -list`, every shard's
# -run is ANCHORED (^(A|B|...)$), the partition's union is ASSERTED equal to
# the list before anything runs, and afterwards the top-level `--- PASS` lines
# across all shards (-v), plus the skips that name the profile, must equal the
# number of tests listed. A FAIL, any other SKIP, or a missing verdict fails
# the script.
#
# What the tag EXCLUDES is not decided here. A test that cannot hold in the
# Refugium build either carries `//go:build !refugium` on its file (the proof
# trigger files; -list omits them) or calls skipUnderRefugium with one of six
# named reasons (gui/profile_skip_test.go), and is printed below by name. The
# Refugium-only tests are gui/refugium_profile_test.go.
#
# The tag goes in through GOFLAGS (the shard script in mnemonic-engrave does
# not forward -tags; R0 M-3), and is asserted to have taken: the run must
# contain TestRefugiumProfileIsOn, which exists only under the tag.
set -euo pipefail

SHARDS="${1:-$(nproc)}"
PKG=./gui/
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
cd "$(dirname "$0")/.."
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=refugium"

go test "$PKG" -list '.*' | grep -E '^(Test|Example|Fuzz)' | sort -u >"$OUT/all.txt"
TOTAL=$(wc -l <"$OUT/all.txt")
[ "$TOTAL" -gt 0 ] || { echo "no tests enumerated -- refusing to report success" >&2; exit 2; }
grep -qx TestRefugiumProfileIsOn "$OUT/all.txt" || {
	echo "TestRefugiumProfileIsOn is not in the list: the refugium tag did not take" >&2
	exit 2
}
echo "=== $TOTAL top-level tests under -tags refugium, $SHARDS shards ==="

python3 - "$OUT" "$SHARDS" <<'PY'
import os, sys
out, k = sys.argv[1], int(sys.argv[2])
names = [l.strip() for l in open(os.path.join(out, "all.txt")) if l.strip()]
buckets = [names[i::k] for i in range(k)]
if sorted(x for b in buckets for x in b) != sorted(names):
    sys.exit("PARTITION IS NOT EXHAUSTIVE")
for i, b in enumerate(buckets):
    with open(os.path.join(out, f"shard{i}.txt"), "w") as f:
        f.write("\n".join(b) + ("\n" if b else ""))
PY

pids=()
for i in $(seq 0 $((SHARDS - 1))); do
	[ -s "$OUT/shard$i.txt" ] || continue
	RE="^($(paste -sd'|' "$OUT/shard$i.txt"))\$"
	go test "$PKG" -v -run "$RE" -count=1 -timeout 20m >"$OUT/out$i.txt" 2>&1 &
	pids+=("$!:$i")
done
fail=0
for p in "${pids[@]}"; do
	if ! wait "${p%%:*}"; then
		echo "shard ${p##*:}: FAIL"
		grep -E '^(--- FAIL|panic:|FAIL)' "$OUT/out${p##*:}.txt" | head -20
		fail=1
	fi
done

cat "$OUT"/out*.txt >"$OUT/all-out.txt"
# Top-level verdicts only (subtests are indented). A SKIP is accepted only if
# its message names the profile (skipUnderRefugium, gui/profile_skip_test.go):
# those are the default-build tests that step aside on purpose, and they are
# listed by name below. Any other SKIP fails the run.
python3 - "$OUT/all-out.txt" "$TOTAL" "$fail" <<'PY'
import re, sys
lines = open(sys.argv[1]).read().split("\n")
total, shard_fail = int(sys.argv[2]), int(sys.argv[3])
# The three tests that skip in EVERY build unless the environment opts in
# (md on PATH; TX_JOURNEY_OUT; SH2_REALCLOCK). They skip in the default
# suite too, so they are not the profile's; they are named here so a fourth
# skip of any other kind still fails the run.
ENV_SKIPS = {
    "TestFableTwoKeylessPathsAgreeWithTheHostOracle",
    "TestCaptureTransactionJourney",
    "TestIdleTimerUnderSH2ShapedEventLoop",
}
passed, failed, profile_skips, env_skips, other_skips = [], [], [], [], []
start = {}  # top-level test -> index of its "=== RUN" line
for i, l in enumerate(lines):
    r = re.match(r"^=== RUN   (\S+)$", l)
    if r and "/" not in r.group(1):
        start[r.group(1)] = i
        continue
    m = re.match(r"^--- (PASS|FAIL|SKIP): (\S+)", l)
    if not m:
        continue
    verdict, name = m.groups()
    if verdict == "PASS":
        passed.append(name)
    elif verdict == "FAIL":
        failed.append(name)
    else:
        # -v prints t.Skip's message between the test's RUN line and its SKIP
        # line. Only the test's FIRST log line counts, so a skip reached after
        # the test has run (and logged) cannot borrow the profile's name.
        body = [x.strip() for x in lines[start.get(name, i) + 1:i] if x.startswith("    ")]
        ok = bool(body) and re.match(r"^\S+_test\.go:\d+: refugium profile: ", body[0])
        if ok:
            profile_skips.append(name)
        elif name in ENV_SKIPS:
            env_skips.append(name)
        else:
            other_skips.append(name)
print(f"=== top-level: {len(passed)} PASS, {len(profile_skips)} SKIP (refugium profile), "
      f"{len(env_skips)} SKIP (environment, every build), {len(other_skips)} other SKIP, "
      f"{len(failed)} FAIL of {total} listed ===")
for n in sorted(env_skips):
    print(f"    environment skip: {n}")
for n in sorted(profile_skips):
    print(f"    profile skip: {n}")
for n in other_skips:
    print(f"    UNEXPECTED SKIP: {n}")
for n in failed:
    print(f"    FAIL: {n}")
if shard_fail or failed or other_skips or len(passed) + len(profile_skips) + len(env_skips) != total:
    print("RESULT: FAIL")
    sys.exit(1)
print(f"RESULT: ok -- {len(passed)} passed, {len(profile_skips)} stepped aside for the profile and "
      f"{len(env_skips)} for the environment, of {total} tests under -tags refugium")
PY
