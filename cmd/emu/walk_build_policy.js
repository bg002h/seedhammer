// The BUILD-POLICY driver: reach `Engrave Multisig -> Build policy`, and prove
// which flow you are in.
//
//   const w = await import("./walk_build_policy.js");
//   w.run().then(r => { window.__walk = r });      // seconds, no engraving
//
// NOT loaded by index.html, for walk_trace_a.js's reason.
//
// WHY THIS FILE EXISTS (F-169, F-174).
//
// Every stage from S1 on edits buildMultisigPolicyFlow (gui/multisig_build.go).
// The only walk that existed drove `Engrave Bundle`, which dispatches to
// bundleFlow instead — so five "by test and by emulator walk" gates named a
// flow no walk had ever entered. This is that walk.
//
// THE PART THAT IS NOT OBVIOUS, and the reason a stage author cannot simply
// point the old script at a different carousel entry: BOTH flows end at the
// SAME gather screen, drawn by the same shared gatherer. Measured 2026-08-14 by
// driving each and squashing shScreen():
//
//   via Build policy    EngraveBundlemd1descriptors:0mk1keys:0Scanacard,orDone.
//   via Engrave Bundle  EngraveBundlemd1descriptors:0mk1keys:0Scanacard,orDone.
//
// Character for character. The title reads "Engrave Bundle" even when the
// operator arrived through Build policy, because layoutTitle is called inside
// the shared gatherer (gui/bundle_flow.go:155). So "the walk reached a card
// gather" is not evidence of anything, and neither is the title.
//
// What IS evidence is a needle with exactly ONE production site. cmd/emu's
// needle_test.go pins that count and fails if it ever drifts; this file asserts
// the needle actually appeared. Those two halves together are the gate — one
// without the other proves nothing.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// ChoiceScreen rows, MEASURED rather than derived from the layout source.
//
// Rows are 24px apart and the stack is centred on y=160 within the content
// band. Established by tapping y=172 on the 2-row front door (landed on choice
// 1, "Build policy") and confirmed on the 4-row n-picker (row 1 selected "3",
// and the next screen read "Required signatures (k of 3)?").
//
// A wrong row here does NOT fail loudly on its own — it silently picks a
// different parameter — so every caller below re-asserts with a needle after
// choosing. That is the only reason it is safe to compute a coordinate at all.
export const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// Needles unique to ONE production flow. Keep in sync with
// cmd/emu/needle_test.go's buildFlowNeedles, which is what proves "unique".
export const NEEDLE_FRONT_DOOR = "Supply or build a policy?"; // gui/multisig.go
export const NEEDLE_TEMPLATE = "Choose policy type"; // gui/multisig_build.go
export const NEEDLE_N = "How many keys (n)?"; // gui/multisig_build.go
export const NEEDLE_SLOT = "Which slot is your key?"; // gui/multisig_build.go

// The gather text, which is deliberately NOT a needle. Named so that a future
// author who reaches for it finds this comment instead.
const GATHER_NOT_A_NEEDLE = "Scan a card, or Done";

async function waitFor(needle, timeoutMs = 10000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(
        `waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(text)}`);
    }
    await sleep(50);
  }
}

const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };

async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

/**
 * Select row `i` of `n` on a ChoiceScreen, then confirm, then REQUIRE `expect`.
 *
 * The post-condition is not optional politeness. Tapping the wrong row picks a
 * different parameter and carries on perfectly happily, which would make a walk
 * report a pass for a policy nobody asked for.
 */
async function choose(i, n, expect, label) {
  await tap([240, rowY(i, n)], 300);
  await tap(CONFIRM, 400);
  try {
    await waitFor(expect);
  } catch (e) {
    throw new Error(`choosing ${label} (row ${i} of ${n}) did not land on ` +
      `${JSON.stringify(expect)}: ${e.message}`);
  }
}

