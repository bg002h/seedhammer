/**
 * shots_pathological.js — the SCREENSHOT half of the pathological-wallet
 * journey, as a committed driver.
 *
 * WHY THIS FILE EXISTS (F-210). `design/journeys/README.md` documents
 * `shot_server.py` as "the receiver the emulator page POSTs canvas.toDataURL()
 * frames to" — and until this file, NOTHING POSTED THEM. The capture was ad-hoc
 * console code in a session that no longer exists, so the journey's PDFs were
 * committed while the process that produced their screenshots was not. That is
 * the same decay as the transcripts' missing intermediates, one layer out: the
 * artifact vouches, and the proof of it was never in git.
 *
 * The transcripts now regenerate byte-identically. These screenshots are what
 * is left, and the builder REFUSES to emit a PDF without them — correctly, so a
 * draft can never be mistaken for the document.
 *
 * WHAT IT CAPTURES, and why the plate shots are not screenshots at all:
 *
 *   a*  the device framebuffer, from the 480x320 canvas — the seed typed by
 *       hand, which is the whole point of this journey (no NFC anywhere).
 *   b*  two kinds. `b<N>-screen` is the framebuffer mid-cut. `b<N>-plate` is
 *       the PLATE OVERLAY, which is #plate-svg — deliberately OUTSIDE the
 *       canvas, because cmd/emu adds no screens the machine does not have. It
 *       is rasterised here rather than posted as SVG so the PDF can embed it.
 *
 * THE PLATE CAPTIONS ARE DATA, NOT PROSE. `#plate-caption` already renders
 * `head X,Ymm` from the live toolpath summary, and build_pdf_pathological.py
 * hardcoded those numbers as text typed beside the image. A caption that is
 * typed can drift from the picture it labels while both look fine; one that is
 * read at capture time cannot. run() returns them, and the runner writes them
 * next to the PNGs.
 *
 * Usage (see design/journeys/capture_pathological.py, which does all of it):
 *   const m = await import("./shots_pathological.js");
 *   const captions = await m.run({ shotURL: "http://127.0.0.1:8732" });
 */

const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// gui.confirmDelay is ONE SECOND of wall clock and releasing early aborts the
// gesture. Measured in walk_trace_a.js; reused rather than re-derived.
const HOLD_MS = 1300;

// ChoiceScreen rows, measured in walk_build_policy.js and reused verbatim.
const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

// The BIP-39 keyboard. gui/keyboard_geometry_test.go recomputes these same tap
// points and asserts op.Drawer.Hit lands on the matching key, so they are
// machine-checked rather than remembered.
const KEY_PITCH = 34;
const KEY_ROWS = [
  { letters: "qwertyuiop", x0: 87, y: 198 },
  { letters: "asdfghjkl", x0: 104, y: 244 },
  { letters: "zxcvbnm", x0: 138, y: 290 },
];

