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
// THE PART THAT WAS NOT OBVIOUS, and the reason a stage author could not simply
// point the old script at a different carousel entry: BOTH flows ended at the
// SAME gather screen, drawn by the same shared gatherer. Measured 2026-08-14 by
// driving each and squashing shScreen():
//
//   via Build policy    EngraveBundlemd1descriptors:0mk1keys:0Scanacard,orDone.
//   via Engrave Bundle  EngraveBundlemd1descriptors:0mk1keys:0Scanacard,orDone.
//
// Character for character, because layoutTitle was called inside the shared
// gatherer with a hard-coded title. S2 FIXED THAT (D-4): the title is the
// caller's now, and the Build path passes its own, so the gather is finally
// anchorable — NEEDLE_GATHER below. The history is kept because the lesson is
// not: a screen drawn by shared code proves nothing about which flow you are in
// unless something on it came from the caller.
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
// S2/D-4. The cosigner gather's own title, passed by the Build flow into the
// shared gatherer. Single-site in gui/multisig_build.go per needle_test.go.
export const NEEDLE_GATHER = "Cosigner Keys"; // gui/multisig_build.go
// S4's three screens, none of which this driver answered until 2026-08-16 — see
// the three "SCREEN THIS DRIVER WALKED PAST" notes below.
export const NEEDLE_SELF_SOURCE = "key on a card?"; // gui/multisig_build_slots.go
export const NEEDLE_KEY_SOURCES = "Where each key comes from:"; // gui/multisig_build_slots.go
export const NEEDLE_GATE_FAIL = "Key does not match seed"; // gui/multisig_build.go
// The plate census, before the tail starts. The TITLE, not the body: "This
// engraves" also occurs in gui/bip85.go, which is exactly the ambiguity the
// pinned-needle list exists to catch.
export const NEEDLE_CENSUS = "Plate Count"; // gui/multisig_build.go

// The gather's BODY text, which is deliberately NOT a needle: it is drawn by the
// shared gatherer for every caller and says nothing about which flow drew it.
// Named so that a future author who reaches for it finds this comment instead.
// (The title above is a needle precisely because it is NOT shared.)
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

/**
 * Wait until ONE of several needles appears, and say which.
 *
 * A fork in the flow needs a fork in the driver. Waiting for one needle and
 * treating a timeout as "the other one happened" would report the wrong arm
 * confidently whenever the flow simply hung — which is the failure a walk exists
 * to catch.
 */
