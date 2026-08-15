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
// S1. Single-site in gui/multisig_build_payload.go, and reachable ONLY when the
// payload supplied more cosigner cards than the policy has open slots — so
// seeing it is proof of a payload-fed set that had to be narrowed, which is the
// `0..n` ruling on screen rather than in a comment.
export const NEEDLE_PICK = "Payload cards"; // gui/multisig_build_payload.go
export const NEEDLE_PICK_CARD = "Use payload card"; // gui/multisig_build_payload.go

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
 * @param {number}  [opts.use=2]            how many payload cards to accept in
 *          the bounded-selection picker. Only consulted on over-supply.
 * @returns {Promise<object>} the needles proven, the gather screen, and the
 *          NFC count — everything a stage gate asserts on.
 *
 * THE PAYLOAD LEG IS S1'S (this header used to say it was NOT done here). S0b
 * proved the flow was reachable and identifiable while the cards still arrived
 * from the reader; moving them to the payload is S1's deliverable, so the driver
 * now boots the payload, earns [compared] through route 2 (the operator digest
 * comparison, gui/sysw_load.go), and lets the Build path feed every ClassMDMK
 * record to the gather. `assertNoNFC` stays on every stage-gate run and is what
 * makes that non-circular: four cards in the tally with ZERO records across the
 * reader can only have come from the payload.
 */
export async function run({ payload = "cards", n = 3, k = 2, selfSlot = 0, includeFp = false, use = 2 } = {}) {
  const t0 = performance.now();
  assertNoNFC("at entry");

  const proven = [];
  window.shSysw(payload);

  // THE PAYLOAD LEG, and the BOOT OFFER IS SKIPPED ON PURPOSE.
  //
  // Measured, not reasoned about: `SyswReader()` is resolved ONCE, by the caller
  // of syswLoadFlow (gui/gui.go:1796 at boot, gui/sysw_unload.go:36,52 from the
  // carousel). The boot offer is already on screen before any script runs, so
  // its reader was chosen before `shSysw` could speak — a walk that loaded there
  // would silently load the RECORDS blob and then refuse at the Build path for
  // want of cosigner cards. That is what the first run of this leg did, and the
  // refusal was correct: the machine really did hold no cards.
  //
  // So: Back past the boot offer, then load from the `Load Payload` carousel
  // entry, whose SyswReader() call happens after shSysw has been set.
  await tap(BACK);                       // Back == skip the boot payload offer
  await waitFor("SeedHammer");
  await goTo("Load Payload");
  await tap(CONFIRM, 500);               // enter Load Payload -> reads the region

  // The digest screen is [compared] route 2 (gui/sysw_load.go), and confirming
  // it is what authorises `takeAll` to hand the cards to a program at all.
  // Declining would UNLOAD, and the Build path would then refuse — correctly —
  // for want of cards.
  await waitFor("Payload Digest");
  await tap(CONFIRM, 400);               // "yes, it matches" -> compared

  // THE WARNINGS LEG, also found by RUNNING this. The cards payload carries a
  // ClassMnemonic (masterA, for S4's `both` slot), so §3.3.3's F1 flag fires —
  // "A SECRET is stored unencrypted in flash" — and it is followed by a
  // KEEP/UNLOAD choice. UNLOAD is choice 1; taking it would drop the session and
  // the Build path would refuse for want of cards, which is correct behaviour
  // and a useless walk. KEEP is choice 0 and the default, so CONFIRM alone is
  // right — but the screen must be WAITED for, because tapping blind through it
  // lands the tap on whatever came next.
  await waitFor("Payload Warnings");
  await tap(CONFIRM, 400);               // acknowledge the flags
  await waitFor("Keep this payload loaded?");
  await tap(CONFIRM, 400);               // KEEP
  await waitFor("Load Payload");          // back at the carousel entry

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

  // THE GATHER, now fed entirely by the payload. The tally is read rather than
  // asserted against a hard-coded number: what matters to S1's gate is that the
  // count is the payload's, not the reader's, and `presented === 0` is what says
  // so. The count itself is REPORTED so a record can show it.
  const gatherScreen = window.shScreen();
  const presented = assertNoNFC("at the cosigner gather");
  const mkTally = /mk1keys:(\d+)/.exec(squash(gatherScreen));
  const cardsGathered = mkTally ? Number(mkTally[1]) : -1;
  if (cardsGathered <= 0) {
    throw new Error(`the gather holds ${cardsGathered} mk1 card(s) with nothing ` +
      `across the reader; the payload feed did not run. Screen: ${JSON.stringify(gatherScreen)}`);
  }

  await tap(CONFIRM, 500);               // Done adding cards

  // BOUNDED SELECTION. It appears only on over-supply — the delivered payload
  // carries four cards and Trace A is a 2-of-3, which is exactly the state the
  // `0..n` ruling exists for. On an equal count the flow auto-fills and this
  // whole leg is skipped, so the needle's PRESENCE is the evidence, and it is
  // reported rather than assumed.
  const openSlots = n - 1;
  let selected = false;
  if (cardsGathered > openSlots) {
    await waitFor(NEEDLE_PICK);
    proven.push(NEEDLE_PICK);
    selected = true;
    await tap(CONFIRM, 400);             // past the read-only list of what arrived
    await waitFor(NEEDLE_PICK_CARD);
    proven.push(NEEDLE_PICK_CARD);
    for (let taken = 0; taken < use; taken++) {
      // The per-card POST-CONDITION, for choose()'s reason: "a wrong row does
      // NOT fail loudly on its own". Without it, a card auto-taken by the
      // remaining-equals-needed short-circuit would leave this loop tapping
      // CONFIRM on whatever screen came next.
      await waitFor(`Use payload card ${taken + 1} of`);
      // "USE THIS CARD" is row 0 and the default, so CONFIRM alone takes it.
      await tap(CONFIRM, 400);
    }
  }

  // S1 ENDS AT A SCREEN, NOT AN ENGRAVE (plan §3 preamble, F-175). The seed
  // entry is the first screen PAST the cosigner set, so reaching it is the
  // proof that the set resolved — and it is where this walk stops.
  const screen = await waitFor("Input Seed");
  const presentedAtEnd = assertNoNFC("after the cosigner set resolved");

  return {
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    carouselHops: hops,
    params: { n, k, selfSlot, includeFp, use },
    // The needles that were actually observed, each single-site by
    // needle_test.go. This list is the proof of WHICH FLOW, and it is the
    // reason the screens below are reportable rather than misleading.
    needlesProven: proven,
    presented: presentedAtEnd,
    cardsGathered,
    openSlots,
    selected,
    gatherScreen,
    screen,
    // Recorded so a reader can see for themselves that the gather text is
    // identical in the sibling flow, and therefore proves nothing alone.
    gatherTextIsNotEvidence: squash(gatherScreen).includes(squash(GATHER_NOT_A_NEEDLE)),
    ok: proven.length === 6 && presentedAtEnd === 0 && cardsGathered > 0 && selected,
  };
}
