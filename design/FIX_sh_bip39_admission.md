# FIX: bip39.Parse refuses ambiguous abbreviations and word counts BIP-39 does not define

**Small track** (one Opus review round). Grade I. Seed-words path (not secret-listed).
Keys: bughunt-1119 `bh-sh-I1` (ambiguous prefix resolves to the first match) and `bh-sh-I2` (a 3-word phrase is a
valid mnemonic). Table row 31, coordinator `agent-reports/architect-bughunt-1119-questions.md` L143.
Ruling (decided, not re-ruled here): **T35** (same report L79): "accept a prefix only when it names exactly one word
(BIP-39's 4-letter uniqueness keeps 4-letter input working)". I2's shape is the grader's (grade-sh.md L66: "allow
12..24 words in multiples of 3"), which is BIP-39's own table. Also closes the deferred engrave FOLLOWUP
`sh-bip39-valid-bounds` (Valid panics on 0 or 27 words), whose skipped RED row this FIX un-skips.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are at that
base (bip39/bip39.go is unchanged since 9aec7a8, where the bug hunt cited it).

## 1. The defect

- **I1.** `Parse` (bip39/bip39.go:312-352) admits any token of 3 or more letters that is a prefix of a word, and takes
  `ClosestWord`'s answer (L95-104): the FIRST word in sorted order with that prefix. `abs` has four matches (absent,
  absorb, abstract, absurd) and resolves to `absent`. When the checksum still passes, the result is a different
  wallet with no error. Measured at base over the whole wordlist: 409 distinct 3-letter prefixes match more than one
  word and are all silently accepted (`TestFixShBip39EveryPrefixNamesExactlyOneWord`); no prefix of 4 or more letters
  is ambiguous, as BIP-39 promises.
- **I2.** `Valid` (L116-121) only requires `len(m)%3 == 0`, so `abandon abandon ability` (3 words, 4 bytes of entropy),
  `abandon abandon abandon abandon abandon able` (6) and `abandon` x9 (9) pass `Valid`, `Parse` and `ParseMnemonic`.
  The same check makes `Valid` panic on 0 words (`index out of range [-1]`) and on 27 or 30 (`entropy too long`), and
  `ParseMnemonic` of 27 words panics (`sh-bip39-valid-bounds`).
- **Reach on the device.** The NFC scanner calls `Parse` (gui/scan.go:80), so a tag holding `abs abandon ... bargain`
  or a 3-word fragment is handed to the flows as a `bip39.Mnemonic`; `seal.Classify` (seal/record.go:263) classifies
  the same records as `BIP-39 mnemonic`. Typed entry is NOT affected: the keyboard completes a word only when it is
  exact or the only match (gui/gui.go:1544-1551), and offers only 12 or 24 words (gui/gui.go:2989,
  gui/derive_xpub.go:237). `sysw.isStrictMnemonic` (sysw/classify.go:87-119) already requires exact lowercase words and
  12..24 words, so the payload-composer path is not affected.

## 2. Rules

- **R1 (T35, ambiguous abbreviation).** In `Parse`, a token that is not itself a wordlist word is accepted only when
  exactly one word starts with it. Otherwise `Parse` returns the error
  `bip39: parse: ambiguous word: "<token>"` (PROPOSED, architect confirms), where `<token>` is the token as given
  (`%q`, original case, as the existing `unknown word` exit prints it). The check runs per token, in input order,
  after the existing `unknown word` check (L339-344), so the first bad token decides the error.
- **R2 (exact word wins).** A token that IS a wordlist word (case-insensitive, as today) is that word even when it is
  also a prefix of longer words (`act` of action/actor/actress/actual, 3- and 4-letter words generally). This is
  forced by BIP-39: every wordlist word must be accepted as itself, so it is not a product choice. The existing
  minimum of 3 letters (L340) is unchanged.
