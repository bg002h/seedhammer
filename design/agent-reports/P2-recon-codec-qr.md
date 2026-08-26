# P2 recon — non-GUI transaction codec/QR packages (sysw/, mt/, txqr/, codex32/)

Worktree: `/scratch/code/shibboleth/_work/p2/seedhammer`, branch `p2/acceptance`.
Read-only recon. All line numbers verified by direct read at the time of writing
(2026-08-25); grep output is pasted verbatim where an ABSENT/PRESENT claim is made.

---

## 1. `sysw.MaxSectionLen`

**Value / formula.** `sysw/wire.go:64`:
```go
MaxSectionLen = (RegionLen - HeaderLen - TagLen) / 2
```
= `(65536 - 52 - 16) / 2 = 32734`. It is a **formula**, not a literal — the
formula itself is documented at `sysw/wire.go:39-63` as the RAISED value
(from 8191), converging on the Rust primary.

**Compile-time assertion.** `sysw/wire.go:82-86`:
```go
// The property MaxSectionLen's formula exists to preserve, at COMPILE time:
// two maxed sections plus header plus tag still fit the region...
var _ [RegionLen - (HeaderLen + 2*MaxSectionLen + TagLen)]struct{}
```
A negative array length fails the build — this is a genuine compile-time
assertion that `HeaderLen + 2*MaxSectionLen + TagLen <= RegionLen`. A second
array-length assertion pins the exact constant at `sysw/wire.go:90`:
`var _ [MaxSectionLen - 32734]struct{}`.

The same property is re-asserted at **runtime** in
`sysw/cap_test.go:46-48` (`TestTheSectionCapMatchesTheRustPrimary`), so the
reasoning survives a refactor that deletes the compile-time array.

**Cross-repo test reading the Rust primary's source.**
`sysw/cap_test.go:39` `TestTheSectionCapMatchesTheRustPrimary`, via the
helper `rustPrimaryWire()` at `sysw/cap_test.go:18-29`. It tries two relative
paths (side-by-side checkout and submodule layout), reads
`crates/me-cli/src/sysw/wire.rs` as text, and asserts (line 65-69) that the
source **contains the literal formula string**
`"pub const MAX_SECTION_LEN: usize = (REGION_LEN - HEADER_LEN - TAG_LEN) / 2;"`
— deliberately a formula match, not a retyped constant, "because a retyped
constant is precisely how a port silently forks" (comment at line 62-64). If
the sibling checkout is absent, the cross-repo half logs and returns
(`cap_test.go:56-60`) rather than failing — but the constant/geometry
assertions above it are unconditional. It also reads the primary's
`seal/wire.rs` (line 74-83) to confirm `seal`'s cap is FROZEN at 8191 and
that this port's `sysw` cap did not get raised onto the wrong constant.

`ParseHeader`'s enforcement site: `sysw/header.go:50-53`
(`if h.PubLen > MaxSectionLen || h.CtLen > MaxSectionLen`).

---

## 2. Record classes: `ClassMt`, `ClassTx`, secrecy

Both exist. `sysw/record.go:24-44`:
```go
type Class int
const (
	ClassUnknown Class = iota
	ClassMnemonic
	ClassCodex32Secret
	ClassPassphrase
	ClassFreeText
	ClassDescriptor
	ClassMDMK
	ClassAddress
	ClassMt   // line 39
	ClassTx   // line 43
)
```

**Secrecy predicate**, `sysw/record.go:53-55`:
```go
func (c Class) IsSecret() bool {
	return c == ClassMnemonic || c == ClassCodex32Secret || c == ClassPassphrase
}
```
Neither `ClassMt` nor `ClassTx` is in the list — **both are NOT secret**.
Confirmed by test `sysw/mt_records_test.go:45-47`:
```go
if ClassMt.IsSecret() || ClassTx.IsSecret() {
	t.Error("mt records are engraved in cleartext; the class must not claim secrecy")
}
```

