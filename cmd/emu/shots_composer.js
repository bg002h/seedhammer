// The WALLET POLICY COMPOSER journey's walk: build a policy that has never
// existed anywhere else, on the device, and prove it is the one the host
// derived.
//
//   const m = await import("./shots_composer.js");
//   await m.run({ shotURL, arm: "keyed", form: "A", expect });
//
// NOT loaded by index.html -- it is driven by design/journeys/capture_composer.py.
//
// WHY THIS WALK EXISTS. Every other Wallet Policy walk is handed a policy: the
// cards arrive over NFC and the device says what they are. Spec §12 items 2 and
// 3 ask the opposite question -- an operator COMPOSES a policy from a payload
// and their own hands, and nothing outside the device has ever seen it. The
// only way to know the device built the wallet it claims is to build the same
// one in Rust on the host and compare, which is what `expect` carries.
//
// IT ASSERTS, IT DOES NOT ONLY CAPTURE. Every row of the plan's itinerary is an
// assertion here; a journey whose driver only takes pictures records whatever
// happened. Three comparisons in particular are the point of the whole file:
//
//   1. the consent screen's ids and all four addresses, against the host's;
//   2. the ENGRAVED STRINGS, byte for byte, against the host's md1/mk1 files;
//   3. the census screen's plate claim against the number of entries the
//      recorder actually saw.
//
// (2) IS THE ONLY CHECK THAT CATCHES THE UNCHUNKED SUBSTITUTION. The keyless
// arm's template encodes on the host as a 47-character UNCHUNKED string and a
// 56-character CHUNKED one; `md verify`, `md inspect` and `md decode` accept
// both identically -- same template, same ids -- while the DEVICE is
// chunk-form-always. No verify step can tell them apart. Only bytes can.
//
// ROWS ARE SELECTED BY TAPPING THEM, at the coordinates window.shTargets()
// reports for the frame that is actually on screen (see cmd/emu/screen.go).
// This walk NEVER injects an Up/Down button event, and no primitive for one
// exists: the SeedHammer II has no directional buttons, so a walk that used
// them would be performing a journey no operator can. That is not hypothetical
// -- it is W-2, found on this very cycle: composerPickScreen's rows had no
// touch target at all, every composer test was green because every one of them
// synthesised `Down`, and the composer could not be driven to a plate by a
// hand. A row this file cannot reach by tapping is a finding, not a workaround.

const sleep = ms => new Promise(r => setTimeout(r, ms));
const squash = s => String(s).replace(/\s+/g, "");

// Device coordinates, shared with every shipped walk.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
const PAGE_BTN = [453, 160];
const NOWHERE = [5, 5];

// gui.confirmDelay is ONE SECOND of wall clock and releasing early aborts the
// gesture. Measured in walk_trace_a.js.
const HOLD_MS = 1300;
const STALL_TICKS = 3;

// ─── The composer's DIGIT PAD, by coordinate ────────────────────────────────
//
// composerDigitEntry is gui.NewKeyboard over the alphabet "123\n456\n789\n0",
// laid out on a fixed pitch at the bottom-centre of the content band, so every
// key's rectangle is a pure function of the display size and the body face. The
// numbers below are that function evaluated for the SH2's 480x320 panel. The
// last row holds "0" and the backspace NewKeyboard appends, so it is centred on
// two keys rather than three and starts one pitch further right.
//
// gui/composer_digitpad_geometry_test.go reads THESE numbers out of this file
// and types 12960 with them through op.Drawer.Hit, so they are machine-checked
// rather than remembered -- the arrangement walk_trace_b.js and
// gui/keyboard_geometry_test.go already use for the BIP-39 keyboard.
export const DIGIT_KEY_PITCH = 34;
export const DIGIT_KEY_ROWS = [
  { digits: "123", x0: 206, y: 152 },
  { digits: "456", x0: 206, y: 198 },
  { digits: "789", x0: 206, y: 244 },
  { digits: "0", x0: 240, y: 290 },
];

/** digitPoint returns the tap point for one digit, or throws. */
export function digitPoint(ch) {
  for (const row of DIGIT_KEY_ROWS) {
    const j = row.digits.indexOf(ch);
    if (j >= 0) return [row.x0 + j * DIGIT_KEY_PITCH, row.y];
  }
  throw new Error(`no key for ${JSON.stringify(ch)} on the composer digit pad`);
}

/**
 * Force a redraw.
 *
 * THE GUI DRAWS ON EVENTS, so the frame standing after a flow transition can be
 * a partial one -- measured on this walk at the lock's digit pad, where the
 * accept was taken, the pad stopped drawing, and the Path menu behind it did not
 * appear until another pointer event arrived. walk_s3_nested.js nudges the same
 * way before reading the engrave screen, and for the same reason.
 *
 * (5,5) is inside the 44 px title band, above every composer screen's content
 * box (composerPageLines starts at leadingSize+8) and left of the navigation
 * column, so it lands on no control on any screen this walk visits. The shipped
 * walks use the same point.
 */
const nudge = async (settle = 120) => { window.shTap(...NOWHERE); await sleep(settle); };

async function waitFor(needle, timeoutMs = 20000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  let polls = 0;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(`waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(text)}`);
    }
    // Nudge on the way, not only at the end: a frame that never redraws is
    // indistinguishable from a flow that never moved, and only one of them is
    // this walk's fault.
    if (++polls % 6 === 0) await nudge(0);
    await sleep(50);
  }
}

