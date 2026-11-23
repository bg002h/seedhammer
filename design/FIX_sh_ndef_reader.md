# FIX: the NDEF readers return the same bytes at every buffer size and for every legal io.Reader

**Small track** (one Opus review round). Grade I (bh-sh-I8); bh-sh-I9 is graded M and rides in the same file.
Keys: bughunt-1119 `bh-sh-I8`, `bh-sh-I9`. Table row 56, coordinator `agent-reports/architect-bughunt-1119-questions.md`
L168 (blocked-by: none). Grades: coordinator `agent-reports/bughunt-1119/grade-sh.md` L37-38; fix shape there at L70
("Keep pending-prefix state, and handle EOF with a zero length plus the skipped-TLV byte count"). P7 (library-only
reach, recommend I) is why I8 stays I although the device cannot reach it (grade-sh.md L37, L84). No T-ruling and no
Brian ruling (B1-B10) applies; none is needed.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are
`nfc/ndef/ndef.go` at that base (the file is unchanged since the bug hunt's 9aec7a8).

## 1. The defects

- **bh-sh-I8 (RecordReader, L202-224).** A URI record's identifier code expands to a prefix (`https://`, up to 12
  bytes for `https://www.`). L223 copies as much of it as fits in the caller's buffer and drops the rest, then the
  next Read continues with the payload. Record `d1 01 02 55 04 61` (HTTPS, body `a`) reads `https://a` with a 4 KiB
  buffer, `ha` with a 1-byte buffer and `httpa` with 4 bytes, each with a normal io.EOF (seen, §6). Every caller that
  reads through `iotest.OneByteReader` or any buffer under the prefix length gets the wrong URI with no error.
- **bh-sh-I9 (MessageReader, L38-49).** When the underlying reader returns the last bytes of a TLV together with
  io.EOF (legal under the io.Reader contract; `iotest.DataErrReader` does it), L42-43 returns those bytes with
  `ndef: tlv: EOF`, before L45 looks at the skip flag. A complete NDEF TLV `03 03 61 62 63` reads `abc` plus that
  error (it reads `abc`, nil over `bytes.Reader`); a skipped TLV `fd 03 61 62 63` reads `abc` plus the error, so a
  container the API promises to skip is exposed (seen, §6).
- **Same class in RecordReader (author's scope call, same file).** L101-104 returns the bytes of a skipped record
  when the stream ends early (`d2 01 05 78 61 62 63` through DataErrReader reads `abc`); `discard` (L238-248) fails a
  record whose type or ID ends the stream with its last bytes delivered alongside io.EOF (`d1 02 00 61 62` reads
  `ndef: message: EOF` through DataErrReader, nothing and nil through `bytes.Reader`).
- **Truncation reads as end of stream.** Every mid-structure read wraps io.EOF (`ndef: tlv: EOF`,
  `ndef: message: EOF`), so a caller that tests `errors.Is(err, io.EOF)` takes a truncated TLV or record for a
  complete one. The consultant's fix line for I9 asks for "unexpected EOF when the declared length is still unmet".

Device reach (grade-sh.md L37-38, L84; unchanged by this FIX): the device reads NDEF only through
bufio(256) -> MessageReader -> RecordReader -> gui/scan.go's 8 KiB buffer (nfc/poller/poller.go:84-90,
gui/scan.go:29-52). bufio never returns data with io.EOF, so I9 cannot occur there, and an I8 short buffer only occurs
when fewer than 8 bytes of the scan buffer are left, where the scan overflows and is dropped either way.

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (URI prefix, I8).** RecordReader keeps the not-yet-returned part of a URI record's expanded prefix across
  Reads and returns it before any payload byte. For every buffer size and every underlying reader, the sequence of
  records a caller reads (a record ends at a Read that returns io.EOF) is byte-for-byte the sequence it reads with a
  4 KiB buffer over `bytes.Reader`. That reference result itself is unchanged by this FIX, including the case where a
  URI record is only its identifier code (see §10).
- **R2 (data with io.EOF, I9).** Bytes a reader returns together with io.EOF are accounted before the EOF is handled.
  When they complete the TLV (MessageReader) or the type/ID being discarded (RecordReader `discard`), the structure is
  complete: MessageReader returns the TLV's bytes with a nil error and the next Read returns `0, io.EOF`; `discard`
  succeeds and the record is processed as it is over `bytes.Reader`. RecordReader's payload path already does this
  (L102) and is unchanged.
