// TRACE B: the flagship. n=4, k=3, the operator holds THREE slots across TWO
// masters, the fourth slot comes off a payload cosigner card, and the whole
// backup is cut.
//
//   const w = await import("./walk_trace_b.js");
//   w.run().then(r => { window.__b = r });        // ~10 minutes: 17 plates
//
// NOT loaded by index.html, for walk_trace_a.js's reason: this drives the
// machine, and a page that starts engraving because somebody opened it is a trap.
//
// WHAT THIS WALK IS FOR, and why S5 cannot close without it.
//
// The plan's §2 makes Trace B an ACCEPTANCE CRITERION rather than an
// illustration: "n=4, k=3. The operator holds @0 = A-acct0, @1 = A-acct1,
// @2 = B-acct0; @3 is a cosigner's card on the payload. Expected: correct
// descriptor engraved, from the end of S5, with divergent origins, one mk1 per
// held slot, and ms1 for BOTH masters in full mode."
//
// Every earlier walk holds exactly ONE slot, from ONE master, at ONE origin. So
// until this file the three things S5 exists to deliver -- multi-slot self,
// divergent origins, and an ms1 per distinct master -- had never been driven
// through the real flow at all, and the gate record for a BUILT policy had never
// been minted. A gate that has never executed is a hypothesis, not a gate.
//
// THE FOUR THINGS IT PROVES THAT NO OTHER WALK CAN
//
//   1. The @S picker is a SET. Three passes through "Do you hold another slot?"
//      put @0, @1 and @2 in the held set; NEEDLE_MORE_SLOT and
//      NEEDLE_OTHER_SLOT are its two screens and both are single-site.
//   2. The accounts DIVERGE. buildSlotSources numbers a held slot by its ordinal
//      among the held slots sharing a MASTER, so @0 and @1 are master A's
//      accounts 0 and 1 while @2 is master B's account 0. The Key-sources review
//      says so in words, and this walk asserts that sentence
//      (`@1  yours: derived from your seed for @1, account 1`) rather than
//      inferring it from a tap.
//   3. TWO masters are held, so full mode owes TWO ms1 plates. That needs a seed
//      the payload does not carry -- see THE KEYBOARD below.
//   4. The fourth card fills @3. Cards A@0, A@1 and B@0 would each duplicate a
//      key the device already holds, so C@0 is the only card this policy can
//      take, and the picker is driven SKIP, SKIP, SKIP to reach it.
//
// THE KEYBOARD, and why this walk had to grow one.
//
// cmd/emu's cards payload carries exactly ONE ClassMnemonic (master A), and
// syswSession.take is FIRST-MATCH and non-consuming (gui/sysw_session.go), so
// every "FROM PAYLOAD" seed entry in one flow returns the SAME master. Trace B
// needs a second one. F-181 recorded a keyboard driver as "genuinely optional";
// it stopped being optional the moment a walk had to hold two masters.
//
// It is driven BY TAPS AT DEVICE COORDINATES -- no new emulator primitive, no
// rune injection, nothing the panel could not deliver. The coordinates are not
// guessed: gui/keyboard_geometry_test.go computes each key's tap point with the
// SAME formula keyPoint() below uses and asserts, through op.Drawer.Hit on a real
// rendered frame, that it lands on that key's own Clickable. So a font or layout
// change turns a Go test red instead of turning this walk into a mystery.
//
// F-181's own two empirical data points agree with the formula, which is worth
// something precisely because they were measured before it existed:
// shTap(80,180) typed Q (Q's rect is 74,180-100,216) and shTap(300,200) typed U
// (278,180-304,216).
//
// AND EVERY KEYSTROKE HAS A POST-CONDITION. The word label is read back after
// each letter, so a coordinate that drifted fails on letter one naming the word
// it was typing, instead of assembling a wallet from a seed nobody chose.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const PAGE = [453, 160];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
const NOWHERE = [5, 5];

// ChoiceScreen rows, measured in walk_build_policy.js and re-used verbatim.
export const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// The cards payload's digest, as cmd/emu/sysw_cards_payload.go pins it. Loading
// the records blob by mistake is the failure this catches, and it is caught
// BEFORE any parameter is picked.
export const CARDS_DIGEST = "25271e583f3eaa03ae18f359c72b76e3";

