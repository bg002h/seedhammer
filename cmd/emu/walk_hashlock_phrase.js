// H2 acceptance walk (IMPLEMENTATION_PLAN_hashlock_H2_device.md Task 5 Step 1):
// a hashlock phrase typed on the machine derives the digest ms hashlock derives
// on the host.
//
//   const w = await import("./walk_hashlock_phrase.js");
//   await w.run();
//
// NOT loaded by index.html: this drives the machine. It composes a policy and
// assigns a hash on the last trial, and a page that starts driving because
// somebody opened it is a trap.
//
// THE ORACLE IS THE CORPUS, NEVER THIS FILE'S ARITHMETIC. Every expected value
// below is a CONSTANT copied from ms-codec 0.8.0's own vectors, vendored at
// hashlock/testdata/hashlock-v0.8.json and pinned by
// hashlock/testdata/hashlock-v0.8.provenance.json (sha256
// a46c197a...11d30, mnemonic-secret cd0a60f). Nothing here recomputes a
// digest -- a walk that derived its own expectation would agree with a wrong
// device. The corpus is not fetchable from this page (it is outside the served
// cmd/emu directory), so each row names the corpus field it was copied from.
//
// Four trials, all through ONE `Which hash?` screen: Back at the phrase screen
// drops the phrase and returns there (spec §4.6), so the next trial starts
// clean without leaving the composer.
//
//   1. typed     "correct horse battery staple", SHA-256  -> derivation[0].sha256_h
//   2. control   "correct horse battery stapl"  (one char short), SHA-256
//                -> must NOT show trial 1's digest. This is what makes the
//                   walk falsifiable: without it, a screen that ignored the
//                   typed bytes entirely would still "pass" three times.
//   3. mixed     "Correct Horse Battery Staple", SHA-256  -> the mixed-case
//                   row's sha256_h. A screen that lowercased, trimmed or
//                   otherwise normalised the phrase (spec §2 forbids all of
//                   it) shows trial 1's digest here instead.
//   4. hardened  "correct horse battery staple", Hardened -> derivation[0]
//                   .hardened_h, after the countdown; then HOLD, so the digest
//                   is actually assigned and the reconciliation screen (§4.5)
//                   is reached.
//
// WHAT THE SCREEN SAYS IS NOT WHAT THE POLICY HOLDS (H5 §4, F-485). Trials 1-4
// above assert DISPLAYED tokens, and until this revision that was the whole
// walk -- so two defects passed it: a hash assigned BEFORE the hold-to-confirm
// (Back after reading the digest would have left it set), and a stored digest
// that differs from the displayed one. Both are red in CI's gui tests; neither
// was visible to the gate the stage closes on.
//
// So trial 4 also reads window.shComposerPathHashes(), the composition-state
// seam (gui/composer_state_hook.go, cmd/emu/composer_js.go):
//
//   * with the confirm modal UP and before the hold, the edited path's hash is
//     `null` -- the assignment has not happened yet. The read is pinned to that
//     frame rather than taken "some time before the hold", because a read taken
//     earlier passes trivially and proves nothing about the ORDER.
//   * after the hold, it is the corpus's FULL 64-hex hardened digest, and its
//     first8..last8 is the token the confirm modal displayed. Full hex on both
//     sides: comparing one abbreviation against another would accept 2^192
//     wrong digests.
//   * the reconciliation screen carries that same token AND the same
//     `chars: <n>` the confirm modal carried (§1.5).
//
// IT ONLY EVER READS. Driving stays with shTap/shPress/shRelease, which inject
// the events a finger would; a walk that reached past a screen would prove less
// than the operator's own hands do.
//
// ROW PICKING STAYS BY INDEX, and F-485's note about it is answered rather than
// deferred. chooseRow(i, expect, label) taps the i-th rectangle shTargets
// reports and then ASSERTS WHERE IT LANDED, so a moved row fails at the landing
// assertion with the screen it reached. A label pick is not available: shTargets
// returns bare rectangles because frameTargets drops the tag on purpose
// (cmd/emu/screen.go), so picking by label would need a second gui seam for no
// safety the landing assertion does not already give.
//
// Helpers are inlined from walk_h0_preimage.js / shots_composer.js (they are
// not exported there).
//
// THE KEYBOARD GRID WAS PROBED, NOT DERIVED -- during H2, on the emulator build
// of e1bf137, and NOT re-probed for H5. Nothing in this stage moves it: H5 §3
// narrows the lead's band and the lead stays two lines (44 px) at both widths,
// so kbd.MaxHeight is 209 before and after (gui/composer_hashlock_geometry_test
// .go logs it on every run) and the grid the centres below belong to is the
// same one. Run (a) of the three-run acceptance re-proves it end to end anyway:
// a moved key mistypes the phrase and trial 1 misses its digest.
// window.shTargets() is no help here: it hit-tests only the CENTRE COLUMN, and
// on the 10-key `qwertyuiop` row x=240 lands in the 8 px gap BETWEEN two keys,
// so that row is missing from its output entirely. The phrase screen has no
// usable readout either -- hashlockPhraseFlow gives the keyboard a MaxHeight
// that leaves no room for one, so PassphraseKeyboard.Layout clamps it away and
// `show` reveals nothing to read a character back from.
//
// So the grid was measured by TAPPING: x swept in 4 px steps across each row
// with the n/100 counter as the oracle (a tap that increments it hit a key; one
// that does not fell in a gap). The bands come out as
//
//   y=152  qwertyuiop  centres  86 120 154 188 222 256 290 324 358 392
//   y=198  asdfghjkl   centres 103 137 171 205 239 273 307 341 375
//   y=244  zxcvbnm     centres 137 171 205 239 273 307 341
//
// -- a uniform 34 px pitch (26 px of it live, 8 px dead) with every row centred
// on x=239, i.e. key j of an n-key row at 239 - 17(n-1) + 34j. The page-cycle
// and reveal keys were found the same way, tapping along y=290.
//
// None of that is what makes the mapping trustworthy: the DIGEST is. One
// mistyped character changes sha256 completely, so trial 1 landing on
// b867db87..edbc96cb is a 28-character proof that every press hit the key this
// file believes it did -- and trial 2 proves the assertion can fail.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// ── the corpus constants (hashlock/testdata/hashlock-v0.8.json) ──────────────
const ANCHOR = "correct horse battery staple";        // derivation[0].phrase
const ANCHOR_SHA_H = "b867db87..edbc96cb";            // derivation[0].sha256_h, first8..last8
const ANCHOR_HARD_H = "3cf5d421..b70a4c12";           // derivation[0].hardened_h, first8..last8
// The same row's hardened_h WHOLE, for the stored-versus-displayed comparison.
// Copied from the corpus, never recomputed here.
const ANCHOR_HARD_FULL =
  "3cf5d421caf2a9c8eb9de1d400866ea7d475e6ba978861bb0167a37cb70a4c12";
