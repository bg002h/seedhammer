// S3's walk: build a NESTED-SEGWIT sh(wsh) policy and read the restore doc.
//
//   const w = await import("./walk_s3_nested.js");
//   w.run().then(r => { window.__w = r });      // minutes: it engraves
//
// NOT loaded by index.html, for walk_trace_a.js's reason.
//
// WHY THIS IS ITS OWN FILE. Each stage writes its own walk (plan S0b: "S0b does
// NOT own the five per-stage walk scripts"), and S3's differs from
// walk_build_policy.js in three ways that are the whole point of the stage:
//
//   1. it picks TEMPLATE ROW 1, sh(wsh) (nested segwit), instead of row 0;
//   2. it picks "Full policy md1" at the wallet-policy form, and says why;
//   3. it drives PAST the engrave and the verify offer to the RESTORE DOC, which
//      is the surface SPEC 4.4 calls "the one that matters most" and which no
//      walk had ever reached.
//
// WHAT (2) IS ABOUT (F-172, owned by S3). gui/multisig_build.go guards the
// restore-doc call with `if !template`, so the TEMPLATE-ONLY form skips the
// restore doc entirely. A walk that took that branch would run to the end of the
// flow with this gate's subject never drawn, and would report exactly what a
// broken screen reports: the name was not there. The choice is therefore made
// explicitly below rather than inherited from a default.
//
// HOW IT KNOWS IT REALLY BUILT sh(wsh), which is the assertion a template picker
// cannot make for itself: a tap on row 1 is indistinguishable from a tap on row 0
// unless something downstream SAYS which one landed. The policy review screen
// does, because 0.1a made the origin announcement template-dependent, and the
// nested-segwit sentence exists in exactly one production site. So the walk
// asserts the NOTE, not the tap.
//
// WHAT IT DOES NOT DO: assert a hand-derived plate count. `plates` is reported as
// DATA. A count a human wrote in a doc comment is not evidence about what the
// machine cut, and putting it inside `ok` lets a caller pass its own expectation
// in and get it back as a verdict (S0b review I-1). The engrave tail here is
// terminated by an OBSERVED screen, the verify offer, not by reaching a number.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// ChoiceScreen rows, measured in walk_build_policy.js and re-used verbatim.
// Rows are 24px apart and the stack is centred on y=160.
export const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// Needles unique to ONE production flow. cmd/emu/needle_test.go pins the count,
// and walk_needle_binding_test.go pins that THESE literals are the pinned ones —
// before that seam existed, a walk could edit a needle to an ambiguous string and
// leave every Go test green (S0b review I-2).
export const NEEDLE_FRONT_DOOR = "Supply or build a policy?"; // gui/multisig.go
export const NEEDLE_TEMPLATE = "Choose policy type"; // gui/multisig_build.go
export const NEEDLE_N = "How many keys (n)?"; // gui/multisig_build.go
export const NEEDLE_SLOT = "Which slot is your key?"; // gui/multisig_build.go
export const NEEDLE_PICK = "Payload cards"; // gui/multisig_build_payload.go
export const NEEDLE_PICK_CARD = "Use payload card"; // gui/multisig_build_payload.go
export const NEEDLE_GATHER = "Cosigner Keys"; // gui/multisig_build.go
// S3, and the two that make this walk S3's rather than a re-run of S2's.
//
// The NOTE proves the sh(wsh) TEMPLATE took: buildOriginAnnouncement emits it
// only for md.MultisigShWsh, so it cannot appear on a wsh or a legacy-sh build.
export const NEEDLE_NESTED_NOTE = "BIP-48 for nested segwit (script type 1h)"; // gui/multisig_build.go
// The NAME is the gate's subject: scriptName's nested-segwit arm, one production
// site in gui/md1_inspect.go, reaching the screen through desc4Display.
export const NEEDLE_NESTED_NAME = "P2SH-P2WSH"; // gui/md1_inspect.go