- **R3 (word count).** BIP-39 defines 12, 15, 18, 21 and 24 words ("The allowed size of ENT is 128-256 bits",
  bip-0039.mediawiki, checked against the BIP text on bitcoin/bips master). One unexported predicate
  `validLength(n)` (12 <= n <= 24, n%3 == 0) is used by:
  - `Valid`: returns false for any other length, before `splitMnemonic` (so it never panics). Its doc comment says it
    checks the word count and the checksum.
  - `Parse`: after the loop and before `Valid`, any other count returns
    `bip39: parse: wrong word count: <n>` (PROPOSED). The existing 25th-word exit `bip39: parse: mnemonic too long`
    (L334-337) is unchanged, so `<n>` is at most 24 here (in practice 1-11, 13, 14, 16, 17, 19, 20, 22, 23).
  - `ParseMnemonic`: first, before any word lookup or allocation, any other count of space-separated tokens returns
    `bip39: wrong word count: <n>` (PROPOSED; same prefix style as its `bip39: unknown word` exit), including 27.
  - `LastWordCandidates`: the guard at L166 becomes `!validLength(len(prefix))` (returns nil); doc comment L160-162
    says "nil if len(prefix) is not 12, 15, 18, 21 or 24". At base a 3-, 6- or 9-word prefix returned 256 candidates and
    27 panicked.
- **R4 (wipe).** Both new `Parse` error exits zero the accumulator (`clear(m)`) before returning, like the three
  existing exits (the L321-332 comment and `TestParseZeroesItsAccumulatorOnEveryErrorExit`). `ParseMnemonic`'s
  count exit allocates nothing.
- **R5 (device effect).** No device text changes. A scanned record refused by R1 or R3 falls through the scanner's
  other formats to the existing `errScanUnknownFormat` status ("Unknown format"); `seal.Classify` returns
  `ClassUnknown` for it, and the unlock path's existing `This record is not a readable BIP-39 mnemonic.`
  (gui/unlock_session.go:284) covers a sealed record of that shape.
- **R6 (unchanged).** Unknown-word, too-long and checksum exits keep their texts; every 12/15/18/21/24-word phrase of
  full words or unique abbreviations parses exactly as before; `New`, `FixChecksum`, `Entropy`, `ClosestWord` and
  the keyboard are unchanged (`Entropy` already gates on `Valid`; `FixChecksum` is a generation helper with no
  admission role and is out of scope).

## 3. Holder-visible changes (complete list)

| where | input | before | after |
|---|---|---|---|
| NFC scan (gui/scan.go:80) | tag text with an ambiguous abbreviation (e.g. `abs ...`) whose checksum passes for the first match | opened as that mnemonic (wrong wallet) | "Unknown format" status (existing) |
| NFC scan | 3-, 6- or 9-word checksum-passing phrase | opened as a mnemonic | "Unknown format" status (existing) |
| seal.Classify | the same records | `BIP-39 mnemonic` | `unknown format` (`ClassUnknown`) |
| `bip39.Parse` / `ParseMnemonic` error texts (library) | R1, R3 inputs | accepted, or `bip39: invalid checksum`, or a panic (27 words) | R1/R3 texts above |

No device screen gets new wording. The three new error strings are library errors: on the device every caller of
`Parse`/`ParseMnemonic` discards the text (gui/scan.go:80, seal/record.go:263, gui/unlock_session.go:282-285,
gui/derive_xpub.go:321-324, gui/gui.go:2956); only the dev tools cmd/buildpayloadcards and cmd/buildpayloadcomposer
print a `ParseMnemonic` error, on their own built-in fixtures. No flag, verb or type is added. No README, doc or
toolkit-manual text describes abbreviation or word-count admission (`git grep -i 'abbreviat\|four letters'` over
`*.md` outside design/agent-reports: nothing); toolkit manual lockstep: none.

## 4. Code changes (builder applies)

All in bip39/bip39.go (the scratch version is `design/FIX_sh_bip39_admission-probe.diff`; evidence, not a patch to
copy blindly):

1. Add `validLength(n int) bool` above `Valid`, comment citing BIP-39's ENT range and this FIX.
2. `Valid`: replace `len(m)%3 != 0` (and the stale "Panics in splitMnemonic" comment) with `!validLength(len(m))`;
   doc comment per R3.
3. `LastWordCandidates`: guard and doc comment per R3.
4. `Parse`: after the `unknown word` check, R1's check. Because `ClosestWord` returns the first word in sorted order
   carrying the prefix, the token is ambiguous exactly when it is not that word's full label and the next word
   (`closest+1`, if it exists) also starts with it. Comment names this FIX and T35. After the loop, R3's count check.
   Both exits `clear(m)` first (R4).
5. `ParseMnemonic`: R3's count check at the top.

Source files this FIX changes (for build ordering against other sh rows): `bip39/bip39.go`,
`bip39/bughunt_sh2_s3_test.go` (one line), new `bip39/fix_sh_bip39_admission_test.go`,
`seal/fix_sh_bip39_admission_test.go`, `gui/fix_sh_bip39_admission_test.go`. No overlap with #16
(cmd/biptool, cmd/picosign; merged as 25abe8e) or #32 (mk/encode.go).

