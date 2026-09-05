// H0 acceptance walk (IMPLEMENTATION_PLAN_hashlock_H0_reader_guards.md Task 3
// Step 2): both direct doors, fed a kind-0x03 hashlock PREIMAGE plate.
//
//   const w = await import("./walk_h0_preimage.js");
//   await w.run();
//
// NOT loaded by index.html: this drives the machine. It cuts nothing -- the
// whole point is that neither door reaches an engrave screen -- but a page
// that starts driving because somebody opened it is still a trap.
//
// Door 1, typed: Backup Wallet -> Input Seed -> M*1 STRING -> type the plate ->
//   expect the named refusal ("Hashlock preimage"), never "Confirm Codex32
//   Secret" / "EngraveSeed" / "Engrave Plate".
// Door 2, NFC: present the plate over the emulator's reader at the start screen
//   -> expect "Unknown format" (Scan mirrors seal.Classify and classifies the
//   plate as no known object), never a confirm screen.
//
// Helpers are inlined from walk_verify.js / walk_s4_gate.js (they are not
// exported there); MS1_KEY_ROWS was mapped by probing the live emulator on
// 2026-08-19 (walk_verify.js:531-592).

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const squash = (s) => String(s).replace(/\s+/g, "");
const BACK = [453, 70];
const CONFIRM = [453, 249];
const CAROUSEL_NEXT = [455, 160];
const rowY = (i, n) => 160 - (n - 1) * 12 + i * 24;

const PLATE = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c";
const REFUSAL = "Hashlock preimage";            // gui/codex32_polish.go engraveCodex32
const FORBIDDEN = ["Confirm Codex32 Secret", "EngraveSeed", "Engrave Plate", "Unshared secret"];

const MS1_KEY_ROWS = [
  { chars: "1234567890", x0: 87, y: 152 },
  { chars: "qwertyuiop", x0: 87, y: 198 },
  { chars: "asdfghjkl", x0: 104, y: 244 },
  { chars: "zxcvbnm", x0: 138, y: 290 },
];
const MS1_KEY_PITCH = 34;
function ms1KeyPoint(ch) {
  const c = ch.toLowerCase();
  for (const row of MS1_KEY_ROWS) {
    const j = row.chars.indexOf(c);
    if (j >= 0) return [row.x0 + j * MS1_KEY_PITCH, row.y];
  }
  throw new Error(`no key for ${JSON.stringify(ch)} on the ms1 keyboard`);
}

async function waitFor(needle, timeoutMs = 15000) {
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
const tap = async ([x, y], settle = 250) => { window.shTap(x, y); await sleep(settle); };
async function goTo(program, max = 14) {
  const want = squash(program);
  for (let i = 0; i < max; i++) {
    if (squash(window.shScreen()).startsWith(want)) return i;
    await tap(CAROUSEL_NEXT, 200);
  }
  throw new Error(`goTo(${program}) never arrived; screen reads ${JSON.stringify(window.shScreen())}`);
}
function assertNoneOf(text, needles, where) {
  const s = squash(text);
  for (const n of needles) {
    if (s.includes(squash(n))) throw new Error(`${where}: reached ${JSON.stringify(n)} -- the plate was taken for a seed; screen ${JSON.stringify(text)}`);
  }
}
/** Watch the screen for `ms` and fail the moment any forbidden needle shows. */
async function guardFor(ms, where) {
  const deadline = Date.now() + ms;
  const seen = new Set();
  while (Date.now() < deadline) {
    const t = window.shScreen();
    assertNoneOf(t, FORBIDDEN, where);
    seen.add(String(t).replace(/\s+/g, " ").slice(0, 80));
    await sleep(100);
  }
  return [...seen];
}

export async function run() {
  if (typeof window.shScreen !== "function" || typeof window.shNFC !== "object") {
    throw new Error("shScreen/shNFC missing -- stale emu.wasm; rebuild from the hashlock-h0 branch and serve on a FRESH port");
  }
  const out = { plate: PLATE, typed: null, nfc: null };

  // ─── DOOR 1: typed M*1 STRING ──────────────────────────────────────────────
  await waitFor("SeedHammer");
  await goTo("Backup Wallet");
  await tap(CONFIRM, 500);
  // With no systemwide payload loaded there is no "Seed from where?" offer;
  // the typed chooser is next.
  await waitFor("Input Seed");
  await tap([240, rowY(2, 5)], 300);          // "M*1 STRING" is row 2 of 5
  await tap(CONFIRM, 400);
  await waitFor("Input m*1 string");
  for (const ch of PLATE) await tap(ms1KeyPoint(ch), 90);
  await tap(CONFIRM, 800);
  const typedScreen = await waitFor(REFUSAL, 8000);
  assertNoneOf(typedScreen, FORBIDDEN, "typed door");
  const typedTrail = await guardFor(1500, "typed door (after refusal)");
  out.typed = { refusal: String(typedScreen).replace(/\s+/g, " ").slice(0, 160), trail: typedTrail };
  await tap(CONFIRM, 400);                      // dismiss the error modal
  // Back out to the start screen however deep we are.
  for (let i = 0; i < 6 && !squash(window.shScreen()).includes("SeedHammer"); i++) await tap(BACK, 300);
  await waitFor("SeedHammer");

  // ─── DOOR 2: NFC ───────────────────────────────────────────────────────────
  window.shNFC.present(PLATE);
  // The start screen shows the scan status text ("Unknown format") rather than
  // navigating anywhere; watch for it and for any forbidden screen.
  const nfcScreen = await waitFor("Unknown format", 10000);
  assertNoneOf(nfcScreen, FORBIDDEN, "NFC door");
  const nfcTrail = await guardFor(1500, "NFC door (after status)");
  out.nfc = { status: String(nfcScreen).replace(/\s+/g, " ").slice(0, 160), trail: nfcTrail, presented: window.shNFC.presented() };
  window.shNFC.clear();

  // shScreen() text is whitespace-free; compare squashed to squashed.
  out.ok = squash(out.typed.refusal).includes(squash(REFUSAL)) && squash(out.nfc.status).includes(squash("Unknown format"));
  return out;
}
