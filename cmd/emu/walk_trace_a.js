// Trace A as a script: load the cosigner payload, gather three cosigner cards
// over NFC, engrave the bundle, and return the census of what was cut.
//
//   const w = await import("./walk_trace_a.js");
//   await w.run();                       // ~4 minutes at the default pace
//
// NOT loaded by index.html, deliberately: this drives the machine, and a page
// that starts engraving because somebody opened it is a trap.
//
// WHY THIS FILE EXISTS. §4.5 makes an automated walk the closing gate of every
// stage, and the first walk that ever reached a completed engrave lived only in
// a session's console. A gate that has to be re-derived is one nobody runs --
// and re-deriving this one cost an afternoon, most of it spent on the three
// traps below, each of which is a wrong answer that looks right.
//
// TRAP 1 -- THE SCREEN IS NOT A PROGRESS SIGNAL DURING A CUT. The harness
// documents shWaitFor (poll the TEXT) as the way to wait. During an engrave the
// text is stale: measured, 0 frames drawn across 6s while 1,020,312 steps
// executed, with every page timer cleared so the browser could not be the
// cause. The countdown sits frozen for most of a plate. A walk waiting for
// "Engraving completed successfully" therefore times out against a perfectly
// healthy machine. So progress is read from shToolpath, and the screen is
// consulted only after motion STALLS -- with a tap first, to force a redraw.
// See F-161: whether the device shares this is an open question needing
// hardware.
//
// TRAP 2 -- THE BROWSER CACHES emu.wasm, and a cache-buster on index.html does
// not bust it. Serve on a fresh port and check a symbol only the new build has
// before believing any negative result. run() checks shPace for exactly that.
//
// TRAP 3 -- shSysw("cards") MUST precede a load from the MENU, because the boot
// offer has already consumed the first payload read. Switching at boot is too
// late. The digest tells you which blob you actually got, so run() asserts it
// rather than trusting the call.
//
// The engraved strings are the payload's own chunks, byte for byte, which is
// what makes this a gate rather than a demo: compare run()'s census against
// `go run ./cmd/buildpayloadcards`.

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");

// HOLD_MS is the one wait here that cannot be replaced by a condition:
// gui.confirmDelay (gui/gui.go) is ONE SECOND of wall clock and releasing early
// aborts the gesture, so this is that second plus margin. The margin is not
// generosity -- raising the pace slows the GUI's own refresh (2.0 frames/s at
// pace 1, 1.4 at 2048, 0.87 at 4096, measured), so the loop that watches the
// press runs less often at exactly the paces a walk uses.
const HOLD_MS = 1300;
// Ticks of no motion that mean "idle, not cutting" rather than "between
// batches". Three at the poll interval below.
const STALL_TICKS = 3;

// The cards payload's digest, as sysw_cards_payload.go pins it. The records
// blob is 55adb800...; getting the other one is the failure this catches.
export const CARDS_DIGEST = "25271e583f3eaa03ae18f359c72b76e3";

// Trace A's cosigner set: three cards, two chunks each, in gather order. Every
// mk1 carrying an xpub is >= 2 chunks, and a card is added only once its whole
// chunk-set has crossed the reader. Regenerate with
// `go run ./cmd/buildpayloadcards`; these are BIP-39's published vectors, so
// they are public by construction. Never put funds behind them.
export const CARDS = [
  ["A@0",
   "mk1qpd8cwpqqsq4kj90x4eutks2q5zg3vs7rnefw94m5rru59s2su80aw2q4wgdpapgfl4pkhsdyytkwl5z8lphut2hvvpp5av5muuc0cmfrjw2",
   "mk1qpd8cwpp806lhaeh6reknylagmwyjycf8044xtt9flsdlkvt6f6cthyl995lpm5zlrp6yv6kc36tw"],
  ["B@0",
   "mk1qpmm93pqqsq4kj90xkux3r03q5zg3vs7llvu2xd8x2rk7av9gmew82jq5zap9302ynhp37ggd6z5u4emag0zr8gh9upnj5stuxqtzdn4uxkd",
   "mk1qpmm93ppwyp4dfykwfkgg6fxyxetdcmythf4hsqzd3v879jprztejzs7rl0vk8mlv7lzva7w6nage"],
  ["C@0",
   "mk1qp073xpqqsq4kj90x55xg5qxq5zg3vs7yxvkgrq39p0l68ewgkud5t4n95k8j84204m9xvr7cunrjqfurja893lkxup4zlyyf7spk75exfku",
   "mk1qp073xpp09hdq9zxg25mzsetnyhhc2j7yvnr37vchl8xc05vf2pnnz46ffl6ymp2sa0x397e56ypk"],
];

