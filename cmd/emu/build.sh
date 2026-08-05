#!/usr/bin/env sh
# Build the firmware GUI to emu.wasm, next to index.html.
#
# wasm_exec.js MUST come from the same Go that compiled the wasm -- the two are
# a matched pair, and a mismatched one fails at load with an opaque error.
set -eu
cd "$(dirname "$0")"
GOROOT=$(go env GOROOT)
cp "$GOROOT/lib/wasm/wasm_exec.js" wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -o emu.wasm .
echo "built emu.wasm ($(wc -c < emu.wasm) bytes); serve this directory and open index.html"
