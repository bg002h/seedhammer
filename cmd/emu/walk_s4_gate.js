// S4's walk: DRIVE THE SEED<->KEY GATE, in both directions.
//
//   const w = await import("./walk_s4_gate.js");
//   w.run().then(r => { window.__w = r });              // the honest `both` case
//   w.run({ arm: "fail" }).then(r => { window.__f = r });// the loud failure
//
// NOT loaded by index.html, for walk_trace_a.js's reason.
//
// WHAT THIS STAGE'S GATE IS. SPEC 4.3, from the operator: "If both seed and key
// are present, sh2 must verify key can be derived from seed and if not, fail
// loudly." A `both` slot is the only shape that triggers it, and a `both` slot
// is in NEITHER trace, so the plan sends this walk to the cosigner payload (S0
// deliverable 2) for the case. Its provenance comment states the inventory and
// names both arms outright:
//
//   S4  the HONEST `both` case: card A@0's key genuinely derives from the
//       ClassMnemonic record at that card's declared origin, so the gate can be
//       walked green. The FAILING case needs no extra record -- assign B@0 or
//       C@0 to a `both` slot against master A and the gate must refuse.
//
// So ONE payload and ONE driver serve both arms, and the ONLY difference between
// them is which card the operator's own slot is filled from. That is what makes
// the fail arm a NEGATIVE CONTROL rather than a second walk: everything else is
// held constant, so a green happy path and a red failure cannot both be
// explained by the driver.
//
// THE ARMS, in the payload's own card order (A@0, A@1, B@0, C@0) at n=2, where
// two slots are open and the picker stops asking once the remaining cards equal
// the remaining slots:
//
//   pass:  USE  card 1 (A@0)          -> @0 = A@0, @1 = C@0
//          SKIP card 2 (A@1)             master A's seed DOES derive A@0's key
//          SKIP card 3 (B@0)             at m/48h/0h/0h/2h  -> PROCEED
//   fail:  SKIP card 1 (A@0)          -> @0 = B@0, @1 = C@0
//          SKIP card 2 (A@1)             master A's seed does NOT derive B@0's
//                                        key anywhere -> FAIL, naming @0
//
// WHY n=2 AND NOT 3. The failing arm has to put a FOREIGN card in the operator's
// own slot, and at n=3 the only way to reach that with this payload is to let
// A@1 land at @0 -- which S2's interim foreign-origin refusal catches first, at
// a different screen, for a different reason. At n=2 both arms' cards sit at the
// shared origin, so the ONLY thing that can separate them is the gate.
//
// WHAT IT DOES NOT DO: assert a hand-derived plate count inside `ok`. `plates`
// is reported as DATA (S0b review I-1). The census screen's own number IS
// asserted, but against the emulator's own plate stream rather than against a
// number written in this comment -- see gateCensus below.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// ChoiceScreen rows, measured in walk_build_policy.js and re-used verbatim.
export const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// Needles unique to ONE production flow. cmd/emu/needle_test.go pins the count
// and TestWalkNeedleLiteralsAreAllPinned pins that THESE literals are the pinned
// ones, so a walk cannot quietly anchor on an ambiguous string.
export const NEEDLE_FRONT_DOOR = "Supply or build a policy?"; // gui/multisig.go
export const NEEDLE_TEMPLATE = "Choose policy type"; // gui/multisig_build.go
export const NEEDLE_N = "How many keys (n)?"; // gui/multisig_build.go
export const NEEDLE_SLOT = "Which slot is your key?"; // gui/multisig_build.go
export const NEEDLE_PICK = "Payload cards"; // gui/multisig_build_payload.go
export const NEEDLE_PICK_CARD = "Use payload card"; // gui/multisig_build_payload.go
export const NEEDLE_GATHER = "Cosigner Keys"; // gui/multisig_build.go
// S4, and these four are what make this walk S4's rather than a re-run of S2's.
//
// The QUESTION that creates a `both` slot at all. It is drawn only when the
// payload can fill the operator's own slot too, so seeing it is itself evidence
// about the payload.
export const NEEDLE_SELF_SOURCE = "key on a card?"; // gui/multisig_build_slots.go
// The pre-assembly REVIEW of the assignment (SPEC 4.3 requires one).
export const NEEDLE_KEY_SOURCES = "Where each key comes from:"; // gui/multisig_build_slots.go
// THE GATE'S FAIL SCREEN. No honest build can draw this, and nothing else in the
// firmware draws it at all, so a walk that sees it drove the gate to a refusal.
export const NEEDLE_GATE_FAIL = "Key does not match seed"; // gui/multisig_build.go
// The plate census, before the tail starts.
export const NEEDLE_CENSUS = "Plate Count"; // gui/multisig_build.go

