// shots_csid_warning.js — the SCREENSHOT half of the device-csid-warning
// cycle: drives the mk1-INSPECT flow with the vendored corpus's pinned
// (mismatched) key card and its clean twin, over simulated NFC, and captures
// the Contract-2 warning modal plus (cheaply, if reached) the Contract-3
// bundle-review-list marker.
//
//   const m = await import("./shots_csid_warning.js");
//   await m.run({ shotURL, pinned: [chunk0, chunk1], clean: [chunk0, chunk1] });
//
// NOT loaded by index.html — driven by
// design/journeys/capture_csid_warning.py, which supplies `pinned` and
// `clean` from mk/testdata/csid_ext_v0.1.json's SEED_pinned_12345_ef12f /
// SEED_plate_b_ef12f rows (never hand-minted — SPEC
// design/SPEC_device_csid_warning.md's own rule for these fixtures).
//
// IT ASSERTS, IT DOES NOT ONLY CAPTURE (shots_seating.js's own rule, applied
// here): the pinned card's second chunk MUST produce the host warning text
// on screen, and the clean twin's second chunk MUST NOT — a walk that only
// takes pictures records whatever happened, and the point of this driver is
// that the device actually warns on the one row and stays silent on the
// other.
//
// PRESENTED AT THE HOME SCREEN, deliberately (not inside an already-open
// gather). gui/gui.go's uiFlow -> StartScreen.Flow loop reads a scan at
// idle and dispatches through engraveObjectFlow -> mdmkFlow, which is the
// mk1 "Inspect key" door a real operator taps a stray key card against —
// the same door gui/mk1_inspect_test.go's TestMdmkFlowMK1ShowsInspect
// exercises in Go, walked here end to end with the real emulator instead.

const sleep = ms => new Promise(r => setTimeout(r, ms));
const squash = s => String(s).replace(/\s+/g, "");

// Device coordinates, shared with shots_seating.js / shots_operator.js —
// fixed geometry across every driver.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

async function waitFor(needle, timeoutMs = 20000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(`waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(text)}`);
    }
    await sleep(50);
  }
}

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

const tap = async ([x, y], settle = 300) => { window.shTap(x, y); await sleep(settle); };

const present = async s => { window.shNFC.present(String(s).replace(/\s+/g, "")); await sleep(60); };

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

// goTo walks the carousel by NAME, not by a hardcoded tap count (shots_operator.js's
// own lesson: "THE CAROUSEL IS COUNTED, NOT ASSERTED" — a fixed index silently
// opens the wrong program the moment an entry is inserted ahead of it).
async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

async function bootToHome(shotURL, taken, prefix) {
  const first = await raceFor(["SeedHammer", "systemwide payload is present"]);
  if (first !== "SeedHammer") {
    await tap(BACK);
    await waitFor("SeedHammer");
  }
  taken.push(await screenShot(shotURL, `${prefix}00-boot.png`));
}