async function raceFor(needles, timeoutMs = 15000) {
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

// HOLD_MS is the one wait here that cannot be replaced by a condition:
// gui.confirmDelay is ONE SECOND of wall clock and releasing early aborts the
// gesture, so this is that second plus margin. Carried over from
// walk_trace_a.js, where it was measured.
const HOLD_MS = 1300;
// Ticks of no motion that mean "idle, not cutting" rather than "between
// batches".
const STALL_TICKS = 3;

// The screens the engrave tail knows how to answer. Anything else STOPS the
// walk — a driver that taps past what it does not recognise manufactures a pass.
const ENGRAVE_HANDLERS = [
  { name: "engrave-prompt", match: "Holdbuttontostarttheengravingprocess", act: "hold" },
  { name: "engrave-done", match: "Engravingcompletedsuccessfully", act: "confirm" },
  { name: "choose-variant", match: "Chooseengraving", act: "confirm" },
];

/**
 * Drive the engrave tail from the policy review to a completed bundle.
 *
 * The machinery is walk_trace_a.js's, and it is reused rather than rewritten
 * because every constant in it was measured there: progress comes from
 * shToolpath and never from the screen (the display lags during a cut), the
 * hold is wall-clock because gui.confirmDelay is, and a screen the handler list
 * does not recognise stops the walk instead of being tapped past.
 *
 * `plates` here is a LOOP BOUND: how long to keep watching the toolpath before
 * giving up. It is deliberately not an assertion — F-170 is the rule that a walk
 * asserting what it produced asserts nothing, and I-1 is the finding that
 * asserting what its CALLER said it would produce is no better.
 */
async function runEngraveTail({ plates, pollMs = 75, settleMs = 150 }) {
  const acts = [], digests = [];
  const NOWHERE = [5, 5];
  await tap(CONFIRM, 400);               // past the Policy Review
  await waitFor("Which md1?");
  await tap(CONFIRM, 400);               // Full policy md1 (row 0)
  await waitFor("EXPERIMENTAL");
  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);                  // > gui.confirmDelay, which is 1s
  window.shRelease(...CONFIRM);
  await waitFor("What to engrave?");
  await tap(CONFIRM, 400);               // Full (seed + keys), row 0

  // S4's PLATE CENSUS, the FOURTH screen this driver walked past. It is
  // unconditional (confirmReviewScreen "Plate Count", gui/multisig_build.go step
  // 9a) and sits between the mode choice and the first plate, so waiting for
  // "Choose engraving" could only time out:
  //
  //   waitFor("Chooseengraving") timed out; screen reads "PlateCountThis
  //   engraves9plates.ms1secretshare:1plate..."
  //
  // Read, then confirmed. The screen's own claim is compared against the plate
  // stream the emulator produced, never against a count written in this file.
  const censusScreen = await waitFor(NEEDLE_CENSUS);
  const censusClaim = (() => {
    const m = /Thisengraves(\d+)plates?\./.exec(squash(censusScreen));
    return m ? Number(m[1]) : -1;
  })();
  await tap(CONFIRM, 400);
  await waitFor("Chooseengraving");

  let stall = 0, lastSteps = -1;
  for (let guard = 0; guard < 20000; guard++) {
    const c = JSON.parse(window.shToolpath.strings());
    if (c.strings.length >= plates) break;
    const steps = JSON.parse(window.shToolpath.summary()).steps;
    if (steps === lastSteps) stall++; else { stall = 0; lastSteps = steps; }
    if (stall < STALL_TICKS) { await sleep(pollMs); continue; }

    await tap(NOWHERE, settleMs);        // force a redraw before reading
    const screen = squash(window.shScreen());
    const h = ENGRAVE_HANDLERS.find((h) => screen.includes(h.match));
    if (!h) {
      acts.push({ act: "STALLED", screen: screen.slice(0, 90) });
      break;
    }
    if (h.name === "engrave-done") {
      digests.push(JSON.parse(window.shToolpath.summary()).digest);
    }
    acts.push({ act: h.act, screen: h.name });
    if (h.act === "hold") {
      window.shPress(...CONFIRM);
      await sleep(HOLD_MS);
      window.shRelease(...CONFIRM);
      const before = JSON.parse(window.shToolpath.summary()).steps;
      for (let i = 0; i < 100; i++) {
        if (JSON.parse(window.shToolpath.summary()).steps !== before) break;
        await sleep(pollMs);
      }
    } else {
      window.shTap(...CONFIRM);
      for (let i = 0; i < 100; i++) {
        window.shTap(...NOWHERE);
        await sleep(pollMs);
        if (!squash(window.shScreen()).includes(h.match)) break;
      }
    }
    stall = 0; lastSteps = -1;
  }
  return { census: JSON.parse(window.shToolpath.strings()), censusScreen, censusClaim, digests, acts };
}

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
 * the payload-supplied-cards feature exists. So a stage gate asserts the cards
 * did NOT come from a tag.
 *
 * The SH2 HAS a working reader (soldered ST25R3916); phase 1 takes its cards
 * from the payload and the keyboard by SCOPE, not by hardware absence. Because
 * the reader works, this assertion is what makes payload-sourcing provable.
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
      `present ZERO — the cards must come from the payload, which is this phase's ` +
      `primary data entry alongside the keyboard (the reader exists and works, ` +
      `which is why this is asserted rather than assumed)`);
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
 * @param {string[]} [opts.picks]        per-card decisions, "use" or "skip", in
 *          payload order. Defaults to `use` copies of "use", which is what every
 *          pre-S2 caller meant. THE PICKER STOPS ASKING once the remaining cards
 *          equal the remaining slots (gui/multisig_build_payload.go:317-321), so
 *          this list is applied only while the picker is actually on screen.
 * @param {string}  [opts.seedFrom=null]  where the SELF seed comes from.
 *          null stops at the held slot's seed screen ("Seed for @S"), which is
 *          S1's gate and this walk's original end. "payload" takes the
 *          ClassMnemonic the cards blob carries (master A) with confirm-taps
 *          only.
 * @param {boolean} [opts.engrave=false] drive past the review to a completed
 *          engrave and return the toolpath census.
 * @param {number}  [opts.plates=4]      runEngraveTail's LOOP BOUND — when the
 *          driver stops watching the toolpath. It appears in no term of `ok`
 *          (I-1): a caller-supplied count cannot stand in for content, and this
 *          one was hand-derived in prose, which makes it worse. What the engraved
 *          strings should BE is settled by the derived expectation committed
 *          beside the gate record, never here. See the note on `ok` below.
 *          FOUR SINCE F-423, and it was nine: a full 2-of-3 build gathers
 *          ms1(1) + mk1(2) + md1(6) = 9 STRINGS, which bundlePlatePlan now packs
 *          within each card onto 1 + 1 + 2 plates.
 * @param {string}  [opts.expect="engrave"]  "engrave" or "duplicate". The
 *          refusal arm is a first-class outcome, not a failure.
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
export async function run({ payload = "cards", n = 3, k = 2, selfSlot = 0, includeFp = false,
                            use = 2, picks = null, seedFrom = null, engrave = false,
                            plates = 4, expect = "engrave",
                            pollMs = 75, settleMs = 150 } = {}) {
  const t0 = performance.now();
  assertNoNFC("at entry");
  if (engrave && typeof window.shToolpath !== "object") {
    throw new Error("shToolpath is missing — this is a STALE emu.wasm. The browser " +
      "caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }

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

  // S5's @S picker is MULTI-SELECT: the first screen picks one slot, then a
  // second screen asks whether another is held. This walk holds exactly one, so
  // it takes row 0, "NO, THAT IS ALL", and the held set stays {@selfSlot} --
  // identical to what this driver produced before the picker widened.
  // (The lead is deliberately not a NEEDLE_ constant: it has never been counted
  // for production sites, and cmd/emu/needle_test.go binds only declared
  // needles. Anchoring on it as one would need a pin there first.)
  await choose(selfSlot, n, "Do you hold another slot?", `self slot @${selfSlot}`);
  await choose(0, 2, "Include key fingerprints?", "no further held slots");

  // ─── THE S4 SCREEN THIS DRIVER WALKED PAST, and the reason it was never seen ─
  //
  // FOUND 2026-08-16 BY RUNNING THIS FILE, which is the only way it could have
  // been found. S4 added buildSelfSourceFlow between the fingerprint picker and
  // the gather; it is drawn whenever the payload can supply n cards
  // (gui/multisig_build.go), and the delivered payload carries FOUR against this
  // driver's default n=3. So from S4 onward this walk timed out here:
  //
  //   choosing omit fingerprints (row 0 of 2) did not land on "Scan a card, or
  //   Done": screen reads "Isyour@0keyonacard?NO,JUSTMYSEEDYES,CHECKTHECARD..."
  //
  // walk_s4_gate.js answers the screen because S4 wrote it; this file and
  // walk_s3_nested.js were both EDITED after S4 (they gained the multi-select
  // taps above) and neither was RUN, so nothing noticed. CI does not run walks —
  // it runs `GOOS=js go vet` and the static needle checks — which is exactly the
  // blind spot the plan's §5 names.
  //
  // IT IS RACED, NOT ASSUMED. The question exists only on over-supply, so a
  // caller at n=5 against this four-card payload legitimately never sees it.
  // Waiting for it unconditionally would swap one hang for another — and racing
  // is also what turns "the fp choice landed somewhere unexpected" into a named
  // failure rather than a timeout, which is choose()'s post-condition kept.
  await tap([240, rowY(includeFp ? 1 : 0, 2)], 300);   // the fp choice itself
  await tap(CONFIRM, 400);
  const afterFp = await raceFor([NEEDLE_SELF_SOURCE, GATHER_NOT_A_NEEDLE], 15000);
  const selfSourceAsked = afterFp === NEEDLE_SELF_SOURCE;
  if (selfSourceAsked) {
    proven.push(NEEDLE_SELF_SOURCE);
    // "NO, JUST MY SEED" is row 0: the operator's slot is DERIVED from their
    // seed, which is what this driver has always built. Row 1 is S4's `both`
    // slot and belongs to walk_s4_gate.js.
    await choose(0, 2, GATHER_NOT_A_NEEDLE, "NO, JUST MY SEED");
  }

  // THE GATHER, now fed entirely by the payload. The tally is read rather than
  // asserted against a hard-coded number: what matters to S1's gate is that the
  // count is the payload's, not the reader's, and `presented === 0` is what says
  // so. The count itself is REPORTED so a record can show it.
  //
  // Since D-4 the screen carries a title only this flow passes, so the gather is
  // ASSERTED to be the Build path's rather than assumed from the tap sequence.
  await waitFor(NEEDLE_GATHER);
  proven.push(NEEDLE_GATHER);
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
  const decisions = [];
  if (cardsGathered > openSlots) {
    await waitFor(NEEDLE_PICK);
    proven.push(NEEDLE_PICK);
    selected = true;
    await tap(CONFIRM, 400);             // past the read-only list of what arrived
    await waitFor(NEEDLE_PICK_CARD);
    proven.push(NEEDLE_PICK_CARD);
    // PER-CARD DECISIONS, and the loop is driven by what is ON SCREEN rather
    // than by a count. The picker STOPS ASKING once the cards that remain equal
    // the slots that remain (gui/multisig_build_payload.go:317-321), so a loop
    // that counted its own taps would carry on tapping CONFIRM into whatever
    // screen came next. That short-circuit is exactly what the clean arm relies
    // on: SKIP, SKIP and the last two cards are taken without a third question.
    const plan = picks || Array(use).fill("use");
    for (let i = 0; i < plan.length; i++) {
      const want = `Use payload card ${i + 1} of`;
      if (!squash(window.shScreen()).includes(squash(want))) break; // short-circuited
      // The per-card POST-CONDITION, for choose()'s reason: "a wrong row does
      // NOT fail loudly on its own".
      await waitFor(want);
      // "USE THIS CARD" is row 0 and the default; "SKIP IT" is row 1.
      if (plan[i] === "skip") {
        await tap([240, rowY(1, 2)], 300);
        decisions.push(`skip:${i + 1}`);
      } else {
        decisions.push(`use:${i + 1}`);
      }
      await tap(CONFIRM, 400);
    }
  }

  // THE SEED ENTRY is the first screen PAST the cosigner set, so reaching it is
  // the proof that the set resolved. With `seedFrom` unset the walk STOPS here,
  // which is S1's gate (plan §3 preamble, F-175) and what this driver did before
  // S2.
  //
  // THE TITLE IS THE SLOT'S, NOT "Input Seed" — THE SECOND THING RUNNING THIS
  // FILE FOUND (2026-08-16). `seedEntryTitle = "Input Seed"` is what every seed
  // entry OUTSIDE the multisig build path uses (gui/derive_xpub.go); the build
  // path asks per HELD SLOT and titles each screen for its slot,
  // `seedEntryFlowTitled(ctx, th, "Seed for "+label, label)`
  // (gui/multisig_build.go's buildSeedForSlot) — because with several seeds in
  // one flow an unlabelled second prompt is indistinguishable from a repeat of
  // the first. This driver had been waiting for the wrong title since that
  // landed, and nothing noticed because nothing ran it:
  //
  //   waitFor("Input Seed") timed out; screen reads
  //   "Wherefrom?FROMPAYLOADTYPEITSCANSeedfor@0"
  //
  // Anchoring on the slot title is also strictly stronger: it proves the flow
  // reached THIS slot's entry rather than any seed screen in the firmware.
  const screen = await waitFor(`Seed for @${selfSlot}`);
  const presentedAtEnd = assertNoNFC("after the cosigner set resolved");

  // ─── S2: past the seed, to a refusal or to a completed engrave ─────────────
  //
  // TAP-ONLY, AND THAT IS THE POINT. S2's first attempt at this leg assumed the
  // self seed had to be TYPED, and stopped on the cost of driving the on-device
  // keyboard by coordinate (F-181). The assumption was false: the cards payload
  // carries a ClassMnemonic — master A, `cmd/emu/sysw_cards_payload.go` — so
  // "FROM PAYLOAD" is row 0 of the source picker and the whole leg is confirms.
  // The plan's own wording said so all along: "default taps + PAYLOAD SEED".
  //
  // It is also the only route by which the self-seed-from-payload path gets
  // exercised at all: every Go test on this flow seeds mk1 chunks only, so
  // syswSeedPicker is "a menu of one and is skipped" and this screen never
  // appears. That is one of §0.1b's two ruled primary data entries.
  let refusal = null, census = null, digests = [], acts = [], reviewScreen = null,
    keySourcesScreen = null, censusScreen = null, censusClaim = -1;
  if (seedFrom === "payload") {
    // "FROM PAYLOAD" is row 0 of three (FROM PAYLOAD / TYPE IT / SCAN) and is
    // the default, so CONFIRM alone takes it — but the ROW IS TAPPED anyway,
    // because a default that silently moves is how a walk ends up driving the
    // keyboard while reporting it drove the payload.
    await tap([240, rowY(0, 3)], 300);
    await tap(CONFIRM, 400);
    // The acceptance screen syswSourceAccept draws for a payload-sourced secret.
    await waitFor("systemwide payload");
    await tap(CONFIRM, 400);
    await waitFor("Add a BIP-39 passphrase?");
    await tap(CONFIRM, 500);            // Skip is row 0

    // ─── S4's KEY-SOURCES REVIEW, THE THIRD SCREEN THIS DRIVER WALKED PAST ───
    //
    // FOUND 2026-08-16 BY RUNNING THIS FILE, after the two above were fixed and
    // it got one screen further each time. S4 added an UNCONDITIONAL review of
    // the slot assignment between the gate and assembly
    // (buildSlotSourceReviewFlow, gui/multisig_build.go step 4a), so racing
    // straight for the policy stub could only ever time out:
    //
    //   none of ["Duplicate key","Policy stub"] appeared within 15000ms; screen
    //   reads "KeysourcesWhereeachkeycomesfrom:@0yours:derivedfromyourseed..."
    //
    // Three screens, one cause: this file was edited by S5 and last RUN before
    // S4. Racing on the gate's refusal too, so a `both`-slot failure arriving
    // here is named rather than reported as a hang.
    const beforeAssembly = await raceFor(
      [NEEDLE_KEY_SOURCES, NEEDLE_GATE_FAIL, "Duplicate key"], 20000);
    if (beforeAssembly !== NEEDLE_KEY_SOURCES) {
      throw new Error(`the build did not reach the slot-source review: it reached ` +
        `${JSON.stringify(beforeAssembly)}. Screen: ${JSON.stringify(window.shScreen())}`);
    }
    proven.push(NEEDLE_KEY_SOURCES);
    keySourcesScreen = window.shScreen();
    await tap(CONFIRM, 400);

    // THE FORK. Both outcomes are legitimate results of THIS payload: the
    // default taps take cards A@0 and A@1, and A@0 is master A's own account-0
    // key, so the assembled set repeats the self key. SKIP, SKIP reaches Trace
    // A's B@0 + C@0 instead.
    const dupSeen = await raceFor(["Duplicate key", "Policy stub"], 15000);
    if (dupSeen === "Duplicate key") {
      refusal = window.shScreen();
      if (expect !== "duplicate") {
        throw new Error(`the build was refused as a duplicate, but this run expected ` +
          `${JSON.stringify(expect)}. Screen: ${JSON.stringify(refusal)}`);
      }
    } else {
      reviewScreen = window.shScreen();
      if (expect !== "engrave") {
        throw new Error(`the build reached the policy review, but this run expected a ` +
          `${JSON.stringify(expect)} refusal. Screen: ${JSON.stringify(reviewScreen)}`);
      }
      // §0.1a: the origin announcement must be ON the confirmation surface.
      for (const want of ["m/48h/0h/0h/2h", "BIP-48"]) {
        if (!squash(reviewScreen).includes(squash(want))) {
          throw new Error(`the Policy Review reached the display without §0.1a's ` +
            `origin announcement: no ${JSON.stringify(want)} in ` +
            `${JSON.stringify(reviewScreen)}`);
        }
      }
      if (engrave) {
        const r = await runEngraveTail({ plates, pollMs, settleMs });
        census = r.census; digests = r.digests; acts = r.acts;
        censusScreen = r.censusScreen; censusClaim = r.censusClaim;
      }
    }
  }

  return {
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    carouselHops: hops,
    params: { n, k, selfSlot, includeFp, use, picks, seedFrom, engrave, expect, plates },
    decisions,
    refusal,
    reviewScreen,
    census,
    digests,
    acts,
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
    // OBSERVED, not configured: whether S4's slot-source question was drawn at
    // all. It is a property of the payload against n, so it is reported rather
    // than assumed, and `ok` below counts needles accordingly.
    selfSourceAsked,
    keySourcesScreen,
    // The census SCREEN's own promise, and whether the recorder kept it. Both
    // terms are the emulator's: one is what it said it would cut, the other is
    // what it handed back after cutting.
    censusScreen,
    censusClaim,
    censusHeld: census === null ? null : censusClaim === census.strings.length,
    // DATA, not a verdict (I-1). Reported so a reader can see the count; nothing
    // here compares it to anything.
    plateCount: census === null ? null : census.strings.length,
    // THE NEEDLE TERMS ARE NAMED, NOT COUNTED, since 2026-08-16. `proven.length
    // === 7` was the shape before, and it stopped working the moment the walk
    // had legs of different lengths: S4's slot-source question is drawn only on
    // over-supply, and the key-sources review is only reached when a seed was
    // entered at all. A count then needs a term per leg, which is arithmetic
    // standing in for the fact anybody cares about — WHICH needles were seen.
    // Naming them keeps the protection (a needle silently dropped fails here)
    // and says which one went missing.
    //
    // `ok` NAMES ITS OUTCOME, and CONTAINS ONLY TERMS THE EMULATOR WAS OBSERVED
    // TO PRODUCE (I-1). A run that expected the duplicate refusal is green when
    // it got one; a run that expected an engrave is green only when something
    // was actually cut and everything cut was attributable.
    //
    // The term `census.strings.length === plates` USED TO BE HERE, and `plates`
    // is supplied by the caller — the one term in this expression standing in
    // for CONTENT that the walk never observed. Its default of 9 was worse than
    // caller-supplied: it was hand-derived in a doc comment ("1 ms1 + 2 mk1
    // chunks + 6 md1 chunks"), which is precisely the shape this project's rules
    // forbid. A run of `w.run({ engrave: true, plates: 9 })` that cut nine WRONG
    // strings returned ok: true.
    //
    // A walk cannot derive, so it no longer claims to. What the strings SHOULD
    // be is settled by the expectation committed beside the gate record
    // (oracle/gaterecords/*.expect.json), derived from the primary toolchain,
    // which cmd/gaterecord refuses to mint a record without and which
    // oracle.TestEveryGateRecordCensusMatchesItsCommittedExpectation compares on
    // every machine with no toolchain and no skip path. `plates` survives only
    // as runEngraveTail's loop bound — when to stop watching, never whether it
    // was right.
    ok: [NEEDLE_FRONT_DOOR, NEEDLE_TEMPLATE, NEEDLE_N, NEEDLE_SLOT,
      NEEDLE_PICK, NEEDLE_PICK_CARD, NEEDLE_GATHER].every((x) => proven.includes(x)) &&
      (!selfSourceAsked || proven.includes(NEEDLE_SELF_SOURCE)) &&
      (seedFrom === null || proven.includes(NEEDLE_KEY_SOURCES)) &&
      !proven.includes(NEEDLE_GATE_FAIL) &&
      presentedAtEnd === 0 && cardsGathered > 0 && selected &&
      (seedFrom === null
        || (expect === "duplicate" && refusal !== null)
        || (expect === "engrave" && !engrave && reviewScreen !== null)
        || (expect === "engrave" && engrave && census !== null
            && census.strings.length > 0 && census.unattributed === 0)),
  };
}