/**
 * Assert nothing has crossed the emulated NFC reader.
 *
 * F-174: a cosigner gather completed over the reader is green whether or not
 * the payload-supplied-cards feature exists, and phase-1 hardware has no reader
 * at all. So a stage gate asserts the cards did NOT come from a tag.
 *
 * Exported so the mutation proof can call it directly after presenting one.
 */
export function assertNoNFC(where) {
  if (typeof window.shNFC.presented !== "function") {
    throw new Error("shNFC.presented is missing — this is a STALE emu.wasm. " +
      "The browser caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const n = window.shNFC.presented();
  if (n !== 0) {
    throw new Error(`${where}: ${n} record(s) crossed the NFC reader; a stage-gate run must ` +
      `present ZERO — the cards come from the payload, and phase-1 hardware has no reader`);
  }
  return n;
}

/**
 * Drive to the Build-policy cosigner gather.
 *
 * @param {object}  [opts]
 * @param {string}  [opts.payload="cards"]  which systemwide blob to serve.
 * @param {number}  [opts.n=3]              cosigner count; row index is n-2.
 * @param {number}  [opts.k=2]              threshold; row index is k-1.
 * @param {number}  [opts.selfSlot=0]       which slot is the operator's.
 * @param {boolean} [opts.includeFp=false]  fingerprint presence.
 * @returns {Promise<object>} the needles proven, the gather screen, and the
 *          NFC count — everything a stage gate asserts on.
 *
 * NOT DONE HERE, deliberately: loading the payload first. S0b's job is to prove
 * the flow is reachable and identifiable; the cards still arrive from the
 * reader today, and moving them to the payload IS S1's deliverable. When S1
 * lands, this driver grows a payload leg and the gather assertions change with
 * it — stated in the plan as a known cost rather than discovered later.
 */
export async function run({ payload = "cards", n = 3, k = 2, selfSlot = 0, includeFp = false } = {}) {
  const t0 = performance.now();
  assertNoNFC("at entry");

  const proven = [];
  window.shSysw(payload);
  await tap(BACK);                       // Back == skip the boot payload offer
  await waitFor("SeedHammer");

  const hops = await goTo("Engrave Multisig");
  await tap(CONFIRM, 400);
  await waitFor(NEEDLE_FRONT_DOOR);
  proven.push(NEEDLE_FRONT_DOOR);

  // "Build policy" is choice 1; "Supply policy (md1)" is 0 and is the default,
  // so failing to move the selection lands in the WRONG flow with no complaint.
  await choose(1, 2, NEEDLE_TEMPLATE, "Build policy");
  proven.push(NEEDLE_TEMPLATE);

  // template -> n -> k(n) -> @S(n) -> fp, per buildParamPickFlow.
  await choose(0, 3, NEEDLE_N, "wsh (native segwit)");
  proven.push(NEEDLE_N);

  await choose(n - 2, 4, `Required signatures (k of ${n})?`, `n = ${n}`);
  await choose(k - 1, n, NEEDLE_SLOT, `k = ${k}`);
  proven.push(NEEDLE_SLOT);

  await choose(selfSlot, n, "Include key fingerprints?", `self slot @${selfSlot}`);
  await choose(includeFp ? 1 : 0, 2, GATHER_NOT_A_NEEDLE,
    includeFp ? "include fingerprints" : "omit fingerprints");

  const screen = window.shScreen();
  const presented = assertNoNFC("at the cosigner gather");

  return {
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    carouselHops: hops,
    params: { n, k, selfSlot, includeFp },
    // The needles that were actually observed, each single-site by
    // needle_test.go. This list is the proof of WHICH FLOW, and it is the
    // reason the screen below is reportable rather than misleading.
    needlesProven: proven,
    presented,
    screen,
    // Recorded so a reader can see for themselves that the gather text is
    // identical in the sibling flow, and therefore proves nothing alone.
    gatherTextIsNotEvidence: squash(screen).includes(squash(GATHER_NOT_A_NEEDLE)),
    ok: proven.length === 4 && presented === 0,
  };
}
