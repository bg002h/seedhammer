#!/usr/bin/env sh
# Build the firmware GUI to emu.wasm, next to index.html.
#
# wasm_exec.js MUST come from the same Go that compiled the wasm -- the two are
# a matched pair, and a mismatched one fails at load with an opaque error.
set -eu
cd "$(dirname "$0")"
GOROOT=$(go env GOROOT)
# -f, because GOROOT may be read-only -- under Nix it is mode 444 in the store,
# so the copy lands unwritable and every rebuild after the first died with
# "cannot create regular file 'wasm_exec.js': Permission denied".
cp -f "$GOROOT/lib/wasm/wasm_exec.js" wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -o emu.wasm .
echo "built emu.wasm ($(wc -c < emu.wasm) bytes); serve this directory and open index.html"
