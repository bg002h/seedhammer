// D3's FIRST HALF on the device: gather a KEYLESS TEMPLATE together with its
// mk1 KEY CARDS, and read the addresses the device seats them into.
//
//   const m = await import("./shots_seating.js");
//   await m.run({ shotURL, md1, keyCards, expect });
//
// The template carries no keys. Each key card declares an origin, and the device
// seats it at the slot whose declared origin matches — never by gather order,
// which is why this walk presents the cards in a deliberately arbitrary order.
//
// It is the "can a user do the thing" check for F-216: every part of seating has
// unit coverage, and none of that proves an operator holding a template and two
// key cards ever sees an address.
//
// NOT loaded by index.html — it is driven by design/journeys/capture_seating.py.
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
// which are the half that matters. Button2 pages; the screen wraps to the top,
// so this stops when a page repeats rather than counting pages it cannot know.
async function readAllPages(shotURL, prefix, maxPages = 8) {
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

export async function run({ shotURL = "http://127.0.0.1:8732", md1 = [], keyCards = [],
                            expect = {}, expectRefusal = null, prefix = "s" } = {}) {
  // SHOT NAMES ARE NAMESPACED BY CALLER. shots/ is one flat directory shared by
  // every journey, so a fixed "s" prefix meant the hashvault walk silently
  // overwrote the seating walk's frames -- two different wallets, one set of
  // filenames, and nothing to tell them apart afterwards. That is the
  // mislabelled-evidence failure this constellation has already had once.
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
    taken.push(await screenShot(shotURL, `${prefix}00a-payload-prompt.png`));
    await tap(BACK);
    await waitFor("SeedHammer");
  }
  taken.push(await screenShot(shotURL, `${prefix}00-boot.png`));

  // (2) Reach the program by NAME, not by counting alone.
  for (let i = 0; i < TAPS_TO_WALLET_POLICY; i++) await tap(CAROUSEL_NEXT);
  await waitFor("WalletPolicy");
  taken.push(await screenShot(shotURL, `${prefix}01-carousel.png`));

  // (3) Enter it. The composer's door is now the first screen in every state
  //     (SPEC_wallet_policy_composer §7a) and opens on "Scan cards".
  await tap(CONFIRM);
  await tap(CONFIRM);
  await waitFor("md1descriptors:0");
  taken.push(await screenShot(shotURL, `${prefix}02-gather-empty.png`));

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
  await waitFor("Cardadded", 30000);
  const tally = squash(window.shScreen()).match(/md1descriptors:(\d+)/);
  if (!tally || tally[1] !== "1") {
    throw new Error(`the ${md1.length}-chunk set did not assemble into ONE card; ` +
      `screen reads ${JSON.stringify(window.shScreen())}`);
  }

  // (4b) THE KEY CARDS. Presented after the template and, within the set, in
  // whatever order the caller passed — seating is by DECLARATION, so the walk
  // deliberately does not sort them. If gather order ever became an input, the
  // addresses below would move and this would fail.
  for (const card of keyCards) {
    for (const c of card) {
      window.shNFC.present(String(c).replace(/\s+/g, ""));
      await sleep(60);
    }
    await waitFor("Cardadded", 20000);
  }
  const keyTally = squash(window.shScreen()).match(/mk1keys:(\d+)/);
  if (keyCards.length && (!keyTally || Number(keyTally[1]) !== keyCards.length)) {
    throw new Error(`gathered ${keyTally ? keyTally[1] : "?"} key cards, expected ` +
      `${keyCards.length}; screen reads ${JSON.stringify(window.shScreen())}`);
  }
  taken.push(await screenShot(shotURL, `${prefix}03-gather-full.png`));

  // (5) Done → the consent screen, or the REFUSAL.
  await tap(CONFIRM);

  // A4's refusal arm. A refusal must be a SENTENCE the operator can act on —
  // the whole reason each seating error has its own — so this asserts the
  // wording, not merely that something went wrong. And it asserts that NO
  // address appears: a refusal that still shows addresses would be worse than
  // either outcome alone.
  if (expectRefusal) {
    const text = await waitFor(expectRefusal, 20000);
    taken.push(await screenShot(shotURL, `${prefix}05-refusal.png`));
    if (/bc1[a-z0-9]{20,}/.test(squash(text))) {
      throw new Error(`the refusal screen also shows an address: ${JSON.stringify(text)}`);
    }
    return { shots: taken, refused: expectRefusal, chunksPresented: md1.length,
             cardsGathered: 1, keyCardsGathered: keyCards.length };
  }
  // "-ID:" and not "Policy-ID": a KEYLESS template's authoritative identity is
  // the key-STABLE Template-ID, and the screen correctly says so. Waiting for
  // the keyed label here timed out against a perfectly good screen that already
  // had the addresses on it.
  await waitFor("-ID:");
  const consent = await readAllPages(shotURL, `${prefix}04-consent-p`);
  taken.push(...consent.names);

  // (6a) THE SEATING MUST HAVE HAPPENED. If the device fell back to
  // "keyless template - no addresses" it has ignored the key cards, which is
  // exactly the pre-F-216 behaviour and exactly what a regression looks like.
  for (const forbidden of ["noaddresses", "Keylesstemplate", "can'tderive"]) {
    if (squash(consent.joined).includes(squash(forbidden))) {
      throw new Error(`the consent screen says ${JSON.stringify(forbidden)} — the key ` +
        `cards were not seated: ${JSON.stringify(consent.joined)}`);
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
    keyCardsGathered: keyCards.length,
    consentPages: consent.pages,
    matched: {
      walletPolicyId: expect.walletPolicyId || null,
      addresses: expect.addresses || [],
    },
  };
}