// NOT needles, and named here so a future author who reaches for one finds out
// why before trusting it.
//
//   "Scan a card, or Done"   the gather's BODY, drawn by the shared gatherer for
//                            every caller. The gather's TITLE is a needle; its
//                            body is not, and the difference is D-4's point.
//   "Verify the engraved plates?"  three production sites, so it is a
//                            post-condition for a tap and never evidence of
//                            which flow drew it.
const GATHER_BODY_NOT_A_NEEDLE = "Scan a card, or Done";
const VERIFY_OFFER = "Verify the engraved plates?";

async function waitFor(needle, timeoutMs = 15000) {
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

// gui.confirmDelay is ONE SECOND of wall clock and releasing early aborts the
// gesture. Measured in walk_trace_a.js.
const HOLD_MS = 1300;
const STALL_TICKS = 3;

const ENGRAVE_HANDLERS = [
  { name: "engrave-prompt", match: "Holdbuttontostarttheengravingprocess", act: "hold" },
  { name: "engrave-done", match: "Engravingcompletedsuccessfully", act: "confirm" },
  { name: "choose-variant", match: "Chooseengraving", act: "confirm" },
];

async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

/** Select row `i` of `n`, confirm, then REQUIRE `expect`. A wrong row picks a
 * different parameter and carries on happily, so the post-condition is the only
 * thing that makes a coordinate safe to compute. */
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

/** F-174: the cards must come from the PAYLOAD, and the SH2 has a working
 * reader, so this is load-bearing rather than redundant. */
export function assertNoNFC(where) {
  if (typeof window.shNFC.presented !== "function") {
    throw new Error("shNFC.presented is missing - this is a STALE emu.wasm. " +
      "The browser caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const n = window.shNFC.presented();
  if (n !== 0) {
    throw new Error(`${where}: ${n} record(s) crossed the NFC reader; a stage-gate run must ` +
      `present ZERO, because the cards must come from the payload`);
  }
  return n;
}

/**
 * Read the plate census screen and return what it CLAIMS, as a number.
 *
 * The claim is checked later against the plate stream the emulator actually
 * produced, never against a count written in this file. A census is a promise
 * about the tail; the only thing that can falsify it is the tail.
 */
function gateCensus(screen) {
  const m = /Thisengraves(\d+)plates?\./.exec(squash(screen));
  return m ? Number(m[1]) : -1;
}

/**
 * Drive the engrave tail until the VERIFY OFFER is on screen.
 *
 * TERMINATED BY AN OBSERVED SCREEN, not by a plate count (walk_s3_nested.js's
 * reason, and S0b review I-1's).
 */
async function runEngraveTail({ pollMs = 75, settleMs = 150 }) {
  const acts = [], digests = [];
  const NOWHERE = [5, 5];

  await tap(CONFIRM, 400);               // past the Policy Review

  await waitFor("Which md1?");
  await tap([240, rowY(0, 2)], 300);     // Full policy md1 (row 0), F-172
  await tap(CONFIRM, 400);

  await waitFor("EXPERIMENTAL");
  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);                  // > gui.confirmDelay, which is 1s
  window.shRelease(...CONFIRM);
  await waitFor("What to engrave?");
  await tap(CONFIRM, 400);               // Full (seed + keys), row 0

  // S4's PLATE CENSUS, before a single plate is cut. Read, then confirmed.
  const censusScreen = await waitFor(NEEDLE_CENSUS);
  const censusClaim = gateCensus(censusScreen);
  await tap(CONFIRM, 400);

  await waitFor("Chooseengraving");

  let stall = 0, lastSteps = -1, reachedVerifyOffer = false;
  for (let guard = 0; guard < 20000; guard++) {
    const steps = JSON.parse(window.shToolpath.summary()).steps;
    if (steps === lastSteps) stall++; else { stall = 0; lastSteps = steps; }
    if (stall < STALL_TICKS) { await sleep(pollMs); continue; }

    await tap(NOWHERE, settleMs);        // force a redraw before reading
    const screen = squash(window.shScreen());
    if (screen.includes(squash(VERIFY_OFFER))) { reachedVerifyOffer = true; break; }
    const h = ENGRAVE_HANDLERS.find((x) => screen.includes(x.match));
    if (!h) {
      // A screen the handler list does not recognise STOPS the walk. Tapping
      // past what it cannot name is how a driver manufactures a pass.
      acts.push({ act: "STALLED", screen: screen.slice(0, 120) });
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
  return {
    census: JSON.parse(window.shToolpath.strings()),
    censusScreen, censusClaim, digests, acts, reachedVerifyOffer,
  };
}

/**
 * S4's gate run.
 *
 * @param {object} [opts]
 * @param {string} [opts.arm="pass"]   "pass" drives the honest `both` case to a
 *        completed engrave; "fail" drives the SAME flow with a foreign card in
 *        the operator's own slot and requires the gate to refuse. The two differ
 *        in exactly one tap, which is what makes "fail" a negative control.
 * @param {string} [opts.payload="cards"]
 * @param {number} [opts.n=2]  n=2 keeps BOTH arms' cards at the shared origin,
 *        so S2's interim foreign-origin refusal cannot pre-empt the gate.
 * @param {number} [opts.k=2]
 * @param {number} [opts.selfSlot=0]
 */
export async function run({ arm = "pass", payload = "cards", n = 2, k = 2,
                            selfSlot = 0, pollMs = 75, settleMs = 150 } = {}) {
  const t0 = performance.now();
  if (arm !== "pass" && arm !== "fail") {
    throw new Error(`unknown arm ${JSON.stringify(arm)}; want "pass" or "fail"`);
  }
  assertNoNFC("at entry");
  if (typeof window.shToolpath !== "object") {
    throw new Error("shToolpath is missing - this is a STALE emu.wasm. The browser " +
      "caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }

  const proven = [];
  window.shSysw(payload);

  // THE PAYLOAD LEG. The boot offer is skipped on purpose: SyswReader() is
  // resolved ONCE by the caller of syswLoadFlow, and the boot offer is already on
  // screen before any script runs, so its reader was chosen before shSysw could
  // speak. Load from the `Load Payload` carousel entry instead.
  await tap(BACK);
  await waitFor("SeedHammer");
  await goTo("Load Payload");
  await tap(CONFIRM, 500);
  await waitFor("Payload Digest");
  await tap(CONFIRM, 400);               // [compared], route 2
  await waitFor("Payload Warnings");     // the blob carries a ClassMnemonic
  await tap(CONFIRM, 400);
  await waitFor("Keep this payload loaded?");
  await tap(CONFIRM, 400);               // KEEP
  await waitFor("Load Payload");

  const hops = await goTo("Engrave Multisig");
  await tap(CONFIRM, 400);
  await waitFor(NEEDLE_FRONT_DOOR);
  proven.push(NEEDLE_FRONT_DOOR);

  // "Build policy" is choice 1; "Supply policy (md1)" is 0 and is the default.
  await choose(1, 2, NEEDLE_TEMPLATE, "Build policy");
  proven.push(NEEDLE_TEMPLATE);

  // wsh (row 0). This walk's subject is the gate, not the template; S3 owns the
  // nested-segwit shape and asserts it on the restore doc.
  await choose(0, 3, NEEDLE_N, "wsh (native segwit)");
  proven.push(NEEDLE_N);

  await choose(n - 2, 4, `Required signatures (k of ${n})?`, `n = ${n}`);
  await choose(k - 1, n, NEEDLE_SLOT, `k = ${k}`);
  proven.push(NEEDLE_SLOT);

  await choose(selfSlot, n, "Include key fingerprints?", `self slot @${selfSlot}`);

  // S4's SLOT-SOURCE QUESTION. Reaching it is itself an assertion: it is drawn
  // only when the payload holds at least n cards, so a build that could not fill
  // the operator's own slot from the payload would never show it and this walk
  // would stop here instead of quietly walking a `derived` build.
  await choose(0, 2, NEEDLE_SELF_SOURCE, "omit fingerprints");
  proven.push(NEEDLE_SELF_SOURCE);
  const selfSourceScreen = window.shScreen();
  // "YES, CHECK THE CARD" is row 1. THIS IS THE TAP THE WHOLE STAGE IS ABOUT:
  // row 0 would build a `derived` slot and the gate would have nothing to check.
  await choose(1, 2, GATHER_BODY_NOT_A_NEEDLE, "YES, CHECK THE CARD");

  await waitFor(NEEDLE_GATHER);
  proven.push(NEEDLE_GATHER);
  const gatherScreen = window.shScreen();
  assertNoNFC("at the cosigner gather");
  const mkTally = /mk1keys:(\d+)/.exec(squash(gatherScreen));
  const cardsGathered = mkTally ? Number(mkTally[1]) : -1;
  if (cardsGathered <= 0) {
    throw new Error(`the gather holds ${cardsGathered} mk1 card(s) with nothing across ` +
      `the reader; the payload feed did not run. Screen: ${JSON.stringify(gatherScreen)}`);
  }
  await tap(CONFIRM, 500);               // Done adding cards

  // BOUNDED SELECTION. `both` opens n slots rather than n-1, so at n=2 the
  // payload's four cards still over-supply and the picker appears.
  const openSlots = n; // the `both` arm fills the operator's slot from a card too
  if (cardsGathered <= openSlots) {
    throw new Error(`the payload supplied ${cardsGathered} card(s) for ${openSlots} open ` +
      `slot(s), so no selection happens and neither arm can choose which card lands ` +
      `at @${selfSlot} -- the walk cannot distinguish its own arms`);
  }
  await waitFor(NEEDLE_PICK);
  proven.push(NEEDLE_PICK);
  await tap(CONFIRM, 400);
  await waitFor(NEEDLE_PICK_CARD);
  proven.push(NEEDLE_PICK_CARD);

  // THE ONE DIFFERENCE BETWEEN THE ARMS. Card 1 is A@0, the operator's own key.
  //   pass: USE it  -> @0 holds their key and the gate must PROCEED
  //   fail: SKIP it -> @0 holds B@0, another master's key, and the gate must REFUSE
  const picks = arm === "pass" ? ["use", "skip", "skip"] : ["skip", "skip"];
  const decisions = [];
  for (let i = 0; i < picks.length; i++) {
    const want = `Use payload card ${i + 1} of`;
    if (!squash(window.shScreen()).includes(squash(want))) break; // short-circuited
    await waitFor(want);
    if (picks[i] === "skip") {
      await tap([240, rowY(1, 2)], 300);
      decisions.push(`skip:${i + 1}`);
    } else {
      decisions.push(`use:${i + 1}`);
    }
    await tap(CONFIRM, 400);
  }

  // THE SELF SEED, from the payload (the cards blob carries master A as a
  // ClassMnemonic). Row 0 is FROM PAYLOAD and is the default; it is tapped
  // anyway, because a default that silently moves is how a walk ends up driving
  // the keyboard while reporting it drove the payload.
  const seedScreen = await waitFor("Where from?");
  await tap([240, rowY(0, 3)], 300);
  await tap(CONFIRM, 400);
  await waitFor("systemwide payload");
  await tap(CONFIRM, 400);
  const passphraseScreen = await waitFor("Add a BIP-39 passphrase?");
  await tap(CONFIRM, 500);               // Skip is row 0

  // ─── THE GATE ──────────────────────────────────────────────────────────────
  //
  // THREE outcomes are reachable from here and only one is this arm's. Racing
  // rather than waiting is what makes a wrong outcome a FAILURE instead of a
  // timeout that reads like a hang: the fail arm reaching the review, or the
  // pass arm reaching the refusal, is the finding.
  const outcome = await raceFor(
    [NEEDLE_GATE_FAIL, NEEDLE_KEY_SOURCES, "Duplicate key"], 20000);
  const gateScreen = window.shScreen();

  // ONE RETURN, at the end, for both arms. cmd/emu/needle_test.go's I-1 guard
  // reads the `ok:` expression textually and stops at the first object literal
  // closing at column 2, so a walk with an early return per arm hands it a span
  // covering BOTH -- which is how this file first tripped the guard on the word
  // "plates" belonging to the other arm entirely.
  let refusal = null, tail = null, reviewScreen = "", reviewSaysChecked = false;

  if (arm === "fail") {
    if (outcome !== NEEDLE_GATE_FAIL) {
      throw new Error(`the FAIL arm put another master's key in the operator's own ` +
        `slot and the gate did not refuse: it reached ${JSON.stringify(outcome)}. ` +
        `Screen: ${JSON.stringify(gateScreen)}`);
    }
    proven.push(NEEDLE_GATE_FAIL);
    // The refusal must NAME THE SLOT, must say nothing was engraved, and must
    // not make silencing it the obvious next move. All three are read off the
    // RENDERED FRAME, not off the source.
    const s = squash(gateScreen);
    refusal = {
      namesSlot: s.includes(`slot@${selfSlot}`),
      saysNothingEngraved: s.includes(squash("Nothing was engraved")),
      saysSuppresses: s.includes(squash("SUPPRESSES the check rather than fixing it")),
      namesHostRoute: s.includes(squash("me sysw pack")),
    };
  } else {
    if (outcome !== NEEDLE_KEY_SOURCES) {
      throw new Error(`the PASS arm put the operator's OWN card in their own slot and ` +
        `the flow did not reach the slot-source review: it reached ` +
        `${JSON.stringify(outcome)}. Screen: ${JSON.stringify(gateScreen)}`);
    }
    proven.push(NEEDLE_KEY_SOURCES);
    // The review must say the slot was CHECKED, not merely list it. A review
    // that rendered a `both` slot the same as a `payloadKey` one would leave the
    // operator confirming a cross-check nobody told them happened.
    reviewSaysChecked = squash(gateScreen).includes(squash("checked against"));
    await tap(CONFIRM, 400);
    reviewScreen = await waitFor("Policy stub");
    tail = await runEngraveTail({ pollMs, settleMs });
    if (!tail.reachedVerifyOffer) {
      throw new Error(`the engrave tail never reached the verify offer; acts ` +
        `${JSON.stringify(tail.acts)}`);
    }
  }

  // THE RECORDER, read the same way on both arms. On the fail arm this is the
  // assertion the whole arm exists for -- a refusal that still cut steel would
  // be worse than no gate -- and on the pass arm it is what the census promise
  // is checked against.
  const census = tail ? tail.census : JSON.parse(window.shToolpath.strings());
  // The census SCREEN said a number before the tail started; the recorder handed
  // back a count after it finished. Both terms are the emulator's.
  const censusHeld = tail ? tail.censusClaim === tail.census.strings.length
                          : census.strings.length === 0;

  return {
    arm,
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    carouselHops: hops,
    params: { n, k, selfSlot, picks },
    decisions,
    outcome,
    selfSourceScreen, gatherScreen, seedScreen, passphraseScreen,
    gateScreen, reviewScreen, reviewSaysChecked, refusal,
    censusScreen: tail ? tail.censusScreen : "",
    censusClaim: tail ? tail.censusClaim : -1,
    censusHeld,
    cardsGathered, openSlots,
    // DATA, not a verdict (S0b review I-1): a count a human wrote in a doc
    // comment is not evidence about what the machine cut.
    cutCount: census.strings.length,
    unattributed: census.unattributed,
    census,
    digests: tail ? tail.digests : [],
    acts: tail ? tail.acts : [],
    needlesProven: proven,
    presented: assertNoNFC("after the gate"),
    // `ok` NAMES ITS OUTCOME and every term is something the EMULATOR produced.
    //
    // fail: the refusal appeared, named the slot, said nothing was engraved,
    //       said reassigning SUPPRESSES rather than fixes, named the host route,
    //       and the recorder holds ZERO strings.
    // pass: nine needles observed on screen, a payload-fed set that had to be
    //       narrowed, the review saying the slot was checked, a completed tail,
    //       the screen's own count matching what the recorder returned, and
    //       nothing unattributed in the recorder's own attribution.
    // The needle terms NAME the decisive one rather than counting to it. Both
    // arms observe the same eight flow anchors and differ only in the ninth, so
    // a bare count would be satisfied by the WRONG arm's ninth needle -- which
    // is the one thing these two runs exist to tell apart.
    ok: arm === "fail"
      ? (proven.includes(NEEDLE_SELF_SOURCE) && proven.includes(NEEDLE_GATE_FAIL) &&
         !proven.includes(NEEDLE_KEY_SOURCES) &&
         refusal.namesSlot && refusal.saysNothingEngraved &&
         refusal.saysSuppresses && refusal.namesHostRoute && censusHeld &&
         census.strings.length === 0 && census.unattributed === 0)
      : (proven.includes(NEEDLE_SELF_SOURCE) && proven.includes(NEEDLE_KEY_SOURCES) &&
         !proven.includes(NEEDLE_GATE_FAIL) &&
         cardsGathered > openSlots && reviewSaysChecked &&
         tail.reachedVerifyOffer && censusHeld && census.unattributed === 0),
  };
}