const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };

async function raceFor(needles, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = squash(window.shScreen());
    for (const n of needles) if (text.includes(squash(n))) return n;
    if (Date.now() >= deadline) {
      throw new Error(`none of ${JSON.stringify(needles)} appeared within ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(window.shScreen())}`);
    }
    await sleep(50);
  }
}

/** must asserts a substring of the CURRENT frame, spaces stripped. */
function must(text, needle, why) {
  if (!squash(text).includes(squash(needle))) {
    throw new Error(`${why}: the screen does not carry ${JSON.stringify(needle)}.\n` +
      `Screen: ${JSON.stringify(text)}`);
  }
}

/** mustNot is the absence half; a screen claiming less than it shows. */
function mustNot(text, needle, why) {
  if (squash(text).includes(squash(needle))) {
    throw new Error(`${why}: the screen carries ${JSON.stringify(needle)} and must not.\n` +
      `Screen: ${JSON.stringify(text)}`);
  }
}

/**
 * Select row `i` of the frame that is on screen NOW and take it.
 *
 * THE ROW COORDINATES ARE READ, NOT DERIVED. window.shTargets() reports the hit
 * regions op.Drawer recorded for the last frame -- the same lookup the event
 * router performs for a fingertip -- so this taps where a finger would land and
 * cannot tap a row that is not there. walk_build_policy.js's rowY formula is
 * right for ChoiceScreen and wrong for the composer's paged screens, whose rows
 * advance by each line's measured height under a lead that wraps.
 *
 * `expect` is not optional politeness. A tap on the wrong row picks a different
 * parameter and the flow carries on happily; the post-condition is the only
 * thing that makes a coordinate safe (walk_build_policy.js's own words).
 */
async function chooseRow(i, expect, label, settle = 350) {
  // HOISTED ABOVE THE CALL (review N-1). It used to sit one line below
  // `window.shTargets()`, where a stale emu.wasm threw
  // `TypeError: window.shTargets is not a function` first and this message could
  // never print. Kept rather than deleted -- run()'s copy fires once at entry,
  // and this function is also reached from the engrave loop's variant handler,
  // long after that check has passed.
  if (typeof window.shTargets !== "function") {
    throw new Error("shTargets is missing -- this is a STALE emu.wasm. The browser " +
      "caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const targets = window.shTargets();
  if (i >= targets.length) {
    throw new Error(`choosing ${label}: the frame offers ${targets.length} tappable row(s), ` +
      `so row ${i} cannot be reached BY TOUCH. On a device whose only input is the panel ` +
      `that is a defect in the screen, not in this walk.\n` +
      `Screen: ${JSON.stringify(window.shScreen())}`);
  }
  await tap([targets[i].cx, targets[i].cy], 300);
  await tap(CONFIRM, settle);
  if (expect === null) return;
  try {
    await waitFor(expect);
  } catch (e) {
    throw new Error(`choosing ${label} (row ${i} of ${targets.length}) did not land on ` +
      `${JSON.stringify(expect)}: ${e.message}`);
  }
}

async function post(shotURL, name, dataURL) {
  const r = await fetch(shotURL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, png: dataURL }),
  });
  if (!r.ok) throw new Error(`shot ${name}: server said ${r.status} ${await r.text()}`);
}

async function screenShot(shotURL, name) {
  const c = document.getElementById("screen");
  if (!c) throw new Error("no #screen canvas");
  const data = c.toDataURL("image/png");
  // "data:," is what toDataURL returns for a canvas it could not rasterise, and
  // the receiver writes it as a zero-byte PNG with a 200 OK. Refuse it here so
  // the driver cannot report a successful capture of nothing.
  if (!data || data.length < 1000) {
    throw new Error(`shot ${name}: canvas produced ${data.length} chars, not an image`);
  }
  await post(shotURL, name, data);
  return name;
}

/**
 * Read a paged screen to its end, one shot per page.
 *
 * PAGING WRAPS: after the last page comes page 0 again (composerReadScreen,
 * composerPickScreen), so this stops when the FIRST page recurs rather than
 * counting pages it cannot know. maxPages is a guard against a screen that
 * never repeats, never a bound on how many pages are expected -- the expected
 * count is an assertion at the call site.
 *
 * It also leaves the screen on page 0 with the checkmark armed:
 * composerReadScreen withholds Button3 until the last page has been laid out
 * once, so a walk that consented without paging would be consenting to a
 * wallet whose proof was never drawn.
 */
async function readAllPages(shotURL, prefix, maxPages = 20) {
  const pages = [], names = [];
  for (let i = 0; i < maxPages; i++) {
    const text = squash(window.shScreen());
    if (pages.length && text === pages[0]) break;   // wrapped to the first page
    if (pages.includes(text)) break;                // no forward progress
    pages.push(text);
    names.push(await screenShot(shotURL, `${prefix}${i}.png`));
    await tap(PAGE_BTN, 350);
  }
  if (pages.length >= maxPages) {
    throw new Error(`${prefix}: paged ${maxPages} times without the first page recurring; ` +
      `this screen does not wrap and the walk cannot know it reached the end`);
  }
  return { pages, names, joined: pages.join("") };
}

