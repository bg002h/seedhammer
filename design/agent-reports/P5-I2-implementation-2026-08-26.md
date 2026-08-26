# P5 I-2 — implementation note

**Date:** 2026-08-26
**Branch / worktree:** `fold/p5-i2` in `/scratch/code/shibboleth/_work/i2/seedhammer`
**Finding:** `design/agent-reports/P5-whole-diff-fable-2026-08-26.md` § I-2
**Ruling folded (operator, 2026-08-26, verbatim):**

> "P5 I-2 last wins is fine but message to user that this is the rule is
> required so they can try again in different order. Message should say
> something like duplicate plate 13 of 20 found, last wins"

Last-wins is **unchanged**. What was added is the message.

---

## 1. What changed

All in `gui/transaction.go`, plus one new test file.

| Site | Change |
| --- | --- |
| `txCandidate` | two new fields — `dupIdx []int` (0-based, ascending, the colliding indices) and `dupTotal int` (the set's **declared** chunk count) |
| `duplicateChunkIndices(set)` **(new)** | names every index holding more than one distinct string, and returns the declared count from the header |
| `namedChunks(idx, total)` **(new)** | `"string 4 of 6"` / `"strings 4 and 5 of 6"` / `"strings 1, 4 and 5 of 6"` — mirrors the existing `namedInputs` |
| `transactionDuplicateLines(idx, total)` **(new)** | the screen block |
| `payloadTransactions` | computes the pair from `set` **before** `orderByIndex` consumes it, and carries it on both the confirmed and unconfirmed candidate |
| `txGather.offer` | same, for the NFC path — the finding's second consumer |
| `transactionReviewLines` | emits the block: third from the top on the unconfirmed-set screen, last on the confirmed one |

`orderByIndex` itself is **untouched**. The legend substitution, the
confirmed/unconfirmed split and the QR withholding are untouched.

Nothing else in the tree was changed. `go vet ./gui/` still fails for the
pre-existing go1.26-vs-go.mod-1.25.10 reason; not touched. `gofmt -l` flags
`gui/transaction.go`, `gui/transaction_golden_test.go` and
`gui/transaction_txrecord_test.go` — **all three already flagged at HEAD**
(verified by `git stash`; the only hunk in `transaction.go` is a stray double
blank line at :191, not mine), so they were left alone.

## 2. The message, exactly as it renders

Machine-rendered from `transactionReviewLines` for a payload holding run 1
whole plus the two strings run 2 disagrees on. The full unconfirmed-set screen:

```
 0 |UNCONFIRMED SET|
 1 ||
 2 |Set 2dcf2, 6 string(s).|
 3 ||
 4 |DUPLICATE STRINGS - LAST WINS|
 5 ||
 6 |Duplicate strings 4 and 5 of 6 found, last wins.|
 7 ||
 8 |The earlier copies are NOT|
 9 |engraved. Re-pack the payload|
10 |in a different order to cut a|
11 |different copy.|
12 ||
13 |This does NOT reassemble into|
...
```

One collision reads `Duplicate string 13 of 20 found, last wins.` — the
operator's sentence, with the noun corrected (see §3). Three read
`Duplicate strings 1, 4 and 5 of 6 found, last wins.`

**Where it lands on the real panel.** Driven through the actual flow at
`sh2DisplaySize` (480x320 — the shipped panel, not the 240x240 test fiction),
page one of the screen the operator presses Continue from is:

```
Transaction UNCONFIRMED SET Set 2dcf2, 6 string(s).
DUPLICATE STRINGS - LAST WINS
Duplicate strings 4 and 5 of 6 found, last wins.
```

The rule and every colliding index are **on page one**; the remedy is on page
two, reached by the page button. This matters because page one of this screen
holds only about seven lines — measured, not estimated — which is the same
constraint G-P3.20 found for the bearer warning.

**No string body is ever rendered.** `mt1` strings are bearer material; the
message names slots. Asserted by
`TestTheReviewScreenNamesEveryDuplicatedString`, which fails if any packed
string's first 24 characters appear in the lines.

## 3. Why the number is the one the operator sees — and why "string", not "plate"

**1-based.** Not inferred; the convention is written down in the host and
matched in three places:

- `crates/me-cli/src/main.rs`, `describe_set_problem`: *"**Chunk numbers are
  1-BASED here and everywhere an operator reads them**, which is `mt`'s own
  convention (SPEC_mt §1.1: the wire index is 0-based and appears in no
  message). Printing the wire index would send someone counting the strings on
  their desk and finding the wrong one."* Its `Missing` arm prints literally
  `MISSING string(s) 4 and 5 of 6` — the same noun, the same `X and Y of N`
  join. `namedChunks` reproduces that shape.
- `gui/transaction.go`, `txGather.offer`: `"String %d of %d."` at
  `h.ChunkIndex+1`.
- `mt/mt.go`, `Decode`: `"mt: missing chunk %d of %d"` at `i+1`.

So the device now agrees with the CLI the operator will use to re-pack.

**"string" and not "plate", deliberately, though the ruling said plate.**
`planTransactionTextPlates` packs *as many strings per plate as fit* — the
pinned 6-chunk vector is **one** plate (`TestTextPlatesPackMultipleStringsPerPlate`).
"Plate 4 of 6" would send the operator to a plate that does not exist. The
strings are engraved in index order, so "string 4 of 6" is the fourth one
reading down the set, and it is the number `mt`'s own refusals use.

**The total is the DECLARED count**, from the header — not `len(strs)` and not
the number of distinct indices present. The deduped length is precisely the
number the screen was already showing while the drop went unnamed, so using it
would have put the defect inside its own warning.

## 4. The RED

Written first, watched fail. Stubs (`return nil, 0` / `return ""`) were added
so the failures were assertions rather than a build error:

```
--- FAIL: TestDuplicateChunkIndicesNamesEveryCollisionOneBased
    declared count 0, want 6 (the header's, not the deduped length)
    got [], want both colliding indices [3 4]
--- FAIL: TestTheReviewScreenNamesEveryDuplicatedString
    the screen never names the dropped strings:
--- FAIL: TestTheGatherNamesDuplicatesToo
    the scanned set's review screen names nothing:
--- FAIL: TestWalkADuplicatedStringIsNamedBeforeTheCut
    the operator can press Continue from this page without ever learning a string was dropped:
    "Duplicate strings 4 and 5 of 6 found, last wins." is on no page the operator can reach.
```

Two of the eight tests passed on the first run and are declared as what they
are — pins on behaviour that must **not** change (`TestLastWinsIsStillTheRule`,
`TestTheTwoRunFixtureCollidesTheWayTheFindingSays`) plus one false-alarm guard
(`TestAnIdenticalDuplicateIsNotReported`).

**The fixture.** `txEvenAlt34` is chunks 3 and 4 of the pinned `even`
transaction **re-signed**: the same 222 bytes with two bytes flipped inside the
witness DER signature (raw offsets 120 and 155, inside the 71-byte signature
push at 112), re-encoded by the same independent Python encoder that minted
every other mt1 fixture in this tree
(`mnemonic-engrave/scripts/gen-mt1-vectors.py`, which reproduces `txEven` byte
for byte from the same input). The txid is computed over the witness-**stripped**
form, so it is unchanged and the `chunk_set_id` is still `0x2dcf2` — the
review's scenario exactly, with no collision luck.

**Mutation-tested, because a green suite proves little.** Eight mutations, all
killed:

| # | Mutation | Killed by |
| --- | --- | --- |
| M1 | `i+1` → `i` (0-based) | 4 tests, incl. the walk |
| M2 | `strings.EqualFold` → `==` | `TestAnIdenticalDuplicateIsNotReported` |
| M3 | report only the first collision | 4 tests |
| M4 | declared count → deduped length | `TestTheTotalIsTheDeclaredCountNotTheDedupedLength` |
| M5 | block dropped from the unconfirmed screen | 3 tests |
| M6 | gather does not carry `dupIdx` | `TestTheGatherNamesDuplicatesToo` |
| M7 | payload path does not carry `dupIdx` | 3 tests |
| M8 | block moved **below** the legend (off page one) | `TestWalkADuplicatedStringIsNamedBeforeTheCut` |

M4 **survived the first round** and is why
`TestTheTotalIsTheDeclaredCountNotTheDedupedLength` exists: on a complete
two-run set the declared count, the distinct-index count and the deduped length
are all 6, so any of the three passed. The added fixture is a set that is both
short *and* duplicated (declared 6, four indices present) — which is also the
likelier accident.

## 5. Things found on the way that the review did not mention

1. **The unconfirmed screen contradicts its own legend.** Its body is hardcoded
   `"but the set is not complete."`, but a two-run payload has **every** index
   present — `setIsComplete` returns true and the legend three lines below
   correctly reads `INCOMPLETE - DOES NOT DECODE - RE-ENCODE PAYLOAD`. So the
   screen tells the operator the set is short while the legend tells them it
   does not decode, and only one of those is true. The two legends are already
   distinguished (`legendSubstitution(complete bool)`); the body text is not.
   **Not fixed here** — it is ruled screen wording, outside I-2, and it wants
   the operator's call on the sentence. Worth filing.

2. **The review's escalation is real, and now machine-checked.** The spliced
   last-wins output — chunks 0,1,2,5 from run 1 and 3,4 from run 2 — was run
   through `mt.Decode`: it returns **no error**, yields
   `txid = 2dcf2b97…f630` (the operator's own txid), and reports
   `EveryInputSigned = true`. So the engraved subset would confirm at recovery
   under a permanent plate legend saying `DOES NOT DECODE`, carrying a
   signature spliced from two ceremonies. That is asserted in
   `TestTheTwoRunFixtureCollidesTheWayTheFindingSays` so the scenario cannot
   quietly stop being reachable.

3. **A CONFIRMED candidate can carry `dupIdx` only via a forged string.**
   `mt.Decode` compares payload **bytes**; `payloadBytes` truncates the final
   chunk's padding bits rather than rejecting them (deliberately, to match the
   Rust primary). So two strings that differ here and are identical there must
   differ only in that padding — reachable only by forging. Nothing of value is
   dropped in that case, which is why the block goes **last** on the confirmed
   screen (page one there belongs to the bearer warning, ruled by G-P3.20) and
   **third from the top** on the unconfirmed one.

4. **`incomplete` counts raw records, so a two-run set adds 8 rather than 6.**
   Benign today: `txNothingToEngrave(ctx.sysw, incomplete)` is only reached
   when `len(cands) == 0`, and a duplicated set always produces a candidate.
   Recorded because the counter's sentence ("belong to no complete set") would
   be wrong if that guard ever moved.

5. **Case-folded duplicates are deliberately not reported.** `mt.Decode`
   tolerates an exact duplicate ("a well-kept drawer, not an error") and
   `TestDecodeIsOrderIndependentAndDuplicateTolerant` pins an
   `strings.ToUpper(even[3])` copy as acceptable. Comparing exact Go strings
   would have warned about it and sent the operator to re-pack a payload that
   lost nothing — M2 above.

## 6. Gate

Run inside the worktree, via the Nix devshell.

```
$ nix develop --command bash scripts/gui-shard-test.sh ./gui/ 24
    990 top-level tests
    partition verified exhaustive: 990 == 990
=== running 24 shards in parallel (timeout 20m each) ===
=== wall: 62s ===
RESULT: ok -- all 990 tests ran across 24 shards

$ nix develop --command go test ./mt/...
ok  	seedhammer.com/mt
```

990 = 982 at HEAD + 8 new. (The runner's "top-level tests" counts `Test` and
`Fuzz` entries together: 975 + 7 fuzz at HEAD, 983 + 7 now — measured with
`go test ./gui/ -list '.*'` on a stashed tree, not inferred.) Sharded rather
than serial: the runner enumerates
its partition from `go test -list` and asserts the union is exhaustive before
running, so no test is silently dropped.

## 7. Tests added — `gui/transaction_duplicates_test.go`

- `TestTheTwoRunFixtureCollidesTheWayTheFindingSays`
- `TestLastWinsIsStillTheRule`
- `TestDuplicateChunkIndicesNamesEveryCollisionOneBased`
- `TestTheTotalIsTheDeclaredCountNotTheDedupedLength`
- `TestAnIdenticalDuplicateIsNotReported`
- `TestTheReviewScreenNamesEveryDuplicatedString`
- `TestTheGatherNamesDuplicatesToo`
- `TestWalkADuplicatedStringIsNamedBeforeTheCut`
