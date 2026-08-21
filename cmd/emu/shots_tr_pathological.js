// The TAPROOT PATHOLOGICAL journey's walk: gather the constellation's hardest
// wallet over NFC, and read the PROOF the device shows for it.
//
// The wallet is a depth-3 taproot tree — four tiers of a degrading vault, two
// sha256 hashlocks, both timelock flavours, multi_a at 3/2/2/1, NUMS internal
// key, eleven cosigners across three masters. Until F-214 the device could not
// derive a single address for it: every leaf is `and_v(v:…)` wrapping a
// timelock or hashlock, and the tap-leaf DESCRIBER named only pk / multi_a /
// sortedmulti_a.
//
//   const m = await import("./shots_tr_pathological.js");
//   await m.run({ shotURL, md1, expect });
//
// NOT loaded by index.html — it is driven by design/journeys/capture_tr_pathological.py.
//
// WHY THIS WALK EXISTS. Stage 4 gave the device a program that proves which
// wallet a supplied policy is: a named wallet id and derived addresses, on the
// consent path. Every gate on it so far has been a Go test calling into the
// package. This is the other kind of evidence — the emulator, over NFC, with a
// card set the host actually produced.
//
// IT ASSERTS, IT DOES NOT ONLY CAPTURE. `expect` carries the id and the
// addresses the HOST derived for the same policy, and this walk fails if the
// screen disagrees. A journey whose driver only takes pictures records whatever
// happened; the point here is that the device and the host agree across the air
// gap, and that is a comparison or it is nothing.

const sleep = ms => new Promise(r => setTimeout(r, ms));
const squash = s => String(s).replace(/\s+/g, "");

// Device coordinates, shared with walk_verify.js.
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];

// Wallet Policy is the 8th carousel entry (Backup Wallet is 1st), so seven
// right-taps reach it. Derived from gui.go's program enum order, and asserted
// below by title rather than trusted — a miscount would otherwise open the
// neighbouring program and this walk would document that one instead.
const TAPS_TO_WALLET_POLICY = 7;

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

// PAGE THE CONSENT SCREEN. Its lines do not fit one screenful — the id alone
// wraps — so reading only the first frame would miss the addresses entirely,
// which are the half that matters. maxPages is 20 rather than the sibling
// journey's 8: ELEVEN key slots each print a line before the addresses do, so a
// bound tuned for a four-key wallet would stop paging before reaching them and
// the comparison below would fail for the wrong reason. Button2 pages; the screen wraps to the top,
// so this stops when a page repeats rather than counting pages it cannot know.
async function readAllPages(shotURL, prefix, maxPages = 20) {
  const pages = [], names = [];
  for (let i = 0; i < maxPages; i++) {
    const text = squash(window.shScreen());
    if (pages.length && text === pages[0]) break;   // wrapped to the first page
    if (pages.includes(text)) break;                // no forward progress
    pages.push(text);
    names.push(await screenShot(shotURL, `${prefix}${i}.png`));
    await tap([453, 160], 350);                     // Button2 = page
  }
  return { pages, names, joined: pages.join("") };
}

