# P2 recon — GUI/device-facing side of Engrave Transaction

Worktree: `/scratch/code/shibboleth/_work/p2/seedhammer`, branch `p2/acceptance`,
HEAD `5fed302`. Read-only recon; nothing edited outside this file. Go binary
used for machine-checks: `/nix/store/33fw5m31lfcnk4ff2f0df7j2bxnh8lgk-go-1.26.3/bin/go`.

---

## 1. Carousel lockstep

`type program int` at `gui/gui.go:175`; `engraveTransaction` is a const in that
`iota` block at `gui/gui.go:222`, inserted mid-enum (comment at :215-221
explains the house rule: unconditional programs insert before `loadPayload` so
`bip85Derive` stays the bound `StartScreen.lastNav()` returns).

- `uiFlow`'s program dispatch: `gui/gui.go:2056-2058`
  ```go
  case engraveTransaction:
      engraveTransactionFlow(ctx, th)
      continue
  ```
- `StartScreen.draw`'s title switch: `gui/gui.go:2214-2215`
  ```go
  case engraveTransaction:
      titleTxt = "Engrave Transaction"
  ```
- `layoutMainPlates`: `gui/gui.go:2442-2444` — `engraveTransaction` is listed in
  the shared case (`backupWallet, engravePassphrase, ..., engraveTransaction,
  loadPayload, bip85Derive, unlockPayload:`), all sharing `assets.Hammer`; no
  panic path is reachable for it.
- `engraveObjectFlow`'s scanned-object type switch: `gui/gui.go:2480` (func),
  case at `gui/gui.go:2492-2495`:
  ```go
  case mtText:
      // A scanned chunk enters the transaction program's gather, so it is
      // never dropped and never engraved alone.
      engraveTransactionFlowSeeded(ctx, th, string(scan))
  ```

**`txScan` type: ABSENT.** `grep -rn "txScan" gui/` returns nothing. The type
that carries a scanned mt1 chunk into `engraveObjectFlow` is `mtText`
(`gui/scan.go:134: type mtText string`), and `engraveObjectFlow` has a case for
it (above). There is no `txScan` type anywhere in the tree.

All four sites plus the `mtText` case verified present. Carousel navigation to
"Engrave Transaction" and beyond (Wallet Policy → Engrave Transaction → Load
Payload) is also exercised by `TestEngraveMultisigProgramNavigable`
(`gui/multisig_program_test.go:51-60`) and `TestBip85DeriveProgramNavigable`
(`gui/bip85_program_test.go`) — both **PASS** (see Machine-checks below).

---

## 2. `gui/scan.go`

**`tx:` prefix branch producing a tx-typed scan result: ABSENT from the
scanner.** `isSyswEncoded` (`gui/scan.go:107-110`) only matches
`sysw.TextPrefix` ("text:") and `sysw.PassPrefix` ("pass:") —
`sysw.TxPrefix` ("tx:", defined at `sysw/record.go:21`) is not one of them. A
`tx:`-prefixed record read over NFC is not caught by any branch in
`scanner.Scan` and falls through every sniffer to `errScanUnknownFormat`
(`gui/scan.go:102-104`). `tx:` is used only for classifying records already
inside the systemwide payload container (`sysw/record.go:74-75,110`), never as
an NFC scan-time prefix.

**Bare (unprefixed) `mt1` strings ARE accepted**, via BCH validation rather than
a prefix, at `gui/scan.go:93-97`:
```go
} else if codex32.ValidMT(string(buf)) {
    // One chunk of an mt1 signed-transaction set. Routed to the Engrave
    // Transaction program's gather, which accumulates the set and refuses
    // to engrave until it CONFIRMS (mt.Decode).
    return mtText(buf), nil
```
This does **not** fall through to `freeTextScan` — it returns `mtText`, its own
type, matched earlier than the address-decode branches (98-101) and the final
`errScanUnknownFormat` fallback.

**NFC buffer size constant:** `gui/scan.go:31`, `s.buf = make([]byte, 8*1024)`
— 8192 bytes, allocated lazily on first `Scan` call, unchanged by the recent
`sysw.MaxSectionLen` bump (8191 → 32,734, `sysw/wire.go:34-56`) — that constant
is documented at `sysw/wire.go:42-46` as governing the FLASH payload path only
("a sysw container reaches the device by picotool ... never on a tag");
`gui/scan.go`'s allocation is untouched.

