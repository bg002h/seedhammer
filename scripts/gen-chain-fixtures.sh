#!/usr/bin/env bash
# Regenerate gui/testdata/chain/chain_payloads.json by RUNNING `me sysw pack`.
#
# The fixtures this writes are the first link of the payload chain: a container
# built by the producer CLI, not a Go literal. Every walk in gui/chain_walk_test.go
# starts from one of them, so what the device ingests is what `me` emits.
#
# THIS SCRIPT IS THE REPRODUCTION PATH, and it is committed for that reason: a
# generator nobody re-runs rots while its artifact keeps vouching for it.
# gui/chain_fixture_live_test.go (build tag `oraclelive`) re-runs the same `me`
# invocations and asserts the committed bytes still match, so the rot is caught
# by a command rather than noticed by accident.
#
# `me sysw pack` is DETERMINISTIC for the unsealed variant -- measured, three
# identical runs -- because salt and IV are only consumed on the sealed path.
# A sealed fixture could not be pinned this way and there is none here.
#
# Usage:
#   ./scripts/gen-chain-fixtures.sh            # uses `me` from $ME or $PATH
#   ME=/home/bcg/.cargo/bin/me ./scripts/gen-chain-fixtures.sh
set -euo pipefail

ME="${ME:-me}"
MT="${MT:-mt}"
# By PATH, resolved and reported: a bare name can be a shell alias or a stale
# install, and this file's whole claim is about which binary produced the bytes.
ME="$(command -v "$ME")"
MT="$(command -v "$MT")"
ME_VERSION="$("$ME" --version)"
MT_VERSION="$("$MT" --version)"
echo "me: $ME ($ME_VERSION)" >&2
echo "mt: $MT ($MT_VERSION)" >&2

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$root/gui/testdata/chain/chain_payloads.json"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The pinned "even" vector: the same 6-chunk mt1 set gui/transaction_test.go
# holds, so the chain and the unit tests are exercised on ONE artifact.
cat > "$work/even.mt1" <<'EOF'
mt1p9h8jqq9qqqqgqqqqqqqyqherdfykhhpey6z2cvafak8804qd7g0dl6v8ex9wr2cvky023skwkeud2229sax
mt1p9h8jqq9qqphgdqqqqqqqq0mllllupyqj6vqqqqqqqqzcqpfsw7ph2rt5w54kt768636cls8zxg0najlzunp
mt1p9h8jqq9qqzj8yqpnzw4vl2rwffqyqqqqqkqq282yyhc2vavd20hvk94pz39hts3u5s9a0qd8pwskxfl7ju5
mt1p9h8jqq9qqrqfrnq3qzyp77h37cnxzvwutegzmzy5zrrrfvrpykdfsckvk03dcq6rcjtvlsfcglv7zx43yaz
mt1p9h8jqq9qqylgpzqmhcwhuupdvnrc82rncvzzdahpgjsdwgu52jd7vmxsve9x3w5ujeqyssuvddxvwqze4ve
mt1p9h8jqq9qq9qdcc7h75twfxyf340c4sgqzhfdq6xtgt7zhxngpwa049l0z59l6jqcqqqqqq5k5y2ye5nv8yf
EOF

# The raw signed transaction comes from `mt decode` over that set, never from a
# constant typed beside it: a hex literal that drifted from the strings would
# make the merge test pass for the wrong reason.
"$MT" decode --in "$work/even.mt1" --quiet > "$work/even.rawhex"
tr -d '\n' < "$work/even.rawhex" > "$work/even.hex"
signed="$(cat "$work/even.hex")"

# The signature-stripped form of the SAME transaction: 113 bytes, identical
# txid. This is G-P3.10's input, and `me` refuses it without the escape hatch.
stripped='02000000017c8da925af70e49a12b0cea7b639df5037c87b7fa61f262b86ac32c47aa3ba1a0000000000fdffffff02404b4c0000000000160014c1de0dd435d1d4ad97ed1f51d63f91c800cc4eab3ea1b92901000000160014751097c299d6354fbb2c5a84512dd708f2902f5e60000000'