// Device coordinates (480x320). Nav buttons sit on the RIGHT edge.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
// A corner holding no widget, tapped only to force a redraw. Note this does NOT
// prove a tap missed: the GUI redraws on any pointer event, so a tap on empty
// screen draws frames too.
const NOWHERE = [5, 5];

async function waitFor(needle, timeoutMs = 10000) {
  const want = squash(needle);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const text = window.shScreen();
    if (squash(text).includes(want)) return text;
    if (Date.now() >= deadline) {
      throw new Error(
        `waitFor(${JSON.stringify(needle)}) timed out after ${timeoutMs}ms; ` +
        `screen reads ${JSON.stringify(text)} at frame ${window.shScreenSeq()}`);
    }
    await sleep(50);
  }
}

const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };

// Spin the start carousel until the named program is under the cursor. Bounded
// so a rename fails here, loudly, instead of looping forever.
async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}

// The screens this walk knows how to answer. Anything else stops the walk --
// a driver that taps past what it does not recognise manufactures a pass.
const HANDLERS = [
  { name: "engrave-prompt", match: "Holdbuttontostarttheengravingprocess", act: "hold" },
  { name: "engrave-done", match: "Engravingcompletedsuccessfully", act: "confirm" },
  { name: "choose-variant", match: "Chooseengraving", act: "confirm" },
];

/**
 * Drive Trace A end to end.
 *
 * @param {object}  [opts]
 * @param {number}  [opts.pace=2048]    Writes between yields; see cmd/emu/pace.go.
 *        2048 measured 186s for this bundle against 236s at 512, and 8192 buys
 *        only three seconds more -- past here the driver's own sleeps dominate.
 * @param {number}  [opts.plates=6]     Stop once this many plates are engraved.
 * @param {boolean} [opts.perPlateDigest=false] Record each plate's toolpath
 *        digest as it completes. The recording resets per plate, so this is the
 *        only moment a plate's digest can be read.
 * @param {number}  [opts.pollMs=75]     How often to sample progress.
 * @param {number}  [opts.settleMs=150]  Pause after the redraw tap before reading.
 * @param {number}  [opts.chunkGapMs=150] Gap between queued NFC chunks.
 *        These three are the driver's own overhead, which dominates the walk
 *        once the pace is past ~2048 -- so on a big bundle they are the lever,
 *        not the pace. Lower them and the walk gets faster right up to the
 *        point it starts missing screens; the defaults were measured, not
 *        guessed at.
 * @returns {Promise<object>} census, digests, acts and elapsed time.
 */
