# Fork fold review — the four P5 folds (integration gate)

**Reviewer:** independent fold-review agent, 2026-08-26
**Object:** `git diff d305713..d5fa9fd` on `review/fork-folds` — exactly four commits:

```
41056de  P5 I-2: last wins, and the operator is told which strings lost
91be26a  P5 M-3 and M-1: the screen stops contradicting its legend; EncodeSet stops panicking
2735fac  P5 M-4, N-2, N-3: three operator instructions that were wrong in the detail
d5fa9fd  P5 M-6 (Go port): refuse a segwit marker with no witness data
```

The 17 commits before `d305713` were not reviewed — they have been through their gate.

**Two questions:** (1) does each fold close its finding? (2) did any fold introduce a
new defect — hardest on the two that add a refusal.

---

## Part A — finding closure

| Finding | Verdict | Evidence |
| --- | --- | --- |
| **I-2** (Important) | **CLOSED** | Per the operator ruling the dedup is unchanged; `duplicateChunkIndices` (`gui/transaction.go:516-568`) is computed from `set` **before** `orderByIndex` consumes it, on **both** delivery paths (`payloadTransactions:428`, `txGather.offer:686`), and `transactionDuplicateLines` names every colliding index 1-based against the declared header count. `TestWalkADuplicatedStringIsNamedBeforeTheCut` asserts the headline is on **page one** of the real screen (`w.until("UNCONFIRMED SET")`), not merely in a slice. |
| **M-1** (Minor) | **CLOSED** | `txqr.go:87-93` guard `(k-1)*per >= len(data)` is necessary **and** sufficient for "k non-empty parts" (proof below). Mutation-verified: with the guard disabled the suite reproduces `slice bounds out of range [120:113]` at `txqr.go:99` — the exact arithmetic the finding cited. The false comment at `gui/transaction.go:1324` ("EncodeSet REFUSES a payload it cannot split into k non-empty parts") is now **true of the code**, which is the better of the two fix shapes the finding offered. |
| **M-3** (Minor) | **PARTIAL** — see B-0 | The fold repaired the *second* of the two claims M-3 called false ("the set is not complete" → "incomplete or does not decode") but left the *first* ("This does NOT reassemble into a transaction"), and neither replacement is true of the `ErrUnsignedInputs` case M-3 named explicitly. Rendered output below. |
| **M-4** (Minor) | **CLOSED** for the reported case | `transactionPostCutLines:1021` now branches `c.subst != "" && len(c.strs) == 0` → "This WILL decode there." A `tx:` record is the only candidate shape with bytes and no strings (`payloadTransactions:475-484`), so the branch is exactly the finding's input. The sibling branch keeps a misprediction for a case M-4 did not enumerate — B-2, pre-existing, not fold-introduced. |
| **M-6** (Minor) | **CLOSED** | `mt.go:344-352`: refuse iff `segwit && every witness stack empty`. Exact Bitcoin Core parity (`!tx.HasWitness()` → "Superfluous witness record"). Rust-primary parity confirmed at `me-cli/src/sysw/tx.rs:258-266` (`TxError::EmptyWitnessOnSegwitMarker`, identical predicate `input_has_witness.iter().all(|&w| !w)`) — the Go commit is a convergence port, not a lead. Mutation-verified: disabling the guard fails `TestASegwitMarkerWithNoWitnessDataIsRefused`, so the crafted bytes really do parse without it. |
| **N-2** (Nit) | **CLOSED** | `planTransactionQRPlates:1273-1277` swaps the remedy to "…no TEXT plates to fall back to. Re-pack the transaction as an mt1 set." when `len(c.strs) == 0`. Untested — the whole refusal path is near-unreachable, which is why N-2 was a Nit; noted, not charged. |
| **N-3** (Nit) | **CLOSED** | The ceiling is now quoted as "at most **about** N bytes (module fit; the legend and title take a little more)". |

---

## Part B — new defects

### B-0 (Minor) — M-3's fold leaves the unsigned-SET screen contradicting its own legend

**File:** `gui/transaction.go:797-838` (`transactionReviewLines`, `!c.confirmed` branch).

M-3 named two cases: a complete set failing the txid binding, **and** "a complete set
carrying an unsigned transaction (`ErrUnsignedInputs`)". For the second, `mt.Decode`
reassembles the bytes perfectly, parses them, and binds the txid — it fails only the
signature predicate (`mt.go:258`). `substitutionFor` handles it correctly and says so in
its own comment: *"The bytes ARE a transaction and the txid IS right. Calling that
'DOES NOT DECODE' would send the operator to re-encode a payload that is encoded
perfectly well."* Two hundred lines below, the screen now says exactly that.

**Rendered, by executing `transactionReviewLines` on that candidate:**

```
UNCONFIRMED SET

Set 2dcf2, 2 string(s).

This does NOT reassemble into
a transaction. The strings are
engraveable and each is valid,
but the set is incomplete or
does not decode.

The plate legend WILL be
replaced with:
UNSIGNED INPUT - CANNOT BE BROADCAST - RE-EXPORT
```

