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

# emit_file <name> <note> <path-relative-to-repo-root> <command...>
#
# Records a container that ALREADY EXISTS in the tree instead of writing a new
# one. There is exactly one user -- cmd/emu/sysw_cards_payload.bin -- and the
# reason is the whole point of this function: cmd/emu/walk_trace_a.js drives
# that blob to a completed engrave in the browser, and a SECOND CLI-built
# ClassMDMK container beside it would be two payloads that can drift apart with
# only one of them failing a CI run. So the go test reads the emulator's own
# bytes, by path, and nothing is copied.
#
# The digest comes from `me sysw show` over that file, so the JSON entry is
# still bound to the CLI rather than to a constant somebody typed.
# TestChainMdMkFixtureIsTheEmulatorsOwnPayload then requires this digest, the
# `syswCardsDigest` constant in cmd/emu/sysw_cards_payload.go and
# walk_trace_a.js's CARDS_DIGEST to be one value.
emit_file() {
  local name="$1" note="$2" rel="$3"; shift 3
  local blob="$root/$rel"
  "$ME" sysw show "$blob" > "$work/$name.out" 2> "$work/$name.err"
  python3 - "$name" "$note" "$rel" "$blob" "$work/$name.out" "$work/$name.err" "$@" <<'PY' >> "$work/entries.json"
import hashlib, json, os, sys
name, note, rel, blob, outf, errf, *cmd = sys.argv[1:]
b = open(blob, 'rb').read()
digest = ""
for line in open(outf).read().splitlines() + open(errf).read().splitlines():
    if line.startswith("digest:"):
        digest = line.split(":", 1)[1].strip()
# The path is stored RELATIVE TO THE JSON, because that is the only anchor the
# Go test has: it resolves testdata/chain/<file>. gui/testdata/chain -> root is
# three levels up.
json.dump({
    "name": name, "note": note, "command": cmd, "records": [],
    "file": os.path.join("..", "..", "..", rel),
    "bytes": len(b),
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

# ─── one payload per remaining PACKABLE class ───────────────────────────────
#
# All four are TEST MATERIAL, public by construction, and the two secret ones
# are published vectors: never put funds behind them. Each holds exactly ONE
# record, because the subject of these chains is the class and a second record
# would only add screens.
#
# `me sysw pack` REFUSES Descriptor and Address (rc=4, "Descriptors and
# addresses are not yet classifiable here -- see sysw::classify"), so there is
# no fixture for either and there cannot be one.

# ClassMnemonic. BIP-39's own all-zero-entropy vector, which is also
# cmd/emu/sysw_test_payload.bin's seed and gui's fixtureMasterA.
printf 'abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about\n' \
  > "$work/chain-seed.rec"
emit chain-seed \
  "ClassMnemonic alone. A SECRET in cleartext, so the device raises F1 and offers KEEP/UNLOAD -- two screens the transaction chains never reach." \
  "$work/chain-seed.rec"

# ClassCodex32Secret. THE LENGTH MATTERS AND IT IS NOT BIP-93's.
# `me`'s ms_codec v0.1 accepts codex32 string lengths [50, 56, 62, 69, 75] only.
# The fork's own committed ms1 fixtures are 48 (backup_test.go's `ms13cash…`)
# and 74 (gui/sysw_cells_test.go's cellMs1); Go's codex32.New accepts both and
# `me sysw pack` REFUSES both at rc=4. This is the crate's own 50-char vector.
printf 'ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f\n' > "$work/chain-codex32.rec"
emit chain-codex32 \
  "ClassCodex32Secret alone -- the 50-char vector, which is the length me accepts. Also a secret, so F1 fires here too." \
  "$work/chain-codex32.rec"

# ClassFreeText / ClassPassphrase. Spec §5.3.1 makes both bodies LOWERCASE HEX
# and refuses a non-hex body as ClassUnknown, so the hex is required rather than
# stylistic -- the same trap sysw_test_payload.go records.
python3 - "$work" <<'PY'
import binascii, sys
w = sys.argv[1]
open(w + "/chain-text.rec", "w").write(
    "text:" + binascii.hexlify(b"SEEDHAMMER II CHAIN WALK").decode() + "\n")
open(w + "/chain-pass.rec", "w").write(
    "pass:" + binascii.hexlify(b"correct horse battery staple").decode() + "\n")
PY
emit chain-text \
  "ClassFreeText alone. Not secret, so no F1: the load flow goes offer -> digest -> program." \
  "$work/chain-text.rec"
emit chain-pass \
  "ClassPassphrase alone -- the xkcd string, hex-encoded per §5.3.1. Secret, so F1 fires." \
  "$work/chain-pass.rec"

# ClassMDMK: THE EMULATOR'S OWN BYTES, BY PATH, NOT A SECOND COPY.
# cmd/emu/walk_trace_a.js drives this exact blob to a completed engrave in the
# browser. Packing a second ClassMDMK container here would create two CLI-built
# payloads that can drift, with only one of them failing a CI run.
emit_file chain-mdmk \
  "ClassMDMK: cmd/emu/sysw_cards_payload.bin itself -- four cosigner cards (9 mk1 chunks) plus master A's mnemonic. The SAME bytes cmd/emu/walk_trace_a.js loads, read by path so the two cannot drift." \
  cmd/emu/sysw_cards_payload.bin \
  "go run ./cmd/buildpayloadcards | me sysw pack --no-passphrase --in - --out cmd/emu/sysw_cards_payload.bin"

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
