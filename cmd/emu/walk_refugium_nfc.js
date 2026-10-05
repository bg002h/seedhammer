// REFUGIUM NFC WALK: the parent plan's F7 Done-when, "an emulator test shows NFC
// off from seed entry to power-off" (Refugium plan F7 §4.2).
//
//   const w = await import("./walk_refugium_nfc.js");
//   w.run({ build: "default" }).then(r => { window.__r = r });   // minutes: it engraves
//
// NOT loaded by index.html, for walk_trace_a.js's reason: this drives the
// machine, and a page that starts engraving because somebody opened it is a trap.
// scripts/emu-refugium-walk.sh builds the wasm, serves it and runs this headless.
//
// THE ROUTE. Power on, skip the boot payload offer, start Engrave Single-Sig,
// type a seed, take the defaults, engrave every card, take "Verify now", and end
// on the flow's final screen, the Restore Doc. gui has no power-off prompt; the
// end of the flow is the end of the session.
//
// AT EVERY SCREEN A RECORD IS PRESENTED that triggers nothing on its own: plain
// text no scanner arm recognises, so the start screen reports an unknown format
// and does not navigate (an md1/mk1 would derail the walk). The boot offer is
// SKIPPED, never loaded: the emulator's default `records` payload holds a
// `pass:` record, which the Refugium build refuses at load (§4.3).
//
// THREE BUILDS, ONE SCRIPT, three verdicts on three counters:
//
//   "default"            positive control. delivered() > 0, first at the start
//                        screen before the flow is entered: the instrument can
//                        see a read. readerAsks() > 0: gui asks the platform.
//   "refugium"           `-tags refugium`. The emulator twin reports no
//                        FeatureNFC and hands out no reader: presented() > 0,
//                        delivered() == 0, readerAsks() == 0.
//   "refugium-attached"  the same build with the reader FORCED on
//                        (shNFC.forceReader()), so the platform WOULD hand gui a
//                        reader if gui asked. Same verdict, and readerAsks() == 0
//                        is the point: gui's ctx.nfcReader() answers "no reader"
//                        under the profile without asking the platform at all.
//                        (Corrected, review M-5: this arm does NOT put a reader
//                        in front of startScanner, because gui never takes it.
//                        startScanner's own nil-ing is the second gui layer and
//                        is pinned by gui's TestRefugiumStartScannerReadsNothing.)

import { keyPoint } from "./walk_trace_b.js";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "").toLowerCase();

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
const NOWHERE = [5, 5];
const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;
const HOLD_MS = 1300;

// BIP-39's published 12-word vector; public by construction, never fund it.
export const SEED_WORDS =
  "legal winner thank year wave sausage worth useful legal winner thank yellow";

// Lowercased and squashed, like everything compared against the screen.
const PASSPHRASE_PROMPT = "Add a BIP-39 passphrase?";
const PASSPHRASE_NOTICE = "This build takes no BIP-39 passphrase.";

async function waitFor(needle, timeoutMs = 20000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(`waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(text)}`);
    }
    await sleep(50);
  }
}

async function raceFor(needles, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = squash(window.shScreen());
    for (const n of needles) {
      if (text.includes(squash(n))) return n;
    }
    if (Date.now() >= deadline) {
      throw new Error(`none of ${JSON.stringify(needles)} appeared within ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(window.shScreen())}`);
    }
    await sleep(50);
  }
}

const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };

async function hold([x, y]) {
  window.shPress(x, y);
  await sleep(HOLD_MS);
  window.shRelease(x, y);
  await sleep(300);
}

async function goTo(program, max = 16) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

