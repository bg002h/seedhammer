# FIX: a bip32 path element without a hardened marker is refused at 2^31 and above

**Small track** (one Opus review round). Important (regrade of bh-sh-M6, Minor -> Important).
Key: bughunt-1119 `bh-sh-M6` (coordinator `agent-reports/bughunt-1119-mn-regrade/sh-grade.md` L64-75), the same defect
as FOLLOWUPS `sh-parsepath-unhardened-overflow` (mnemonic-engrave `design/FOLLOWUPS.md` L20903-20912, sh2 finding 35).
Ruling (decided, not re-ruled here): the coordinator ruled REFUSE on the grader's open question (brief
`briefs/fix-doc-bh-sh-bip32-unhardened.md`).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-24. Base: seedhammer `origin/main` a042d824a684d3637b68f05e4f7927eef3caad66.
All line anchors below are at that base.

## 1. The defect

`bip32.ParsePathElement` (bip32/bip32.go:69-84) strips an `h` or `'` suffix, parses the digits with
`strconv.ParseInt(p, 10, 0)` (:75), and bounds the value only against uint32 (:79-82). An element with no hardened
marker and a value of 2^31 or more therefore passes, and its value lands in the hardened half: `2147483648` becomes
`0x80000000` (0h) and `4294967295` becomes `0xffffffff` (2147483647h).

The parser is on the descriptor path (`bip380.ParseKey` origin via `bip32.ParsePath`, bip380/bip380.go:401; use-site steps via `ParsePathElement`, :498, :502, :516). The acceptance happens only where Go's `int` is 64 bits: `strconv.ParseInt(p, 10, 0)` uses the `int` width, which is 32 bits on the SeedHammer II (TinyGo, scripts/test-32bit.sh:5-6). There, 2147483648 and above already fail with `bip32: invalid path element`, and the descriptor is already refused (`Unknown format`). On 64-bit builds (host biptool, the js/wasm emulator, the host test suite), a scanned `wpkh([d34db33f/84h/0h/2147483648]tpub.../0/*)` is accepted with origin `m/84h/0h/0h`. The fix makes every build refuse it, as the device already does.

### The rule, from the source text (bitcoin/bips master, fetched 2026-11-24)

- BIP-380 L73 (key origin): "Followed by zero or more `/NUM` or `/NUMh` path elements to indicate the unhardened or
  hardened derivation steps between the fingerprint and the key that follows." L81 (after an extended key): "Followed
  by zero or more `/NUM` or `/NUMh` path elements indicating BIP 32 derivation steps to be taken after the given
  extended key." L89: "the hardened indicator `h` may be replaced with alternative hardened indicator of `'`."
- BIP-380 L251, in the list of invalid expressions: "Derivation index out of range: `xprv9s21ZrQH143K31xYSDQ...`
  `/2147483648`". An unmarked 2147483648 is invalid, not an alternative spelling of 0h.
- BIP-32 L66: "The normal child keys use indices 0 through 2<sup>31</sup>-1. The hardened child keys use indices
  2<sup>31</sup> through 2<sup>32</sup>-1. To ease notation for hardened key indices, a number i<sub>H</sub>
  represents i+2<sup>31</sup>."

So an unmarked NUM is an unhardened step and must be below 2^31; a marked NUM must be below 2^31 before the 2^31 offset
is added. The fix refuses an element whose digits are 2^31 or more, marked or not.

## 2. Spec delta

- R1. `ParsePathElement` refuses an element whose digit value (after any `h`/`'` suffix is stripped) is negative or
  2^31 or more, with the existing error text `bip32: path element out of range: %q` (%q is the element with its suffix
  stripped). It is checked after `ParseInt` succeeds and before the hardened offset is added.
- R2. Every value that parsed before and is below 2^31 parses to the same uint32 as before; `0h`/`0'` give
  0x80000000 and `2147483647h` gives 0xffffffff. Every element refused before stays refused; on 64-bit builds with the same text, and on the 32-bit device with the text `out of range` where it was `invalid path element` (M1, §4).
  (On 64-bit builds `2147483648h` and `-1` already gave `bip32: path element out of range`; on the 32-bit device `2147483648h` gave `bip32: invalid path element`.)
- R3. `ParsePath` refuses any path that holds such an element (it calls `ParsePathElement`, bip32.go:94). No caller
  changes.
