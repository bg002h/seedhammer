#!/usr/bin/env bash
# Vendor the Rust primary's compose REFUSAL vectors (compose_refusal_*) into
# md/testdata/vectors/ and write their own provenance pin.
#
# SEPARATE FROM scripts/vendor-compose-vectors.sh ON PURPOSE. That script pins
# the whole compose corpus at one commit; a refusal vector can land on a
# commit that also carries codec changes the Go port has not taken, and
# re-vendoring everything to reach one file imports the rest in silence.
# md/testdata/compose_refusal_vectors.provenance.json states the measurement
# behind that.
#
#   scripts/vendor-compose-refusal-vectors.sh [/path/to/descriptor-mnemonic]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${1:-$HERE/../descriptor-mnemonic}"
VEC="$SRC/crates/md-codec/tests/vectors"
DST="$HERE/md/testdata/vectors"
PIN="$HERE/md/testdata/compose_refusal_vectors.provenance.json"
[ -d "$VEC" ] || { echo "no vectors at $VEC" >&2; exit 2; }
commit=$(git -C "$SRC" rev-parse HEAD)
branch=$(git -C "$SRC" rev-parse --abbrev-ref HEAD)
clean=true; [ -z "$(git -C "$SRC" status --porcelain -- crates/md-codec/tests/vectors)" ] || clean=false
mapfile -t files < <(cd "$VEC" && ls | grep -E '^compose_refusal_' | sort)
[ "${#files[@]}" -gt 0 ] || { echo "no compose_refusal_* vectors in $VEC" >&2; exit 2; }
for f in "${files[@]}"; do cp "$VEC/$f" "$DST/$f"; done
python3 - "$PIN" "$commit" "$clean" "$branch" "$DST" "${files[@]}" <<'PY'
import hashlib, json, sys, os, datetime
pin, commit, clean, branch, dst, files = sys.argv[1], sys.argv[2], sys.argv[3] == "true", sys.argv[4], sys.argv[5], sys.argv[6:]
rows = [{"name": f, "sha256": hashlib.sha256(open(os.path.join(dst, f), "rb").read()).hexdigest()} for f in files]
doc = json.load(open(pin)) if os.path.exists(pin) else {}
doc.update({
  "repo": "descriptor-mnemonic",
  "remote": "git@github.com:bg002h/descriptor-mnemonic.git",
  "commit": commit,
  "commit_on_branch": branch,
  "repo_clean_when_recorded": clean,
  "path": "crates/md-codec/tests/vectors",
  "files": rows,
  "file_count": len(rows),
  "vectors": len(rows),
  "recorded_at": datetime.date.today().isoformat(),
})
json.dump(doc, open(pin, "w"), indent=2); open(pin, "a").write("\n")
print("vendored %d refusal vector(s), primary %s" % (len(rows), commit[:12]))
PY