mkdir -p "$(dirname "$out")"

# emit <name> <note> <records-file> [extra pack flags...]
emit() {
  local name="$1" note="$2" recs="$3"; shift 3
  local blob="$work/$name.bin"
  "$ME" sysw pack --in "$recs" --no-passphrase --out "$blob" "$@" 2> "$work/$name.err"
  python3 - "$name" "$note" "$recs" "$blob" "$work/$name.err" "$@" <<'PY' >> "$work/entries.json"
import hashlib, json, sys
name, note, recs, blob, errf, *flags = sys.argv[1:]
b = open(blob, 'rb').read()
records = [l for l in open(recs).read().split('\n') if l.strip()]
cmd = ["me", "sysw", "pack", "--in", "<records>", "--no-passphrase", "--out", name + ".bin"] + flags
# The digest `me sysw show` prints -- the number the DEVICE asks the operator to
# compare on its own Payload Digest screen. Pinned from the CLI's stderr so the
# two are bound rather than each computed and hoped to agree.
digest = ""
for line in open(errf):
    if line.startswith("digest:"):
        digest = line.split(":", 1)[1].strip()
json.dump({
    "name": name, "note": note, "command": cmd, "records": records,
    "blob": b.hex(), "bytes": len(b),
    "sha256": hashlib.sha256(b).hexdigest(),
    "digest": digest,
}, sys.stdout, indent=2)
sys.stdout.write(",\n")
PY
}

: > "$work/entries.json"

{ cat "$work/even.mt1"; printf 'tx:%s\n' "$signed"; } > "$work/chain-tx.rec"
emit chain-tx \
  "one transaction delivered BOTH ways: the complete 6-chunk mt1 set and a tx: record of the same 222 bytes. Both plate kinds are offered." \
  "$work/chain-tx.rec"

{ cat "$work/even.mt1"; printf 'tx:%s\n' "$stripped"; } > "$work/chain-gp310.rec"
emit chain-gp310 \
  "G-P3.10's input: the same mt1 set plus the SIGNATURE-STRIPPED form of its transaction (113 B, identical txid). me names it on stderr and packs it under --allow-unsigned-inputs; the device DROPS it." \
  "$work/chain-gp310.rec" --allow-unsigned-inputs

printf 'tx:%s\n' "$signed" > "$work/chain-txonly.rec"
emit chain-txonly \
  "a tx: record alone -- the QR-only payload. No mt1 strings, so TEXT PLATES is not offered." \
  "$work/chain-txonly.rec"

python3 - "$out" "$ME_VERSION" "$MT_VERSION" "$work/entries.json" <<'PY'
import json, sys
out, me_version, mt_version, entries = sys.argv[1:]
body = open(entries).read().rstrip().rstrip(',')
doc = {
  "_comment": [
    "GENERATED by scripts/gen-chain-fixtures.sh -- do not hand-edit.",
    "",
    "Each blob is a SYSTEMWIDE container written by the `me` CLI named below,",
    "by running the recorded command. It is the first link of the payload",
    "chain: gui/chain_walk_test.go loads these bytes through sysw.FileReader",
    "and syswLoadFlow, exactly as the device reads flash, so the walk starts",
    "from what the producer emits rather than from a Go literal.",
    "",
    "`digest` is the number `me sysw show` printed for the blob. The device's",
    "Payload Digest screen recomputes it from the bytes; the walk asserts the",
    "screen shows THIS string, which is what binds host and device.",
    "",
    "TO RE-SYNC after an `me` change: ./scripts/gen-chain-fixtures.sh",
    "gui/chain_fixture_live_test.go (build tag `oraclelive`) re-runs the same",
    "invocations and fails if the committed bytes have drifted."
  ],
  "me_version": me_version,
  "mt_version": mt_version,
  "payloads": json.loads("[" + body + "]"),
}
open(out, "w").write(json.dumps(doc, indent=2) + "\n")
print("wrote", out, "with", len(doc["payloads"]), "payloads")
PY