**Classification function.** `sysw/record.go:97-122` (`Classify`), dispatching
`tx:`-prefixed records at lines 110-119 (hex decode via `DecodeBody` +
structural `mt.ParseTx` parse; only a record that decodes AND parses becomes
`ClassTx`, else `ClassUnknown`) and `sysw/classify.go:34-58`
(`classifyConstellation`), which reaches `ClassMt` at line 52-56 (`codex32.ValidMT`
+ `mt.ParseHeader` must both succeed).

**Ruling 2026-08-25b (unconfirmed/incomplete must NOT be classed SECRET).**
The `Class` values themselves are unaffected by confirmation state — `Classify`
returns `ClassMt`/`ClassTx` regardless of whether the set is complete, and
`IsSecret()` never returns true for them (test above). **However**, there is a
SEPARATE, orthogonal boolean — not part of `Class` — that treats an unconfirmed
mt/mdmk record with secret-like caution for **display flags** (§3.3.3), not for
classification:

- `sysw/confirm.go:150`: doc comment on `MTUnconfirmed`: *"an unconfirmed record
  loads and counts as SECRET for flag evaluation"*.
- `sysw/confirm.go:33`: identical comment on `MDMKUnconfirmed`.
- Consumer (GUI, out of this package scope but shown for completeness):
  `gui/sysw_session.go:99-106`, comment: *"An unconfirmed mt record counts as
  SECRET for flags exactly as an unconfirmed md1/mk1 does."* This populates a
  `syswRecord.unconfirmed bool` field (`gui/sysw_session.go` struct), separate
  from `.class`.

So: **`Class.IsSecret()` — the classification predicate — never returns true for
`ClassMt`/`ClassTx`, confirmed or not** (this is the literal ruling, and the
code satisfies it). The *unconfirmed* flag is a different mechanism layered on
top for UI caution, not a reclassification to SECRET. Separately, ruling
2026-08-25b (per `gui/transaction.go:42-43,100`, GUI, out of scope) is actually
about **engraveability**: an unconfirmed/incomplete mt set is still ENGRAVED,
with the operator's legend forcibly replaced by a warning
(`gui/transaction.go:96-102`, `legendSubstitution`) rather than refused.

---

## 3. `codex32.ValidMT`

Exists at `codex32/mtdata.go:35-42`:
```go
func ValidMT(s string) bool {
	_, data := splitHRP(s)
	if len(data) < mtMinDataLen || len(data) > mtMaxDataLen {
		return false
	}
	return verifyMDMK(s, "mt", newShortChecksum().generator,
		mtRegularTargetHi, mtRegularTargetLo, mdmkShortSyms)
}
```
It is **NOT** a call to `ValidMD`/`ValidMK` — it hard-codes its own HRP literal
`"mt"` and its own NUMS target constants
`mtRegularTargetHi = 0x1`, `mtRegularTargetLo = 0xa2fc877f9528d7c1`
(`codex32/mtdata.go:20-21`), passed into the shared `verifyMDMK` engine
(`codex32/mdmk.go` — same engine `ValidMD`/`ValidMK` call, but with THEIR OWN
HRP/target literals, e.g. `codex32/mdmk.go:137-141` for `ValidMD` using `"md"` +
`mdRegularTargetHi/Lo`). Structurally parallel, independently keyed.

**NUMS constant re-derived from SHA-256 in a test.**
`codex32/mtdata_test.go:68-88`, `TestMTTargetReproducesFromDomain`:
```go
d := sha256.Sum256([]byte("shibbolethnumstransaction"))
... // top 65 bits of the 128-bit big-endian value = value >> 63
if wantHi != mtRegularTargetHi || wantLo != mtRegularTargetLo { t.Errorf(...) }
```
also asserts (lines 84-88) the target differs from both `mk`'s and `md`'s
targets, guarding against a copy-pasted sibling constant.

---

## 4. Package `mt`

All in `mt/mt.go` (416 lines).

**a. Canonical compactSize + whole-buffer-consumed.** YES, both enforced.
- Canonical compactSize: `(*parser).varint()`, `mt/mt.go:349-391`. Each of the
  three multi-byte prefixes (`0xFD`, `0xFE`, `0xFF`) has an explicit
  minimal-encoding floor check that rejects the non-canonical range, e.g.
  `mt/mt.go:361-363` (`if v < 0xFD { return 0, errNotATransaction }` for the
  `0xFD` prefix), mirroring Bitcoin Core's `ReadCompactSize` — comment at
  `mt/mt.go:346-348`.
