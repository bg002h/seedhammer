#!/usr/bin/env bash
# MUTATION-TEST THE PAYLOAD CHAINS: prove each one can actually FAIL.
#
# A chain that cannot fail is decoration. This script breaks each chain in the
# two ways that matter and requires it to notice:
#
#   (1) WRONG FIXTURE.  Give a chain another payload's BYTES while keeping its
#       own pinned `digest`. The device recomputes the digest from what it read,
#       so the walk must stop at the Payload Digest screen -- the assertion that
#       binds `me sysw show` to what the operator compares by hand. The donor's
#       sha256 and byte count are swapped in too, deliberately: leaving them
#       would make chainBytes' hash check fire first, which is a DIFFERENT
#       assertion and would prove nothing about the ingest.
#
#   (2) WRONG GOLDEN.  Overwrite a chain's golden with another chain's, so the
#       plate reached from CLI bytes is compared against the wrong steel. Done
#       by swapping the FILE rather than by editing the test, so nothing about
#       the assertion itself is touched.
#
# Both mutations are reverted before the next one runs, and the tree is verified
# clean at the end. Nothing here is left behind.
#
# Usage:
#   ./scripts/chain-mutation-check.sh                 # all chains
#   ./scripts/chain-mutation-check.sh chain-seed      # one
#
# `go` is NOT on PATH on the machine this was written on; it comes from the Nix
# store. Set GO=/path/to/go, or let this find it.
set -uo pipefail

GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1; then
  echo "chain-mutation-check: no \`$GO\` on PATH; set GO=/path/to/go" >&2
  exit 2
fi
GO="$(command -v "$GO")"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
json="gui/testdata/chain/chain_payloads.json"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# name | go test -run pattern | fixture | donor fixture | golden | donor golden
#
# DONORS ARE CHOSEN SO THE F1 CLASSIFICATION MATCHES. A chain asserts up front
# whether its payload puts a secret in flash unencrypted (chainWalk.assertF1),
# and that check runs BEFORE the walk -- so swapping a secret payload for a
# non-secret one would stop at assertF1 and never reach the digest screen the
# mutation is aimed at.
CASES=(
  "chain-tx|TestChainFromAMePackedPayloadToACutQRPlate|chain-tx|chain-txonly|tx-qr|tx-unsigned-qr"
  "chain-seed|TestChainMnemonicFromAMePackedPayloadToASeedPlate|chain-seed|chain-codex32|chain-seed|chain-pass"
  "chain-codex32|TestChainCodex32FromAMePackedPayloadToAnMs1Plate|chain-codex32|chain-pass|chain-codex32|chain-seed"
  "chain-text|TestChainFreeTextFromAMePackedPayloadToATextPlate|chain-text|chain-txonly|chain-text|chain-pass"
  "chain-pass|TestChainPassphraseFromAMePackedPayloadToAPasswordPlate|chain-pass|chain-seed|chain-pass|chain-text"
  "chain-mdmk|TestChainMdMkFromTheEmulatorsOwnPayloadToNinePlates|chain-mdmk|chain-seed|chain-mdmk-md1-1|chain-codex32"
)

cp "$json" "$work/chain_payloads.orig.json"

swap_bytes() {  # <target> <donor>
  python3 - "$json" "$1" "$2" <<'PY'
import json, sys
path, target, donor = sys.argv[1:]
doc = json.load(open(path))
by = {p["name"]: p for p in doc["payloads"]}
t, d = by[target], by[donor]
# Everything about the CONTAINER moves; the pinned digest stays. That is the
# whole mutation: the bytes and the number the operator compares now disagree.
for k in ("blob", "file", "bytes", "sha256"):
    if k in d:
        t[k] = d[k]
    else:
        t.pop(k, None)
json.dump(doc, open(path, "w"), indent=2)
PY
}

pass_n=0
fail_n=0
report="$work/report.txt"
: > "$report"

for row in "${CASES[@]}"; do
  IFS='|' read -r name pattern fixture donor golden dgolden <<<"$row"
  if [ "$#" -gt 0 ] && [ "$1" != "$name" ]; then continue; fi

  # ─── mutation 1: the wrong fixture ───────────────────────────────────────
  swap_bytes "$fixture" "$donor"
  "$GO" test ./gui/ -run "$pattern" -count=1 -vet=off -timeout 30m \
    > "$work/$name.m1.txt" 2>&1
  rc=$?
  cp "$work/chain_payloads.orig.json" "$json"
  if [ "$rc" -eq 0 ]; then
    echo "$name  MUTATION 1 (wrong fixture)  SURVIVED -- the chain passed with $donor's bytes" >> "$report"
    fail_n=$((fail_n + 1))
  else
    line="$(grep -m1 -F 'digest screen does not show' "$work/$name.m1.txt" || true)"
    if [ -z "$line" ]; then
      line="$(grep -m1 -E 'FAIL|never reached' "$work/$name.m1.txt" || true)"
      echo "$name  MUTATION 1 (wrong fixture)  KILLED but NOT at the digest assertion: $line" >> "$report"
      fail_n=$((fail_n + 1))
    else
      echo "$name  MUTATION 1 (wrong fixture)  KILLED at ingest: ${line:0:150}" >> "$report"
      pass_n=$((pass_n + 1))
    fi
  fi

  # ─── mutation 2: the wrong golden ────────────────────────────────────────
  cp "gui/testdata/$golden.bin" "$work/$golden.bin.orig"
  cp "gui/testdata/$dgolden.bin" "gui/testdata/$golden.bin"
  "$GO" test ./gui/ -run "$pattern" -count=1 -vet=off -timeout 30m \
    > "$work/$name.m2.txt" 2>&1
  rc=$?
  cp "$work/$golden.bin.orig" "gui/testdata/$golden.bin"
  if [ "$rc" -eq 0 ]; then
    echo "$name  MUTATION 2 (wrong golden)   SURVIVED -- the chain passed against $dgolden" >> "$report"
    fail_n=$((fail_n + 1))
  else
    line="$(grep -m1 -F 'is NOT the plate' "$work/$name.m2.txt" || true)"
    if [ -z "$line" ]; then
      line="$(grep -m1 -E 'FAIL|never reached' "$work/$name.m2.txt" || true)"
      echo "$name  MUTATION 2 (wrong golden)   KILLED but NOT at the plate comparison: $line" >> "$report"
      fail_n=$((fail_n + 1))
    else
      detail="$(grep -m1 -oE 'spline (lengths )?[0-9]+ (vs|and) [0-9]+.*' "$work/$name.m2.txt" || true)"
      echo "$name  MUTATION 2 (wrong golden)   KILLED at compare: ${detail:-${line:0:120}}" >> "$report"
      pass_n=$((pass_n + 1))
    fi
  fi
done

cat "$report"
echo
echo "killed at the intended assertion: $pass_n     not killed as intended: $fail_n"

# THE TREE MUST BE CLEAN. A mutation left in place is worse than no mutation
# test: it would be committed as if it were the fixture.
dirty="$(git status --porcelain -- "$json" gui/testdata/ | head -20)"
if [ -n "$dirty" ]; then
  echo >&2
  echo "chain-mutation-check: THE TREE IS DIRTY after restoring; do not commit." >&2
  echo "$dirty" >&2
  exit 1
fi
echo "tree clean: every mutation was reverted"
[ "$fail_n" -eq 0 ]
