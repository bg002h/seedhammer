#!/usr/bin/env bash
# emu-refugium-walk.sh -- run cmd/emu/walk_refugium_nfc.js headless.
#
#   scripts/emu-refugium-walk.sh default            # positive control
#   scripts/emu-refugium-walk.sh refugium           # -tags refugium
#   scripts/emu-refugium-walk.sh refugium-attached  # -tags refugium, reader forced on
#
# Refugium plan F7 §4.2: the parent plan's "emulator test shows NFC off from seed
# entry to power-off". It builds cmd/emu to a scratch directory (the default build
# untagged, the two Refugium arms with -tags refugium), serves it, and drives the
# walk through Playwright's Chromium. Exit 0 only if the walk returned ok:true;
# its JSON result is printed either way.
#
# Needs: go (the repo's toolchain), python3, node, and a Playwright install.
#   PLAYWRIGHT_MODULE  module path for require() (default: "playwright")
#   CHROMIUM           optional executablePath, for a Playwright whose pinned
#                      browser revision is not the one installed
#   PLAYWRIGHT_BROWSERS_PATH is honoured by Playwright itself.
#
# What it does NOT do: run in CI. The walk engraves three plates in the
# emulator and takes minutes; CI vets the tagged wasm build instead
# (GOOS=js GOARCH=wasm go vet -tags refugium ./cmd/emu/) and runs the
# delivered-count host tests. The implementer and the reviewer run this.
set -euo pipefail

BUILD="${1:-}"
case "$BUILD" in
default) TAGS="" ;;
refugium | refugium-attached) TAGS="refugium" ;;
*)
	echo "usage: $0 default|refugium|refugium-attached" >&2
	exit 2
	;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$(mktemp -d)"
trap 'kill "${SERVER:-0}" 2>/dev/null || true; rm -rf "$OUT"' EXIT

cd "$ROOT"
GOOS=js GOARCH=wasm go build -trimpath ${TAGS:+-tags "$TAGS"} -o "$OUT/emu.wasm" ./cmd/emu
cp -f "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/"
cp cmd/emu/index.html cmd/emu/*.js "$OUT/"

PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
python3 -m http.server "$PORT" --bind 127.0.0.1 --directory "$OUT" >/dev/null 2>&1 &
SERVER=$!
sleep 1

cat >"$OUT/run.cjs" <<'JS'
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
(async () => {
  const launch = process.env.CHROMIUM ? { executablePath: process.env.CHROMIUM } : {};
  const browser = await chromium.launch(launch);
  const page = await browser.newPage();
  page.on("pageerror", (e) => console.error("pageerror:", e.message));
  await page.goto(`http://127.0.0.1:${process.env.PORT}/index.html`);
  await page.waitForFunction(() => typeof window.shScreen === "function" &&
    typeof window.shNFC === "object", null, { timeout: 120000 });
  let result;
  try {
    result = await page.evaluate(async (build) => {
      const w = await import("./walk_refugium_nfc.js");
      return await w.run({ build });
    }, process.env.BUILD);
  } catch (e) {
    console.log(JSON.stringify({ build: process.env.BUILD, ok: false, error: String(e.message || e) }, null, 2));
    await browser.close();
    process.exit(1);
  }
  console.log(JSON.stringify(result, null, 2));
  await browser.close();
  process.exit(result && result.ok === true ? 0 : 1);
})().catch((e) => { console.error("ERR", e); process.exit(1); });
JS

PORT="$PORT" BUILD="$BUILD" node "$OUT/run.cjs"