- R4. A descriptor text carrying such an element at the origin or at a use-site step is not a descriptor
  (`bip380.Parse` / `bip380.ParseKey` error). At the NFC scan door this makes it an unknown format; at the payload
  door it is `ClassUnknown` (inert). §3 lists exactly what the holder sees.

No new holder-visible text, flag, exit code, type or command.

## 3. Holder-visible changes (complete list)

| where | at base, 64-bit builds (host biptool, wasm emulator, host tests) | after the fix |
|---|---|---|
| NFC scan, main screen: `wpkh([d34db33f/84h/0h/2147483648]K/0/*)` (origin element) | descriptor screen opens; origin shown and engraved as `m/84h/0h/0h` | `scanner.Scan` (gui/scan.go:87) gets an error from `nonstandard.OutputDescriptor`, every later arm declines, so it returns `errScanUnknownFormat` (:105); `startScanner` sets `scanUnknownFormat` (gui/nfc_scan.go:87-88); the main screen subtitle reads the existing text **`Unknown format`** (gui/gui.go:2400-2401) |
| NFC scan, main screen: `wpkh([fp/84h/0h/0h]K/2147483648/*)` (use-site element) | parsed as a hardened `/0h/*` child; already refused by `sysw.UseSitesOK` in `engraveObjectFlow` (gui/gui.go:2644-2648), which shows `Unknown format` | same text `Unknown format`, now from the scanner (`errScanUnknownFormat`). No visible change |
| NFC scan, other screens (verify address gui/verify_address.go:97-98; derive xpub seed gather gui/derive_xpub.go:363-364) | `That tag is not a seed.` (derive xpub, object recognised); address verify ignores non-address objects | existing texts `Unrecognized tag.` (derive xpub) and `Not a recognized address.` (verify address) |
| Payload door (`sysw.Classify`, sysw/record.go:132 -> sysw/descriptor.go:87) | `ClassDescriptor`, offered as a wallet policy | `ClassUnknown`: inert, offered to nobody, counted in the composer door's not-understood count (gui/composer_door.go:34-37, :57). `gui/wallet_policy.go:117`'s `Couldn't read the wallet policy from the payload.` is not reached (the record is not admitted) |
| BlueWallet multisig text, `Derivation: m/48'/0'/0'/2147483648` (nonstandard/parse.go:111) | origin `m/48h/0h/0h/0h` | not a descriptor; at the scan door `Unknown format` as above |
| `biptool derive -path m/83696968h/32h/2147483648 xprv` (cmd/biptool/main.go:143) | exit 0, derives the BIP-85 child at index 0h | exit 2, stderr `biptool: bip32: path element out of range: "2147483648"` (existing text and exit path) |
| the device (TinyGo, 32-bit `int`), every device door above (all but biptool) | already refused: `bip32: invalid path element`, `Unknown format` / `Unrecognized tag.` / `Not a recognized address.` / `ClassUnknown` | unchanged, except that the error text becomes `bip32: path element out of range` (M1 is taken, §4) |

PROPOSED texts for WORDING_REVIEW: none. Every text above already exists at base.

## 4. Code change (builder applies)

bip32/bip32.go `ParsePathElement`: between the `ParseInt` error return (:76-78) and `iu32 := uint32(idx)` (:79),
insert a check that returns `0, fmt.Errorf("bip32: path element out of range: %q", p)` when `idx < 0 || idx >= 1<<31`,
with a two-line comment: BIP-32 puts normal children at 0 through 2^31-1 and BIP-380 reads an unmarked NUM as an
unhardened step, so 2^31 or more is out of range whether or not it is marked. Leave the existing uint32 check at
:80-82 as it is (now unreachable for the overflow case; removing it is not part of this fix). No other production file
changes except the one-line change below. The scratch version is `design/FIX_sh_bip32_unhardened-probe.diff` (§9); it holds the check but NOT the ParseInt change below, which the builder adds by hand.

Also change bip32/bip32.go:75 from `strconv.ParseInt(p, 10, 0)` to `strconv.ParseInt(p, 10, 64)`, so the refusal text
`bip32: path element out of range: %q` is the same on 32- and 64-bit builds (with bitSize 0 the 32-bit device returns
`bip32: invalid path element` for 2147483648 and above; the reviewer measured `path element out of range` for
`2147483648`, `4294967295`, `4294967296` and `2147483648h` on both GOARCH=386 and amd64).