**Gap found:** `gui/scan_test.go`'s `TestScan` table (lines 18-89) has no case
for an `mt1` string reaching the scanner through `scanner.Scan` — `grep -rn
"mtText\|ValidMT" gui/*_test.go` returns nothing. Coverage of the mt1→`mtText`
path exists only at the level of `engraveTransactionFlowSeeded`'s direct string
handling and `transactionGatherFlow`'s NFC loop (which type-asserts an already
scanned `mtText`, `gui/transaction.go:416`), not at the scanner dispatch level
itself.

---

## 3. The payload menu (`syswPayloadMenu`)

**Gains no transaction/content-derived entries.** `gui/sysw_unload.go:34-53`:
with nothing loaded it forwards to `syswLoadFlow`; with something loaded its
only choices are `{"LOAD AGAIN", "UNLOAD"}` (`gui/sysw_unload.go:42`) —
unconditional on what classes/records are present. `grep -rn "ENGRAVE\b"
gui/sysw_unload.go gui/sysw_load.go` finds nothing; no code path adds a
transaction-shaped choice to this menu regardless of whether the loaded
payload holds mt1 chunks or a `tx:` record.

**Invoked ONLY from the `loadPayload` carousel entry, never on the boot
path.** `gui/gui.go:2060` (`case loadPayload: syswPayloadMenu(ctx, th)`) is the
only call site of `syswPayloadMenu` outside its own tests. The boot sequence
calls `syswLoadFlow` **directly**, not `syswPayloadMenu`:
`gui/gui.go:2019: syswLoadFlow(ctx, th, ctx.Platform.SyswReader(), true)`. The
two functions differ: `syswLoadFlow(..., atBoot)` is the read-and-open flow;
`syswPayloadMenu` is a thin picker that exists so the carousel entry has
something to show once a payload is already loaded (re-load vs. unload).

---

## 4. Identity-digest compare screen (`gui/sysw_load.go`)

Located at `gui/sysw_load.go:164-176` (inside `syswLoadFlow`, gated on
`h.PubLen > 0`):
```go
lines := []string{
    "Compare this against what",
    "`me sysw pack` printed:",
    "",
    sysw.FormatHash(d),
}
```
shown via `confirmReviewScreen(ctx, th, "Payload Digest", lines)`
(`gui/sysw_load.go:173`).

**Still names `me sysw pack`. Does NOT name `me sysw show`.**
`grep -rn "sysw show" gui/*.go sysw/*.go` returns nothing anywhere in the
repo — no `me sysw show` command is referenced. Every `me sysw pack` mention in
`gui/` (11 sites across `sysw_load.go`, `transaction.go`, `bundle_flow.go`,
`multisig_build*.go`) is unchanged.

---

## 5. R11′ two messages

`TestNoPayloadAndNoTransactionAreDifferentMessages`: `gui/transaction_test.go:353`
(labeled "GRAFT 3 / R11′" in the doc comment at `gui/transaction_test.go:337`).
It covers `txNothingToEngrave` (`gui/transaction.go:115-134`), called from
`engraveTransactionFlowSeeded` (`gui/transaction.go:214`) when
`payloadTransactions` returns zero candidates.

Literal strings, from source (not the doc-comment table):

- No payload at all — `gui/transaction.go:117-118`:
  ```go
  return "No payload is loaded.\n\nLoad one with Load Payload, or write one with " +
      "`me sysw pack --region`."
  ```