Both sentences are false of this candidate, and they sit four lines above a legend that
is right. This is the same contradiction M-3 reported, in the same place, for one of the
two cases it named.

**Concrete input:** `txCandidate{confirmed: false, strs: [...], subst: legendUnsigned}` —
i.e. any payload whose mt1 set decodes to a transaction with an unsigned input.

**Why the fold's own test missed it:**
`TestTheUnconfirmedScreenDoesNotContradictItsOwnLegend` loops over
`legendSubstitution(true)` and `legendSubstitution(false)` and never instantiates
`legendUnsigned` — the third legend, and the one M-3 was about.

**Fix shape:** one branch on `c.subst == legendUnsigned`, a field the candidate already
carries. Does not touch the settled ruling, which chose between "incomplete" and "does
not decode" for the two *decode-failure* cases.

**Severity:** Minor. The legend on the steel is the correct one; only the screen lies.

---

### B-1 (Minor, introduced by 41056de) — the duplicate message names a channel the NFC operator does not have

**File:** `gui/transaction.go:614-621` (`transactionDuplicateLines`).

The new message ends: *"Re-pack the payload in a different order to cut a different
copy."* `txGather.offer` attaches `dupIdx`/`dupTotal` to NFC-gathered candidates
(`:703`, `:708-709`), and those reach the identical screen via
`engraveTransactionFlowSeeded → transactionGatherFlow → transactionReviewAndEngrave`
(`:311-313`, `:331-333`, `:355-357`). An operator who arrived by scanning tags has **no
payload**; the one action the message offers does not exist on their path (theirs is to
re-scan in a different order).

**Concrete input:** the fold's own `TestTheGatherNamesDuplicatesToo` builds it — feed
`txEvenTwoRuns()` to `newTxGather()` and read `transactionReviewLines(c)`; the assertion
stops at the sentence naming the indices and never reads the remedy.

**Fix shape:** one branch on `c.src` (already on the candidate), or wording neutral to
the channel ("in a different order").

---

### B-2 (Minor, PRE-EXISTING — not introduced, but adjacent to M-4) — the post-cut screen still mispredicts for an unsigned mt1 SET

**File:** `gui/transaction.go:1036-1041` (the `else if c.subst != ""` branch).

M-4's fold discriminates by **channel** (`len(c.strs) == 0`), not by **cause**. An
unconfirmed *set* whose `mt.Decode` failed with `ErrUnsignedInputs` therefore keeps
*"This set did NOT confirm here, so expect it to fail there too."* Executed
`transactionPostCutLines` on that candidate:

```
Type one string into
`mt verify`, then `mt decode`
the whole set.
...
This set did NOT confirm
here, so expect it to fail
there too.
```

`mt decode` runs no signature check at all (`mnemonic-transaction/crates/mt-cli/src/main.rs:1033-1075`
— pipeline decode, content-id guard, report, hex; the unsigned refusal at
`validate.rs:95-113` is scoped to the verb `encode`). So `mt verify` succeeds and
`mt decode` succeeds, and the device predicted failure — M-4's exact class.

**Reachability is low**, which is why this is Minor and not more: `mt encode` refuses to
encode an unsigned transaction and has no override, so an unsigned mt1 *set* must come
from a foreign encoder — the same reachability class as M-6, which was still fixed.

**Fix shape:** discriminate by cause — `c.subst == legendUnsigned` — which closes M-4's
reported case and this one with the same branch.

---

### B-3 (Nit, introduced by 41056de) — "Duplicate string 5 of 2": the declared total may come from a different run than the named index

**File:** `gui/transaction.go:544-552` (`duplicateChunkIndices`).

`total` is taken from the **first parseable header** (`if total == 0 { total = h.TotalChunks }`)
while `idx` may hold an index contributed by a string with a **larger** declared count.
The function's own doc comment asserts this is impossible:

> *"A header whose index is out of range for its count cannot reach this: `mt.ParseHeader`
> refuses it, so the named number is never larger than the total it is named against."*

`parseHeaderSyms` (`mt/mt.go:99`) bounds `ChunkIndex` against **that string's own**
`TotalChunks` only. Nothing bounds it against the first string's. The rest of the code
explicitly anticipates mixed declared counts inside one csid group — `setIsComplete`
rejects them (`gui/transaction.go:141-143`) and `mt.Decode` returns `errCountMismatch`
(`mt/mt.go:246-248`) — so the assumption is contradicted two functions away.

**Concrete input:** three same-csid strings — A(count=2, index=0) first, then B and C
(count=6, index=4, differing payloads). `total = 2`, `idx = [4]` → *"Duplicate string 5
of 2 found, last wins."* Needs ≥3 strings and ≥2 distinct declared counts with the
smaller-count string first, so it takes a foreign encoder or a 20-bit csid collision;
the set is already unconfirmed by then.

**Fix shape:** take the total from the same header the named index came from, or fall
back to `max(TotalChunks)`, or refuse to name a total when the counts disagree.

---

## Checked and clean — so the fold does not re-derive it