async function typeWord(word, index, total) {
  const n = index + 1;
  for (let i = 0; i < word.length; i++) {
    await tap(keyPoint(word[i]), 120);
    const want = squash(`${n}:${word.slice(0, i + 1)}`);
    if (!squash(window.shScreen()).includes(want)) {
      throw new Error(`typing word ${n} (${word}): after letter ${i + 1} the screen does not ` +
        `show ${JSON.stringify(want)}; it reads ${JSON.stringify(window.shScreen())}`);
    }
  }
  await tap(CONFIRM, 400);
  if (n < total) await waitFor(`Word ${n + 1} of ${total}`);
}

/** Type the 12 words, after whatever leads to the keyboard. */
async function typeSeed(present) {
  const words = SEED_WORDS.split(" ");
  await waitFor("Choose number of words");
  present("word count");
  await tap([240, rowY(0, 2)], 300);   // 12 WORDS
  await tap(CONFIRM, 400);
  await waitFor(`Word 1 of ${words.length}`);
  present("keyboard");
  for (let i = 0; i < words.length; i++) await typeWord(words[i], i, words.length);
}

/**
 * Run the walk. Returns a record of what happened; `ok` is set true only after
 * the last assertion, and every assertion throws.
 *
 * @param {object} opts
 * @param {"default"|"refugium"|"refugium-attached"} opts.build
 */