async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 220);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

// ─── The engrave loop ────────────────────────────────────────────────────────
//
// walk_s3_nested.js's, with the two changes the composer makes (r0 I-9):
//
//   - there is NO verify step, so the loop cannot terminate on a verify offer;
//   - after the last plate bundleEngrave draws a modal, "Bundle engraved. Also
//     hand-engrave your ms1 share(s) ...", shown even on a watch-only run
//     (shipped copy; F-463), and the flow returns to the DOOR.
//
// So the handler list gains a fourth entry and the loop ends on the door's own
// row. An unrecognised screen STOPS the walk: tapping past what it cannot name
// is how a driver manufactures a pass.
const ENGRAVE_HANDLERS = [
  { name: "engrave-prompt", match: "Holdbuttontostarttheengravingprocess", act: "hold" },
  { name: "engrave-done", match: "Engravingcompletedsuccessfully", act: "confirm" },
  { name: "choose-variant", match: "Chooseengraving", act: "confirm" },
  { name: "bundle-engraved", match: "Bundleengraved", act: "confirm" },
];

const DOOR_ROW = "Buildanewpolicy";

async function runEngraveTail({ shotURL, prefix, variant, pollMs = 75, settleMs = 150 }) {
  const acts = [], digests = [], variants = [], shots = [], rowsTaken = [];
  let stall = 0, lastSteps = -1, reachedDoor = false, shot = false;
  for (let guard = 0; guard < 20000; guard++) {
    const steps = JSON.parse(window.shToolpath.summary()).steps;
    if (steps === lastSteps) stall++; else { stall = 0; lastSteps = steps; }
    if (stall < STALL_TICKS) { await sleep(pollMs); continue; }

    await tap(NOWHERE, settleMs);        // force a redraw before reading
    const screen = squash(window.shScreen());
    if (screen.includes(DOOR_ROW)) { reachedDoor = true; break; }
    const h = ENGRAVE_HANDLERS.find(x => screen.includes(x.match));
    if (!h) {
      acts.push({ act: "STALLED", screen: screen.slice(0, 160) });
      break;
    }
    if (h.name === "engrave-done") {
      digests.push(JSON.parse(window.shToolpath.summary()).digest);
    }
    if (h.name === "choose-variant") {
      // WHICH VARIANTS ARE OFFERED IS AN ASSERTION, AND SO IS WHICH ONE IS
      // TAKEN (review M-2). A packed md1/mk1 plate offers TEXT ONLY and nothing
      // else; a single short chunk offers TEXT + QR, TEXT ONLY, QR ONLY -- and
      // QR ONLY would hand the operator a plate the SH2 can never read back,
      // because it has no camera.
      //
      // The first draft asserted by INCLUSION (`must(v, "TEXT ONLY")`), which
      // would still have passed if a packed plate began offering all three, and
      // took the row with a bare CONFIRM on whatever was selected by default --
      // so it established neither "alone" nor "TEXT + QR is the row taken".
      const v = window.shScreen();
      variants.push(v);
      if (variant) {
        const n = window.shTargets().length;
        if (n !== variant.rows.length) {
          throw new Error(`Choose engraving offers ${n} row(s), the walk expects ` +
            `${variant.rows.length} (${variant.rows.join(", ")}).\nScreen: ${JSON.stringify(v)}`);
        }
        // Joined, so this asserts ORDER as well as presence: the rows are drawn
        // top to bottom and shTargets reports them in the same order, which is
        // what makes `take` an index into a known list.
        must(v, variant.rows.join(""), "Choose engraving's rows, in order");
        for (const f of (variant.forbid || [])) {
          mustNot(v, f, "Choose engraving offers a variant the plan says it must not");
        }
        // Taken BY ROW rather than by the default selection.
        await chooseRow(variant.take, null, `Choose engraving: ${variant.rows[variant.take]}`);
        for (let i = 0; i < 200; i++) {
          window.shTap(...NOWHERE);
          await sleep(pollMs);
          if (!squash(window.shScreen()).includes(h.match)) break;
        }
        acts.push({ act: "chooseRow", screen: h.name, row: variant.rows[variant.take] });
        rowsTaken.push(variant.rows[variant.take]);
        stall = 0; lastSteps = -1;
        continue;
      }
    }
    if (h.name === "bundle-engraved" && !shot) {
      shots.push(await screenShot(shotURL, `${prefix}bundle-engraved.png`));
      shot = true;
    }
    acts.push({ act: h.act, screen: h.name });
    if (h.act === "hold") {
      window.shPress(...CONFIRM);
      await sleep(HOLD_MS);
      window.shRelease(...CONFIRM);
      const before = JSON.parse(window.shToolpath.summary()).steps;
      for (let i = 0; i < 200; i++) {
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
  if (!reachedDoor) {
    throw new Error(`the engrave tail never returned to the composer door; acts ` +
      `${JSON.stringify(acts)}`);
  }
  return { census: JSON.parse(window.shToolpath.strings()), digests, acts, variants, shots, taken: rowsTaken };
}

/**
 * THE BYTE COMPARISON, and the entry-count half beside it.
 *
 * shToolpath.strings() is ONE ENTRY PER PLATE, newline-joined (notifyPlateText,
 * r0 I-2). So the flat list is every entry split on "\n" and concatenated in
 * order, and that is what is compared against the host file byte for byte.
 *
 * The ENTRY COUNT is checked separately against the census screen's own claim,
 * because the two catch different things: the flat list catches a wrong string,
 * the entry count catches a repacking that cut the same strings onto a
 * different number of plates.
 */
function compareEngraved(census, censusClaim, expect, label) {
  const entries = census.strings || [];
  if (entries.length !== expect.entries) {
    throw new Error(`${label}: the recorder saw ${entries.length} plate(s), the walk expects ` +
      `${expect.entries}. Census: ${JSON.stringify(entries)}`);
  }
  if (censusClaim !== entries.length) {
    throw new Error(`${label}: the Plates To Cut screen promised ${censusClaim} plate(s) and ` +
      `${entries.length} were cut. A census that does not match what the machine did is the ` +
      `one number an operator cannot check for themselves.`);
  }
  if (census.unattributed !== 0) {
    throw new Error(`${label}: ${census.unattributed} plate(s) were cut that the census cannot ` +
      `name. On a watch-only run there is nothing that should produce one.`);
  }
  const flat = [];
  for (const e of entries) for (const s of String(e).split("\n")) if (s !== "") flat.push(s);
  const want = expect.strings;
  if (flat.length !== want.length) {
    throw new Error(`${label}: the plates split into ${flat.length} string(s), the host wrote ` +
      `${want.length}.\n  device: ${JSON.stringify(flat)}\n  host:   ${JSON.stringify(want)}`);
  }
  for (let i = 0; i < want.length; i++) {
    if (flat[i] !== want[i]) {
      throw new Error(`${label}: string ${i + 1} of ${want.length} does not match the host's ` +
        `BYTE FOR BYTE.\n  device: ${JSON.stringify(flat[i])} (${flat[i].length} chars)\n` +
        `  host:   ${JSON.stringify(want[i])} (${want[i].length} chars)\n` +
        `Every md verb accepts the chunked and the unchunked form of a short template ` +
        `identically, so this comparison is the only one that can tell them apart.`);
    }
  }
  return flat;
}

/** The census screen's own plate claim. */
function censusClaimOf(screen) {
  const m = /Thisengraves(\d+)plates?\./.exec(squash(screen));
  if (!m) {
    throw new Error(`the Plates To Cut screen does not state a plate count.\n` +
      `Screen: ${JSON.stringify(screen)}`);
  }
  return Number(m[1]);
}

// ─── The boot leg ────────────────────────────────────────────────────────────
//
// The boot offer's reader is resolved before any script can speak
// (walk_s4_gate.js), so the offer is DECLINED and the payload is chosen and
// loaded from the carousel afterwards. Back IS a decline here: gui/sysw_load.go
// treats !ok and choice 1 identically.
async function bootAndChoosePayload(shotURL, taken, which) {
  const first = await raceFor(["SeedHammer", "systemwide payload is present"]);
  if (first !== "SeedHammer") {
    taken.push(await screenShot(shotURL, "c00a-boot-offer.png"));
    await tap(BACK);
    await waitFor("SeedHammer");
  }
  const got = window.shSysw(which);
  if (got !== which) {
    throw new Error(`shSysw(${JSON.stringify(which)}) returned ${JSON.stringify(got)} -- this ` +
      `emu.wasm does not know that payload. Rebuild it from this worktree.`);
  }
}

/** The keyed arm's payload leg: load the composer blob and compare its digest. */
async function loadComposerPayload(shotURL, taken, expect) {
  await goTo("Load Payload");
  await tap(CONFIRM, 500);
  const digestScreen = await waitFor("Payload Digest");
  taken.push(await screenShot(shotURL, "c01-payload-digest.png"));
  // THE FIRST COMPARISON ACROSS THE AIR GAP: the sixteen hex digits an operator
  // reads off this screen against `me sysw show` on the host.
  must(digestScreen, expect.digest,
    "the device's payload digest does not equal the host's `me sysw show`");
  await tap(CONFIRM, 400);
  const warn = await waitFor("Payload Warnings");
  must(warn, "A SECRET is stored unencrypted in flash.", "the F1 warning");
  await tap(CONFIRM, 400);
  await waitFor("Keep this payload loaded?");
  await tap(CONFIRM, 400);               // KEEP is row 0
  await waitFor("Load Payload");
}

// ─── The shape, shared by both arms ──────────────────────────────────────────

/** Add a spend path with n keys and threshold k, by tapping the pickers. */
async function addKeyPath(n, k, pathNo, expectRow) {
  await chooseRow(0, "What can spend on this path?", "Add a spend path");
  await chooseRow(0, `Path ${pathNo}: how many keys?`, "Keys");
  // The pickers list 1..max, so the value v is row v-1 on the first page.
  await chooseRow(n - 1, `Path ${pathNo}: how many must sign?`, `n = ${n}`);
  await chooseRow(k - 1, expectRow, `k = ${k}`);
}

export async function run({ shotURL = "http://127.0.0.1:8732", arm = "keyed",
                            form = "A", expect = {} } = {}) {
  if (typeof window.shTargets !== "function") {
    throw new Error("shTargets is missing -- this is a STALE emu.wasm. The browser caches it " +
      "and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const taken = [];
  const t0 = performance.now();
  const proven = [];

  if (arm === "keyless") {
    // §12 item 3's state: NO payload loaded AND no region present, so the door
    // draws the key-less lead rather than "A payload is in flash but not
    // loaded" (r0 I-10). shots_operator.js switches the reader off the same way.
    await bootAndChoosePayload(shotURL, taken, "none");

    await goTo("Wallet Policy");
    await tap(CONFIRM, 450);
    const door = await waitFor("Build a new policy");
    must(door, "No keys loaded. This builds a key-less template.", "the key-less door lead");
    must(door, "Scan cards", "the door's Scan cards row");
    taken.push(await screenShot(shotURL, "k01-door.png"));

    await chooseRow(1, "Which script?", "Build a new policy");
    await chooseRow(0, "Start from?", "Taproot (tr)");
    await chooseRow(0, "Add a spend path", "Build my own paths");
    await addKeyPath(3, 2, 1, "Path 1: 2-of-3");

    // DONE IS THE LAST ROW, and on a list holding a path it is not row 0 --
    // which is exactly the row W-2 made unreachable.
    const pathList = window.shScreen();
    must(pathList, "Path 1: 2-of-3", "the path list after a 2-of-3 path");
    const nRows = window.shTargets().length;
    await chooseRow(nRows - 1, "Sorted keys, or your order?", "Done");
    proven.push("key-order asked on a one-path list");
    await chooseRow(0, "Template-ID", "Sorted (usual)");

    const stub = await readAllPages(shotURL, "k02-stub-p");
    taken.push(...stub.names);
    if (stub.pages.length !== 2) {
      throw new Error(`the stub screen paged ${stub.pages.length} time(s); the plan expects 2 ` +
        `(the ids, then the per-slot expected origins).\n${JSON.stringify(stub.pages)}`);
    }
    must(stub.joined, `Template-ID: ${expect.templateId}`, "the stub screen's Template-ID");
    must(stub.joined, `mk1 stub (template): ${expect.templateStub}`, "the mk1 template stub");
    must(stub.joined, "Slot @0 expects a key at m/48h/0h/0h/3h", "slot @0's expected origin");
    must(stub.joined, "Slot @1 expects a key at m/48h/0h/1h/3h", "slot @1's expected origin");
    must(stub.joined, "Slot @2 expects a key at m/48h/0h/2h/3h", "slot @2's expected origin");

    // With NO sources the seating step asks whether to seat at all (r0 I-4).
    await tap(CONFIRM, 450);
    await waitFor("Seat keys into this template?");
    await chooseRow(0, "Review", "Engrave a key-less template");

    const consent = await readAllPages(shotURL, "k03-consent-p");
    taken.push(...consent.names);
    must(consent.joined, "Path 1: 2-of-3", "the consent's path line");
    must(consent.joined, "KEY PATH: NONE (NUMS)", "the consent's key-path line");
    must(consent.joined, `Template-ID: ${expect.templateId}`, "the consent's Template-ID");
    must(consent.joined, `mk1 stub (template): ${expect.templateStub}`, "the consent's stub");
    must(consent.joined, "Keyless template - no addresses.", "the no-addresses line");
    must(consent.joined, "Verify off-device.", "the verify-off-device line");

    await tap(CONFIRM, 500);
    await waitFor("Nothing outside this device");
    window.shPress(...CONFIRM);
    await sleep(HOLD_MS);
    window.shRelease(...CONFIRM);

    // A MODAL, not a picker: with no slot seated there is nothing to choose
    // between, so composerEngraveStep says so and moves on.
    const modal = await waitFor("No slot is seated");
    must(modal, "No slot is seated, so there is a template and nothing else.",
      "the key-less form modal");
    await tap(CONFIRM, 450);

    const censusScreen = await waitFor("Plates To Cut");
    must(censusScreen, "This engraves 1 plate.", "the census claim");
    must(censusScreen, "md1 template: 1 plate (key-less wallet policy)", "the census line");
    taken.push(await screenShot(shotURL, "k04-census.png"));
    const claim = censusClaimOf(censusScreen);

    await tap(CONFIRM, 500);
    // TEXT + QR is row 0 and is TAKEN by row. QR ONLY is offered here and must
    // never be the one taken: the SH2 has no camera, so a QR-only plate is one
    // the machine that cut it can never read back.
    const tail = await runEngraveTail({ shotURL, prefix: "k05-",
      variant: { rows: ["TEXT + QR", "TEXT ONLY", "QR ONLY"], take: 0 } });
    taken.push(...tail.shots);
    const flat = compareEngraved(tail.census, claim, expect, "the key-less template plate");
    const doorAgain = window.shScreen();
    must(doorAgain, "No keys loaded. This builds a key-less template.",
      "the door after the bundle");

    return {
      arm, shots: taken, elapsedSec: Math.round((performance.now() - t0) / 1000),
      stubPages: stub.pages.length, consentPages: consent.pages.length,
      censusClaim: claim, engraved: flat, variants: tail.variants,
      variantRowsTaken: tail.taken,
      digests: tail.digests, needlesProven: proven,
      matched: { templateId: expect.templateId, templateStub: expect.templateStub,
                 strings: flat },
    };
  }

  // ─── The KEYED arm ─────────────────────────────────────────────────────────
  await bootAndChoosePayload(shotURL, taken, "composer");
  await loadComposerPayload(shotURL, taken, expect);

  await goTo("Wallet Policy");
  await tap(CONFIRM, 450);
  const door = await waitFor("Build a new policy");
  must(door, "Keys loaded: 2, plus 1 seed.", "the door's key-state lead");
  must(door, "Scan cards", "the door's Scan cards row");
  // The payload holds no descriptor and no md1/mk1, so the route that would
  // consume one must not be offered.
  mustNot(door, "From payload", "the door offers a route the payload cannot serve");
  taken.push(await screenShot(shotURL, "c02-door.png"));

  await chooseRow(1, "Which script?", "Build a new policy");
  const scripts = window.shScreen();
  for (const s of ["Taproot (tr)", "Segwit (wsh)", "Nested (sh-wsh)", "Legacy (sh)"]) {
    must(scripts, s, "the script picker");
  }
  await chooseRow(1, "Start from?", "Segwit (wsh)");
  const presets = window.shScreen();
  must(presets, "Build my own paths", "the preset picker's blank row (W-1)");
  taken.push(await screenShot(shotURL, "c03-start-from.png"));

  await chooseRow(0, "Add a spend path", "Build my own paths");
  const empty = window.shScreen();
  must(empty, "slots: 0 / keys available: 2", "the live slots/keys line");
  // The payload's seed is not a composer SOURCE until it is typed in at
  // seating, so the line carries no seed term here (r0 I-5).
  mustNot(empty, "+ seed", "the live line counts a seed that is not yet a source");
  for (const r of ["Add a spend path", "Change the script", "Done"]) {
    must(empty, r, "the empty path list");
  }

  // (9) Path 1 = 2-of-2.
  await addKeyPath(2, 2, 1, "Path 1: 2-of-2");
  const afterP1 = window.shScreen();
  must(afterP1, "slots: 2 / keys available: 2", "the live line after path 1");

  // (10) Path 2 = one key. A 1-key path is rendered `1 key`, never `1-of-1`.
  await chooseRow(1, "What can spend on this path?", "Add a spend path");
  await chooseRow(0, "Path 2: how many keys?", "Keys");
  await chooseRow(0, "Path 2: how many must sign?", "n = 1");
  await chooseRow(0, "Path 2: 1 key", "k = 1");
  mustNot(window.shScreen(), "Path 2: 1-of-1", "a 1-key path is rendered `1 key`");

  // (11) Path 2's time lock: 12960 blocks, relative.
  await chooseRow(1, "Keys", "open Path 2");
  const pathMenu = window.shScreen();
  for (const r of ["Keys", "Time lock", "Hash lock", "Remove path", "Move up"]) {
    must(pathMenu, r, "Path 2's menu");
  }
  await chooseRow(1, "What kind of time lock?", "Time lock");
  await chooseRow(1, "Measured how?", "After a wait");
  await chooseRow(0, "How many blocks?", "Blocks");
  must(window.shScreen(), "1 to 65535 blocks", "the empty digit pad's band hint");
  for (const ch of "12960") await tap(digitPoint(ch), 200);
  const pad = await waitFor("12960 blocks");
  must(pad, "12960 blocks (about 90.0 days)", "the digit pad's live echo line");
  await tap(CONFIRM, 500);

  // §6b's "kind, unit, digits, ECHO" ends on a screen of its own
  // (composerReadScreen after composerLockAccept), not on the pad: the pad's
  // line is live validation and this one is the confirm, where Back discards
  // the lock. It is where the shot belongs.
  const echo = await waitFor("12960 blocks (about 90.0 days)");
  const echoPages = await readAllPages(shotURL, "c04-lock-echo-p");
  taken.push(...echoPages.names);
  if (echoPages.pages.length !== 1) {
    throw new Error(`the lock echo paged ${echoPages.pages.length} time(s); a relative lock's ` +
      `echo is ONE line.\n${JSON.stringify(echoPages.pages)}`);
  }
  // A RELATIVE lock carries NO bound line, though the payload holds a `now:`
  // record (r0 I-7): composerLockBoundLine returns "" for LockOlderBlocks. Its
  // absence is the assertion, and it is asserted against the real copy rather
  // than a paraphrase.
  mustNot(echo, "This device cannot tell the time.", "a relative lock drew a bound line");
  mustNot(echo, "905000", "a relative lock echoed the payload's height");
  await tap(CONFIRM, 500);
  const withLock = await waitFor("Path 2: 1 key + 12960 blocks");
  must(withLock, "Path 2: 1 key + 12960 blocks", "the path row after the lock");

  // (12) Path 2's hash lock, from the payload's own `hash:` record.
  await chooseRow(2, "Which hash?", "Hash lock");
  const hashRows = window.shScreen();
  must(hashRows, "hash 1", "the payload's hash row");
  must(hashRows, "abababab..abababab", "the hash row's digest");
  must(hashRows, "Type 64 hex", "the type-it row");
  must(hashRows, "No hash lock", "the clear row");
  // §8i draws only once the operator is actually TAKING a hash (r0 I-8).
  await chooseRow(0, "The hash must be SHA-256", "hash 1");
  const rule = window.shScreen();
  must(rule, "A hash of the passphrase itself can never be spent.", "the §8i preimage rule");
  taken.push(await screenShot(shotURL, "c05-hash-rule.png"));
  await tap(CONFIRM, 450);
  const withHash = await waitFor("Path 2: 1 key + hash + 12960 blocks");
  must(withHash, "Path 2: 1 key + hash + 12960 blocks", "the path row after the hash");

  await tap(BACK, 400);                  // leave Path 2's menu for the list
  await waitFor("Add a spend path");

  // (13) Done -> straight to the Template screen. The key-order question is
  // asked ONLY when the list has one path (composerSortedIsLegal, r0 I-3).
  const listRows = window.shTargets().length;
  await chooseRow(listRows - 1, "Template-ID", "Done");
  mustNot(window.shScreen(), "Sorted keys, or your order?",
    "the key-order question was asked on a two-path list");

  const stub1 = await readAllPages(shotURL, "c06-stub-p");
  taken.push(...stub1.names);
  if (stub1.pages.length !== 2) {
    throw new Error(`the first stub screen paged ${stub1.pages.length} time(s); the plan ` +
      `expects 2.\n${JSON.stringify(stub1.pages)}`);
  }
  must(stub1.joined, `Template-ID: ${expect.templateId}`, "the unseated Template-ID");
  must(stub1.joined, `mk1 stub (template): ${expect.templateStub}`, "the unseated template stub");
  must(stub1.joined, "Slot @0 expects a key at m/48h/0h/0h/2h", "slot @0's expected origin");
  must(stub1.joined, "Slot @1 expects a key at m/48h/0h/1h/2h", "slot @1's expected origin");
  // UNSEATED, so @2 sits at md's lowest-free account -- 2', not B's own 0'.
  must(stub1.joined, "Slot @2 expects a key at m/48h/0h/2h/2h", "slot @2's unseated origin");

  // (14) Seating. The pick list is drawn AT ONCE: the "Seat keys into this
  // template?" ChoiceScreen belongs to a composition with NO sources (r0 I-4).
  await tap(CONFIRM, 500);
  const seat0 = await waitFor("Slot @0, Path 1 key 1 of 2");
  mustNot(seat0, "Seat keys into this template?",
    "the no-sources question was asked with two key records loaded");
  must(seat0, "73c5da0a m/48h/0h/0h/2h", "slot @0's first source row");
  must(seat0, "73c5da0a m/48h/0h/1h/2h", "slot @0's second source row");
  must(seat0, "Type a seed", "the type-a-seed row");
  must(seat0, "Leave unseated", "the leave-unseated row");
  taken.push(await screenShot(shotURL, "c07-seat-slot0.png"));

  await chooseRow(0, "Slot @1, Path 1 key 2 of 2", "A@0 into slot @0");
  const seat1 = window.shScreen();
  // A CONSUMED SOURCE LEAVES THE LIST, so A@1 is now row 0 and A@0 is gone.
  mustNot(seat1, "73c5da0a m/48h/0h/0h/2h", "the consumed A@0 is still offered");
  must(seat1, "73c5da0a m/48h/0h/1h/2h", "A@1 is still offered");
  await chooseRow(0, "Slot @2, Path 2 key 1 of 1", "A@1 into slot @1");

  // (14a) Slot @2 is the SEED, typed in from the payload.
  const seat2 = window.shScreen();
  must(seat2, "Type a seed", "slot @2's type-a-seed row");
  must(seat2, "Leave unseated", "slot @2's leave-unseated row");
  mustNot(seat2, "73c5da0a", "a key record survived into slot @2's list");
  await chooseRow(0, "Where from?", "Type a seed");
  const whereFrom = window.shScreen();
  must(whereFrom, "FROM PAYLOAD", "the from-payload row");
  must(whereFrom, "TYPE IT", "the type-it row");
  // The emulator reports FeatureNFC, so the scan row is drawn here.
  must(whereFrom, "SCAN", "the scan row under the emulator's NFC feature");
  await chooseRow(0, "Source: the systemwide payload", "FROM PAYLOAD");
  await tap(CONFIRM, 500);
  await waitFor("Add a BIP-39 passphrase?");
  // THE POST-CONDITION IS THE ONE THE TAP CHANGES (review M-1). "seed 1" is
  // already on the passphrase screen -- it is the slot prefix in that screen's
  // own title -- so waiting for it certified nothing about the tap. "(any
  // slots)" is drawn only by composerSourceRow on the re-drawn pick list.
  await chooseRow(0, "(any slots)", "Skip the passphrase");
  const seat2b = window.shScreen();
  must(seat2b, "seed 1", "the seed source row");
  must(seat2b, "(any slots)", "the seed row's any-slots note");
  taken.push(await screenShot(shotURL, "c08-seat-slot2-seed.png"));
  await chooseRow(0, "Key mapping", "the seed into slot @2");

  // (15) The mapping review, including §8g's same-seed warning -- which this
  // fixture fires ON PURPOSE (§2). It is not a defect in the run.
  const mapping = await readAllPages(shotURL, "c09-mapping-p");
  taken.push(...mapping.names);
  must(mapping.joined, "@0: 73c5da0a m/48'/0'/0'/2'", "the mapping's slot @0");
  must(mapping.joined, "@1: 73c5da0a m/48'/0'/1'/2'", "the mapping's slot @1");
  // THE DEVICE SEATS @2 AT B'S OWN ACCOUNT 0', not at md's lowest-free 2'.
  must(mapping.joined, "@2: b8688df1 m/48'/0'/0'/2'", "the mapping's slot @2");
  must(mapping.joined, "This device cannot confirm a key was derived at the origin it declares.",
    "the mapping's honesty line");
  must(mapping.joined, "SAME SEED, SAME PATH", "§8g's same-seed heading");
  must(mapping.joined, "Slots @0 and @1 are the same seed.", "§8g's same-seed body");
  await tap(CONFIRM, 500);

  // (16) The template screen again, now carrying the POLICY id and both stubs.
  await waitFor("Policy-ID");
  const stub2 = await readAllPages(shotURL, "c10-stub2-p");
  taken.push(...stub2.names);
  must(stub2.joined, `Template-ID: ${expect.templateId}`, "the seated Template-ID");
  must(stub2.joined, `mk1 stub (template): ${expect.templateStub}`, "the seated template stub");
  must(stub2.joined, `Policy-ID: ${expect.policyId}`, "the Policy-ID");
  must(stub2.joined, `mk1 stub (policy): ${expect.policyStub}`, "the policy stub");
  must(stub2.joined, "Stamp BOTH stubs on each key card:", "the stamp instruction");
  must(stub2.joined, `--policy-id-stub ${expect.templateStub} --policy-id-stub ${expect.policyStub}`,
    "the stamp command");
  must(stub2.joined, "Slot @0: 73c5da0a m/48h/0h/0h/2h", "the seated slot @0 line");
  must(stub2.joined, "Slot @2: b8688df1 m/48h/0h/0h/2h", "the seated slot @2 line");

  // (17) THE COMPARISON. Both ids and all four addresses, whole.
  await tap(CONFIRM, 500);
  await waitFor("Review");
  const consent = await readAllPages(shotURL, "c11-consent-p");
  taken.push(...consent.names);
  must(consent.joined, "Path 1: 2-of-2", "the consent's path 1");
  must(consent.joined, "Path 2: 1 key", "the consent's path 2");
  must(consent.joined, "12960 blocks (about 90.0 days)", "the consent's lock");
  must(consent.joined, "hash abababab..abababab", "the consent's hash");
  const missing = [];
  if (!consent.joined.includes(squash(`Policy-ID: ${expect.policyId}`))) {
    missing.push(`policy id ${expect.policyId}`);
  }
  for (const a of (expect.addresses || [])) {
    if (!consent.joined.includes(squash(a))) missing.push(`address ${a}`);
  }
  if (missing.length) {
    throw new Error(`the device's proof does not match the host's:\n  ` +
      missing.join("\n  ") + `\nconsent screen read: ${JSON.stringify(consent.joined)}`);
  }

  // (18) The hold.
  await tap(CONFIRM, 500);
  await waitFor("Nothing outside this device");
  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);
  window.shRelease(...CONFIRM);
  const formPick = await waitFor("Which form?");
  must(formPick, "The policy itself", "form A's row");
  must(formPick, "Template plus key cards", "form B's row");

  // (19) The form, then the mode. Watch-only, always: Full would add a BEARER
  // plate of master B's seed and this is an automated run.
  await chooseRow(form === "A" ? 0 : 1, "What to engrave?",
    form === "A" ? "The policy itself" : "Template plus key cards");
  const modePick = window.shScreen();
  must(modePick, "Watch-only (keys)", "the watch-only row");
  must(modePick, "Full", "the full-mode row (asked because a seed-seated slot exists)");
  await chooseRow(1, "Plates To Cut", "Watch-only (keys)");

  const censusScreen = window.shScreen();
  for (const line of (expect.censusLines || [])) {
    must(censusScreen, line, "the census screen");
  }
  taken.push(await screenShot(shotURL, `c12-census-${form}.png`));
  const claim = censusClaimOf(censusScreen);

  // (20) The engrave loop, and the byte comparison.
  await tap(CONFIRM, 500);
  // "TEXT ONLY alone" (plan row 20a), asserted as ALONE: one row, and neither of
  // the other two literals anywhere on the screen.
  const tail = await runEngraveTail({ shotURL, prefix: `c13-${form}-`,
    variant: { rows: ["TEXT ONLY"], take: 0, forbid: ["QR ONLY", "TEXT + QR"] } });
  taken.push(...tail.shots);
  const flat = compareEngraved(tail.census, claim, expect, `form ${form}`);
  const doorAgain = window.shScreen();
  must(doorAgain, "Keys loaded: 2, plus 1 seed.", "the door after the bundle");

  return {
    arm, form, shots: taken, elapsedSec: Math.round((performance.now() - t0) / 1000),
    stubPages: [stub1.pages.length, stub2.pages.length],
    mappingPages: mapping.pages.length, consentPages: consent.pages.length,
    censusClaim: claim, censusScreen, engraved: flat, variants: tail.variants,
    variantRowsTaken: tail.taken,
    digests: tail.digests, needlesProven: proven,
    matched: {
      digest: expect.digest, templateId: expect.templateId,
      templateStub: expect.templateStub, policyId: expect.policyId,
      policyStub: expect.policyStub, addresses: expect.addresses || [],
      strings: flat,
    },
  };
}