export async function run({ pace = 2048, plates = 6, perPlateDigest = false,
                            pollMs = 75, settleMs = 150, chunkGapMs = 150 } = {}) {
  if (typeof window.shPace !== "function") {
    throw new Error("shPace is missing -- this is a STALE emu.wasm. " +
      "The browser caches it and a cache-buster on index.html does not help; serve on a fresh port.");
  }
  const t0 = performance.now();
  const acts = [];
  const digests = [];

  window.shPace(pace);
  window.shSysw("cards");                 // must precede the menu load; the boot offer ate the first read
  await tap(BACK);                        // Back == skip, per gui/sysw_load.go
  await waitFor("SeedHammer");

  await goTo("LoadPayload");
  await tap(CONFIRM);
  await waitFor("PayloadDigest");
  const digest = /([0-9a-f]{32})/.exec(squash(window.shScreen()))?.[1];
  if (digest !== CARDS_DIGEST) {
    throw new Error(`loaded the wrong payload: digest ${digest}, want ${CARDS_DIGEST} (the cards blob)`);
  }
  await tap(CONFIRM); await waitFor("PayloadWarnings");
  await tap(CONFIRM); await waitFor("Keepthispayloadloaded?");
  await tap(CONFIRM); await waitFor("SeedHammer");        // KEEP is choice 0

  await goTo("EngraveBundle");
  await tap(CONFIRM); await waitFor("Firstcardfromwhere?");
  // FROM PAYLOAD seeds ONE CHUNK, not a whole card, so Done drops it and the
  // gather starts clean. Confirmed by the message the drop produces.
  await tap(CONFIRM); await waitFor("Scanacard,orDone");
  await tap(CONFIRM); await waitFor("Droppedanincompletecard");
  await tap(CONFIRM); await waitFor("Scanacard,orDone");

  // shNFC.present QUEUES, so every chunk crosses the one reader the screen
  // fetched at entry. Before the queue existed no bundle gather could finish.
  //
  // The delay between chunks is deliberately small. A card's INTERMEDIATE
  // chunks produce no visible change -- only the last one prints "Card added"
  // -- so there is nothing to wait ON until the set completes, and the queue
  // means nothing is lost by presenting faster than the flow drains. The real
  // check is the waitFor below: if a chunk were dropped the card never
  // completes and this fails loudly rather than engraving a short bundle.
  const gathered = [];
  for (const [name, ...chunks] of CARDS) {
    for (const c of chunks) { window.shNFC.present(c); await sleep(chunkGapMs); }
    await waitFor("Cardadded");
    gathered.push({ card: name, screen: window.shScreen() });
  }

  await tap(CONFIRM); await waitFor("cardsverified");
  await tap(CONFIRM); await waitFor("Chooseengraving");

  // The cut loop. Progress comes from the toolpath, never the screen -- trap 1.
  //
  // Every wait here is on an OBSERVABLE condition rather than a guessed
  // duration, except the hold, which cannot be: gui.confirmDelay is one second
  // of wall clock and releasing early aborts the gesture.
  let stall = 0, lastSteps = -1;
  for (let guard = 0; guard < 20000; guard++) {
    const census = JSON.parse(window.shToolpath.strings());
    if (census.strings.length >= plates) break;
    const steps = JSON.parse(window.shToolpath.summary()).steps;
    if (steps === lastSteps) stall++; else { stall = 0; lastSteps = steps; }
    if (stall < STALL_TICKS) { await sleep(pollMs); continue; }

    await tap(NOWHERE, settleMs);                  // force a redraw before reading
    const screen = squash(window.shScreen());
    const h = HANDLERS.find((h) => screen.includes(h.match));
    if (!h) {
      acts.push({ act: "STALLED", screen: screen.slice(0, 80) });
      break;
    }
    if (perPlateDigest && h.name === "engrave-done") {
      digests.push(JSON.parse(window.shToolpath.summary()).digest);
    }
    acts.push({ act: h.act, screen: h.name });
    if (h.act === "hold") {
      window.shPress(...CONFIRM);
      await sleep(HOLD_MS);                        // > gui.confirmDelay, which is 1s
      window.shRelease(...CONFIRM);
      // Wait for MOTION, not for a screen: the cut starting is the thing that
      // proves the hold registered, and it is visible in the toolpath at once.
      const before = JSON.parse(window.shToolpath.summary()).steps;
      for (let i = 0; i < 100; i++) {
        if (JSON.parse(window.shToolpath.summary()).steps !== before) break;
        await sleep(pollMs);
      }
    } else {
      window.shTap(...CONFIRM);
      // Wait for the screen to actually leave the one just answered, instead of
      // sleeping long enough that it probably has.
      for (let i = 0; i < 100; i++) {
        window.shTap(...NOWHERE);
        await sleep(pollMs);
        if (!squash(window.shScreen()).includes(h.match)) break;
      }
    }
    stall = 0; lastSteps = -1;
  }

  const census = JSON.parse(window.shToolpath.strings());
  return {
    pace,
    elapsedSec: Math.round((performance.now() - t0) / 1000),
    census,
    digests,
    gathered,
    acts,
    screen: window.shScreen(),
    // unattributed > 0 means something was cut that this census cannot NAME
    // (F-160). It is a gate failure, not noise.
    ok: census.strings.length === plates && census.unattributed === 0,
  };
}