- **R3 (skipped containers, I9).** A skipped TLV (any TLV type other than 0x03) or a skipped record (chunked,
  TNF other than well-known, unknown well-known type, UTF-16 Text, unknown URI code) returns zero bytes from every
  Read, including a Read that returns an error.
- **R4 (truncation).** A stream that ends after a TLV's type byte and before the end of its value, or after a
  record's first header byte and before the end of its payload, returns an error that wraps `io.ErrUnexpectedEOF`
  and does not wrap io.EOF. Texts (Go's `io.ErrUnexpectedEOF` text after the existing prefixes; PROPOSED, architect
  confirms): `ndef: tlv: unexpected EOF` and `ndef: message: unexpected EOF`. Bytes of a non-skipped TLV or record
  read before the truncation are still returned (as today).
- **R5 (unchanged).** A stream that ends between structures stays a bare `io.EOF` (MessageReader: before a type byte,
  or at the 0xfe terminator; RecordReader: before a record header, including a message whose last record lacks ME,
  L117-120). Every other error text is unchanged (`ndef: tlv: <err>` / `ndef: message: <err>` for non-EOF reader
  errors, `ndef: message: expected start record`, `ndef: message: text language too long`). Output over
  `bytes.Reader` with a 4 KiB buffer is unchanged for every input (TestRecords, TestMessages pass unmodified).

## 3. Holder-visible changes (complete list)

| where | condition | before | after |
|---|---|---|---|
| device screen | any NFC scan | (no change: the scanner drops every error that is not a bare io.EOF, gui/scan.go:43-51, before and after) | same |
| device debug log, gui/scan.go:50 `nfc poller: <err>` | tag data truncated inside a TLV or record | `nfc poller: ndef: tlv: EOF` / `... ndef: message: EOF` | `nfc poller: ndef: tlv: unexpected EOF` / `... ndef: message: unexpected EOF` (PROPOSED) |
| `seedhammer.com/nfc/ndef` API | §1 inputs | wrong bytes / leaked bytes / wrapped io.EOF | R1-R4 |

No flag, verb, command, screen or type is added. No other package matches the error strings
(`git grep -n 'ndef: tlv\|ndef: message'` finds only nfc/ndef/ndef.go). Toolkit manual: none (no `mr`/`md-cli`
text; NFC is device-only).

## 4. Code changes (builder applies; all in nfc/ndef/ndef.go)

The scratch version is `design/FIX_sh_ndef_reader-probe.diff` (fix plus the §5 test file; evidence, not a patch to
copy blindly; `git apply --check` passes on 25abe8e).

1. RecordReader (L21-27) gets a `prefix string` field (comment: the part of a URI prefix not yet returned, R1).
2. RecordReader.Read: at the top of the loop (before L97), a non-empty `prefix` is copied into `buf`, the copied part
   removed, and `n, nil` returned. The URI case (L223-224) stores the prefix in the field and `continue`s instead of
   copying it; the `n := 0` at L181 goes (unused).
3. MessageReader.Read L40-48: after `r.length -= n`, a skipped TLV sets `n = 0` (R3); `err == io.EOF && r.length == 0`
   becomes nil (R2); a remaining error is returned through the helper of step 6 (R4).
4. RecordReader.Read L101-104: in the error branch, a skipped record sets `n = 0` (R3); the error goes through the
   helper (R4).
5. `discard` (L238-248): `err == io.EOF` with nothing left to discard returns nil (R2).
6. A helper `truncated(err error) error` maps exactly `io.EOF` to `io.ErrUnexpectedEOF` (comment: names this FIX and
   R4). It wraps the error at every mid-structure read: MessageReader L68-69 (length), L76-77 (2-byte length), the
   value read of step 3; RecordReader L134-135 (32-bit length), L141-142 (short length), L149-151 (ID length),
   L159-160 (type byte), L166-167 (type/ID discard), L185-186 (Text status byte), L198-199 (language discard),
   L204-205 (URI code), and step 4. Not at L52-56 or L117-121 (start of a structure: R5).

## 5. Tests