export async function run(opts = {}) {
  const build = opts.build;
  if (!["default", "refugium", "refugium-attached"].includes(build)) {
    throw new Error(`run({build}) needs "default", "refugium" or "refugium-attached", got ${JSON.stringify(build)}`);
  }
  if (typeof window.shNFC.delivered !== "function" || typeof window.shNFC.readerAsks !== "function") {
    throw new Error("shNFC.delivered is missing - this is a STALE emu.wasm; serve on a fresh port.");
  }
  const refugium = build !== "default";
  const out = { build, screens: [], presentedAt: [], deliveredAt: [], ok: false };
  let seq = 0;
  // present one inert record and remember where; it is plain text with no
  // scanner arm (gui/scan.go), so nothing navigates on it.
  const present = (where) => {
    seq++;
    window.shNFC.present(`refugium-walk-inert-record-${seq}`);
    out.presentedAt.push(where);
  };
  const note = (where) => {
    out.screens.push(`${where}: ${window.shScreen()}`);
    out.deliveredAt.push(`${where}: presented=${window.shNFC.presented()} delivered=${window.shNFC.delivered()} asks=${window.shNFC.readerAsks()}`);
  };

  if (build === "refugium-attached") {
    if (typeof window.shNFC.forceReader !== "function") {
      throw new Error("shNFC.forceReader is missing: this is not a Refugium emulator build");
    }
    window.shNFC.forceReader();
  }

  // Boot: the systemwide offer. SKIP (Back), never LOAD; see the header.
  await waitFor("Load it?", 60000);
  note("boot offer");
  present("boot offer");
  await tap(BACK, 500);

  // The start screen, BEFORE the flow is entered: the default build's scanner
  // reads the record here, which is where the positive control is taken.
  await waitFor("Backup Wallet");
  present("start screen");
  await sleep(3000);
  note("start screen");
  const atStart = window.shNFC.delivered();
  if (!refugium && atStart < 1) {
    throw new Error(`default build: the start screen did not read the presented record ` +
      `(delivered=${atStart}); the instrument cannot see a read, so the Refugium verdict would mean nothing`);
  }
  if (refugium && atStart !== 0) {
    throw new Error(`${build}: the start screen READ ${atStart} record(s) over NFC`);
  }

  await goTo("Engrave Single-Sig");
  await tap(CONFIRM, 500);

  // Seed source. The default build offers TYPE IT / SCAN; the Refugium build has
  // no reader, so the picker is a menu of one and is skipped (§13 D9).
  const first = await raceFor(["Where from?", "Choose number of words"]);
  note("seed source");
  present("seed source");
  if (first === "Where from?") {
    if (refugium) {
      throw new Error(`${build}: the seed picker was drawn: ${window.shScreen()}`);
    }
    if (!squash(window.shScreen()).includes("scan")) {
      throw new Error(`default build: the seed picker offers no SCAN row: ${window.shScreen()}`);
    }
    await tap([240, rowY(0, 2)], 300);  // TYPE IT
    await tap(CONFIRM, 400);
  }
  await typeSeed(present);

  await waitFor("Choose address type");
  note("wallet type");
  present("wallet type");
  await tap(CONFIRM, 400);             // the default, row 0

  const pp = await raceFor([PASSPHRASE_PROMPT, PASSPHRASE_NOTICE]);
  note("passphrase");
  present("passphrase");
  if (refugium !== (pp === PASSPHRASE_NOTICE)) {
    throw new Error(`${build}: passphrase step drew ${JSON.stringify(pp)}`);
  }
  await tap(CONFIRM, 500);             // Skip (row 0) / acknowledge

  await waitFor("What to engrave?");
  note("engrave mode");
  present("engrave mode");
  await tap(CONFIRM, 400);             // Full, row 0

  await waitFor("Which md1?");
  present("which md1");
  await tap(CONFIRM, 400);             // Full policy md1, row 0

  await waitFor("Plates To Cut");
  note("census");
  present("census");
  await tap(CONFIRM, 500);

  // The engrave tail, terminated by an OBSERVED screen: the verify offer.
  const variants = [];
  for (let guard = 0; guard < 400; guard++) {
    const scr = squash(window.shScreen());
    if (scr.includes(squash("Verify the engraved plates?"))) break;
    if (scr.includes(squash("Choose engraving"))) {
      variants.push(window.shScreen());
      present(`variant ${variants.length}`);
      await tap(CONFIRM, 500);
      continue;
    }
    if (scr.includes(squash("Hold button to start the engraving process"))) {
      present("engrave prompt");
      await hold(CONFIRM);
      continue;
    }
    if (scr.includes(squash("Engraving completed successfully"))) {
      present("engrave done");
      await tap(CONFIRM, 500);
      continue;
    }
    await tap(NOWHERE, 300);
    await sleep(700);
  }
  out.variants = variants;
  await waitFor("Verify the engraved plates?");
  note("verify offer");
  present("verify offer");
  await tap([240, rowY(0, 2)], 300);   // Verify now
  await tap(CONFIRM, 500);

  const v = await raceFor(["Choose number of words", "Verify on another build"]);
  if (v === "Choose number of words") {
    if (refugium) throw new Error(`${build}: verify asked for the seed before refusing the readback`);
    await typeSeed(present);
    await waitFor("Choose address type");
    await tap(CONFIRM, 400);
    await waitFor(PASSPHRASE_PROMPT);
    await tap(CONFIRM, 500);
    await waitFor("Scan a card, or Done");
    present("verify gather");
    await sleep(3000);
    note("verify gather");
    await tap(BACK, 500);
  } else {
    if (!refugium) throw new Error(`default build: verify refused the readback`);
    note("verify refused");
    present("verify refused");
    await tap(CONFIRM, 500);
  }

  await waitFor("Restore Doc");
  present("restore doc");
  await sleep(3000);
  note("restore doc");

  out.presented = window.shNFC.presented();
  out.delivered = window.shNFC.delivered();
  out.readerAsks = window.shNFC.readerAsks();
  if (refugium && out.readerAsks !== 0) {
    throw new Error(`${build}: gui asked the platform for a reader ${out.readerAsks} time(s)`);
  }
  if (!refugium && out.readerAsks < 1) {
    throw new Error("default build: gui never asked for a reader, so readerAsks() proves nothing");
  }
  if (out.presented < 1) throw new Error("nothing was presented, so this walk proves nothing");
  if (refugium && out.delivered !== 0) {
    throw new Error(`${build}: ${out.delivered} record(s) were READ over NFC`);
  }
  if (!refugium && out.delivered < 1) {
    throw new Error("default build: no record was read");
  }
  out.ok = true;
  return out;
}