export async function run({ shotURL = "http://127.0.0.1:8732", md1 = [], expect = {} } = {}) {
  if (!md1.length) throw new Error("run({md1}): the walk needs the card set to present");
  const taken = [];

  // (1) Boot.
  //
  // The emulator ships a systemwide payload, so the first screen is the LOAD /
  // SKIP prompt rather than the home screen. This journey's operator has no
  // payload — the wallet policy arrives on cards — so SKIP is the honest
  // action, and Back IS a skip here: gui/sysw_load.go treats !ok and choice 1
  // identically, because backing out of a prompt has never meant "yes" on this
  // device. Handled as a RACE rather than assumed, so the walk also works on a
  // machine with no payload at all.
  const first = await raceFor(["SeedHammer", "systemwide payload is present"]);
  if (first !== "SeedHammer") {
    taken.push(await screenShot(shotURL, "t00a-payload-prompt.png"));
    await tap(BACK);
    await waitFor("SeedHammer");
  }
  taken.push(await screenShot(shotURL, "t00-boot.png"));

  // (2) Reach the program by NAME, not by counting alone.
  for (let i = 0; i < TAPS_TO_WALLET_POLICY; i++) await tap(CAROUSEL_NEXT);
  await waitFor("WalletPolicy");
  taken.push(await screenShot(shotURL, "t01-carousel.png"));

  // (3) Enter it. The gather screen is what it opens on.
  await tap(CONFIRM);
  await waitFor("md1descriptors:0");
  taken.push(await screenShot(shotURL, "t02-gather-empty.png"));

  // (4) Present the card set.
  //
  // THE TALLY COUNTS CARDS, NOT CHUNKS, and there is nothing to wait on in
  // between: a card's intermediate chunks change nothing on screen, and only
  // the last one of a complete set prints "Card added". Waiting per chunk hangs
  // forever on chunk 1 of 8 — measured, that is exactly how this walk first
  // failed. shNFC.present QUEUES, so presenting faster than the flow drains
  // loses nothing.
  //
  // A dropped chunk is still caught, and loudly: the set never completes, so
  // the waitFor below times out rather than the walk carrying on with a short
  // card.
  //
  // SPACES ARE STRIPPED. `md encode` prints codex32 in five-character groups
  // for a human to read off a plate; an NFC record carries the string itself.
  for (const c of md1) {
    window.shNFC.present(String(c).replace(/\s+/g, ""));
    await sleep(60);
  }
  // Scaled to the card set: 24 chunks at ~60 ms each is well inside this, and a
  // fixed 30 s would be a coin flip on a slower machine.
  await waitFor("Cardadded", 30000 + md1.length * 2000);
  const tally = squash(window.shScreen()).match(/md1descriptors:(\d+)/);
  if (!tally || tally[1] !== "1") {
    throw new Error(`the ${md1.length}-chunk set did not assemble into ONE card; ` +
      `screen reads ${JSON.stringify(window.shScreen())}`);
  }
  taken.push(await screenShot(shotURL, "t03-gather-full.png"));

  // (5) Done → the consent screen.
  await tap(CONFIRM);
  await waitFor("Policy-ID");
  const consent = await readAllPages(shotURL, "t04-consent-p");
  taken.push(...consent.names);

  // (6a) ABSENCE ASSERTIONS. The consent screen must not be claiming less than
  // it shows. "display only" would mean F-214 regressed and the device is
  // refusing the leaves again; a no-addresses line would mean the derivation
  // silently produced nothing while the screen still looked plausible.
  for (const forbidden of ["displayonly", "noaddresses", "can'tderive"]) {
    if (consent.joined.toLowerCase().includes(forbidden)) {
      throw new Error(`the consent screen says ${JSON.stringify(forbidden)} — ` +
        `the device is refusing this wallet, not proving it: ${JSON.stringify(consent.joined)}`);
    }
  }

  // (6) THE COMPARISON. The host derived these from the same template and the
  // same keys, through a different implementation in a different language.
  const missing = [];
  if (expect.walletPolicyId && !consent.joined.includes(squash(expect.walletPolicyId))) {
    missing.push(`wallet id ${expect.walletPolicyId}`);
  }
  for (const a of (expect.addresses || [])) {
    if (!consent.joined.includes(squash(a))) missing.push(`address ${a}`);
  }
  if (missing.length) {
    throw new Error(`the device's proof does not match the host's:\n  ` +
      missing.join("\n  ") + `\nconsent screen read: ${JSON.stringify(consent.joined)}`);
  }

  return {
    shots: taken,
    chunksPresented: md1.length,
    cardsGathered: Number(tally[1]),
    consentPages: consent.pages,
    matched: {
      walletPolicyId: expect.walletPolicyId || null,
      addresses: expect.addresses || [],
    },
  };
}