**The txqr precondition is exactly right at the boundary.** With `per = ceil(len/k)` and
`len > 0`, parts `0..k-2` have length `per >= 1` and part `k-1` has length
`len - (k-1)*per`; so *"all k parts non-empty"* ⟺ `(k-1)*per < len`. The guard is the
exact negation — necessary and sufficient, not conservative. `k == 1` returns earlier and
is unaffected; the older `k > len(data)` guard is now subsumed (harmless).

**No legitimate artifact is wrongly refused.** The inputs the new guard refuses split in
two: those the old code **panicked** on (`(k-1)*per > len`), and those it **encoded with
an EMPTY trailing symbol** (`(k-1)*per == len`). Only the second class is a behaviour
regression, and it was swept exhaustively: **120 (n,k) pairs for k = 2..16, every one with
n <= 225** (n <= 15k), and **every single one has a smaller k that encodes** — measured, not
argued. Since `planTransactionQRPlates` iterates k **ascending** and returns the first fit,
it can never be pushed into the refusal. Spot-checked with the shipped platform params:

```
n=113 -> 1 plate(s), 1 QR, ECC H, 0.9mm modules
n=222 -> 1 plate(s), 1 QR, ECC H, 0.6mm modules
n=225 -> 1 plate(s), 1 QR, ECC H, 0.6mm modules   # the largest affected size
n=240 -> 1 plate(s), 1 QR, ECC H, 0.6mm modules
```

`qrCeilingBytes`' binary search starts at `lo = MaxSymbols` and doubles, so it converges
above 225 for any plausible geometry; where it previously *panicked* it now returns false,
which is strictly better. The control test `TestEncodeSetStillEncodesASplittableSet`
(n=113, k=15 → `14*8 = 112 < 113`) sits **one byte** from the boundary — a genuinely tight
control, not a token one.

**No legitimate transaction is wrongly refused by `anyWitness`.** The guard fires only
when **every** stack is empty. A mixed transaction (any input with `items > 0`) sets
`anyWitness` and parses — verified in source and by `TestOneSignedInputDoesNotVouchForTheOthers` still passing. A witness stack holding one zero-length item counts as witness data
on both sides, matching Core's `CScriptWitness::IsNull() == stack.empty()`. Refused bytes
are bytes no node will deserialize, so nothing broadcastable is lost. On the device such a
record simply fails `sysw.Classify` (`sysw/record.go:115`) and is skipped by the same
pre-existing `continue` that skips every unparseable `tx:` record.

**The duplicate arithmetic.** 1-based (`i+1`), declared header count (never `len(strs)`,
pinned by `TestTheTotalIsTheDeclaredCountNotTheDedupedLength` on a set that is *both* short
and duplicated so the three candidate numbers differ), **every** duplicated index and not
just the first (full map scan, `sort.Ints`), no string body rendered (indices only —
correct, mt1 strings are bearer material). Case-blind equality is right: `mt.Decode`
compares payload **bytes** and a codex32 string is consistent-case. Both paths `TrimSpace`
before the set is built (`:415`, `:660`), so whitespace cannot manufacture a false
duplicate. Long lines wrap rather than clip (`widget.Labelw` at a fixed `lineWidth`), so a
six-duplicate message stays whole.

**Nothing else in the device UI is falsified.** `transactionChoiceRow` shows the deduped
`%d strings`, which the new block is what explains; the text-plate title is `plate n of m`
and states no string count, so the steel carries no claim the dedup could break; the
confirmed screen's page one is still the bearer warning's (the duplicate block goes last
there, deliberately). The only surviving contradiction is B-0.

**Suite, at `d5fa9fd`, run here rather than inherited:**

```
gui:   RESULT: ok -- all 992 tests ran across 24 shards   (wall 62s)
mt:    ok      txqr: ok      sysw: ok
go build ./...  OK        go vet ./mt/ ./txqr/ ./sysw/  OK
```

(`go vet ./gui/` fails for the known pre-existing go1.26-vs-go.mod-1.25.10 reason; not
these commits, not reported.)

---

## Counts and verdict

**0 Critical / 0 Important / 3 Minor / 1 Nit**
(Minor: B-0 the M-3 partial, B-1 the NFC wording, B-2 the pre-existing M-4 sibling.
Nit: B-3 the declared-total edge.)

Closure: I-2 **CLOSED**, M-1 **CLOSED**, M-3 **PARTIAL**, M-4 **CLOSED**, M-6 **CLOSED**,
N-2 **CLOSED**, N-3 **CLOSED**. No fold introduced a Critical or Important defect, and
**neither new refusal rejects any artifact a real tool can produce** — the txqr guard was
swept exhaustively over its whole affected domain, and the `anyWitness` guard is
bit-for-bit Core's own rule and the Rust primary's.

### **CLEAR TO INTEGRATE**

Nothing here blocks the fork's `main`. Recommended (not gating, one branch each on a field
the candidate already carries): fold **B-0** and **B-1** before the tag — B-0 because it is
the residue of a finding this cycle believed closed, B-1 because it is the only defect
these four commits actually created. B-2 and B-3 belong in `FOLLOWUPS.md` with an owning
phase.