- Whole-buffer-consumed: `mt/mt.go:300-302`:
  ```go
  if p.pos != len(raw) {
  	return Tx{}, errNotATransaction
  }
  ```
  Verified by test `mt/mt_test.go:143-166` (`TestParseTxMatchesTheNode`),
  which checks every truncated prefix is refused (loop at line 151) and that
  one trailing byte is refused (line 155-157).

**b. Txid: witness-stripped, DISPLAY (reversed) order.** Both true.
- Witness-stripped: the digest is computed only over `version` + the CORE
  span `raw[coreStart:coreEnd]` + `locktime` (`mt/mt.go:304-308`).
  `coreEnd` (`mt/mt.go:282`) is captured immediately after the outputs loop
  and BEFORE the segwit witness-stack loop (`mt/mt.go:283-295`), so witness
  data is excluded from the hash — BIP-141 semantics, as stated in the doc
  comment `mt/mt.go:228-229`.
- Reversal to DISPLAY order: `mt/mt.go:310-314`:
  ```go
  rev := make([]byte, 32)
  for i, b := range d2 {
  	rev[31-i] = b
  }
  ```
  then `TxidDisplay: hex.EncodeToString(rev)` (`mt/mt.go:317`). The `Tx.TxidDisplay`
  field doc (`mt/mt.go:127`) states explicitly: "byte-reversed lowercase hex,
  the form a user reads."

**c. Per-input signature-presence predicate — ABSENT.** Grepped hard,
tree-wide, not just in the four scoped packages:
```
$ grep -rn "scriptSig\|ScriptSig" mt/ sysw/ txqr/
mt/mt.go:263:		if err := p.skipBytes(); err != nil { // scriptSig
$ grep -rniE "everyinputsigned|every_input_signed|inputsigned|is_signed|signed\(" mt/ sysw/ txqr/
sysw/header_test.go:25:func TestSectionCapIsComparedUnsigned(t *testing.T) {   [unrelated: integer signedness]
$ grep -rn "witness|Witness" mt/ sysw/ txqr/
mt/mt.go:229: // its txid over the witness-stripped form (BIP-141). Structural ONLY: ...
$ grep -rniE "every_input_signed" . --include="*.go"     [repo-wide, no hits]
```
The only occurrence of `scriptSig` is the comment on `p.skipBytes()` at
`mt/mt.go:263` inside the input loop (`mt/mt.go:259-269`) — the scriptSig bytes
are **skipped**, never inspected for non-emptiness. Similarly the segwit
witness loop (`mt/mt.go:284-295`) counts `items` per input via `p.count()` but
never stores or checks that `items > 0`. **There is no `every_input_signed`
equivalent anywhere in this Go tree** — `ParseTx` is a pure structural parser
with no signedness judgement, matching its own doc comment at `mt/mt.go:230-232`
("Structural ONLY: no script validation, no signature checks, no judgement
about signedness — those are `mt`'s [the HOST tool's] at encode time"). This is
a **doc-confirmed** absence, not an oversight the doc contradicts, but the
Rust twin (`every_input_signed` in `crates/me-cli/src/sysw/tx.rs` of the
sibling repo) has no Go-side counterpart at all.

**d. chunk_set_id ↔ txid top-20-bits binding.** Checked at `mt/mt.go:222-224`,
inside `Decode`:
```go
if tx.ChunkSetID() != first.ChunkSetID {
	return Tx{}, errTxidBinding
}
```
`Tx.ChunkSetID()` (`mt/mt.go:135-151`) reads the first 5 hex characters of
`TxidDisplay` (= top 20 bits). Tested by `mt/mt_test.go:132-140`
(`TestSmuggledEntropyAndForgedSetIdAreRefused`, "foreign" vector: a real
transaction re-chunked under chunk_set_id `0x00000` — reassembles, parses,
fails binding).