- Payload loaded but not yet compared — `gui/transaction.go:125-126` (a third,
  intermediate message not in the test's two-row table):
  ```go
  return "This payload has not been checked, so nothing may be taken from it.\n\n" +
      "Compare its digest at Load Payload."
  ```
- Payload loaded, compared, holds no transaction — `gui/transaction.go:128`:
  ```go
  msg := "This payload holds no transaction.\n\nIt holds: " + txPayloadHolds(s) + "."
  ```
  optionally suffixed (`gui/transaction.go:129-132`) with:
  ```go
  fmt.Sprintf("\n\n%d mt1 string(s) belong to no complete set. "+
      "Pack every string of the set with `me sysw pack`.", incomplete)
  ```

So there are in fact **three** distinct messages in code (no-payload /
not-yet-compared / loaded-but-empty), though the test and its doc comment name
only two — the "not yet compared" state is a real third branch not exercised
by `TestNoPayloadAndNoTransactionAreDifferentMessages`.

**Test PASSES** (see Machine-checks).

---

## 6. The review screen(s)

One function produces both variants: `transactionReviewLines`
(`gui/transaction.go:460-489`), shown via a single
`confirmReviewScreen(ctx, th, "Transaction", ...)` call
(`gui/transaction.go:492`) — i.e. **one screen**, branching on `c.confirmed`.

**Confirmed-transaction path** (`gui/transaction.go:481-488`):
```go
lines := []string{"Engrave this transaction?", ""}
lines = append(lines, chunkString(c.tx.TxidDisplay, 16)...)
lines = append(lines, "",
    fmt.Sprintf("%d bytes, %d in, %d out", len(c.tx.Raw), c.tx.Inputs, c.tx.Outputs),
    "Source: "+syswSourceName(c.src),
    "BEARER: anyone holding the",
    "plates can broadcast it.")
```
- **Full 64-hex txid: YES.** `TxidDisplay` is `hex.EncodeToString(rev)` over a
  32-byte reversed double-SHA256 digest (`mt/mt.go:315-317`) — 64 hex chars —
  chunked into 16-char lines by `chunkString` (`gui/mk1_inspect.go:21`), so 4
  lines cover it whole. Confirmed by `TestTransactionReviewLines`
  (`gui/transaction_test.go:215-230`), which asserts both `txEvenTxid[:16]` and
  `txEvenTxid[48:]` are present in the joined lines.
- **Input/output counts: YES**, as bare counts (`%d in, %d out`).
- **Addresses, per-output amounts, locktime, nSequence, fee, a "TO" label,
  network parameters: ABSENT.** `grep -in "fee\|locktime\|sequence\|address\|
  amount\|\"TO \|network\|total" gui/transaction.go` matches nothing relevant
  (only unrelated hits: `sysw.ClassAddress` string constant, `TotalChunks`
  loop variables, a comment "minimise total plates"). `mt.Tx` itself
  (`mt/mt.go:122-129`) carries no output/address/amount/locktime/fee fields at
  all — only `Raw`, `TxidDisplay`, `Inputs`, `Outputs`, `SegWit` — so the
  device has no data to show even if it wanted to.
- **No "total" is shown**, so there is no total that could be misread as a
  destination amount — the field doesn't exist on this screen.
- **BEARER warning: YES.** `"BEARER: anyone holding the" / "plates can
  broadcast it."` (`gui/transaction.go:486-487`), asserted by
  `TestTransactionReviewLines` (line 227-229: `strings.Contains(joined,
  "BEARER")`).

**Unconfirmed-candidate path** (`gui/transaction.go:462-479`):
```go
return []string{
    "UNCONFIRMED SET", "",
    fmt.Sprintf("Set %05x, %d string(s).", c.csid, len(c.strs)),
    "",
    "This does NOT reassemble into",
    "a transaction. The strings are",
    "engraveable and each is valid,",
    "but the set is not complete.",
    "",
    "The plate legend WILL be",
    "replaced with:",
    c.subst,
    "",
    "QR plates are unavailable:",
    "there are no transaction bytes.",
}
```
No txid is shown here (there isn't one — the set never reassembled); the set
id is the only identifier. Covered by
`TestUnconfirmedSetIsEngraveableWithASubstitutedLegend`
(`gui/transaction_test.go:311-335`).

**DERIVED vs ASSERTED voice for txid: the txid is always DERIVED, never
asserted/typed.** It only ever appears once `c.confirmed == true`, and
`confirmed` is only set true by `mt.Decode` (`mt/mt.go:165-226`) or
`mt.ParseTx` (`mt/mt.go:233` on) successfully computing `TxidDisplay` via
double-SHA256 over the parsed transaction core — see Q8 below. There is no
code path where a txid string arrives from the payload/host and is displayed
without being recomputed by the device. The zero-value default of
`txCandidate.confirmed` is `false` (fail-closed) — noted explicitly in
`gui/transaction_test.go:216-217` ("confirmed: true is REQUIRED — the zero
value is UNCONFIRMED").

All three tests referenced here (`TestTransactionReviewLines`,
`TestUnconfirmedSetIsEngraveableWithASubstitutedLegend`,
`TestNoPayloadAndNoTransactionAreDifferentMessages`) **PASS**.

---

## 7. Legend substitution (rulings 2026-08-25 / 25b)

**Where:** `legendSubstitution` (`gui/transaction.go:99-107`):
```go
func legendSubstitution(complete bool) string {
    if complete {
        return "INCOMPLETE - DOES NOT DECODE - RE-ENCODE PAYLOAD"
    }
    return "INCOMPLETE - MISSING STRINGS - RE-ENCODE PAYLOAD"
}
```
Applied in `payloadTransactions` (`gui/transaction.go:284-299`, the `!confirmed`
branch, `subst: legendSubstitution(complete)`) for a payload-sourced set that
is either incomplete or complete-but-does-not-decode, and consumed by
`planTransactionTextPlates` (`gui/transaction.go:605-623`), which prepends
`subst` as the FIRST paragraph on the plate, ahead of the operator's mt1
strings, when `subst != ""`.

**Substituted legend text** (the two literal strings, verbatim, quoted above):
`"INCOMPLETE - DOES NOT DECODE - RE-ENCODE PAYLOAD"` and
`"INCOMPLETE - MISSING STRINGS - RE-ENCODE PAYLOAD"`.

**Un-overridable: YES, by design and by code shape.** The doc comment at
`gui/transaction.go:54-57` states it plainly ("`subst` replaces their chosen
text, un-overridably... The device has no camera and can never read a plate
back to warn anyone later"), and mechanically: `planTransactionTextPlates`
takes `subst` from `c.subst` and there is no branch, flag, or ChoiceScreen that
lets the operator supply their own text instead — the operator's own mt1
strings are never treated as carrying a legend at all; `subst` is the only
legend line ever emitted for an unconfirmed candidate. `git show 422acba`
(commit message, captured verbatim below) states the same: "there is no code
path that restores it."

**Does the device REFUSE any of these, or does everything engrave?**
**Nothing is refused — everything reachable engraves.** `payloadTransactions`
(`gui/transaction.go:263-327`) offers the unconfirmed candidate rather than
dropping it (ruling 2026-08-25a, comment at :42-52); `transactionReviewLines`
shows it with the "UNCONFIRMED SET" framing but the review screen still
proceeds to `transactionReviewAndEngrave` → plate planning → `ENGRAVE` on
confirm. The only place a transaction-shaped input is refused/withheld
outright is QR plate generation for an unconfirmed candidate (no transaction
bytes exist), handled by *withholding the QR choice*, not by refusing the
whole flow (`gui/transaction.go:499-504`, comment: "a gate that cannot pass is
worse than one that is absent"). The NFC-gather path drops an ambiguous
complete-but-non-decoding set rather than offering it
(`gui/transaction.go:391-398`, message `"Set complete but does not confirm as
one transaction. Dropped."`) — this is the one place something IS dropped, and
it's the gather (NFC) path specifically, not the payload path, and only for a
set that is complete but fails `mt.Decode`.

**Commit `422acba`:** `git show 422acba --stat` (run verbatim):
```
 gui/transaction.go                | 135 +++++++++++++++++++++++++++++++++++---
 gui/transaction_crosslang_test.go |   2 +-
 gui/transaction_test.go           |  53 +++++++++++++--
 3 files changed, 176 insertions(+), 14 deletions(-)
```
Message (verbatim, in full): "gui: an unconfirmed set is ENGRAVEABLE with a
substituted legend, not dropped" — states rulings 2026-08-25a/2026-08-25b were
folded, that the payload path previously did `continue` (dropped), the NFC
path previously said "Dropped.", and that `txCandidate` gained
`confirmed`/`csid`/`subst` fields with the zero value UNCONFIRMED (fail-closed
default).

---

## 8. Does the DEVICE compute the semantic confirmation itself?

**YES — full structural parse + txid↔chunk_set_id binding, computed on-device.
No host flag byte is trusted; none even exists in the wire format**
(`grep -n "confirmed\|Confirmed" sysw/wire.go sysw/record.go` returns nothing).

Call sites in `gui/` into package `mt`:
- `gui/transaction.go:287` — `tx, err := mt.Decode(set)` in `payloadTransactions`
- `gui/transaction.go:314` — `tx, err := mt.ParseTx(body)` for a `tx:` record
- `gui/transaction.go:391` — `tx, err := mt.Decode(set)` in `transactionGatherFlow` (NFC path)
- `gui/transaction.go:274, 337, 367, 382` — `mt.ParseHeader(...)` at various header-only checks

`mt.Decode` (`mt/mt.go:165-226`) reassembles chunks, calls `ParseTx(raw)`
(`mt/mt.go:218`), then explicitly binds the result:
```go
tx, err := ParseTx(raw)
...
if tx.ChunkSetID() != first.ChunkSetID {
    return Tx{}, errTxidBinding
}
```
`ParseTx` (`mt/mt.go:233` onward) is a structural Bitcoin transaction parser
(BIP-141 witness-stripped) that computes `TxidDisplay` itself via double-SHA256
(`mt/mt.go:303-317`) — never reads a txid from anywhere else.

The same logic backs the `unconfirmed` flag shown at load time:
`sysw.MTUnconfirmed` (`sysw/confirm.go:151-186`) calls `mt.Decode(set)` at
line 180 to decide confirmation per chunk-set-id group, invoked from
`(*syswSession).load` at `gui/sysw_session.go:106`. There is no alternate path
that marks a set confirmed without this call.

---

## 9. Post-cut instruction

**No dedicated post-cut instruction SCREEN exists for the transaction
feature.** The generic `EngraveScreen.Engrave` (`gui/gui.go:3198-3274`) that
drives every plate cut (called from `engraveTransactionPlates`,
`gui/transaction.go:564`) shows only progress/back/select controls — no
per-plate textual instruction about what was just cut or what to do next. On
Back mid-set it shows: `gui/transaction.go:557-561`:
```go
fmt.Sprintf("Stopped at plate %d of %d. A partial set does not carry the "+
    "whole transaction. A re-run cuts the same plates byte for byte, but "+
    "starts at plate 1 - finish a set in one sitting, or start over.",
    i+1, len(plates))
```
— a job-level statement (plate N of M), not per-plate-content-derived, and
only shown on early exit, not after a normal finish. When all plates finish,
`engraveTransactionPlates` just returns `true` and the flow returns with no
further screen (`gui/transaction.go:546-570`, `gui/transaction.go:536-538`).

**The relevant instructional text is the plate LEGEND itself** (engraved onto
the steel, not a UI screen): `transactionLegend`
(`gui/transaction.go:665-677`):
```go
lines := []string{
    "txid " + tx.TxidDisplay,
    fmt.Sprintf("raw signed bitcoin tx, %d bytes, %d qr, ecc %s", len(tx.Raw), symbols, eccName),
}
if symbols > 1 {
    lines = append(lines, "scan all qr, any order, then broadcast")
} else {
    lines = append(lines, "scan, then broadcast")
}
```
- **This is a function of the JOB's total symbol count (`symbols` = the whole
  set), not of what's ON that specific plate.** Notably, when `legendAlone`
  (a standalone legend-only plate with zero QR symbols on it,
  `gui/transaction.go:701-703,762-768`), that plate still carries "scan all qr,
  any order, then broadcast" — a job-level instruction on a plate carrying no
  QR at all.
- **Scan order does not matter: stated, "any order"** — YES, when
  `symbols > 1` (`gui/transaction.go:672`), verified by
  `TestTransactionLegendNamesTheFacts` (`gui/transaction_test.go:113-126`,
  asserts `strings.Contains(many, "any order")`).
- **TEXT plates carry no equivalent order-independence instruction** — the
  TEXT-plate builder (`gui/transaction.go:608-623`) only emits `subst` (if
  present) and the mt1 strings themselves; no "order doesn't matter" text is
  ever added to a TEXT plate.
- **`mt inspect`: ABSENT everywhere.** `grep -rn "mt inspect" .` (excluding
  `.git/`) across the entire repo returns zero matches. The device never
  references this (or any) host inspection subcommand for a partial set.

---

## 10. Plate count and cut time before commit

**Plate count: YES**, stated on a `confirmReviewScreen(ctx, th, "Plates",
...)` immediately before the first `ENGRAVE` (`gui/transaction.go:533`):
```go
if !confirmReviewScreen(ctx, th, "Plates", []string{note, "", "Engrave?"}) {
```
where `note` is `fmt.Sprintf("%d plate(s), %d string(s)", ...)` for TEXT
(`gui/transaction.go:521`) or `fmt.Sprintf("%d plate(s), %d QR, ECC %s, %s
modules", ...)` for QR (`gui/transaction.go:712-714`).

**Cut TIME: NOT stated anywhere on this screen or before it.** The adjacent
comment (`gui/transaction.go:530-532`) claims "plate count and ECC are the two
numbers the operator budgeted blanks **and time** by," but no wall-clock or
duration estimate is actually rendered in `note` or anywhere else in
`transactionReviewAndEngrave`. `grep -n "minutes\|blanks\|takes"
gui/transaction.go` matches only that one comment. Contrast with a sibling
program, `gui/multisig_build_census.go:43`, which DOES surface a qualitative
time statement: `"Each plate takes minutes to cut. Have that many blanks ready
..."`. The transaction confirm screen has no analogous sentence — a real gap
between what the comment claims the operator can budget by and what the code
actually shows. `Plate.Duration` (`gui/gui.go:747`) exists and is used for the
live per-plate countdown inside `EngraveScreen` once a cut is already running
(`gui/gui.go:3179`, `:3276-3296`), but it is never summed or surfaced before
commit.

---

## 11. R16 — refusal on no configuration found

**Present.** `planTransactionQRPlates` (`gui/transaction.go:692-724`)
exhausts every `(plateCount, layout, ecc, scale)` combination up to
`txqr.MaxSymbols+1` plates and, if none fits, returns an error rather than a
plate list (`gui/transaction.go:721-723`):
```go
return nil, nil, "", fmt.Errorf("transaction too large for QR plates at ECC M "+
    "(%d bytes; the Structured Append cap is %d symbols) - use TEXT plates",
    len(tx.Raw), txqr.MaxSymbols)
```
Handled at the call site in `transactionReviewAndEngrave`
(`gui/transaction.go:523-528`): a non-nil `err` from either plate planner is
shown via `showError(ctx, th, "Engrave Transaction", err.Error())` and the
choice loop re-offers plate-kind selection rather than proceeding — the
operator is told to use TEXT plates instead of being left on a broken screen.
(No literal "R16" label exists in code or in this worktree's `design/` — that
directory contains only an empty `agent-reports/` — so this is matched by
behavior, not by a code-searchable requirement ID.)

---

## 12. Legend cut LAST

**Confirmed**, at two levels:

- **Mechanism** (`backup.EngraveText`, `backup/backup.go:493-517`): the title
  and footer rows are emitted as the LAST yields of the generator, after every
  paragraph — comment at :493-514 states this explicitly ("AN UNSIGNED PLATE
  IS AN UNFINISHED PLATE... Cut first, as it was, a plate abandoned at minute
  20 already carried that claim and LOOKED FINISHED"). Test:
  `TestTheTitleAndFooterAreEmittedLast` (`backup/engravetext_test.go:368-418`)
  — asserts by KNOT ORDER (not just presence) that every title/footer knot's
  emission index is ≥ the last body knot's index; fails loudly if any of the
  three bands never fires (line 409-412), so it cannot pass vacuously.
- **Artifact** (a real transaction plate through the real planner):
  `TestTransactionPlateCutsItsTitleLast` (`gui/transaction_test.go:415-`,
  "GRAFT 4") builds plates via `planTransactionQRPlates` and walks the actual
  `Plate.Spline` knot stream, asserting the title band's first knot index is
  never before the body band's last knot index, per plate.

Both tests **PASS** (see Machine-checks). Related, absolute-row placement is
separately pinned by `TestTextTitleFooterAreAbsoluteRows`
(`backup/engravetext_test.go:269`) — also PASS.

---

## 13. Any end-to-end UI walk driving the transaction program?

**ABSENT.** No `runUITouch`-style choice→review→plan-confirm→engrave loop
exists for `engraveTransaction`. Evidence:

- `grep -n "runUITouch" gui/transaction_test.go gui/transaction_crosslang_test.go`
  → no matches (grep exit code 1).
- `runUITouch` IS used extensively elsewhere in the package (39 other
  `*_test.go` files, e.g. `gui/multisig_build_walk_test.go`,
  `gui/multisig_engrave_tail_walk_test.go`) — so the harness for this style of
  test exists and is a proven pattern in this codebase; it is simply not
  applied to the transaction program.
- The only `runUI` (non-touch) use in transaction tests is
  `gui/transaction_test.go:362`, inside
  `TestNoPayloadAndNoTransactionAreDifferentMessages`, which only pumps frames
  looking for an error message string — it never drives a `ChoiceScreen`
  selection, never reaches plate planning, never reaches `EngraveScreen`.
- `TestHostPackedMtPayloadLoadsAndConfirms`
  (`gui/transaction_crosslang_test.go:22-81`) is the closest thing to an
  end-to-end test — it loads a real host-packed (`me sysw pack`) binary fixture
  and calls `payloadTransactions`, `planTransactionTextPlates`,
  `planTransactionQRPlates`, and `mt.Decode` directly — but it calls these
  functions programmatically, with no `ChoiceScreen`/ `confirmReviewScreen`
  interaction, no simulated button clicks, and no `EngraveScreen` pass. It is a
  cross-language data-plumbing test, not a UI walk.
- No golden images: `find . -iname "*golden*"` under `gui/` scoped to
  transaction, and `grep -rln "olden" gui/transaction*.go`, both empty.
- No emulator/journey files: `find . -iname "*journey*" -not -path "./.git/*"`
  returns nothing in this worktree.
- `TestEngraveMultisigProgramNavigable` / `TestBip85DeriveProgramNavigable`
  (§1 above) DO drive real button clicks through the carousel and assert the
  "Engrave Transaction" title renders — but they stop at the title screen and
  never enter the program body.

**Net: the transaction program's unit-level logic (message selection, review
line composition, legend substitution, plate planning, txid binding) is well
tested; the interactive screen-by-screen walk a real operator experiences
(pick transaction → review → choose plate kind → confirm plate count/ECC →
per-plate ENGRAVE → completion) has no automated test at all.**

---

## Machine-checks run

```
go test -list '.*' ./gui/    (confirms test names exist, matched against report claims above)
go test -list '.*' ./backup/
go test -list '.*' ./mt/
go test -run '<transaction-related regex>' -v ./gui/     -> PASS (16/16), 0.568s
go test -run '^(TestTheTitleAndFooterAreEmittedLast|TestTextTitleFooterAreAbsoluteRows)$' -v ./backup/  -> PASS (2/2)
```
Full PASS list from the `./gui/` run (log read in full, not just exit code):
`TestBip85DeriveProgramNavigable`, `TestEngraveMultisigProgramNavigable`,
`TestScan` (+8 subtests), `TestScanRecognizesAddress`,
`TestHostPackedMtPayloadLoadsAndConfirms`,
`TestTextPlatesPackMultipleStringsPerPlate`, `TestTextPlatesKeepIndexOrder`,
`TestQRPlanSmallTransactionIsOnePlateAboveTheFloor`,
`TestTransactionLegendNamesTheFacts`, `TestPayloadTransactionsConfirmsAndMerges`,
`TestSessionMarksIncompleteMtSetsUnconfirmed`, `TestTransactionReviewLines`,
`TestTransactionPlateTextIsEngraveable`, `TestQRPlanScalesWithTransactionSize`,
`TestUnconfirmedSetIsEngraveableWithASubstitutedLegend`,
`TestNoPayloadAndNoTransactionAreDifferentMessages`,
`TestTransactionPlateCutsItsTitleLast`. All PASS, no skips, no `t.Skip`.

`git show 422acba --stat` and the commit message were read and quoted verbatim
in §7 above.