## 5. Tests

New (test function names are the TRACE row names), in the probe diff:
- bip39/fix_sh_bip39_admission_test.go (helper `fixShPhrase(n, first)` builds a checksum-valid n-word phrase with
  `FixChecksum`, so 3-, 6- and 9-word fixtures pass the checksum and only R3 refuses them):
  - `TestFixShBip39AmbiguousPrefixRefused` (R1): `abs`/`ABS` + 10 `abandon` + `bargain` refused with
    `bip39: parse: ambiguous word: "abs"` / `"ABS"`; controls `absent`, `absorb` spelled out parse.
  - `TestFixShBip39EveryPrefixNamesExactlyOneWord` (R1, R2): every distinct prefix of 3+ letters of every word
    (6020 prefixes): accepted as the right word when it is a word or has exactly one match (5611), refused with R1's
    text otherwise (409). Each phrase's checksum is fixed for the intended word, so a wrong resolution cannot pass.
  - `TestFixShBip39ExactWordAccepted` (R2 control): all 2048 words spelled in full parse as themselves, `act` included.
  - `TestFixShBip39WordCountRefused` (R3): `Valid`, `Parse` (`bip39: parse: wrong word count: N`) and `ParseMnemonic`
    (`bip39: wrong word count: N`) refuse 3, 6, 9 checksum-passing phrases; `Valid` is false without panic for 0, 1,
    2, 4, 11, 13, 14, 23, 25, 26, 27, 30; `ParseMnemonic` refuses 1, 13, 27 (`abandon` xN) without panic;
    `LastWordCandidates` is nil without panic for 3, 6, 9, 27; `Parse` of 25 keeps `bip39: parse: mnemonic too long`
    and of 13 gives `wrong word count: 13`.
  - `TestFixShBip39DefinedLengthsAccepted` (R3 control): 12, 15, 18, 21, 24 parse through `Valid`, `Parse`,
    `ParseMnemonic`; `LastWordCandidates` gives 2^(11-n/3) candidates (128 for 12, 8 for 24).
  - `TestFixShBip39NewErrorExitsZeroAccumulator` (R4): `zoo` x11 + `abs` and `zoo` x9 give the new errors and leave the
    `parseWordsHook` accumulator all zero (fixtures are Word 2047, so zero is not vacuous).
- seal/fix_sh_bip39_admission_test.go: `TestFixShBip39ClassifyRefusesAmbiguousAndShort` (R5): the four records above
  classify `ClassUnknown`; control `absent ...` is `ClassMnemonic`.
- gui/fix_sh_bip39_admission_test.go: `TestFixShBip39ScanRefusesAmbiguousAndShort` (R5): `scanner.Scan` returns
  `errScanUnknownFormat` for the same four records; control `absent ...` scans as `bip39.Mnemonic`.