**e. Correction — pure verifier, never corrects.** `codex32.ValidMT`'s own doc
(`codex32/mtdata.go:30-34`): "Pure verify, no error correction — the string is
engraved verbatim, so a record needing repair is refused rather than
corrected into steel damage." Confirmed structurally: `codex32.Correct`'s HRP
dispatch table, `paramsForHRP` (`codex32/correct.go:44-76`), switches only on
`"ms"`, `"md"`, `"mk"` (lines 46, 51, 60) — **no `"mt"` case** — so `Correct()`
falls through to `return bchParams{}, false` (`codex32/correct.go:76`) for any
mt1 fragment, meaning error correction is structurally unreachable for mt1.
Test evidence: `sysw/mt_records_test.go:33` ("mt1 damaged (no correction)" →
`ClassUnknown`, not repaired-then-classified) and `mt/mt_test.go:127-129`
("a damaged string must be refused, not corrected").

---

## 5. `txqr`

All in `txqr/txqr.go` (195 lines).

- **Byte mode always, including k=1.** `EncodeSet`'s `k==1` branch
  (`txqr/txqr.go:53-70`) explicitly calls `encodeByte` → `encodeSegments(coding.String(data), ...)`
  rather than the vendored library's mode-selecting `qr.Encode`, precisely to
  avoid mode selection (comment lines 54-64). The `k>=2` path (`encodeSA`,
  `txqr/txqr.go:135-141`) also uses `coding.String(part)`. Both paths funnel
  through the single `encodeSegments` (`txqr/txqr.go:149-188`), so there is one
  byte-mode implementation, not two. Regression-pinned by
  `txqr/capgate_test.go:221-251` (`TestOneSymbolIsByteModeToo`) — this test's
  own comment (`capgate_test.go:204-220`) documents that this package was
  ONCE caught failing this exact invariant on the k=1 path (F-234) and was
  fixed.
- **ISO 18004 Structured Append for 2..16 symbols.** `EncodeSet`
  (`txqr/txqr.go:46-92`): `k==1` is plain byte mode; `k>=2` builds
  `saHeader{index, count, parity}` per symbol (`txqr/txqr.go:74-91`, `encodeSA`
  at 135-141) — 20-bit header (4 mode + 4 index + 4 count-1 + 8 parity),
  `saHeader.Encode` at `txqr/txqr.go:117-122`.
- **Hard 16-symbol cap.** `const MaxSymbols = 16` (`txqr/txqr.go:38`), enforced
  at `EncodeSet` entry (`txqr/txqr.go:50-52`, `k > MaxSymbols → errTooManySymbols`)
  and again in `saHeader.Check()` (`txqr/txqr.go:106-111`, `s.count > MaxSymbols`).