// ─── Needles: strings with exactly ONE production site ──────────────────────
//
// cmd/emu/needle_test.go pins the count and TestWalkNeedleLiteralsAreAllPinned
// pins that THESE literals are the pinned ones, so a walk cannot quietly anchor
// on an ambiguous string. The walk asserting the needle APPEARED and the test
// asserting it could only have come from one place are the two halves of the
// gate; neither is one alone.
export const NEEDLE_FRONT_DOOR = "Supply or build a policy?"; // gui/multisig.go
export const NEEDLE_TEMPLATE = "Choose policy type"; // gui/multisig_build.go
export const NEEDLE_N = "How many keys (n)?"; // gui/multisig_build.go
export const NEEDLE_SLOT = "Which slot is your key?"; // gui/multisig_build.go
// S5's MULTI-SELECT @S picker, and these two are what make this walk Trace B's
// rather than another single-slot run. Neither existed before S5.
export const NEEDLE_MORE_SLOT = "Do you hold another slot?"; // gui/multisig_build.go
export const NEEDLE_OTHER_SLOT = "Which other slot is yours?"; // gui/multisig_build.go
// S4's slot-source question in its PLURAL form, which only a multi-slot build can
// reach: buildSelfSourceLead's singular arm says "key on a card?" and its plural
// arm says "keys on cards?", and the two are deliberately different substrings so
// each can be pinned separately.
export const NEEDLE_SELF_SOURCES = "keys on cards?"; // gui/multisig_build_slots.go
export const NEEDLE_PICK = "Payload cards"; // gui/multisig_build_payload.go
export const NEEDLE_PICK_CARD = "Use payload card"; // gui/multisig_build_payload.go
export const NEEDLE_GATHER = "Cosigner Keys"; // gui/multisig_build.go
export const NEEDLE_KEY_SOURCES = "Where each key comes from:"; // gui/multisig_build_slots.go
export const NEEDLE_CENSUS = "Plate Count"; // gui/multisig_build.go
// The gate's FAIL screen. No honest build draws it; this walk races AGAINST it so
// that a refusal is reported as a refusal rather than as a timeout.
export const NEEDLE_GATE_FAIL = "Key does not match seed"; // gui/multisig_build.go

// NOT needles, and named so a future author who reaches for one finds out why.
//
//   "Scan a card, or Done"        the gather's BODY, drawn by the shared gatherer
//                                 for every caller. The gather's TITLE is a
//                                 needle; its body is not.
//   "Verify the engraved plates?" three production sites, so it is a
//                                 post-condition for a tap and never evidence of
//                                 which flow drew it.
const GATHER_BODY_NOT_A_NEEDLE = "Scan a card, or Done";
const VERIFY_OFFER = "Verify the engraved plates?";

// THE TWO MASTERS. BIP-39's own published vectors, public by construction; never
// put funds behind them. Master A arrives FROM THE PAYLOAD (the cards blob's one
// ClassMnemonic); master B is TYPED, because the payload cannot supply a second.
export const MASTER_B_WORDS =
  "legal winner thank year wave sausage worth useful legal winner thank yellow";

// gui.confirmDelay is ONE SECOND of wall clock and releasing early aborts the
// gesture. Measured in walk_trace_a.js.
const HOLD_MS = 1300;
const STALL_TICKS = 3;

const ENGRAVE_HANDLERS = [
  { name: "engrave-prompt", match: "Holdbuttontostarttheengravingprocess", act: "hold" },
  { name: "engrave-done", match: "Engravingcompletedsuccessfully", act: "confirm" },
  { name: "choose-variant", match: "Chooseengraving", act: "confirm" },
];

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

/** Wait until ONE of several needles appears, and say which. A fork in the flow
 * needs a fork in the driver: waiting for one and treating a timeout as "the
 * other one happened" reports the wrong arm confidently whenever the flow simply
 * hung, which is the failure a walk exists to catch. */
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
 * thing that makes a computed coordinate safe. */
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
 * Read EVERY page of a confirmReviewScreen, concatenated.
 *
 * The review screens this walk asserts on are longer than one frame, and
 * confirmReviewScreen's pager wraps back to page 0 at the end
 * (gui/multisig_build.go). A walk that read only the first frame would assert on
 * whatever happened to fit -- which is F-185's defect from the other side: there
 * the operator could not see past the fold, here the DRIVER cannot.
 *
 * Terminated by seeing page 0 again, never by a page count.
 */