const MIXED = "Correct Horse Battery Staple";         // the mixed-case derivation row's phrase
const MIXED_SHA_H = "95d44470..2297a7ff";             // that row's sha256_h, first8..last8
const CONTROL = "correct horse battery stapl";        // NOT a corpus row: one character short

// ── the keyboard ────────────────────────────────────────────────────────────
const PP_PAGES = [
  ["qwertyuiop", "asdfghjkl", "zxcvbnm"],
  ["QWERTYUIOP", "ASDFGHJKL", "ZXCVBNM"],
];
const PP_ROW_Y = [152, 198, 244];
const PP_FN_Y = 290;
const PP_PAGE_CYCLE = [130, PP_FN_Y];   // "ABC" / "?123" / "#+=" / "abc"
const PP_SPACE = [216, PP_FN_Y];        // page 0's `space` key (shTargets: x=177 w=78)
const PP_PITCH = 34;
const PP_NPAGES = 4;                    // lower, upper, symbols, symbols2

/** Where key j of an n-key row sits: rows are centred on x=239 at a 34px pitch. */
const ppKeyX = (n, j) => 239 - 17 * (n - 1) + PP_PITCH * j;

/** [page, x, y] for a character this walk types, or null if it is not on a letter page. */
function ppKeyPoint(ch) {
  for (let p = 0; p < PP_PAGES.length; p++) {
    for (let r = 0; r < PP_PAGES[p].length; r++) {
      const j = PP_PAGES[p][r].indexOf(ch);
      if (j >= 0) return [p, ppKeyX(PP_PAGES[p][r].length, j), PP_ROW_Y[r]];
    }
  }
  if (ch === " ") return [0, PP_SPACE[0], PP_SPACE[1]];
  throw new Error(`no key for ${JSON.stringify(ch)} on the pages this walk drives`);
}