## 5. Callers (grep `ParsePathElement(\|ParsePath(` over non-test .go files at base)

Behaviour changes (an unmarked element of 2^31 or more can arrive as text from outside):

| caller | input | effect |
|---|---|---|
| bip32/bip32.go:94 `ParsePath` | path text | refuses (R3) |
| bip380/bip380.go:401 `ParseKey`, key origin | scanned / payload descriptor | `hdkey: invalid derivation path` error; descriptor refused |
| bip380/bip380.go:516 `parsePath`, fixed use-site step | same | `bip32: path element out of range` error; descriptor refused |
| bip380/bip380.go:498, :502 `parsePath`, range `<a;b>` ends | same | acceptance unchanged (:507-508 already refused an end of 2^31 or more); the error text for that case changes from `invalid range path element: "<0;2147483648>"` to `bip32: path element out of range: "2147483648"`. Not holder-visible (the scan door shows `Unknown format` either way) |
| nonstandard/parse.go:111 BlueWallet `Derivation:` header | scanned / payload text | refused |
| reached through `bip380.Parse` / `ParseKey` / `nonstandard.OutputDescriptor`: gui/scan.go:87 (NFC scan door), sysw/descriptor.go:87, :160, :267 (payload door, `sysw.Classify`), seal/record.go:272 (`seal.Classify`), gui/wallet_policy.go:117 | as above | see §3. `seal.Classify` now gives `ClassUnknown` for such a record, but no screen sees it: `AdmitSection` refuses any record carrying an extended key at its lowercase pass (seal/record.go:330) before it classifies |
| cmd/biptool/main.go:143 `derive -path` | operator flag | refused, exit 2 (§3). biptool is host tooling, not a supported recovery path (ruling B9) |

No behaviour change (the text is rendered by the device from a uint32 path, so an unmarked element is below 2^31 by
construction):

| caller | why |
|---|---|
| gui/composer_sources.go:130, gui/key_card_seating.go:129, gui/multisig_build_slots.go:415, gui/multisig_build_tail.go:95, gui/multisig_build.go:1400 | `mk.Card.Path`, produced by `mk.Decode` through `pathString` (mk/mk.go:369-381), which writes an element without the hardened bit as plain digits, below 2^31 |
| mk/encode.go:139 `resolvePath` | its callers pass `bip32.Path.String()` (gui/derive_xpub.go:449, gui/singlesig_derive.go:86, gui/multisig_derive.go:51), `composerOriginText` of components built by `originComponents` from a `bip32.Path` (gui/composer_cards.go:43, :51; gui/singlesig_derive.go:138-150), or a card re-encoded from `mk.Decode` (gui/template_engrave.go:48, path from `pathString`) |
| sysw/composer_records.go:352 | the loop at :342-351 already refuses digits of 2^31 or more before calling it |
| cmd/buildpayloadcards/main.go:89, cmd/buildpayloadcomposer/main.go:161, cmd/journeykeys/main.go:35 | fixture generators with constant `m/48'/0'/N'/2'` paths |

## 6. Tests and RED rows

Test function names are the TRACE row names. All keys are testnet/signet `tpub`s, or the mainnet `xpub`s already in
the BlueWallet fixtures (`bhBWKey1`/`bhBWKey2`, public keys only).

Un-skipped: bip32/bughunt_sh2_s3_test.go `TestBughuntSh235ParsePathUnhardenedOverflow` (delete the
`t.Skip("deferred-minor: sh-parsepath-unhardened-overflow")` line, :14). Its body is unchanged.

New files (in the probe diff):
- bip380/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedOriginRefused` (origins `84h/0h/2147483648`,
  `84h/0h/4294967295`, `2147483648`, `48'/1'/2147483648/2'`: `Parse` errors);
  `TestFixShBip32UnhardenedChildRefused` (use-sites `/2147483648/*`, `/0/2147483648`, `/4294967295/*` error; range
  `/<0;2147483648>/*` stays refused); `TestFixShBip32UnhardenedBoundaryAccepted` (control: origin
  `m/84h/1h/2147483647` and child 2147483647 unhardened; origin `2147483647h` is 0xffffffff).