// NOT needles, and named here so a future author who reaches for one finds out
// why before trusting it.
//
//   "Restore Doc"   the screen TITLE. TWO production sites (the shared
//                   restoreDocScreen in gui/singlesig_restore.go and an error
//                   title in gui/multisig_restore.go), so it identifies a screen
//                   and never a flow. Used below only to corroborate a frame the
//                   needles already located.
//   "First receive:" likewise two sites, both restore docs.
//   "Scan a card, or Done"  the gather's BODY, drawn by the shared gatherer for
//                   every caller. The gather's TITLE is a needle; its body is
//                   not, and the difference is D-4's whole point.
const RESTORE_TITLE_NOT_A_NEEDLE = "Restore Doc";
const GATHER_BODY_NOT_A_NEEDLE = "Scan a card, or Done";
// Three production sites (singlesig, supply-multisig, build-multisig), so this is
// a post-condition for a tap and never evidence of which flow drew it.
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
    throw new Error("shNFC.presented is missing — this is a STALE emu.wasm. " +
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
 * Drive the engrave tail until the VERIFY OFFER is on screen.
 *
 * TERMINATED BY AN OBSERVED SCREEN, not by a plate count. walk_build_policy.js
 * loops until `census.strings.length >= plates`, where `plates` is a number a
 * human wrote in a doc comment; S0b's review found that the same number is then
 * the only driver-supplied term inside `ok`, so a caller could pass its own
 * expectation in and get it back as a verdict. Here the count is never a
 * condition: the loop stops when the flow reaches the screen that FOLLOWS the
 * engrave, and the census is returned as data for the record.
 */
async function runEngraveTail({ pollMs = 75, settleMs = 150 }) {
  const acts = [], digests = [];
  const NOWHERE = [5, 5];

  await tap(CONFIRM, 400);               // past the Policy Review

  // (6b) THE WALLET-POLICY FORM, and this is F-172. "Full policy md1" is row 0;
  // "Template-only md1" is row 1 and SKIPS THE RESTORE DOC ENTIRELY
  // (gui/multisig_build.go guards the call with `if !template`). Taking that
  // branch would end the walk with this gate's subject never drawn, which is
  // indistinguishable from the screen failing to say it. The row is tapped
  // explicitly even though it is the default, for choose()'s reason.
  await waitFor("Which md1?");
  await tap([240, rowY(0, 2)], 300);
  await tap(CONFIRM, 400);

  await waitFor("EXPERIMENTAL");
  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);                  // > gui.confirmDelay, which is 1s
  window.shRelease(...CONFIRM);
  await waitFor("What to engrave?");
  await tap(CONFIRM, 400);               // Full (seed + keys), row 0
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
  return { census: JSON.parse(window.shToolpath.strings()), digests, acts, reachedVerifyOffer };
}

/**
 * S3's gate run.
 *
 * @param {object}   [opts]
 * @param {string}   [opts.payload="cards"]   which systemwide blob to serve.
 * @param {number}   [opts.n=3]               cosigner count.
 * @param {number}   [opts.k=2]               threshold.
 * @param {number}   [opts.selfSlot=0]        the operator's slot.
 * @param {string[]} [opts.picks]             per-card decisions in payload order.
 *          The emulator payload is A@0, A@1, B@0, C@0; SKIP, SKIP reaches Trace
 *          A's B@0 + C@0, and the picker stops asking once the remaining cards
 *          equal the remaining slots. Taking A@0 would duplicate the self key
 *          (master A) and hit 4.1's refusal instead.
 * @param {number}   [opts.templateRow=1]     the template picker row. 1 is
 *          sh(wsh) (nested segwit) per multisigScriptChoices(); it is a PARAM so
 *          the same driver can be pointed at row 0/2 to show the note changes,
 *          but the ASSERTIONS below are keyed to nested segwit.
 */