function keyPoint(ch) {
  for (const row of KEY_ROWS) {
    const j = row.letters.indexOf(ch);
    if (j >= 0) return [row.x0 + j * KEY_PITCH, row.y];
  }
  throw new Error(`no key for ${JSON.stringify(ch)} on the BIP-39 keyboard`);
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

async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(
    `goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

/**
 * Type one BIP-39 word, checking EVERY letter.
 *
 * The word line renders as "%2d: <fragment or completed word>", so after letter
 * i the squashed screen must contain `<n>:<PREFIX>` — true both before and
 * after auto-completion, since the completed word starts with the prefix. A
 * drifted coordinate therefore fails on the first letter, naming the word,
 * rather than silently entering a seed nobody chose.
 */
async function typeLetters(word, n, upto) {
  for (let i = 0; i < upto; i++) {
    await tap(keyPoint(word[i]), 110);
    const want = `${n}:${word.slice(0, i + 1).toUpperCase()}`;
    if (!squash(window.shScreen()).includes(want)) {
      throw new Error(
        `typing word ${n} (${word}): after letter ${i + 1} (${word[i]}) the screen does ` +
          `not show ${JSON.stringify(want)}; it reads ${JSON.stringify(window.shScreen())}`);
    }
  }
}

/** POST one data URL to shot_server.py under `name`. */
async function post(shotURL, name, dataURL) {
  const r = await fetch(shotURL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, png: dataURL }),
  });
  if (!r.ok) throw new Error(`shot ${name}: server said ${r.status} ${await r.text()}`);
}

/** Capture the device framebuffer exactly — the canvas, not a cropped window. */
async function screenShot(shotURL, name) {
  const c = document.getElementById("screen");
  if (!c) throw new Error("no #screen canvas");
  await post(shotURL, name, c.toDataURL("image/png"));
  return name;
}

/**
 * Capture the plate overlay as a PNG, and return the caption rendered with it.
 *
 * The overlay is an SVG, and the PDF embeds PNGs, so it is rasterised through
 * an offscreen canvas. The SVG is serialised and inlined as a data URL rather
 * than fetched, because the page is served from a plain http.server and an
 * <img> pointing at a blob would be one more thing to keep alive.
 */
async function plateShot(shotURL, name, px = 1200) {
  const host = document.getElementById("plate-svg");
  const svg = host && host.firstElementChild;
  if (!svg) throw new Error(`plate overlay is not live; cannot capture ${name}`);

  // THE viewBox IS IN DEVICE UNITS, NOT PIXELS. It measures 544000x544000 --
  // the toolpath's own coordinate space. The first version of this used those
  // numbers as the canvas size, which makes a canvas ~1,088,000px wide; the
  // browser rejects it, `toDataURL` then returns the empty URL "data:,", and
  // shot_server.py faithfully writes that as a ZERO-BYTE PNG. No error is
  // raised anywhere along that path, so the driver reported ok:true for four
  // empty files. Rasterise at a fixed pixel size and let the viewBox scale.
  //
  // The size is set on a CLONE before serialising: an SVG with no width/height
  // has an intrinsic size of 150x150, and drawing that into a large rect
  // upscales a 150px raster instead of rendering at the target resolution.
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
  // The plate is drawn dark-on-light in the PDF; without this the transparent
  // SVG background rasterises to black and the black cut strokes vanish.
  g.fillStyle = "#ffffff";
  g.fillRect(0, 0, px, px);
  g.drawImage(img, 0, 0, px, px);

  const data = cv.toDataURL("image/png");
  // A capture that produced no bytes is a FAILURE, said here rather than
  // discovered later as a 0-byte file that every downstream step accepts.
  if (data.length < 1000) {
    throw new Error(`rasterising ${name}: canvas produced ${data.length} bytes of data URL`);
  }
  await post(shotURL, name, data);
  const caption = document.getElementById("plate-caption")?.textContent ?? "";
  return { name, caption };
}

export async function run({ shotURL = "http://127.0.0.1:8732" } = {}) {
  const captions = [];
  const taken = [];

  // NO PAYLOAD. This journey's premise is that the seed goes in by hand, so a
  // loaded payload would offer a seed source the document says it never used.
  // The boot offer is already on screen before any script runs, so it is
  // dismissed rather than answered (walk_trace_b.js records the same ordering).
  window.shSysw("none");
  await tap(BACK, 500);
  await waitFor("SeedHammer");

  await goTo("Backup Wallet");
  taken.push(await screenShot(shotURL, "a00-boot.png"));

  await tap(CONFIRM, 600);
  await waitFor("Choose number of words");
  taken.push(await screenShot(shotURL, "a01-input-seed.png"));

  // 12 WORDS IS ALREADY SELECTED, so this confirms rather than picks a row.
  //
  // The picker has FIVE choices here -- 12 WORDS, 24 WORDS, M*1 STRING,
  // SLIP-39, SEEDXOR -- not the two a seed-length picker suggests. Computing a
  // row for the wrong count is how the first run of this driver ended up typing
  // into "Input m*1 string" with a digits keyboard, three screens from where it
  // thought it was. Confirming the default needs no count at all, and the
  // post-condition below is what makes that safe.
  await tap(CONFIRM, 500);
  await waitFor("Word 1 of 12");
  taken.push(await screenShot(shotURL, "a02-word-entry.png"));

  const PHRASE =
    "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";
  const words = PHRASE.split(" ");

  // THE POINT OF a03: three letters is enough. Type "aba", capture the screen
  // showing the single match auto-completed, and only then accept the word.
  await typeLetters(words[0], 1, 3);
  taken.push(await screenShot(shotURL, "a03-typing-aba.png"));
  await tap(CONFIRM, 400);
  // CAPITAL W, and squash() does not fold case. This program's counter reads
  // "Word 1 of 12"; the multisig path's reads "@0 word 1 of 12", which is where
  // the lower-case spelling in the walk drivers comes from. Reusing theirs here
  // waits forever on a screen that is already correct.
  await waitFor("Word 2 of 12");

  for (let i = 1; i < words.length; i++) {
    await typeLetters(words[i], i + 1, words[i].length);
    await tap(CONFIRM, 400);
    if (i < words.length - 1) await waitFor(`Word ${i + 2} of 12`);
  }

  // THE THREE SCREENS AFTER THE LAST WORD, asserted one at a time rather than
  // walked with a bounded "tap CONFIRM until something matches" loop. Mashing
  // confirm through unknown screens is how a journey ends up documenting a flow
  // nobody read -- and on this program the third screen is the one that starts
  // a cut, so an extra confirm is not a harmless retry.
  //
  //   seed review  -- all 12 words, titled "Engrave Seed"
  //   passphrase   -- "Add a BIP-39 passphrase?", Skip is the default
  //   engrave      -- "Insert a blank plate ... Hold button to start"
  await sleep(600);
  await waitFor("Engrave Seed");
  taken.push(await screenShot(shotURL, "a05-after-seed.png"));

  await tap(CONFIRM, 700);
  await waitFor("Add a BIP-39 passphrase?");
  taken.push(await screenShot(shotURL, "a06-after-seed-confirm.png"));

  await tap(CONFIRM, 700);   // Skip -- this journey's seed carries no passphrase
  await waitFor("Hold button to start");
  taken.push(await screenShot(shotURL, "a07-after-passphrase.png"));

  // ─── the cut ───────────────────────────────────────────────────────────────
  window.shPress(...CONFIRM);
  await sleep(HOLD_MS);
  window.shRelease(...CONFIRM);

  // SAMPLED BY CUT PROGRESS, NOT BY WALL CLOCK.
  //
  // The first version slept a fixed number of seconds between samples, and
  // b6-plate and b8-plate came out BYTE-IDENTICAL: the later samples landed
  // after the cut had finished, so the document would have carried the same
  // picture twice under two different captions. In a journey whose README opens
  // "Nothing in these documents is illustrative", two stages that are secretly
  // one is precisely the defect to avoid.
  //
  // `shToolpath.summary().steps` is the recorded step count so far, so
  // thresholds on it are thresholds on the cut itself. Measured on this plate
  // (12 words, master A): the cut runs ~28s and totals PLAN_STEPS steps.
  const PLAN_STEPS = 189557188;
  const PLAN_TOLERANCE = 0.02;

  const steps = () => {
    try { return JSON.parse(window.shToolpath.summary()).steps || 0; } catch (e) { return 0; }
  };
  const at = (frac) => Math.round(PLAN_STEPS * frac);

  // Each stage names the plate shot it takes and, where the PDF shows one, the
  // framebuffer beside it.
  const STAGES = [
    { frac: 0.15, plate: "b0-plate.png", screen: "b1-screen.png" },
    { frac: 0.40, plate: "b3-plate.png" },
    { frac: 0.65, plate: "b6-plate.png", screen: "b6-screen.png" },
    { frac: 0.90, plate: "b8-plate.png" },
  ];

  let last = -1;
  for (const st of STAGES) {
    const want = at(st.frac);
    const deadline = Date.now() + 90000;
    for (;;) {
      const now = steps();
      if (now >= want) break;
      if (Date.now() > deadline) {
        throw new Error(
          `the cut never reached ${Math.round(st.frac * 100)}% (${want} steps); ` +
          `it stalled at ${now}. If the plan legitimately changed, re-measure ` +
          `PLAN_STEPS -- do not lower the threshold.`);
      }
      await sleep(120);
    }
    const seen = steps();
    if (seen <= last) {
      throw new Error(
        `stage ${st.plate} sampled at ${seen} steps, not past the previous ${last}: ` +
        `two stages would show the same cut`);
    }
    last = seen;
    if (st.screen) taken.push(await screenShot(shotURL, st.screen));
    const c = await plateShot(shotURL, st.plate);
    c.steps = seen;
    c.percent = Math.round((seen / PLAN_STEPS) * 100);
    captions.push(c);
    taken.push(c.name);
  }

  // THE PLAN ITSELF IS CHECKED, once the cut is done. If a firmware change moves
  // the toolpath, the four stages above are no longer 15/40/65/90% of anything
  // and their captions describe a plate that is not this one -- so say so here
  // rather than letting the PDF rebuild quietly around a different cut.
  const doneBy = Date.now() + 90000;
  let total = steps();
  for (;;) {
    await sleep(400);
    const now = steps();
    if (now === total) break;
    total = now;
    if (Date.now() > doneBy) break;
  }
  const drift = Math.abs(total - PLAN_STEPS) / PLAN_STEPS;
  if (drift > PLAN_TOLERANCE) {
    throw new Error(
      `the cut totalled ${total} steps, ${(drift * 100).toFixed(1)}% off the ` +
      `PLAN_STEPS this driver samples against (${PLAN_STEPS}). The toolpath ` +
      `changed; re-measure and update the stages, and re-check the captions.`);
  }

  return { ok: taken.length === 13, taken, captions, planSteps: total, screen: window.shScreen() };
}