New file nfc/ndef/fix_sh_ndef_reader_test.go (in the probe diff); test function names are the TRACE row names. All
fixtures are hex NDEF bytes with no key material.
- `TestFixShNdefURIPrefixEveryBuffer`: URI records with codes 0x00-0x04 and body `a`
  (`a`, `http://www.a`, `https://www.a`, `http://a`, `https://a`), every buffer size 1 to len(want)+1, plus
  `iotest.OneByteReader` over the RecordReader; want exactly one record equal to the literal.
- `TestFixShNdefRecordsEveryBuffer`: the four short TestRecords fixtures plus URI-then-Text
  (`91010255046151010354006869`), prefix-only-URI-then-Text (`910101550451010354006869`) and skipped-URI-then-URI
  (`910102552461510102550261`); buffers 1 to 32 over `bytes.Reader` and `iotest.OneByteReader`; want the record list
  read with a 4 KiB buffer over `bytes.Reader` (computed in the test, not a literal).
- `TestFixShNdefMessageDataWithEOF`: MessageReader over `bytes.Reader`, `OneByteReader` and `DataErrReader` for
  `0303616263` (want `abc`, nil), `fd03616263` (want empty, nil), TestMessages' first fixture (want its content) and
  `030161fd03626364` (NDEF TLV then a skipped TLV that ends the stream; want `a`, nil).
- `TestFixShNdefRecordDataWithEOF`: RecordReader over the same three readers, buffers 1 and 4096: Text
  `d101045400616263` (want `[abc]`), type-ends-stream `d102006162` (want no record, nil), media record
  `d2010378616263` (want no record, nil), URI `d10102550461` (want `[https://a]`).
- `TestFixShNdefTruncatedIsUnexpectedEOF`: 13 truncations, each over `bytes.Reader` and `DataErrReader`, want the
  listed bytes and an error whose text is exact and which `errors.Is` io.ErrUnexpectedEOF and not io.EOF:
  MessageReader `0305616263` (`abc`), `fd05616263` (empty), `03`, `03ff01`, `03ff`; RecordReader `d101`, `d90101`,
  `d10100`, `d1020061`, `d10105540061` (`a`), `d2010578616263` (empty), `c101`, `c1010000`. Controls in the same
  row: MessageReader on empty, `fe`, `0300` and RecordReader on empty, `d00000` each return `0, io.EOF` (R5).

Tests that change: none. TestRecords and TestMessages pass unmodified; TestBughuntSh218NdefEmptyTextURIRecord stays
skipped (`deferred-minor: sh-ndef-empty-record`).

## 6. RED rows (run on base 25abe8e with the §5 test file added; go1.26.4, GOTOOLCHAIN=local)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixShNdef' ./nfc/ndef/`