export async function run({ payload = "cards", n = 3, k = 2, selfSlot = 0,
                            picks = ["skip", "skip"], templateRow = 1,
                            pollMs = 75, settleMs = 150 } = {}) {
  const t0 = performance.now();
  assertNoNFC("at entry");
  if (typeof window.shToolpath !== "object") {
    throw new Error("shToolpath is missing — this is a STALE emu.wasm. The browser " +
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

  // THE TEMPLATE, and the one pick this whole walk exists for. Row 1 is
  // "sh(wsh) (nested segwit)" (multisigScriptChoices, order-locked with
  // multisigScriptFor). The tap is NOT the evidence; the policy review's origin
  // note is, and it is asserted below.
  await choose(templateRow, 3, NEEDLE_N, "sh(wsh) (nested segwit)");
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
  await choose(0, 2, GATHER_BODY_NOT_A_NEEDLE, "omit fingerprints");

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

  // BOUNDED SELECTION: the payload over-supplies (four cards, two open slots), so
  // the picker appears and its PRESENCE is the evidence.
  const openSlots = n - 1;
  let selected = false;
  const decisions = [];
  if (cardsGathered > openSlots) {
    await waitFor(NEEDLE_PICK);
    proven.push(NEEDLE_PICK);
    selected = true;
    await tap(CONFIRM, 400);
    await waitFor(NEEDLE_PICK_CARD);
    proven.push(NEEDLE_PICK_CARD);
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
  }

  // THE SELF SEED, from the payload (the cards blob carries master A as a
  // ClassMnemonic). "FROM PAYLOAD" is row 0 of three and is the default; the row
  // is tapped anyway, because a default that silently moves is how a walk ends up
  // driving the keyboard while reporting it drove the payload.
  await waitFor("Input Seed");
  await tap([240, rowY(0, 3)], 300);
  await tap(CONFIRM, 400);
  await waitFor("systemwide payload");
  await tap(CONFIRM, 400);
  await waitFor("Add a BIP-39 passphrase?");
  await tap(CONFIRM, 500);               // Skip is row 0

  // THE FORK. Both are legitimate outcomes of this payload; only one is this
  // walk's, and reaching the other is a FAILURE rather than a silent retry.
  const arm = await raceFor(["Duplicate key", "Policy stub"], 20000);
  if (arm === "Duplicate key") {
    throw new Error(`the build was refused as a duplicate: picks ${JSON.stringify(picks)} ` +
      `selected the self key's own card. Screen: ${JSON.stringify(window.shScreen())}`);
  }
  const reviewScreen = window.shScreen();

  // 0.1a ON THE CONFIRMATION SURFACE, and the proof that row 1 landed.
  // buildOriginAnnouncement emits this sentence ONLY for md.MultisigShWsh, so a
  // wsh or legacy-sh build cannot show it. This is the assertion the template tap
  // cannot make for itself.
  if (!squash(reviewScreen).includes(squash(NEEDLE_NESTED_NOTE))) {
    throw new Error(`the Policy Review does not carry the nested-segwit origin note, ` +
      `so this build is NOT sh(wsh) whatever row was tapped: ` +
      `${JSON.stringify(reviewScreen)}`);
  }
  proven.push(NEEDLE_NESTED_NOTE);

  const tail = await runEngraveTail({ pollMs, settleMs });
  if (!tail.reachedVerifyOffer) {
    throw new Error(`the engrave tail never reached the verify offer; acts ` +
      `${JSON.stringify(tail.acts)}`);
  }

  // (10) Skip the verify offer: "Verify now" is row 0, "Skip" is row 1. The
  // restore doc is step (11) and is what this stage gates on.
  await waitFor(VERIFY_OFFER);
  await tap([240, rowY(1, 2)], 300);
  await tap(CONFIRM, 500);

  // ─── THE GATE ──────────────────────────────────────────────────────────────
  //
  // The restore doc must NAME the nested-segwit script. Before S3 both
  // sh(wsh(sortedmulti)) and bare sh(sortedmulti) rendered "P2SH", and on this
  // branch the restore doc named no script type at all.
  const restoreScreen = await waitFor(NEEDLE_NESTED_NAME, 20000);
  proven.push(NEEDLE_NESTED_NAME);
  const namedOnRestoreDoc = squash(restoreScreen).includes(squash(NEEDLE_NESTED_NAME));
  // Corroboration, NOT proof of flow: the title has two production sites.
  const onRestoreDocScreen = squash(restoreScreen).includes(squash(RESTORE_TITLE_NOT_A_NEEDLE));
  // The legacy name must not ALSO be claimed. "P2SH-P2WSH" contains "P2SH", so
  // this cannot be tested by absence of "P2SH"; test the policy line instead,
  // which is the exact string a bare-sh build would draw.
  const claimsLegacyToo = squash(restoreScreen).includes(squash("P2SH 2-of-3 multisig"));

  return {
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    carouselHops: hops,
    params: { n, k, selfSlot, picks, templateRow },
    decisions,
    reviewScreen,
    restoreScreen,
    // DATA, not a verdict. The plate count is reported so a record can show it
    // and a human can compare it against the md1/mk1/ms1 chunk counts; it is NOT
    // a term in `ok`, because a number the driver was handed cannot be evidence
    // about what the machine cut (S0b review I-1).
    plateCount: tail.census.strings.length,
    unattributed: tail.census.unattributed,
    census: tail.census,
    digests: tail.digests,
    acts: tail.acts,
    needlesProven: proven,
    presented: assertNoNFC("after the restore doc"),
    cardsGathered,
    openSlots,
    selected,
    gatherScreen,
    namedOnRestoreDoc,
    onRestoreDocScreen,
    claimsLegacyToo,
    // `ok` NAMES ITS OUTCOME and every term is something the EMULATOR produced:
    // nine needles observed on screen, zero records across the reader, a
    // payload-fed set that had to be narrowed, the nested-segwit name on the
    // restore doc, and no unattributed strings in the recorder's own
    // attribution. The plate COUNT is deliberately absent.
    ok: proven.length === 9 && cardsGathered > 0 && selected &&
      namedOnRestoreDoc && onRestoreDocScreen && !claimsLegacyToo &&
      tail.reachedVerifyOffer && tail.census.unattributed === 0,
  };
}