Tests that change: `bip39/bughunt_sh2_s3_test.go` `TestBughuntSh212ValidPanicsOnEmptyAndLong` loses its
`t.Skip("deferred-minor: sh-bip39-valid-bounds")` line and passes. Nothing else: the existing suite (including
`TestParse`'s 4-letter truncation of every vector, `TestLastWordCandidates`, `TestParseZeroesItsAccumulatorOnEveryErrorExit`)
passes unchanged on the probe (§8).

## 6. RED rows (run on base 25abe8e with the §5 tests added, no production change)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixShBip39' ./bip39/ ./seal/ ./gui/`

| row | at base (seen) | on the probe fix (seen) |
|---|---|---|
| TestFixShBip39AmbiguousPrefixRefused | FAIL: `Parse("abs" ...) accepted as absent abandon ... bargain; want error "bip39: parse: ambiguous word: \"abs\""` (and `ABS`) | PASS |
| TestFixShBip39EveryPrefixNamesExactlyOneWord | FAIL: 409 lines, e.g. `prefix "ABS" (4 words): Parse gave (absent abandon ... absurd, <nil>)`, `prefix "ABO" (2 words): Parse gave (about ...)` | PASS, log `prefixes: 5611 accepted, 409 refused as ambiguous` |
| TestFixShBip39WordCountRefused | FAIL: `Valid(3 words) = true`, `Parse(3 words) = (abandon abandon ability, <nil>)`, same for 6 and 9 and `ParseMnemonic`; `Valid(0 words) ... panic runtime error: index out of range [-1]`; `Valid(27 words) ... panic entropy too long`; `ParseMnemonic(27 words) ... panic entropy too long`; `LastWordCandidates(3 words) = 256 candidates`; `Parse(13 words) error bip39: invalid checksum` | PASS |
| TestFixShBip39NewErrorExitsZeroAccumulator | FAIL: both subtests `= (, bip39: invalid checksum); want error ...` | PASS |
| TestFixShBip39ClassifyRefusesAmbiguousAndShort | FAIL: 4 lines `Classify(...) = BIP-39 mnemonic; want unknown format` | PASS |
| TestFixShBip39ScanRefusesAmbiguousAndShort | FAIL: 4 lines `Scan(...) = bip39.Mnemonic ..., <nil>; want errScanUnknownFormat` | PASS |
| TestFixShBip39ExactWordAccepted, TestFixShBip39DefinedLengthsAccepted | PASS (controls) | PASS |

Mutation seen on the probe: deleting `clear(m)` before the ambiguous-word return fails
`TestFixShBip39NewErrorExitsZeroAccumulator` (`word 0 of the accumulator is still "ZOO"`).

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 ambiguous abbreviation refused, exact text | TestFixShBip39AmbiguousPrefixRefused; TestFixShBip39EveryPrefixNamesExactlyOneWord |
| R2 exact word wins; unique prefix accepted | TestFixShBip39ExactWordAccepted; TestFixShBip39EveryPrefixNamesExactlyOneWord (accepted half); existing TestParse (4-letter truncation) |
| R3 word count in Valid, Parse, ParseMnemonic, LastWordCandidates | TestFixShBip39WordCountRefused; TestFixShBip39DefinedLengthsAccepted; TestBughuntSh212ValidPanicsOnEmptyAndLong |
| R4 new exits wipe | TestFixShBip39NewErrorExitsZeroAccumulator |
| R5 device scan and classify | TestFixShBip39ScanRefusesAmbiguousAndShort; TestFixShBip39ClassifyRefusesAmbiguousAndShort |
| R6 unchanged texts and inputs | TestFixShBip39WordCountRefused (25-word text); the existing bip39, seal and gui suites |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 8`);
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`; `./scripts/test-32bit.sh`;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium`; `./scripts/refugium-tagged-test.sh`; both TinyGo device
builds and `scripts/refugium-elf-check.sh` (CI). Measured on the probe by the author (go1.26.3 from nix,
`GOTOOLCHAIN=local`; CI pins 1.26): every package of `./...` except gui ok in one run; gui sharded 8 ways,
`RESULT: ok -- all 1435 tests ran across 8 shards`; `GOARCH=386 go test ./bip39/ ./seal/` ok; `go vet ./bip39/ ./seal/`
clean; gofmt clean on the new files. Not run by the author: oraclelive, wasm vet, refugium-tagged, TinyGo builds. The
builder runs the whole gate. Firmware size: R1/R3 add a few comparisons and three format strings to `bip39`; no new
import.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. README/docs: none (§3).
- Closes: bh-sh-I1 and bh-sh-I2 with the GREEN merge, and the engrave FOLLOWUP `sh-bip39-valid-bounds`
  (mnemonic-engrave design/FOLLOWUPS.md, old block format; its RED row is un-skipped here). seedhammer has no
  `design/FOLLOWUPS.md`. The proposed lines (verbatim, for the coordinator to direct-push after the merge) are in the
  author report.

## 10. Out of scope

`FixChecksum` and `splitMnemonic` on lengths outside the set (generation helpers, no admission role; `FixChecksum`
still panics on 27 words, unreachable from any admission path); the keyboard (already unique-completing); SLIP-39
`ClosestWord` (slip39 package, separate wordlist, SLIP-39 admission is disabled in the scanner); passphrase NFKD
(bh-sh-I13, rides `bip39-passphrase-nfkd-normalization`, T37).

## 11. How this was checked

RED and GREEN rows run by the author as in §6 (logs `/scratch/cache/sh-bip39-admission-red.log`,
`/scratch/cache/sh-bip39-admission-green.log`, full-suite logs `/scratch/cache/sh-bip39-admission-full-{nongui,gui}.log`);
the probe diff applies with `git apply --check` on 25abe8e and was reverted after the run. BIP-39 facts (ENT 128-256
bits, the 12/15/18/21/24 table, first-four-letters uniqueness) read from bip-0039.mediawiki on bitcoin/bips master;
the 4-letter uniqueness was also measured (no prefix of 4+ letters is ambiguous).
