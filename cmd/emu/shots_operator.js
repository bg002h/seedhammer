/**
 * shots_operator.js — the SCREENSHOT half of the operator (5-of-12 wsh) journey.
 *
 * The sibling of shots_pathological.js, and the same F-210 story: the document
 * was committed while the process that made its screenshots was not. This one
 * covers 19 shots across three program visits:
 *
 *   j*  the carousel, and the doors of two programs the journey talks about but
 *       does not walk (Engrave Bundle's gatherer, Backup Wallet's M*1 entry).
 *   k*  Engrave Text, composing a plate: QR, font, size, the masked text field
 *       and its reveal, the confirmation.
 *   p*  the cut -- two framebuffers and four plate overlays, the last taken
 *       after the head has parked.
 *
 * THE CAROUSEL IS COUNTED, NOT ASSERTED. build_pdf.py captioned j00 "the
 * program carousel, 8 entries" and listed eight by name; the carousel has TEN
 * (Load Payload and Sealed Payload arrived later). Rather than correct a number
 * that will drift again, run() returns the entries it actually walked and the
 * builder renders both the count and the list from that.
 *
 * WHAT IS TYPED, and why it is a label. The Engrave Text pages show a text field
 * being filled, masked and revealed; the string itself lived only in the
 * screenshots, and the published PDF's prose never names it, so it could not be
 * recovered. TEXT below is a wallet label -- plain, public, and honest about
 * being a demonstration of the program rather than a recovered artifact.
 *
 * Usage: design/journeys/capture_operator.py drives this.
 *   const m = await import("./shots_operator.js");
 *   await m.run({ shotURL });            // capture
 *   await m.run({ shotURL, measure: true });  // just report the plan's step total
 */

const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
const HOLD_MS = 1300;

const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// THE TEXT KEYBOARD, mapped by probing the live emulator (2026-08-20). It is
// NOT the BIP-39 keyboard: it has four rows, the top one sits at y=150 rather
// than 198, and the fourth is the function row. The x origins happen to match
// the BIP-39 rows, which is why every character typed below is checked --
// a shared origin is a coincidence to verify, not a fact to rely on.
const TEXT_ROWS = [
  { letters: "qwertyuiop", x0: 87, y: 150 },
  { letters: "asdfghjkl", x0: 104, y: 196 },
  { letters: "zxcvbnm", x0: 138, y: 242 },
];
const TEXT_PITCH = 34;
const KEY_SPACE = [175, 290];
const KEY_SHOW = [260, 290];

function textKey(ch) {
  for (const row of TEXT_ROWS) {
    const j = row.letters.indexOf(ch);
    if (j >= 0) return [row.x0 + j * TEXT_PITCH, row.y];
  }
  throw new Error(`no key for ${JSON.stringify(ch)} on the text keyboard`);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

async function tap(p, ms = 300) {
  window.shTap(p[0], p[1]);
  await sleep(ms);
}

async function waitFor(needle, timeoutMs = 20000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (squash(window.shScreen()).includes(want)) return;
    await sleep(60);
  }
  throw new Error(
    `waitFor(${JSON.stringify(needle)}) timed out; screen reads ` +
      JSON.stringify(window.shScreen()));
}

/** The carousel entry name, with the fixed footer stripped. */
const entryName = () =>
  squash(window.shScreen()).replace(/Firmware:.*$/, "");