- gui/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedScanDoorUnknownFormat` (control origin
  `.../2147483647` scans to a `*bip380.Descriptor`; origin `.../2147483648` and use-site `/2147483648/*` give
  `errScanUnknownFormat` from `scanOnce`).
- sysw/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedPayloadUnknown` (control `[d34db33f/84h/1h/0h]`
  is `ClassDescriptor`; `[d34db33f/84h/1h/2147483648]` is `ClassUnknown`).
- seal/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedSealUnknown` (same pair through `seal.Classify`).
- nonstandard/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedBlueWallet` (control `Derivation:
  m/48'/0'/0'/2147483647` parses; `m/48'/0'/0'/2147483648` and `m/48'/0'/2147483648/2'` refused).
- cmd/biptool/fix_sh_bip32_unhardened_test.go: `TestFixShBip32UnhardenedBiptoolPath` (`derive -path
  m/83696968h/32h/2147483648 xprv` on `fixArgsXprv`: exit 2, no stdout, stderr exactly
  `biptool: bip32: path element out of range: "2147483648"\n`).

Tests that change: only the un-skip above. No other existing test changes; the whole suite passes on the scratch fix
(§8).

RED command (run on base a042d824 with the tests added and the skip removed; Go go1.26.7, `CGO_ENABLED=0`):
`go test -p 6 -count=1 -v -run 'TestFixShBip32Unhardened|TestBughuntSh235' ./bip32/ ./bip380/ ./cmd/biptool/ ./nonstandard/ ./sysw/ ./seal/ ./gui/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestBughuntSh235ParsePathUnhardenedOverflow | FAIL `ParsePath("m/2147483648") accepted as m/0h`, `ParsePath("m/4294967295") accepted as m/2147483647h`, `ParsePath("m/48h/2147483648/0") accepted as m/48h/0h/0`, `ParsePathElement("2147483648") accepted as 0x80000000`, `ParsePathElement("4294967295") accepted as 0xffffffff` | PASS |
| TestFixShBip32UnhardenedOriginRefused | FAIL 4/4, e.g. `Parse("wpkh([d34db33f/84h/0h/2147483648]tpub.../0/*)") accepted; origin m/84h/0h/0h`; `[d34db33f/48'/1'/2147483648/2']` -> `origin m/48h/1h/0h/2h` | PASS |
| TestFixShBip32UnhardenedChildRefused | FAIL 3/3 fixed steps, e.g. `/2147483648/*` -> `children [{Type:0 Index:0 Hardened:true ...} {Type:1 ...}]`; the range line passes | PASS |
| TestFixShBip32UnhardenedBiptoolPath | FAIL `exit 0 stdout "xprv9s21ZrQH143K2srSbCSg4m4kLvPMzcWydgmKEnMmoZUurYuBuYG46c6P71UGXMzmriLzCCBvKQWBUv3vPB3m1SATMhp3uEjXHJ42jFg7myX\n" stderr ""` | PASS |
| TestFixShBip32UnhardenedBlueWallet | FAIL `Derivation m/48'/0'/0'/2147483648 accepted; origin m/48h/0h/0h/0h` and `m/48'/0'/2147483648/2'` -> `origin m/48h/0h/0h/2h` | PASS |
| TestFixShBip32UnhardenedPayloadUnknown | FAIL `Classify("wpkh([d34db33f/84h/1h/2147483648]tpubDCXM.../<0;1>/*)") = 5, want ClassUnknown` (5 is ClassDescriptor) | PASS |
| TestFixShBip32UnhardenedSealUnknown | FAIL `Classify(...) = output descriptor, want ClassUnknown` | PASS |
| TestFixShBip32UnhardenedScanDoorUnknownFormat | FAIL `scan "wpkh([d34db33f/84h/0h/2147483648]tpub.../0/*)" = descriptor with origin m/84h/0h/0h ..., err <nil>; want errScanUnknownFormat`; the use-site text likewise (`children [{... Hardened:true ...}`) | PASS |
| TestFixShBip32UnhardenedBoundaryAccepted | PASS (control) | PASS |

Device word size: the same rows minus gui and biptool, run with `GOARCH=386`, PASS at base (`ok` for bip32, bip380, nonstandard, sysw and seal), because the device's 32-bit `strconv.ParseInt(p, 10, 0)` already refuses. The RED rows are 64-bit-build rows.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 refusal, text, marked and unmarked | TestBughuntSh235ParsePathUnhardenedOverflow (`2147483648h` control, unmarked rows); TestFixShBip32UnhardenedBiptoolPath (exact text); TestFixShBip32UnhardenedPathElementOutOfRangeText (text at both word sizes, run at GOARCH=386) |
| R2 boundaries unchanged | TestBughuntSh235ParsePathUnhardenedOverflow controls (`2147483647`, `0h`, `0'`, `2147483647h`); TestFixShBip32UnhardenedBoundaryAccepted |
| R3 ParsePath | TestBughuntSh235ParsePathUnhardenedOverflow (`m/48h/2147483648/0`) |
| R4 descriptor doors | TestFixShBip32UnhardenedOriginRefused, TestFixShBip32UnhardenedChildRefused, TestFixShBip32UnhardenedScanDoorUnknownFormat, TestFixShBip32UnhardenedPayloadUnknown, TestFixShBip32UnhardenedSealUnknown, TestFixShBip32UnhardenedBlueWallet |
| no other change | whole suite green (§8); §5 second table |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -p 6 -timeout 20m ./...` (gui sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 6`,
which asserts its partition is exhaustive) ; `CGO_ENABLED=0 go test -p 6 -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ;
`./scripts/test-32bit.sh` ; `GOOS=js GOARCH=wasm go vet ./cmd/emu/` and with `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ;
gofmt on the changed files ; both TinyGo device builds and `scripts/refugium-elf-check.sh` (CI; no tinygo here).
Measured on the scratch fix (go1.26.7): non-gui 62 packages ok; gui `RESULT: ok -- all 1464 tests ran across 6 shards`;
oraclelive rc 0; 32-bit `GOARCH=386 test: exit 0`, `GOARCH=arm build: exit 0`; both wasm vets rc 0;
refugium-tagged `RESULT: ok -- 1323 passed, 68 stepped aside for the profile and 2 for the environment, 9 deferred-minor, of 1402 tests`.
(The seal row was added after that full run; `go test ./seal/` alone was rerun green on the scratch fix.)
Not caused by this change and seen at base: `go vet ./gui/` reports `testing.ArtifactDir requires go1.26` in two golden
tests, and `gofmt -l gui` lists gui/transaction.go and two transaction tests.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. No `mr` or `md-cli` flag or printed text changes.
- Closes `sh-parsepath-unhardened-overflow` with the GREEN merge (its RED row is the un-skipped test). The regrade
  line and one new line are proposed in the author's report (coordinator
  `agent-reports/fix-doc-bh-sh-bip32-unhardened.md`) for direct-push to mnemonic-engrave `design/FOLLOWUPS.md` after
  the merge; this PR adds no FOLLOWUPS line.

## 10. Out of scope

- `ParseInt` also accepts a sign and leading zeros: at base `[deadbeef/-0/-0/-0]`, `[deadbeef/+1/0h]` and
  `[deadbeef/007]` all parse (probe, scratch fix applied: `ACCEPTED -0/-0/-0 -> m/0/0/0`, `+1/0h -> m/1/0h`,
  `007 -> m/7`). BIP-380 L246 lists `[deadbeef/-0/-0/-0]...` as invalid. The value is never changed, so the origin
  carried equals the text's number; filed as a FOLLOWUPS line (report), not fixed here.
- The grade states Core refuses the scanned text; that was not run here (no node is touched). BIP-380 L251 is the
  authority this fix cites.

## 11. How this was checked

Worktree /scratch/cache/wt-fix-sh-bip32 on base a042d824: the §6 tests were added and the skip removed, the RED command
was run (log `/scratch/cache/fix-sh-bip32-red.txt`, 8 FAIL, 1 control PASS); the §4 change was applied and the rows
rerun (9 PASS, `/scratch/cache/fix-sh-bip32-green-rows.txt`) and the §8 jobs run; then the worktree was reset. The
combined diff (production change + tests) is committed beside this document as
`design/FIX_sh_bip32_unhardened-probe.diff` (sha256 ca2e50af27b823a42722c45d6ea5630fd08c2e7211c654386125d946e31e4791;
evidence, the builder still writes the change per §4). BIP texts from `raw.githubusercontent.com/bitcoin/bips/master`
(bip-0380.mediawiki sha256 2abd2564a5a7f1905bc45d44255c39bdbae0cb1e7ad1c8b93a6e6c654eac0d2c, bip-0032.mediawiki
sha256 e5e00a8289db2f681052cf24a745320afc225e66b25d1e489a7c884d2fc7f11f). Callers from
`grep -rn "ParsePathElement(\|ParsePath(" --include='*.go'` excluding `_test.go`.