const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };

async function waitFor(needle, timeoutMs = 20000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(`waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; screen reads ${JSON.stringify(text)}`);
    }
    await sleep(50);
  }
}

function must(text, needle, why) {
  if (!squash(text).includes(squash(needle))) {
    throw new Error(`${why}: the screen does not carry ${JSON.stringify(needle)}.\nScreen: ${JSON.stringify(text)}`);
  }
}

/** The first of `needles` to appear -- for a screen that may be gone before it is polled. */
async function raceFor(needles, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = squash(window.shScreen());
    for (const n of needles) if (text.includes(squash(n))) return n;
    if (Date.now() >= deadline) {
      throw new Error(`none of ${JSON.stringify(needles)} appeared within ${timeoutMs}ms; screen reads ${JSON.stringify(window.shScreen())}`);
    }
    await sleep(50);
  }
}

function mustNot(text, needle, why) {
  if (squash(text).includes(squash(needle))) {
    throw new Error(`${why}: the screen carries ${JSON.stringify(needle)} and must not.\nScreen: ${JSON.stringify(text)}`);
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

/**
 * Select row `i` of the frame on screen NOW and take it.
 *
 * The coordinates are READ from window.shTargets(), not derived: it reports the
 * hit regions op.Drawer recorded, so this taps where a finger would land and
 * cannot tap a row that is not drawn. `expect` is not politeness -- a tap on
 * the wrong row picks a different parameter and the flow carries on happily.
 */
async function chooseRow(i, expect, label, settle = 350) {
  if (typeof window.shTargets !== "function") {
    throw new Error("shTargets is missing -- STALE emu.wasm. The browser caches it and a " +
      "cache-buster on index.html does not help; serve on a FRESH port.");
  }
  const targets = window.shTargets();
  if (i >= targets.length) {
    throw new Error(`choosing ${label}: the frame offers ${targets.length} tappable row(s), so row ${i} ` +
      `cannot be reached BY TOUCH.\nScreen: ${JSON.stringify(window.shScreen())}`);
  }
  await tap([targets[i].cx, targets[i].cy], 300);
  await tap(CONFIRM, settle);
  if (expect === null) return;
  try {
    await waitFor(expect);
  } catch (e) {
    throw new Error(`choosing ${label} (row ${i} of ${targets.length}) did not land on ${JSON.stringify(expect)}: ${e.message}`);
  }
}

/**
 * The hold-to-confirm gesture, with an explicit RELEASE.
 *
 * The release is load-bearing, for the same reason the gui harness's own
 * holdConfirm documents: the event router tracks ONE pointer contact, so a
 * second hold with no release in between routes to the FIRST screen's defunct
 * clickable and never leaves 0%. This walk holds three screens in sequence.
 */
async function hold([x, y], ms = 1800) {
  window.shPress(x, y);
  await sleep(ms);
  window.shRelease(x, y);
  await sleep(400);
}

/** Type `s` on the passphrase keyboard, cycling pages by touch as needed. */
async function typePhrase(s) {
  let page = 0;
  for (const ch of s) {
    const [want, x, y] = ppKeyPoint(ch);
    for (let n = 0; page !== want; n++) {
      if (n > PP_NPAGES) throw new Error(`the keyboard never reached page ${want}`);
      await tap(PP_PAGE_CYCLE, 120);
      page = (page + 1) % PP_NPAGES;
    }
    await tap([x, y], 80);
  }
  await waitFor(`${s.length}/100`);
}

/**
 * One trial, from `Which hash?` and back to it.
 *
 * method is "sha256" or "hardened". Returns {modal, firstFrame}: the confirm
 * modal's text, and which of the derivation screen / the modal appeared first.
 * The caller asserts on it; nothing here knows what a right answer looks like.
 *
 * firstFrame is a RACE and not an assertion, deliberately. The hardened
 * countdown is ~10 s of real PBKDF2 on the SH2 and effectively instant in
 * wasm, so on this emulator the confirm modal is usually already up by the
 * time the next poll runs. Demanding "Deriving" here would make the walk fail
 * on a device that is merely fast -- a timing assertion dressed as a
 * behavioural one. `TestHashlockDeriveKeepsAwakeUnderTheScreensaver` is what
 * gates that screen, in CI, on a clock the test controls.
 */
async function trial(phrase, method) {
  await waitFor("Type a hashlock phrase");
  await chooseRow(0, "32-byte value", "Type a hashlock phrase");   // the §8i rule modal
  await tap(CONFIRM, 500);
  await waitFor("Hashlock phrase");
  await typePhrase(phrase);
  await tap(CONFIRM, 500);                                          // OK
  await waitFor("Which method?");
  if (method === "sha256") {
    await chooseRow(1, "brainwallet", "SHA-256");                   // §4.3b, always warns
    await hold(CONFIRM);
  } else {
    await chooseRow(0, null, "Hardened (about 10 s)");              // 28 chars: no §4.3a modal
  }
  const firstFrame = await raceFor(["Deriving", "Write down this phrase"], 60000);
  const modal = await waitFor("Write down this phrase", 60000);     // the countdown is ~10 s on the SH2
  must(modal, "method: " + method, "the confirm modal's method line");
  must(modal, "chars: " + phrase.length, "the confirm modal's char count");
  return { modal, firstFrame };
}

/**
 * The composition's STORED path hashes, as 64-hex or null, in path order.
 *
 * Throws rather than returning undefined when the seam is missing: an emulator
 * built before H5 has no shComposerPathHashes, and a walk that silently skipped
 * the stored-versus-displayed assertions would report the same PASS as one that
 * ran them.
 */
function pathHashes(where) {
  if (typeof window.shComposerPathHashes !== "function") {
    throw new Error("shComposerPathHashes is missing -- STALE emu.wasm. The browser caches it " +
      "and a cache-buster on index.html does not help; serve on a FRESH port.");
  }
  const h = window.shComposerPathHashes();
  if (h === null) {
    throw new Error(`${where}: no composition is running, so there is nothing stored to compare ` +
      `against. The walk is not where it thinks it is.\nScreen: ${JSON.stringify(window.shScreen())}`);
  }
  return h;
}

/** first8..last8 of a 64-hex digest -- the abbreviation gui.hashlockFirst8Last8 draws. */
const short8 = (hex64) => `${hex64.slice(0, 8)}..${hex64.slice(-8)}`;

/**
 * The first8..last8 token a frame DREW, read out of the frame itself.
 *
 * THIS IS WHAT MAKES THE STORED-VERSUS-DISPLAYED ASSERTION FALSIFIABLE. Comparing
 * short8(stored) against a constant this file also compares the stored value
 * against is a tautology: once the corpus check has passed, the abbreviation
 * check cannot fail under any device behaviour, so the assertion spec §4.5(c)
 * exists to exercise would have no failing input. Reading the token the screen
 * actually painted makes "the screen showed one digest and the policy holds
 * another" a claim about two independent sources.
 *
 * The confirm modal and the reconcile screen both open `hash  <first8>..<last8>`
 * (gui/composer_copy.go), and squash() removes the two spaces, so the first
 * match is the header token. Throws rather than returning null: a walk that
 * silently skipped this comparison would report the same PASS as one that ran it.
 */
function drawnToken(frame, where) {
  const m = squash(frame).match(/hash([0-9a-f]{8}\.\.[0-9a-f]{8})/);
  if (m === null) {
    throw new Error(`${where}: no \`hash <first8>..<last8>\` token in the frame, so there is ` +
      `nothing to compare the STORED digest against.\nScreen: ${JSON.stringify(frame)}`);
  }
  return m[1];
}

/** Back out of the confirm modal to `Which hash?`, dropping the phrase (§4.6). */
async function backToWhichHash() {
  await tap(BACK, 400);                       // confirm  -> method pick
  await waitFor("Which method?");
  await tap(BACK, 400);                       // method   -> phrase screen
  await waitFor("Hashlock phrase");
  await tap(BACK, 400);                       // phrase   -> Which hash?
  await waitFor("Type a hashlock phrase");
}

export async function run() {
  for (const fn of ["shScreen", "shTargets", "shTap", "shPress", "shRelease", "shSysw",
                    "shComposerPathHashes"]) {
    if (typeof window[fn] !== "function") {
      throw new Error(`${fn} missing -- stale or wrong emu.wasm; rebuild from the hashlock-h5 branch and serve on a FRESH port`);
    }
  }
  const out = { typed: null, control: null, mixed: null, hardened: null, ok: false };

  // An empty region: `Which hash?` then holds no payload rows, so the phrase
  // row is row 0 and the lead is the one this stage added.
  window.shSysw("none");
  await waitFor("Load it?");
  await tap(BACK, 500);                                    // SKIP
  await waitFor("SeedHammer");
  await goTo("Wallet Policy");
  await tap(CONFIRM, 500);
  await waitFor("Build a new policy");
  await chooseRow(1, "Which script?", "Build a new policy");
  await chooseRow(1, "Start from?", "Segwit (wsh)");       // a key-less path is wsh-only
  await chooseRow(0, "Add a spend path", "Build my own paths");
  await chooseRow(0, "What can spend on this path?", "Add a spend path");
  await chooseRow(1, "EXPERIMENTAL", "A hash, no keys");
  await hold(CONFIRM);                                     // §8a key-less consent
  const which = await waitFor("Type a hashlock phrase");
  must(which, "No hash record in the payload", "the no-payload lead (§4.1)");
  must(which, "ms hashlock on the host", "the no-payload lead names the host route");

  // ── 1. the anchor row, SHA-256 ────────────────────────────────────────────
  const { modal: typed } = await trial(ANCHOR, "sha256");
  must(typed, ANCHOR_SHA_H, "the anchor phrase's sha256 digest (corpus derivation[0].sha256_h)");
  must(typed, "One phrase per policy", "the confirm modal's reuse line");
  out.typed = squash(typed).slice(0, 220);
  await backToWhichHash();

  // ── 2. the negative control: one character short ──────────────────────────
  const { modal: control } = await trial(CONTROL, "sha256");
  mustNot(control, "b867db87", "the CONTROL phrase produced the anchor's digest -- the screen is not " +
    "reading the typed bytes, and every positive row above is worthless");
  out.control = squash(control).slice(0, 220);
  await backToWhichHash();

  // ── 3. the mixed-case row: nothing normalises the phrase (spec §2) ────────
  const { modal: mixed } = await trial(MIXED, "sha256");
  must(mixed, MIXED_SHA_H, "the mixed-case row's sha256 digest (corpus)");
  mustNot(mixed, "b867db87", "the mixed-case phrase produced the LOWERCASE row's digest -- the phrase " +
    "was case-folded somewhere, which spec §2 forbids");
  out.mixed = squash(mixed).slice(0, 220);
  await backToWhichHash();

  // ── 4. hardened, then HOLD: the digest is assigned and §4.5's ────────────
  //      reconciliation screen is reached.
  const { modal: hardened, firstFrame } = await trial(ANCHOR, "hardened");
  must(hardened, ANCHOR_HARD_H, "the anchor phrase's hardened digest (corpus derivation[0].hardened_h)");
  mustNot(hardened, "b867db87", "hardened produced the SHA-256 digest -- the method pick did nothing");
  out.hardened = squash(hardened).slice(0, 220);
  out.hardenedFirstFrame = firstFrame;
  // The token the modal DREW, parsed from its own frame -- not a constant. The
  // must() above has already pinned it to the corpus; this is the value the
  // stored digest is compared against after the hold.
  const displayed = drawnToken(hardened, "the hardened confirm modal");
  out.displayed = displayed;

  // ── the ORDER assertion, pinned to the confirm-modal frame ───────────────
  // The modal is up and the hold has not happened. Nothing may be stored yet:
  // a route that assigned at derivation time would leave the digest set even
  // when the operator reads it and presses Back.
  const before = pathHashes("with the confirm modal up, before the hold");
  if (before.length !== 1) {
    throw new Error(`the composition has ${before.length} path(s), want exactly 1 -- the walk built ` +
      `a different policy than it thinks.\nStored: ${JSON.stringify(before)}`);
  }
  if (before[0] !== null) {
    throw new Error("the path ALREADY holds a hash while the confirm modal is up: the digest is " +
      `assigned before the hold, so Back after reading it would leave it set (F-485).\n` +
      `Stored: ${JSON.stringify(before[0])}`);
  }
  out.storedBeforeHold = before[0];

  await hold(CONFIRM);

  // ── stored versus displayed, then stored versus the corpus ───────────────
  //
  // ORDER IS LOAD-BEARING. The screen comparison runs FIRST, against the token
  // parsed out of the modal's own frame, so spec §4.5(c) -- the stored hash
  // perturbed by one byte after assignment -- fails HERE and not at the corpus
  // check. The corpus check then stays as the oracle for what the value should
  // have been. Reversed, or compared against ANCHOR_HARD_H, this assertion has
  // no failing input at all: it would restate the corpus check.
  const after = pathHashes("after the hold");
  if (typeof after[0] !== "string") {
    throw new Error("the path holds NO hash after the hold: the digest the confirm modal " +
      `displayed was never assigned.\n  stored: ${JSON.stringify(after[0])}`);
  }
  if (short8(after[0]) !== displayed) {
    throw new Error("the stored digest does not abbreviate to the token the confirm modal drew: " +
      "the screen showed one digest and the policy holds another.\n" +
      `  displayed: ${displayed}\n  stored:    ${after[0]} -> ${short8(after[0])}`);
  }
  if (after[0] !== ANCHOR_HARD_FULL) {
    throw new Error("the STORED digest is not the corpus's hardened digest for this phrase.\n" +
      `  stored:   ${JSON.stringify(after[0])}\n  corpus:   ${ANCHOR_HARD_FULL}`);
  }
  out.stored = after[0];

  const reconcile = await waitFor("run ms hashlock with this phrase", 20000);
  must(reconcile, "check the digest matches", "the reconciliation screen (§4.5)");
  // §1.5: the screen that asks for the comparison carries the operands.
  if (drawnToken(reconcile, "the reconciliation screen") !== displayed) {
    throw new Error("the reconciliation screen draws a DIFFERENT token than the confirm modal, " +
      "so the operator is asked to compare against a value they were never shown.\n" +
      `  confirm modal: ${displayed}\n  reconcile:     ${drawnToken(reconcile, "the reconciliation screen")}`);
  }
  must(reconcile, ANCHOR_HARD_H, "the reconciliation screen repeats the confirm modal's token");
  must(reconcile, "chars: " + ANCHOR.length, "the reconciliation screen repeats the confirm modal's char count");
  must(reconcile, "If they differ", "the reconciliation screen says what a mismatch means");
  out.reconcile = squash(reconcile).slice(0, 200);
  await tap(CONFIRM, 500);
  const list = await waitFor("Spend paths", 20000);
  must(list, "hash", "the path row after the hash was assigned");
  out.pathRow = squash(list).slice(0, 200);

  // ok is SET, never recomputed (§4.4). Every assertion above throws, so
  // reaching this line is the whole of the result; restating four of them here
  // -- as this walk used to -- reports a subset of what already passed and
  // silently omits the rest, including both stored-versus-displayed checks.
  out.ok = true;
  return out;
}
