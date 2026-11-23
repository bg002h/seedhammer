# FIX: a UF2 file cut short reads as complete

**Small track** (one Opus review round). Grade I. Not secret-listed, not funds path (firmware integrity, build tooling).
Key: bughunt-1119 `bh-sh-I7`. Table row 55, coordinator `agent-reports/architect-bughunt-1119-questions.md` L167.
Grader: `agent-reports/bughunt-1119/grade-sh.md` L36 (CONFIRMED, I) and L69 (fix shape: "Count blocks against
NumBlocks, and treat an EOF in the middle of a block as io.ErrUnexpectedEOF"); triage evidence
`agent-reports/bughunt-1119/triage-sh-t2.md` L15. No T-ruling or Brian ruling names this key; none is needed.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are at that
base.

## 1. The defect

`uf2.Reader` (uf2/uf2.go) returns a clean `io.EOF` for a file that ends early, so `io.ReadAll` gives a short image
and a nil error:

- At a block boundary: the header read (L121-122) passes `io.EOF` through. Nothing compares the blocks read with the
  `numBlocks` the headers declare (offset 24). A 3-block file cut after block 1 reads 256 bytes, nil.
- Inside a block: a payload read (L72-74) passes the inner reader's `io.EOF` through, and a footer read (L110-111) that
  gets zero bytes returns `io.ReadFull`'s `io.EOF`. A file that ends after a header, inside a payload, or between a
  payload and its footer reads short with a nil error. (A cut inside the footer, after at least one footer byte,
  already gives `io.ErrUnexpectedEOF`.)
- The same holds inside a block of another family, which the reader skips (L131-133).

Consumers: `cmd/picosign` `extract` (main.go L101-105), `hash` (L126-130) and `sign` (L202-206) read the whole file with
`io.ReadAll`. For the picobin fixture used by the picosign tests, picobin's parser catches every block-boundary cut
(the author ran all 23 cuts: each exits 2 with a picobin error, §11), so picosign does not fail open on that
fixture; the defect is at the uf2 layer (grade I as graded, consumer mitigation not proven in general), and the
picosign message names the wrong cause.

UF2 specification (github.com/microsoft/uf2 README, read 2026-11-22): offset 20 "Sequential block number; starts at
0", offset 24 "Total number of blocks in file"; "The total number of blocks in the file and the sequential block number
make it easy for the bootloader to detect that all blocks have been transferred."; "If the `familyID` doesn't match,
the bootloader should disregard the entire block, including `blockNo` and `numBlocks` fields."; "This also allows for
several files with different `familyID` to be simply concatenated together".

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (own-family count).** The reader counts the blocks of its own family that it has accepted (magic, flags and
  contiguity checks passed). The declared total is the `numBlocks` of the first own-family block. Blocks of another
  family are not counted and their `numBlocks` is ignored (the spec's "disregard the entire block").
- **R2 (cut at a block boundary).** When the header read meets end of input with no bytes, and at least one own-family
  block was accepted and fewer were accepted than the declared total, `Read` returns `io.ErrUnexpectedEOF`. With zero
  own-family blocks accepted (empty input, or only other families), end of input stays a clean `io.EOF` (unchanged).
  When the count reaches the declared total, end of input is a clean `io.EOF` (unchanged).
- **R3 (cut inside a block).** End of input anywhere inside a block is `io.ErrUnexpectedEOF`: an `io.EOF` from a payload
  read (whether returned with the last data or after it) and an `io.EOF` from a footer read (zero footer bytes). This
  holds for own-family and other-family blocks alike.
- **R4 (no new text).** The error is the bare `io.ErrUnexpectedEOF` (Go's text `unexpected EOF`), testable with
  `errors.Is`; no new error string is added in package uf2.
- **R5 (scope).** No other check changes: `blockNo` sequence, `numBlocks` that differs between own-family blocks,
  own-family blocks beyond the declared total, and a declared total of 0 behave as today (filed, §9). `Write` shares
  `loadBlock` and so shares R1-R3; it never reads to end of input on a complete file.

## 3. Holder-visible changes (complete list)

Only picosign prints; its exit code (2) and its stdout (none) are unchanged for a truncated file. The stderr line
changes from picobin's complaint to the uf2 cause:

| command (file missing its last UF2 block) | before (seen) | after |
|---|---|---|
| `picosign hash <f>` | `picosign: <f>: picobin: missing block header` | `picosign: <f>: unexpected EOF` |
| `picosign extract <f>` | `picosign: extract: <f>: picobin: missing block header` | `picosign: extract: <f>: unexpected EOF` |
| `picosign sign -clear <f>` | `picosign: sign: <f>: picobin: missing block header` | `picosign: sign: <f>: unexpected EOF` |

The "after" lines are PROPOSED (architect confirms): they are the existing picosign prefixes plus Go's
`io.ErrUnexpectedEOF` text, with no new wording. The alternative (a uf2 sentinel such as `uf2: truncated file`) would
be new wording; not recommended. Other cuts (§1) change the same way from whichever picobin error they gave. A
complete file prints exactly what it printed before (checked on two real sealed firmware images, §11).

## 4. Code changes (builder applies)

uf2/uf2.go only (the scratch version is `design/FIX_sh_uf2_truncation-probe.diff`; evidence, not a patch to copy
blindly):

1. `Reader` (L13-23) gains two `uint32` fields: own-family blocks accepted, and the declared total; comment names this
   FIX.
2. `Read` (L72-74): after the inner read, an `io.EOF` becomes `io.ErrUnexpectedEOF` (R3; a payload read is always
   inside a block, because the footer follows it).
3. `loadBlock` footer read (L110-111): an `io.EOF` becomes `io.ErrUnexpectedEOF` (R3).
4. `loadBlock` header read (L121-122): an `io.EOF` becomes `io.ErrUnexpectedEOF` when the count is above 0 and below
   the total (R2).
5. `loadBlock` before `return nil` (L148-149): on the first accepted block record its `NumBlocks()` as the total; then
   increment the count (R1).

Source files the FIX changes (for build ordering): `uf2/uf2.go`; new tests `uf2/fix_sh_uf2_truncation_test.go` and
`cmd/picosign/fix_sh_uf2_truncation_test.go`. No other package imports `uf2` (only `cmd/picosign`).

## 5. Tests

New (test function names are the TRACE row names), in the probe diff:

- uf2/fix_sh_uf2_truncation_test.go: a block builder (family, address, blockNo, numBlocks; 256-byte payload; footer
  magic) and a file builder (n blocks, blockNo 0..n-1, numBlocks n, contiguous from 0x10000000). Other family: 0xe48bff57.
  - `TestFixShUF2CompleteFile` (control): 2-block file reads 512 bytes, nil.
  - `TestFixShUF2ConcatenatedFamilies` (control): a 3-block other-family file before, and after, a 2-block own file:
    512 bytes, nil each.
  - `TestFixShUF2EmptyInput` (control): empty input reads 0 bytes, nil.
  - `TestFixShUF2BoundaryCut`: 3-block file cut at 512 and at 1024: `io.ErrUnexpectedEOF`.
  - `TestFixShUF2MidBlockCut`: 2-block file cut at 32, 544 (block 1 header only), 644 (inside payload), 800 (payload,
    no footer) and 1022 (inside footer), each through `bytes.Reader` and through `iotest.DataErrReader`:
    `io.ErrUnexpectedEOF`. (Cut 1022 passes at base: a control inside the row.)
  - `TestFixShUF2OtherFamilyBlockCut`: 2-block own file then a 1-block other-family file, cut 32 bytes into the other
    block: `io.ErrUnexpectedEOF`.
  - `TestFixShUF2CountsOwnFamilyOnly`: a 1-block other-family file then the first 2 blocks of a 3-block own file:
    `io.ErrUnexpectedEOF` (catches a counter that counts every block, as the triage's scratch fix did).
- cmd/picosign/fix_sh_uf2_truncation_test.go (uses the existing `fixShUF2` and `fixShRunChild` helpers of
  fix_sh_output_sink_test.go):
  - `TestFixShPicosignTruncatedUF2`: the 24-block fixture with its last block removed; `hash <f>`, `extract <f>` and
    `sign -clear <f>`: exit 2, no stdout, stderr exactly the §3 "after" line, file bytes unchanged.

Tests that change: none. `TestRead` and `TestWrite` use testdata/2blocks.uf2, whose headers declare 23 blocks for a
2-block file; neither reads to end of input (TestRead reads 256 bytes, TestWrite `io.ReadFull`s 500), so both pass
unchanged (seen, §6). The skipped `TestBughuntSh219*`/`TestBughuntSh220*` build blocks with `numBlocks` 0 and are not
affected (R5).

## 6. RED rows (run on base 25abe8e with the §5 tests added)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixSh|TestRead|TestWrite' -v ./uf2/ ./cmd/picosign/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestFixShUF2BoundaryCut | FAIL: `cut at 512 of 3 blocks: read 256 bytes, err <nil>`; `cut at 1024 ...: read 512 bytes, err <nil>` | PASS |
| TestFixShUF2MidBlockCut | FAIL, 8 lines: cuts 32 (0 bytes), 544 (256), 644 (356), 800 (512), each reader, `err <nil>` | PASS |
| TestFixShUF2OtherFamilyBlockCut | FAIL: `cut 1056 bytes into a 3-block concatenation: read 512 bytes, err <nil>` | PASS |
| TestFixShUF2CountsOwnFamilyOnly | FAIL: `1 other-family block + 2 of 3 own blocks: read 512 bytes, err <nil>` | PASS |
| TestFixShPicosignTruncatedUF2 | FAIL: hash, extract, sign each `exit 2 stderr "picosign: [verb: ]<f>: picobin: missing block header\n"` | PASS |
| TestFixShUF2CompleteFile, TestFixShUF2ConcatenatedFamilies, TestFixShUF2EmptyInput, TestRead, TestWrite | PASS (controls) | PASS |

Mutations seen on the scratch fix: counting every block (before the family check) fails
TestFixShUF2CountsOwnFamilyOnly; dropping the payload `io.EOF` conversion fails TestFixShUF2MidBlockCut (the
`eof-with-data` cuts); dropping the footer conversion fails TestFixShUF2MidBlockCut (cut 800) and
TestFixShUF2OtherFamilyBlockCut.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 own-family count, first block's total | TestFixShUF2CountsOwnFamilyOnly; TestFixShUF2ConcatenatedFamilies |
| R2 boundary cut; zero own blocks and complete files stay clean | TestFixShUF2BoundaryCut; TestFixShUF2EmptyInput; TestFixShUF2CompleteFile; TestFixShPicosignTruncatedUF2 |
| R3 cut inside a block, both reader styles, other family too | TestFixShUF2MidBlockCut; TestFixShUF2OtherFamilyBlockCut |
| R4 bare io.ErrUnexpectedEOF, picosign text | every RED row asserts `errors.Is`; TestFixShPicosignTruncatedUF2 asserts the exact lines |
| R5 scope unchanged | TestRead; TestWrite; skipped bughunt tests untouched |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`) ;
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ; `./scripts/test-32bit.sh` ;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI). Go: go1.26.3 with `GOTOOLCHAIN=local` (CI pins 1.26).
Measured on the scratch fix by the author: `go test ./uf2/ ./cmd/picosign/ ./picobin/` ok (23 PASS, 2 SKIP);
`GOARCH=386 go test ./uf2/ ./cmd/picosign/` ok; `go vet ./uf2/ ./cmd/picosign/` ok; gofmt clean. Not run by the
author: the rest of `./...` and the other jobs (the firmware does not import `uf2`). The builder runs the whole gate.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (no `mr`/`md-cli` text; the manual never mentions picosign or uf2).
- Closes: bh-sh-I7 with the GREEN merge. seedhammer has no `design/FOLLOWUPS.md`; nothing tracked there to close.
- Files (proposed, verbatim, for direct-push after the merge; target file is the coordinator's choice):

```
- [sh-uf2-block-numbering] status=open grade=M kind=input owner=none filed=2026-11-22 src=seedhammer:design/FIX_sh_uf2_truncation.md repo=sh :: uf2.Reader checks only the count of own-family blocks against the first block's numBlocks; blockNo sequence, numBlocks differing between blocks, own-family blocks beyond numBlocks, and numBlocks=0 (truncation then undetectable) are unchecked; uf2/testdata/2blocks.uf2 declares 23 blocks for 2.
```

## 10. Out of scope

R5's checks (filed above); bug-hunt #19 payload size over 476 and #20 start address 0 (their own deferred-minor
tests); firmware-side UF2 handling (the device bootloader, not this package).

## 11. How this was checked

RED and GREEN rows run by the author as in §6 (probe diff sha256 in the report); the probe diff applies with
`git apply --check` on 25abe8e and was reverted after the run. Every picosign cut (1 to 23 of 24 blocks) at base:
exit 2 with a picobin error, none exits 0. Two real picotool-sealed firmware UF2s (4668 and 5888 blocks, one family,
blockNo 0..n-1, numBlocks n on every block) give byte-identical `picosign hash` output (sha256 of the output equal) and
the same `extract` output before and after the scratch fix. The UF2 quotes in §1 were read from the specification
text, not recalled.