async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(
    `goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
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
  await post(shotURL, name, c.toDataURL("image/png"));
  return name;
}

/**
 * Rasterise the plate overlay and post it, returning the caption it rendered.
 *
 * The viewBox is in DEVICE UNITS (544000 across), not pixels: using it as a
 * canvas size makes a canvas the browser refuses, `toDataURL` then returns the
 * empty URL "data:," and the receiver writes a zero-byte PNG with a 200 OK.
 * That cost shots_pathological.js four silent empty files, so this rasterises
 * at a fixed pixel size and refuses a data URL that came back too small.
 */
async function plateShot(shotURL, name, px = 1200) {
  const host = document.getElementById("plate-svg");
  const svg = host && host.firstElementChild;
  if (!svg) throw new Error(`plate overlay is not live; cannot capture ${name}`);

  const clone = svg.cloneNode(true);
  clone.setAttribute("width", String(px));
  clone.setAttribute("height", String(px));
  const xml = new XMLSerializer().serializeToString(clone);
  const url = "data:image/svg+xml;base64," + btoa(unescape(encodeURIComponent(xml)));

  const img = new Image();
  await new Promise((res, rej) => {
    img.onload = res;
    img.onerror = () => rej(new Error(`rasterising ${name}: the SVG did not load`));
    img.src = url;
  });

  const cv = document.createElement("canvas");
  cv.width = px;
  cv.height = px;
  const g = cv.getContext("2d");
  g.fillStyle = "#ffffff";
  g.fillRect(0, 0, px, px);
  g.drawImage(img, 0, 0, px, px);

  const data = cv.toDataURL("image/png");
  if (data.length < 1000) {
    throw new Error(`rasterising ${name}: canvas produced ${data.length} bytes of data URL`);
  }
  await post(shotURL, name, data);
  return { name, caption: document.getElementById("plate-caption")?.textContent ?? "" };
}

const steps = () => {
  try { return JSON.parse(window.shToolpath.summary()).steps || 0; } catch (e) { return 0; }
};

/** Wait until the step count stops moving, and return it. */
async function cutTotal(timeoutMs = 120000) {
  const deadline = Date.now() + timeoutMs;
  let last = -1;
  for (;;) {
    await sleep(500);
    const now = steps();
    if (now === last && now > 0) return now;
    last = now;
    if (Date.now() > deadline) return now;
  }
}

// A wallet label: lower case and spaces only, so it needs no page-cycling
// through the ?123 / #+= keyboards. See the header for why this is a label
// rather than a recovered string.
const TEXT = "shibboleth five of twelve";

// Writes between yields during the cut. The emulator starts at 2048 (the walk
// pace); 1 is the human pace and takes about twelve minutes for one plate.
// 128 is measured below to give a cut of about a minute with enough yields to
// sample four distinct stages.
const PACE_FOR_CAPTURE = 128;

export async function run({ shotURL = "http://127.0.0.1:8732", measure = false } = {}) {
  const taken = [];
  const captions = [];
  const shot = async (n) => { taken.push(await screenShot(shotURL, n)); };

  window.shSysw("none");
  await tap(BACK, 500);
  await waitFor("SeedHammer");

  // ─── j: the carousel and two doors ─────────────────────────────────────────
  await goTo("Backup Wallet");
  const entries = [];
  for (let i = 0; i < 14; i++) {
    const name = entryName();
    if (entries.includes(name)) break;
    entries.push(name);
    await tap(CAROUSEL_NEXT, 200);
  }
  await goTo("Backup Wallet");
  if (!measure) await shot("j00-boot.png");

  await goTo("Engrave Bundle");
  if (!measure) await shot("menu-4.png");
  await tap(CONFIRM, 700);
  await waitFor("Engrave Bundle");
  if (!measure) await shot("j06-bundle-enter.png");
  await tap(BACK, 700);
  await waitFor("SeedHammer");

  await goTo("Backup Wallet");
  await tap(CONFIRM, 700);
  await waitFor("Choose number of words");
  if (!measure) await shot("j01-backup-enter.png");

  // The picker has FIVE rows: 12 WORDS, 24 WORDS, M*1 STRING, SLIP-39, SEEDXOR.
  // M*1 STRING is row 2, and the shot is of it SELECTED, before confirming.
  await tap([240, rowY(2, 5)], 400);
  if (!measure) await shot("j02-sel-mstring.png");
  await tap(CONFIRM, 700);
  await waitFor("Input m*1 string");
  if (!measure) await shot("j03-mstring-prompt.png");
  await tap(BACK, 500);
  for (let i = 0; i < 4 && !squash(window.shScreen()).includes("SeedHammer"); i++) {
    await tap(BACK, 500);
  }
  await waitFor("SeedHammer");

  // ─── k: Engrave Text ───────────────────────────────────────────────────────
  await goTo("Engrave Text");
  await tap(CONFIRM, 700);
  await waitFor("QR Code");
  if (!measure) await shot("k01-engrave-text-enter.png");

  // Add QR is row 1 of 2 -- the journey's plate carries one, and the plate
  // overlay pages talk about cutting into it.
  await tap([240, rowY(1, 2)], 300);
  await tap(CONFIRM, 700);
  await waitFor("Font");
  if (!measure) await shot("k04-font.png");

  await tap(CONFIRM, 700);          // sh, row 0
  await waitFor("Size");
  if (!measure) await shot("k06-size.png");

  await tap(CONFIRM, 700);          // Auto-fit, row 0
  await waitFor("lines");

  for (const ch of TEXT) {
    if (ch === " ") { await tap(KEY_SPACE, 120); continue; }
    await tap(textKey(ch), 120);
  }
  if (!measure) await shot("k08-typed.png");

  // The field masks by default; this is the reveal the document shows beside it.
  await tap(KEY_SHOW, 400);
  const revealed = squash(window.shScreen());
  if (!revealed.includes(squash(TEXT))) {
    throw new Error(
      `the revealed field does not read ${JSON.stringify(TEXT)}; screen reads ` +
      JSON.stringify(window.shScreen()));
  }
  if (!measure) await shot("k09-revealed.png");

  // TITLE AND FOOTER ARE BOTH OPTIONAL FIELDS, and both are skipped -- the
  // shot after them is named "k13-after-footer" for a reason. Skipping is a
  // confirm on an empty keyboard, so the needles below are what prove the flow
  // moved rather than that a confirm was accepted somewhere.
  await tap(CONFIRM, 900);
  await waitFor("optional");        // Title
  await tap(CONFIRM, 900);
  await waitFor("Footer");
  await tap(CONFIRM, 900);

  // The confirmation, and the sentence the document quotes from it.
  await waitFor("Nothing here is checked");
  if (!measure) await shot("k13-after-footer.png");

  await tap(CONFIRM, 900);
  await waitFor("Hold button to start");
  if (!measure) await shot("k14-engrave-start.png");

  // ─── p: the cut ────────────────────────────────────────────────────────────
  //
  // SLOWED FOR THE CAMERA, not for the machine. `steps()` only advances when the
  // engraver yields, and at the default walk pace (2048 writes per yield) this
  // short text plate yields so rarely that every sample landed past 54% -- four
  // frames bunched at the end of a cut whose start nobody saw. A lower pace
  // yields more often, so progress is visible at a finer grain.
  //
  // It changes WHEN the emulator hands control back, not what it cuts: the step
  // stream and its total are identical, which is why PLAN_STEPS below is
  // unaffected by this line.
  const pacedFrom = window.shPace();
  window.shPace(PACE_FOR_CAPTURE);

  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);
  window.shRelease(...CONFIRM);

  if (measure) {
    const total = await cutTotal();
    return { measure: true, planSteps: total, entries };
  }

  // Sampled on cut PROGRESS, not wall clock: time-based sampling produced two
  // byte-identical plate images in the pathological journey, which would have
  // put one picture under two captions.
  // MEASURED, not guessed: `run({ measure: true })` above cuts the same plate
  // and reports its step total, which is how this number was obtained and how
  // it should be renewed if the toolpath ever changes.
  const PLAN_STEPS = 45620139;
  // THE STEP STREAM ARRIVES IN BURSTS, so a threshold is a floor and not an
  // appointment: the recorder can jump from 0 to a third of the plate between
  // two polls. Measured on this plate, targets of 15/40/70% produced samples at
  // 51/61/91% -- all past their mark and bunched at the end. The targets are
  // lower and the poll is finer so the four images span the cut; what each one
  // actually caught is reported in its caption rather than assumed.
  const STAGES = [
    { frac: 0.05, plate: "p0-plate.png" },
    { frac: 0.35, plate: "p2-plate.png", screen: "p2-screen.png" },
    { frac: 0.70, plate: "p5-plate.png" },
  ];


  let last = -1;
  for (const st of STAGES) {
    const want = Math.round(PLAN_STEPS * st.frac);
    const deadline = Date.now() + 120000;
    while (steps() < want) {
      if (Date.now() > deadline) {
        throw new Error(
          `the cut never reached ${Math.round(st.frac * 100)}% (${want} steps); it ` +
          `stalled at ${steps()}. Re-measure PLAN_STEPS with { measure: true } ` +
          `rather than lowering the threshold.`);
      }
      await sleep(40);
    }
    const seen = steps();
    if (seen <= last) {
      throw new Error(`stage ${st.plate} sampled at ${seen}, not past ${last}`);
    }
    last = seen;
    if (st.screen) await shot(st.screen);
    const c = await plateShot(shotURL, st.plate);
    c.steps = seen;
    c.percent = Math.round((seen / PLAN_STEPS) * 100);
    captions.push(c);
    taken.push(c.name);
  }

  // THE LAST FRAME IS AFTER THE HEAD PARKS. The document's caption for it is
  // "complete -- head parked at 0.0, 0.0", and the page beside it explains that
  // the job ends by homing so plan and progress share one coordinate space.
  // Capturing it before the cut settles would quietly falsify both.
  const total = await cutTotal();
  const done = await plateShot(shotURL, "p11-plate.png");
  done.steps = total;
  done.percent = 100;
  captions.push(done);
  taken.push(done.name);

  await waitFor("completed");
  await shot("p17-settled.png");

  const drift = Math.abs(total - PLAN_STEPS) / PLAN_STEPS;
  if (drift > 0.02) {
    throw new Error(
      `the cut totalled ${total} steps, ${(drift * 100).toFixed(1)}% off PLAN_STEPS ` +
      `(${PLAN_STEPS}); the toolpath changed, so the sampled stages are no longer ` +
      `15/40/70% of anything. Re-measure and re-check the captions.`);
  }

  window.shPace(pacedFrom);
  return { ok: taken.length === 19, taken, captions, entries, planSteps: total,
           pace: PACE_FOR_CAPTURE, text: TEXT, screen: window.shScreen() };
}