async function readAllPages(maxPages = 12) {
  const first = squash(window.shScreen());
  const pages = [window.shScreen()];
  for (let i = 1; i < maxPages; i++) {
    await tap(PAGE, 300);
    const now = squash(window.shScreen());
    if (now === first) return { text: pages.map(squash).join(""), pages: pages.length, wrapped: true };
    pages.push(window.shScreen());
  }
  return { text: pages.map(squash).join(""), pages: pages.length, wrapped: false };
}

/** F-174: the cards must come from the PAYLOAD. The SH2 HAS a working reader
 * (soldered ST25R3916) and phase 1 takes its cards from the payload and the
 * keyboard by SCOPE, not by hardware absence -- so this assertion is what makes
 * payload-sourcing provable, rather than being redundant. */
export function assertNoNFC(where) {
  if (typeof window.shNFC.presented !== "function") {
    throw new Error("shNFC.presented is missing - this is a STALE emu.wasm. " +
      "The browser caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const n = window.shNFC.presented();
  if (n !== 0) {
    throw new Error(`${where}: ${n} record(s) crossed the NFC reader; a stage-gate run must ` +
      `present ZERO, because the cards and the seeds must come from the payload and the keyboard`);
  }
  return n;
}

// ─── The on-device BIP-39 keyboard, by coordinate ───────────────────────────
//
// GEOMETRY, DERIVED FROM THE LAYOUT AND PINNED IN GO. gui.NewKeyboard lays the
// alphabet out on a fixed pitch and inputWordsFlow anchors the block at the
// bottom-centre of the content band, so every key's rectangle is a pure function
// of the display size and the body face. The numbers below are that function
// evaluated for the SH2's 480x320 panel:
//
//   key rect = (KBD_X0 + col*KEY_PITCH, rowTop) .. + (26, 36)
//
// gui/keyboard_geometry_test.go recomputes the same tap points and asserts
// op.Drawer.Hit lands on the matching key's Clickable, so these are machine-
// checked rather than remembered.
export const KEY_PITCH = 34;
export const KEY_ROWS = [
  { letters: "qwertyuiop", x0: 87, y: 198 },
  { letters: "asdfghjkl", x0: 104, y: 244 },
  { letters: "zxcvbnm", x0: 138, y: 290 },
];

/** keyPoint returns the tap point for one lower-case letter, or throws. */
export function keyPoint(ch) {
  for (const row of KEY_ROWS) {
    const j = row.letters.indexOf(ch);
    if (j >= 0) return [row.x0 + j * KEY_PITCH, row.y];
  }
  throw new Error(`no key for ${JSON.stringify(ch)} on the BIP-39 keyboard`);
}

/**
 * Type one BIP-39 word and accept it.
 *
 * EVERY LETTER IS CHECKED. The word line renders as "%2d: <fragment or completed
 * word>", so after letter i the squashed screen must contain
 * `<n>:<PREFIX-SO-FAR>` -- which holds both before and after the fragment
 * auto-completes, because the completed word starts with the prefix. A drifted
 * coordinate therefore fails on the first letter, naming the word, instead of
 * silently building a wallet from a seed nobody chose.
 *
 * Typing the WHOLE word is safe by construction: updateValidBIP39Keys enables
 * exactly the letters that extend the fragment toward a real word, so every
 * letter of a real word is enabled when its turn comes.
 */
async function typeWord(word, index, total, settle = 120) {
  const n = index + 1;
  for (let i = 0; i < word.length; i++) {
    await tap(keyPoint(word[i]), settle);
    const want = `${n}:${word.slice(0, i + 1).toUpperCase()}`;
    if (!squash(window.shScreen()).includes(want)) {
      throw new Error(`typing word ${n} (${word}): after letter ${i + 1} (${word[i]}) the screen ` +
        `does not show ${JSON.stringify(want)}; it reads ${JSON.stringify(window.shScreen())}`);
    }
  }
  await tap(CONFIRM, 400);
  if (n < total) {
    await waitFor(`word ${n + 1} of ${total}`);
  }
}

/** typePhrase drives a whole mnemonic through the keyboard. */
async function typePhrase(phrase, slotLabel) {
  const words = phrase.trim().split(/\s+/);
  await waitFor(`${slotLabel} word 1 of ${words.length}`);
  for (let i = 0; i < words.length; i++) {
    await typeWord(words[i], i, words.length);
  }
  return words.length;
}

/**
 * Enter the seed for one held slot.
 *
 * @param {string} slotLabel  "@0", "@1", ... -- the screens name it, and the
 *        titles are what this waits on: "Add a BIP-39 passphrase?" is the same
 *        sentence for every slot, so waiting on IT would let a stale frame stand
 *        in for the next slot's screen.
 * @param {string|null} typed  null takes the payload's ClassMnemonic; a phrase
 *        is typed on the keyboard.
 */
async function enterSeed(slotLabel, typed) {
  await waitFor(`Seed for ${slotLabel}`);
  await waitFor("Where from?");
  if (typed === null) {
    // "FROM PAYLOAD" is row 0 of three (FROM PAYLOAD / TYPE IT / SCAN) and is the
    // default, so CONFIRM alone would take it -- but the ROW IS TAPPED anyway,
    // because a default that silently moves is how a walk ends up driving the
    // keyboard while reporting it drove the payload.
    await tap([240, rowY(0, 3)], 300);
    await tap(CONFIRM, 400);
    await waitFor("systemwide payload");
    await tap(CONFIRM, 400);
  } else {
    await tap([240, rowY(1, 3)], 300);   // TYPE IT
    await tap(CONFIRM, 400);
    await waitFor("Choose number of words");
    await tap([240, rowY(0, 2)], 300);   // 12 WORDS
    await tap(CONFIRM, 400);
    await typePhrase(typed, slotLabel);
  }
  await waitFor(`Passphrase ${slotLabel}`);
  await waitFor("Add a BIP-39 passphrase?");
  await tap(CONFIRM, 500);               // Skip is row 0
}

/**
 * Drive the engrave tail from the Policy Review to the verify offer.
 *
 * TERMINATED BY AN OBSERVED SCREEN, never by a plate count (S0b review I-1).
 * `perPlateDigest` is not a flag here: cmd/gaterecord refuses a walk with no
 * plate digests -- a record with none is bound to nothing -- and a flag that must
 * be remembered is a way to finish a ten-minute walk and find it cannot anchor
 * the gate.
 */
async function runEngraveTail({ pollMs = 75, settleMs = 150 }) {
  const acts = [], digests = [];

  const review = await readAllPages();
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

  // THE PLATE CENSUS, before a single plate is cut. Read, then confirmed. The
  // claim is checked later against the plate stream the emulator produced, never
  // against a count written in this file.
  await waitFor(NEEDLE_CENSUS);
  const census = await readAllPages();
  const censusClaim = (() => {
    const m = /Thisengraves(\d+)plates?\./.exec(census.text);
    return m ? Number(m[1]) : -1;
  })();
  await tap(CONFIRM, 400);
  await waitFor("Chooseengraving");

  let stall = 0, lastSteps = -1, reachedVerifyOffer = false;
  for (let guard = 0; guard < 60000; guard++) {
    const steps = JSON.parse(window.shToolpath.summary()).steps;
    if (steps === lastSteps) stall++; else { stall = 0; lastSteps = steps; }
    if (stall < STALL_TICKS) { await sleep(pollMs); continue; }

    await tap(NOWHERE, settleMs);        // force a redraw before reading
    const screen = squash(window.shScreen());
    if (screen.includes(squash(VERIFY_OFFER))) { reachedVerifyOffer = true; break; }
    const h = ENGRAVE_HANDLERS.find((x) => screen.includes(x.match));
    if (!h) {
      // A screen the handler list does not recognise STOPS the walk. Tapping past
      // what it cannot name is how a driver manufactures a pass.
      acts.push({ act: "STALLED", screen: screen.slice(0, 140) });
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
      for (let i = 0; i < 200; i++) {
        window.shTap(...NOWHERE);
        await sleep(pollMs);
        if (!squash(window.shScreen()).includes(h.match)) break;
      }
    }
    stall = 0; lastSteps = -1;
  }
  return {
    census: JSON.parse(window.shToolpath.strings()),
    reviewScreen: review.text, reviewPages: review.pages,
    censusScreen: census.text, censusClaim,
    digests, acts, reachedVerifyOffer,
  };
}

/**
 * Trace B, end to end.
 *
 * @param {object} [opts]
 * @param {string} [opts.payload="cards"]
 * @param {number} [opts.n=4]  the policy's key count.
 * @param {number} [opts.k=3]  the threshold.
 * @param {number[]} [opts.held=[0,1,2]] the slots the operator holds, ascending.
 *        The FIRST is picked on "Which slot is your key?" and each of the rest
 *        through a "YES, ONE MORE" round -- which is the multi-select S5 adds.
 * @param {string[]} [opts.picks=["skip","skip","skip"]] per-card decisions in
 *        payload order. The payload carries A@0, A@1, B@0, C@0 and the device
 *        already holds A@0's, A@1's and B@0's keys, so any of the first three
 *        would be a DUPLICATE KEY refusal: C@0 is the only card this policy can
 *        take, and three skips is how the picker reaches it. The picker stops
 *        asking once the cards that remain equal the slots that remain
 *        (gui/multisig_build_payload.go), so the fourth is taken without a
 *        fourth question.
 * @param {string} [opts.typedSeedFor="@2"] which held slot is TYPED. The payload
 *        carries one master; the second has to come from the keyboard.
 */
export async function run({ payload = "cards", n = 4, k = 3, held = [0, 1, 2],
                            picks = ["skip", "skip", "skip"],
                            typedSeedFor = "@2", typedPhrase = MASTER_B_WORDS,
                            pollMs = 75, settleMs = 150 } = {}) {
  const t0 = performance.now();
  assertNoNFC("at entry");
  if (typeof window.shToolpath !== "object" || typeof window.shPace !== "function") {
    throw new Error("shToolpath/shPace is missing - this is a STALE emu.wasm. The browser " +
      "caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  if (held.length < 2) {
    throw new Error(`Trace B holds SEVERAL slots; held=${JSON.stringify(held)} would walk a ` +
      `single-slot build, which walk_build_policy.js already does`);
  }

  const proven = [];
  const pacing = window.shPace();          // reads, never sets
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
  // WHICH BLOB, asserted rather than assumed. The records blob is 55adb800...;
  // getting it instead is a failure that would otherwise surface four screens
  // later as "no cosigner cards".
  const payloadDigest = /([0-9a-f]{32})/.exec(squash(window.shScreen()))?.[1];
  if (payloadDigest !== CARDS_DIGEST) {
    throw new Error(`loaded the wrong payload: digest ${payloadDigest}, want ${CARDS_DIGEST}`);
  }
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

  // "Build policy" is choice 1; "Supply policy (md1)" is 0 and is the default, so
  // failing to move the selection lands in the WRONG flow with no complaint.
  await choose(1, 2, NEEDLE_TEMPLATE, "Build policy");
  proven.push(NEEDLE_TEMPLATE);

  // wsh (row 0). Trace B's subject is the slot set and the origins, not the
  // wrapper; S3 owns the nested-segwit shape.
  await choose(0, 3, NEEDLE_N, "wsh (native segwit)");
  proven.push(NEEDLE_N);

  await choose(n - 2, 4, `Required signatures (k of ${n})?`, `n = ${n}`);
  await choose(k - 1, n, NEEDLE_SLOT, `k = ${k}`);
  proven.push(NEEDLE_SLOT);

  // ─── S5's MULTI-SELECT @S PICKER ───────────────────────────────────────────
  //
  // The first screen picks one slot from all n. Each further slot costs a "YES,
  // ONE MORE" and a pick from what is LEFT -- so the row index is an index into
  // the REMAINING labels, never a slot number. multisigRemainingSlotChoices is
  // ascending, so the row for a wanted slot is how many already-held slots sit
  // below it.
  await choose(held[0], n, NEEDLE_MORE_SLOT, `first held slot @${held[0]}`);
  proven.push(NEEDLE_MORE_SLOT);
  const heldSoFar = [held[0]];
  for (let i = 1; i < held.length; i++) {
    const want = held[i];
    const remaining = [];
    for (let s = 0; s < n; s++) if (!heldSoFar.includes(s)) remaining.push(s);
    const row = remaining.indexOf(want);
    if (row < 0) {
      throw new Error(`slot @${want} is already held; held=${JSON.stringify(heldSoFar)}`);
    }
    await choose(1, 2, NEEDLE_OTHER_SLOT, "YES, ONE MORE");
    proven.push(NEEDLE_OTHER_SLOT);
    await choose(row, remaining.length, NEEDLE_MORE_SLOT, `held slot @${want}`);
    heldSoFar.push(want);
  }
  await choose(0, 2, "Include key fingerprints?", "NO, THAT IS ALL");
  await choose(0, 2, NEEDLE_SELF_SOURCES, "omit fingerprints");
  proven.push(NEEDLE_SELF_SOURCES);
  const selfSourceScreen = window.shScreen();
  // "NO, JUST MY SEEDS" is row 0. Row 1 would make every held slot a `both` slot
  // filled from a CARD, which is S4's walk and a different wallet: Trace B's held
  // keys are DERIVED, at three different accounts.
  await choose(0, 2, GATHER_BODY_NOT_A_NEEDLE, "NO, JUST MY SEEDS");

  // THE GATHER. The tally is read rather than asserted against a hard-coded
  // number: what matters is that the count is the PAYLOAD's, and
  // `presented === 0` is what says so.
  await waitFor(NEEDLE_GATHER);
  proven.push(NEEDLE_GATHER);
  const gatherScreen = window.shScreen();
  assertNoNFC("at the cosigner gather");
  const mkTally = /mk1keys:(\d+)/.exec(squash(gatherScreen));
  const cardsGathered = mkTally ? Number(mkTally[1]) : -1;
  if (cardsGathered <= 0) {
    throw new Error(`the gather holds ${cardsGathered} mk1 card(s) with nothing across the ` +
      `reader; the payload feed did not run. Screen: ${JSON.stringify(gatherScreen)}`);
  }
  await tap(CONFIRM, 500);               // Done adding cards

  // BOUNDED SELECTION. Trace B opens exactly ONE slot against a four-card
  // payload, which is the widest over-supply the `0..n` ruling admits.
  const openSlots = n - held.length;
  if (cardsGathered <= openSlots) {
    throw new Error(`the payload supplied ${cardsGathered} card(s) for ${openSlots} open ` +
      `slot(s), so no selection happens and this walk cannot choose which card fills @${n - 1}`);
  }
  await waitFor(NEEDLE_PICK);
  proven.push(NEEDLE_PICK);
  const payloadReview = await readAllPages();
  await tap(CONFIRM, 400);
  await waitFor(NEEDLE_PICK_CARD);
  proven.push(NEEDLE_PICK_CARD);
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

  // ─── THE SEEDS, ONE PER HELD SLOT ──────────────────────────────────────────
  //
  // buildMultisigPolicyFlow asks in ASCENDING held-slot order, and each screen
  // names its slot (SPEC 4.1: an unlabelled second prompt is indistinguishable
  // from a repeat of the first). @0 and @1 take the payload's master A; @2 is
  // typed, because syswSession.take is first-match and cannot hand out a second
  // master.
  const seedSources = [];
  for (const slot of held) {
    const label = `@${slot}`;
    const typed = label === typedSeedFor ? typedPhrase : null;
    await enterSeed(label, typed);
    seedSources.push(`${label}:${typed === null ? "payload" : "typed"}`);
  }

  // ─── THE ASSIGNMENT REVIEW, and the three claims that make this Trace B ─────
  //
  // Racing rather than waiting: a gate refusal or a duplicate-key refusal here is
  // a FINDING, and a bare waitFor would report either as a hang.
  const outcome = await raceFor([NEEDLE_KEY_SOURCES, NEEDLE_GATE_FAIL, "Duplicate key"], 30000);
  if (outcome !== NEEDLE_KEY_SOURCES) {
    throw new Error(`the build was refused before assembly: reached ${JSON.stringify(outcome)}. ` +
      `Screen: ${JSON.stringify(window.shScreen())}`);
  }
  proven.push(NEEDLE_KEY_SOURCES);
  const sources = await readAllPages();
  // WHAT THE SCREEN MUST SAY, and it is the multi-account assignment in words.
  // buildSlotSources numbers a held slot by its ordinal among the held slots
  // sharing a MASTER, so a run that had collapsed @0 and @1 onto one account --
  // which §4.1's duplicate-key refusal would then reject -- cannot draw this.
  const claims = {
    multiAccount: sources.text.includes(squash(`@${held[1]}  yours: derived from your seed for @${held[1]}, account 1`)),
    cosignerSlot: sources.text.includes(squash(`@${n - 1}  a cosigner: payload card ${cardsGathered}, taken as supplied`)),
    nothingCrossChecked: sources.text.includes(squash("nothing was cross-checked")),
  };
  await tap(CONFIRM, 400);

  await waitFor("Policy stub");
  const tail = await runEngraveTail({ pollMs, settleMs });
  if (!tail.reachedVerifyOffer) {
    throw new Error(`the engrave tail never reached the verify offer; acts ` +
      `${JSON.stringify(tail.acts)}`);
  }
  proven.push(NEEDLE_CENSUS);

  // Skip the verify (row 1 of 2) and run out the restore doc, so "the flow
  // completed" means the flow RETURNED rather than "the last plate was cut".
  await tap([240, rowY(1, 2)], 300);
  await tap(CONFIRM, 500);
  const restoreDoc = await waitFor("Descriptor:", 20000);

  const census = tail.census;
  const censusHeld = tail.censusClaim === census.strings.length;

  return {
    pace: pacing,
    paceOverridden: false,
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    // The digest the DEVICE showed for the payload this walk loaded, asserted
    // above and returned so a gate record can name what it ran against.
    payloadDigest,
    carouselHops: hops,
    params: { n, k, held, picks, typedSeedFor },
    seedSources,
    decisions,
    cardsGathered,
    openSlots,
    selfSourceScreen,
    gatherScreen,
    payloadReview: payloadReview.text,
    keySourcesScreen: sources.text,
    claims,
    reviewScreen: tail.reviewScreen,
    censusScreen: tail.censusScreen,
    censusClaim: tail.censusClaim,
    censusHeld,
    restoreDoc,
    census,
    // DATA, not a verdict (S0b review I-1): a count a human wrote in a comment is
    // not evidence about what the machine cut. What the strings SHOULD be is
    // settled by the derived expectation committed beside the gate record
    // (oracle/gaterecords/*.expect.json), which cmd/gaterecord refuses to mint
    // without and which compares byte for byte and IN ORDER.
    plateCount: census.strings.length,
    digests: tail.digests,
    acts: tail.acts,
    needlesProven: proven,
    presented: assertNoNFC("after the restore doc"),
    // `ok` NAMES ITS OUTCOME AND CONTAINS ONLY TERMS THE EMULATOR PRODUCED (I-1).
    //
    // The needle terms NAME the decisive ones rather than counting to them: the
    // two multi-select screens and the PLURAL self-source question are what
    // separate this run from walk_build_policy.js's single-slot build, and a bare
    // count would be satisfied without any of them.
    ok: proven.includes(NEEDLE_MORE_SLOT) && proven.includes(NEEDLE_OTHER_SLOT) &&
      proven.includes(NEEDLE_SELF_SOURCES) && proven.includes(NEEDLE_KEY_SOURCES) &&
      !proven.includes(NEEDLE_GATE_FAIL) &&
      claims.multiAccount && claims.cosignerSlot &&
      cardsGathered > openSlots && censusHeld &&
      tail.reachedVerifyOffer && restoreDoc !== "" &&
      census.strings.length > 0 && census.unattributed === 0 &&
      census.announced >= census.strings.length &&
      tail.digests.length === census.strings.length,
  };
}