// walkInspect drives ONE key card (two chunk strings) through the mk1
// "Inspect key" door from the home screen, returns to the home screen at the
// end, and returns the squashed text of whatever screen appeared right after
// the second chunk completed the set — either the Contract-2 warning modal
// (mismatch) or the card display (clean).
async function walkInspect(shotURL, taken, prefix, chunks, opts = {}) {
  if (chunks.length !== 2) {
    throw new Error(`walkInspect: expected a 2-chunk mk1 set, got ${chunks.length}`);
  }
  await waitFor("SeedHammer");
  await present(chunks[0]);
  await waitFor("mk1key", 20000); // mdmkFlow's chooser: title "mk1 key", lead "Choose action"
  if (opts.chooserShot) taken.push(await screenShot(shotURL, opts.chooserShot));
  await tap(CONFIRM); // row 0, "Inspect key", is the default selection
  await waitFor("Captured1of2", 20000);
  if (opts.progressShot) taken.push(await screenShot(shotURL, opts.progressShot));
  await present(chunks[1]);
  // Race the two possible next screens: the Contract-2 warning notice
  // (mismatch) or the card display directly (clean twin, no notice).
  const seen = await Promise.race([
    waitFor("wasnotderivedfrom", 20000).then(t => ({ kind: "warning", text: t })),
    waitFor("Network:", 20000).then(t => ({ kind: "display", text: t })),
  ]);
  let modalText = null;
  if (seen.kind === "warning") {
    modalText = seen.text;
    if (opts.modalShot) taken.push(await screenShot(shotURL, opts.modalShot));
    await tap(BACK); // dismiss the non-blocking notice; proceeds to the card
    await waitFor("Network:", 20000);
  }
  if (opts.displayShot) taken.push(await screenShot(shotURL, opts.displayShot));
  // TWO Backs to reach home: mk1DisplayFlow's Back returns to mdmkFlow's OWN
  // chooser loop (gui/gui.go's mdmkFlow re-shows "mk1 key" / "Choose action"
  // rather than exiting), and a SECOND Back there is what leaves mdmkFlow
  // itself, back to the carousel/home screen.
  await tap(BACK); // leave mk1DisplayFlow -> back to the "mk1 key" chooser
  await waitFor("mk1key", 20000);
  await tap(BACK); // leave the chooser -> home
  await waitFor("SeedHammer");
  return { modalText };
}

export async function run({ shotURL = "http://127.0.0.1:8732", pinned = [], clean = [] } = {}) {
  if (pinned.length !== 2) throw new Error("run({pinned}): need exactly 2 mk1 chunk strings (the pinned/mismatched row)");
  if (clean.length !== 2) throw new Error("run({clean}): need exactly 2 mk1 chunk strings (the clean-twin row)");
  const taken = [];

  // (1) Boot to the home screen.
  await bootToHome(shotURL, taken, "csid");

  // (2) The PINNED (mismatched) card: warning must fire.
  const pinnedResult = await walkInspect(shotURL, taken, "csid", pinned, {
    chooserShot: "csid01-chooser.png",
    progressShot: "csid02-gather-1of2.png",
    modalShot: "csid-warning-modal.png", // THE deliverable
    displayShot: "csid03-card-display.png",
  });
  if (!pinnedResult.modalText) {
    throw new Error("the pinned (mismatched) card did NOT show the csid warning modal");
  }

  // (3) The CLEAN TWIN: same key content, matching id — must stay silent.
  const cleanResult = await walkInspect(shotURL, taken, "csid", clean, {
    displayShot: "csid04-clean-card-display.png",
  });
  if (cleanResult.modalText) {
    throw new Error(`the clean twin unexpectedly showed a csid warning: ${JSON.stringify(cleanResult.modalText)}`);
  }

  // (4) BONUS, cheaply: the Contract-3 bundle-review-list marker, via
  // Engrave Bundle. Best-effort — a failure here does not fail the whole
  // capture, since the task only asks for it "if the harness reaches it
  // cheaply"; the required modal shot above has already succeeded.
  let reviewShot = null;
  let reviewError = null;
  try {
    await goTo("Engrave Bundle");
    taken.push(await screenShot(shotURL, "csid05-carousel-bundle.png"));
    await tap(CONFIRM);
    await waitFor("md1descriptors:0");
    for (const c of pinned) await present(c);
    await waitFor("Cardadded", 20000);
    await tap(CONFIRM); // "Done adding cards" -- set completion (Contract 3)
    await waitFor("wasnotderivedfrom", 20000); // the same notice, at set completion
    taken.push(await screenShot(shotURL, "csid06-bundle-modal.png"));
    await tap(BACK); // dismiss; non-blocking, proceeds to the review list
    await waitFor("cardsverified");
    reviewShot = await screenShot(shotURL, "csid-warning-bundle-review.png"); // THE bonus deliverable
    taken.push(reviewShot);
  } catch (e) {
    reviewError = String(e && e.message || e);
  }

  return {
    shots: taken,
    modalText: squash(pinnedResult.modalText),
    cleanSilent: !cleanResult.modalText,
    reviewShot,
    reviewError,
  };
}