| row | at base (seen; failing lines) | after the fix (seen on the probe) |
|---|---|---|
| TestFixShNdefURIPrefixEveryBuffer | FAIL, 38 lines, e.g. `d10102550461 buffer 1: got ["ha"], <nil>; want ["https://a"], nil` and `d10102550461 OneByteReader: got "ha", <nil>; want "https://a", nil` | PASS |
| TestFixShNdefRecordsEveryBuffer | FAIL, 78 lines, e.g. `d9010c0155ff046578616d706c652e636f6d bytes buffer 1: got ["hexample.com"], <nil>; want ["https://example.com"], nil` | PASS |
| TestFixShNdefMessageDataWithEOF | FAIL, 3 lines: `0303616263 data-err: got "abc", ndef: tlv: EOF; want "abc", nil`; `fd03616263 data-err: got "abc", ndef: tlv: EOF; want "", nil`; `030161fd03626364 data-err: got "abcd", ndef: tlv: EOF; want "a", nil` | PASS |
| TestFixShNdefRecordDataWithEOF | FAIL, 5 lines: `d102006162 data-err buffer 1: got [], ndef: message: EOF; want [], nil` (and buffer 4096); `d10102550461 {bytes,one-byte,data-err} buffer 1: got ["ha"]` | PASS |
| TestFixShNdefTruncatedIsUnexpectedEOF | FAIL, 22 lines, e.g. `skipped tlv value short (fd05616263) data-err: got "abc", "ndef: tlv: EOF" (Is io.EOF=true, Is io.ErrUnexpectedEOF=false); want "", "ndef: tlv: unexpected EOF" wrapping io.ErrUnexpectedEOF only`; `03ff01` and `c1010000` already pass (io.ReadFull's partial read) | PASS |

Mutations seen on the probe (each one change, `go test ./nfc/ndef/`): prefix copied and dropped again (L223 form) fails
URIPrefixEveryBuffer, RecordsEveryBuffer, RecordDataWithEOF; MessageReader skip `n = 0` removed fails
TruncatedIsUnexpectedEOF; MessageReader EOF-complete-is-nil removed fails MessageDataWithEOF; RecordReader skip
`n = 0` removed fails TruncatedIsUnexpectedEOF; `discard` EOF fix removed fails RecordDataWithEOF; `truncated` made
the identity fails TruncatedIsUnexpectedEOF.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 URI prefix at every buffer | TestFixShNdefURIPrefixEveryBuffer; TestFixShNdefRecordsEveryBuffer; URI subtest of TestFixShNdefRecordDataWithEOF |
| R2 data with io.EOF completes the structure | TestFixShNdefMessageDataWithEOF (`0303616263`, `030161fd03626364`); TestFixShNdefRecordDataWithEOF (`d102006162`) |
| R3 skipped containers expose nothing | TestFixShNdefMessageDataWithEOF (`fd03616263`); TestFixShNdefRecordDataWithEOF (`d2010378616263`); TestFixShNdefTruncatedIsUnexpectedEOF (`fd05616263`, `d2010578616263`) |
| R4 truncation is io.ErrUnexpectedEOF | TestFixShNdefTruncatedIsUnexpectedEOF (13 cases, both readers) |
| R5 unchanged | controls in TestFixShNdefTruncatedIsUnexpectedEOF; TestRecords; TestMessages; `bytes` columns of the rows above |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

Gate script line: `CGO_ENABLED=0 go test -timeout 20m ./... && CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/ && ./scripts/test-32bit.sh && GOOS=js GOARCH=wasm go vet ./cmd/emu/ && GOOS=js GOARCH=wasm go vet -tags refugium ./cmd/emu/ && ./scripts/refugium-tagged-test.sh`, plus both TinyGo device builds and `scripts/refugium-elf-check.sh` (CI; .github/workflows/test.yml L75-160). gui may be
sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`.
Measured on the probe by the author (go1.26.4): `go test ./nfc/...` all five packages ok; `go vet ./nfc/ndef/` ok;
`GOARCH=386 go test ./nfc/ndef/` ok; `go test -run 'Scan|NFC|Nfc|Poll' ./gui/` ok; gofmt clean. Not run by the
author: the rest of `./...` and the other jobs. The builder runs the whole gate. The only importer of nfc/ndef is
nfc/poller (poller.go:11).

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. WORDING_REVIEW: the two R4 texts are Go's `io.ErrUnexpectedEOF` after existing prefixes and
  reach only the debug log; the architect confirms whether they need a row.
- Closes: bh-sh-I8 and bh-sh-I9 with the GREEN merge. seedhammer has no `design/FOLLOWUPS.md`, and no FOLLOWUPS line
  in mnemonic-engrave or the coordinator names either key; nothing to close.
- Files: two lines, listed verbatim in the author's report (coordinator
  `agent-reports/fix-doc-bh-sh-ndef-reader.md`), for direct-push after the merge (§10).

## 10. Out of scope

- **Prefix-only URI record merges with the next record.** `910101550451010354006869` (a URI record that is only code
  0x04, then Text `hi`) reads as one record `https://hi` at every buffer size, before and after this FIX: the prefix
  path returns no io.EOF when the payload is empty. R1 keeps that reference result; the record-boundary question is
  the open product question of `sh-ndef-empty-record` (minimal and zero-length Text/URI records), filed as a sibling
  line.
- **Zero-length Read hangs.** `NewRecordReader(...).Read(nil)` on a record with a type or ID to discard
  (`d1020361625400616263`) never returns (`discard` reads into the caller's empty buffer; test timed out at 10 s on
  25abe8e and on the probe). The device never passes an empty buffer (gui/scan.go:33-39 resets on a full buffer).
  Filed.
- A message whose last record lacks ME (R5), chunked records, UTF-16 Text: unchanged.

## 11. How this was checked

RED and GREEN rows run by the author as in §6 (outputs /scratch/cache/sh-ndef-reader-red.txt and
-green.txt; probe diff sha256 in the report). The probe was reverted after the run; the branch carries only this
document and the probe diff. Every quoted RED line is copied from the run, not written by hand.