- **ECC floor at M — NOT enforced inside `txqr`.** The package takes `level
  qr.Level` as a caller-supplied parameter on every entry point
  (`EncodeSet`, `encodeByte`, `encodeSA`, `encodeSegments`) and has no internal
  notion of a floor; its own doc comment states it "deliberately has no
  opinion about plates" (`txqr/txqr.go:23-26`). The floor is a **caller
  policy**, in GUI (out of this recon's scope but cited for completeness):
  `gui/transaction.go:677-684` (doc comment: *"3. ECC floor M — a CONSTRAINT,
  never traded"*) and enforced by the literal search order at
  `gui/transaction.go:704`: `for _, ecc := range []qr.Level{qr.H, qr.Q, qr.M}`
  — M is the last/lowest level ever tried, never below it.
- **Module sizes emitted — decided by the caller, not by `txqr`.**
  `txqr.EncodeSet`/`encodeSegments` hard-codes `Scale: 8` on the returned
  `*qr.Code` (`txqr/txqr.go:187`) and has no mm concept at all. Physical
  module-size selection is GUI policy: `gui/transaction.go:705`
  (`for _, scale := range []int{3, 2} { // 0.9mm, then 0.6mm modules }`).
  **It never emits 0.3mm** — only 0.9mm (scale 3) and 0.6mm (scale 2) are ever
  tried, per that literal slice.

---

## 6. Independent decode proof — `TestZxingMergesTheSetBackToTheTransaction`

`txqr/txqr_test.go:92-114`.
- **k values covered:** `{1, 2, 3, 6}` (`txqr/txqr_test.go:97`).
- **Reverse order:** YES — `txqr/txqr_test.go:104-110` explicitly iterates
  `for i := len(set) - 1; i >= 0; i--`, with comment "REVERSE order on purpose:
  scan order is irrelevant to a standard decoder."
- **Skip condition:** `txqr/txqr_test.go:93-96`:
  ```go
  zxing, err := exec.LookPath("ZXingReader")
  if err != nil {
  	t.Skip("ZXingReader not installed; the scanner round-trip needs it")
  }
  ```
  So yes, it CAN silently pass-as-"ok" with nothing run, on a machine lacking
  the binary — **but on this machine it did not skip**: `ZXingReader` resolved
  to `/usr/bin/ZXingReader`, and the machine-check run below shows the test
  actually RAN (`--- PASS`, not `--- SKIP`).

---

## 7. Capgate tests — `txqr/capgate_test.go`

- `TestTheLibraryCapacityTablesMatchThePublishedLimits`
  (`txqr/capgate_test.go:71-103`): binary-searches (via `largest`,
  `capgate_test.go:45-59`) the vendored library's real max symbol length in
  numeric/alphanumeric/byte mode at all 4 EC levels and asserts equality
  against `publishedV40` (`capgate_test.go:32-42`) — the literal ISO/IEC 18004
  v40 published character-capacity table (e.g. `{qr.M, "M", 5596, 3391, 2331}`).
- `TestStructuredAppendCapacityCostsExactlyTheHeader`
  (`capgate_test.go:110-163`): probes `encodeSA` directly and asserts the
  measured per-symbol byte capacity equals the published byte capacity minus
  the 20-bit SA header (derived arithmetic, `dataBytes - 5`), and that one byte
  past that cap is refused by the version walk, not silently rescued by the
  encoder.
- `TestTheQRDeliveryCeilingIsWhatWeThinkItIs`
  (`capgate_test.go:172-202`): computes and `t.Logf`s the total-bytes ceiling
  (max bytes/symbol × `MaxSymbols`) at every EC level, and specifically
  computes it at `qr.M` (comment line 185: "ECC M is the floor the objective
  never trades below") and compares it (informationally, via `t.Logf`, not a
  hard failure) against the sysw 32,734-byte section cap.
- `TestOneSymbolIsByteModeToo` (`capgate_test.go:221-251`): the F-234
  regression test described in Q5 above.

All four assert against the literal `publishedV40` table declared in the same
file (`capgate_test.go:33-42`), which states it is "ISO/IEC 18004's version-40
character capacity, per EC level" — i.e. yes, against the published limits,
not against the vendored library's own tables (that would beg the question;
the comment at `capgate_test.go:61-70` states this explicitly).

---

## 8. Cross-language seam — `gui/testdata/sysw_mt_payload.bin`

**Note:** this fixture and its consuming test are in package `gui`, outside
this recon's assigned non-GUI scope (`sysw/`, `mt/`, `txqr/`, `codex32/`), but
the task named it explicitly, so it is reported here for completeness.

File exists: `gui/testdata/sysw_mt_payload.bin`. Consumer:
`gui/transaction_crosslang_test.go:22-80`,
`TestHostPackedMtPayloadLoadsAndConfirms`.

**Produced by the Rust host tool.** Doc comment at
`gui/transaction_crosslang_test.go:11-21`: packed via
```
me sysw pack --no-passphrase --in records.txt --out sysw_mt_payload.bin
```
over the pinned "even" vector's six mt1 strings plus its `tx:` record
(`me-cli @ exp/tx-brief-driven`).

**What it proves.** The comment states it directly (lines 18-21): "Every other
test in this feature exercises one implementation against fixtures; this one
exercises the actual seam — bytes the host wrote, read by the device code —
which is the class of test that caught F-212 (two sides computing different
ids while 887 single-repo tests passed either way)." The test body
(`transaction_crosslang_test.go:27-80`) walks the full device path: `sysw.ParseHeader`
→ `sysw.Open` → `syswSession.load` (asserts zero `unconfirmed` records) →
`payloadTransactions` (1 confirmed candidate, correct `TxidDisplay`, 6 strings)
→ both `planTransactionTextPlates` and `planTransactionQRPlates` succeed →
`mt.Decode` re-derives the same txid.

---

## 9. Vectors/fixtures inventory (sysw/, mt/, txqr/, codex32/, for this feature)

| File / fixture | What it proves |
|---|---|
| `sysw/testdata/sysw_vectors.json` + `sysw/testdata/sysw_vectors.provenance.json` | Vendored Rust-primary-generated conformance vectors for the **base sysw container** (names S-A..S-J per `grep '"name"' sysw/testdata/sysw_vectors.json`). **Contains NO mt1/tx: records** — `grep -n "mt1\|tx:" sysw/testdata/sysw_vectors.json` returns nothing. Not part of the transaction-feature fixture set; listed because it lives in the same package. |
| `mt/mt_test.go` `even` (6-string set, `mt/mt_test.go:15-20`) | Real signed 222-byte P2WPKH tx, chunk_set_id `0x2dcf2`, 6 equal-length chunks — the primary happy-path vector. |
| `mt/mt_test.go` `uneven` (8-string set, `mt/mt_test.go:26-33`) | 284-byte tx, 4 outputs, 8 chunks with a short final chunk — exercises the uneven-final-chunk reassembly path. |
| `mt/mt_test.go` `smuggled` (`mt/mt_test.go:44`) | 32 bytes of entropy wrapped as a complete, BCH-valid, reassembling mt1 set that does NOT parse as a transaction — the anti-smuggling semantic gate. |
| `mt/mt_test.go` `foreign` (`mt/mt_test.go:48-54`) | The real "even" tx re-chunked under a foreign `chunk_set_id` — reassembles and parses, fails the txid-binding check. |
| `codex32/mtdata_test.go` `mtEven` (duplicate of `mt`'s `even`, `codex32/mtdata_test.go:13-19`) | `ValidMT` accepts the pinned vector (upper+lower case); rejects damage, mixed case, and cross-HRP confusion with md1. |
| `sysw/mt_records_test.go` `mtEven`, `mtSmuggled`, `evenTxRecord` (`sysw/mt_records_test.go:12-23`, same underlying vectors) | Drives `Classify`/`MTUnconfirmed` conformance against "the RUST primary's answers (me-cli sysw::mt tests)" per file comment. |
| `mt/mt_test.go` `evenRawHex`/`evenTxid`, `unevenTxid` | Pinned expected raw bytes and display txids, cross-checked against an independent Python encoder (`mnemonic-engrave/scripts/gen-mt1-vectors.py`) per `mt/mt_test.go:9-11`. |
| `txqr/txqr_test.go` `evenRawHex` (`txqr/txqr_test.go:17`) | Same "even" transaction, used to drive `TestZxingMergesTheSetBackToTheTransaction` (real ZXing round trip) and other txqr tests. |
| `gui/testdata/sysw_mt_payload.bin` (GUI, out of scope, see Q8) | Cross-language seam: an actual Rust-host-packed container, proving host bytes decode on the device path end to end. |

No standalone `testdata/` directories exist under `mt/`, `txqr/`, or `codex32/`
— confirmed by directory listing (`find` in these dirs returns only `.go`
files). All mt/tx fixtures for this feature are inline Go source constants,
sourced (per file comments) from the same underlying "even"/"uneven" vectors
generated independently in `mnemonic-transaction/crates/mt-codec` and
`mnemonic-engrave/scripts/gen-mt1-vectors.py`.

---

## 10. Text-plate packing

**Not decided in any of the four scoped packages** (`sysw/`, `mt/`, `txqr/`,
`codex32/`) — none of them has a plate/layout concept. The packing decision
lives entirely in `gui/transaction.go` (out of this recon's scope, reported
because the question requires a real citation):

- `planTransactionTextPlates` (`gui/transaction.go:597-660`) packs mt1 strings
  "AS MANY PER PLATE AS FIT" (comment line 589-593).
- **Fitting oracle is the REAL layout, not arithmetic.** Comment at
  `gui/transaction.go:594-596`: "Fit is decided by the real thing: build the
  plate, plan the engraving, toPlate rejects overflow." The `build` closure
  (`gui/transaction.go:606-622`) literally calls
  `toPlate(backup.EngraveText(params, plate), params)` (line 622) inside a
  greedy first-fit probe loop (`gui/transaction.go:624-637`) that grows `hi`
  one string at a time until `toPlate` fails.
- **Face size:** `transactionFontMM = 3.0` mm
  (`gui/transaction.go:38`, doc at lines 34-37: "the tested legibility floor
  ... chosen because the brief's stated priority is the MINIMUM number of
  plates and 3.0 is the smallest proven rung"), used at
  `gui/transaction.go:619` (`FontSize: transactionFontMM`).

---

## Machine-check (mandatory)

Go binary: `/nix/store/33fw5m31lfcnk4ff2f0df7j2bxnh8lgk-go-1.26.3/bin/go`.

```
$ go test -C /scratch/code/shibboleth/_work/p2/seedhammer -count=1 ./sysw/ ./mt/ ./txqr/ ./codex32/
ok  	seedhammer.com/sysw	0.035s
ok  	seedhammer.com/mt	0.001s
ok  	seedhammer.com/txqr	1.651s
ok  	seedhammer.com/codex32	0.003s
```
Log: `/tmp/claude-1000/-scratch-code-shibboleth-mnemonic-engrave/b277edf7-db74-4157-9113-884bb4e5508c/scratchpad/go-nongui.log`

All four packages under this recon's scope pass, count=1 (no cache reuse).

```
$ which ZXingReader
/usr/bin/ZXingReader

$ go test -C /scratch/code/shibboleth/_work/p2/seedhammer -count=1 -run TestZxing -v ./txqr/
=== RUN   TestZxingMergesTheSetBackToTheTransaction
--- PASS: TestZxingMergesTheSetBackToTheTransaction (0.07s)
PASS
ok  	seedhammer.com/txqr	0.068s
```
Log: `/tmp/claude-1000/-scratch-code-shibboleth-mnemonic-engrave/b277edf7-db74-4157-9113-884bb4e5508c/scratchpad/go-zxing.log`

**The zxing test actually RAN (PASS), not SKIP** — `ZXingReader` is installed
on this machine at `/usr/bin/ZXingReader`, so the independent-decoder proof
executed for real, covering k ∈ {1, 2, 3, 6} in reverse symbol order.

---

## Notable findings for the reconciliation

1. **Q4c is a real gap, not a misreading of the doc.** There is no
   `every_input_signed`-equivalent per-input signature-presence predicate
   anywhere in the Go port (`mt`, `sysw`, or `txqr`), tree-wide grep confirms
   it, and `mt.ParseTx`'s own doc comment (`mt/mt.go:230-232`) says this is
   deliberate — signedness judgement is the host's job at encode time, not the
   device's at parse time. Whether that is the intended parity with the Rust
   primary's `every_input_signed` (which DOES exist, per the task brief, in
   `crates/me-cli/src/sysw/tx.rs`) is a spec-vs-code question for the
   reconciliation, not something this recon can resolve — it is reported as a
   fact, not a verdict.
2. **`ClassMt`/`ClassTx` never claim `IsSecret()==true`**, confirmed and
   directly tested (`sysw/mt_records_test.go:45-47`) — satisfies the literal
   ruling. The pre-existing "counts as SECRET for flag evaluation" language on
   the *unconfirmed* boolean (`sysw/confirm.go:33,150`) is a distinct,
   orthogonal display-caution mechanism, not a reclassification — but the
   overlapping vocabulary ("SECRET") between the two mechanisms is worth a
   reviewer's eye, since it reads at first glance like a classification claim.
3. **`sysw_vectors.json` (vendored Rust-primary conformance vectors) has zero
   mt1/tx: coverage** — confirmed by grep. The transaction feature's
   conformance rests entirely on inline Go fixtures cross-checked against an
   independently-generated Python vector set, not on the same vendored-vector
   mechanism the rest of `sysw` uses.
