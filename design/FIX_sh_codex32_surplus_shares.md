# FIX: codex32 Interpolate refuses more than k shares

**Small track** (one Opus review round). Grade I (Fable upheld; B9/P9: biptool is not a supported recovery path, fixed
anyway). Not secret-listed.
Key: bughunt-1119 `bh-sh-C1`. Table row 33, coordinator `agent-reports/architect-bughunt-1119-questions.md` L145.
Ruling (decided, not re-ruled here): **T34** (same report L78): refuse more than k shares, per BIP-93 L256 "The number
of shares is exactly equal to the (common) threshold value." The other option in the grader's Q1 (check each extra
share against the first k) is not taken.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are at that
base. Protocol text: bitcoin/bips master `bip-0093.mediawiki`, fetched 2026-11-22 (sha256 in the author report).

**One point is open (OPEN-1, §3): the threshold-"0" case.** Every other rule below is decided by T34 or is a
PROPOSED text for the architect to confirm.

## 1. The defect

- `codex32.Interpolate` (codex32/codex32.go:191-279) checks that all shares agree (length, hrp, threshold, id: L202-217),
  then runs Lagrange interpolation over **every** share it was given. It refuses fewer than k (L231-233, after the
  input-index early return at L221-229) but never refuses more than k. With k+1 shares where one is valid but from a
  different split, the result is a different secret with a valid checksum and no error.
- Seen at base (public BIP-93 test vector 3 shares, no real key: A, C, D, and E with zero data, `ms13casheqqqqqqqqqqqqqqqqqqqqqqqqqqa8zxete8lyl02`, "X" below):
  `printf '%s\n' A C D X | biptool interpolate -idx s` prints `ms13cashskuq5jppqjsu6zaps4y06dh2r87hc0xg2kdyzghe`,
  exit 0, empty stderr (control A C D: `ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln`). `biptool derive seed` on
  the same four lines writes seed `b7014904209439a17430a91fa6dd433f` (control `ffeeddccbbaa99887766554433221100`),
  exit 0. The order does not matter (X A C D gives the same).
- Two more forms of the same root, seen at base:
  - Target equal to an input's index skips interpolation (L221-229): A C D X with `-idx e` returns X itself, exit 0;
    with `-idx f` it returns `ms13cashfe9jtwc7atzv2556rnhzrrqdvav7vw0ujqgtm4uu` (the consistent set gives
    `ms13cashf8jh6sdrkpyrsp5ut94pj8ktehhw2hfvyrj48704`, BIP-93 vector 3).
  - Threshold "0" (OPEN-1): two different unshared secrets with the same id and length,
    `ms10testsqqqqqqqqqqqqqqqqqqqqqqqqqqs75svv7jal8p5` and `ms10testsqyqqqqqqqqqqqqqqqqqqqqqqqqmwawx2dktvjhg` (made with
    `codex32.NewSeed("ms", 0, "test", 'S', ...)` from 16 zero bytes and from 0x01 followed by 15 zero bytes), return
    whichever is first, exit 0.
- Reach: the device cannot reach it. `gui/codex32_polish.go:184-205` collects exactly k shares, runs
  `ConsistentShares` on each, then calls `Interpolate`. The only other callers are `cmd/biptool/main.go:154` (derive)
  and `:432` (interpolate).

## 2. Spec delta (rules; §6 maps each to its rows)

- **R1 (count).** `Interpolate` refuses a set of more than k shares, where k is the common threshold digit of the set,
  for every target index: `S`, a fresh index, or an index that one of the inputs has. It returns the zero `String` and
  a new unexported sentinel `errTooManyShares`, text **`too many shares`** (PROPOSED, architect confirms; it follows the
  wording of the existing `insufficient shares` sentinel at codex32/codex32.go:31). No extra share is checked or used.
- **R2 (precedence).** The existing per-share agreement checks run first, in their order (length, hrp, threshold, id).
  The R1 count check runs right after them (after L217) and before the input-index early return (L221) and the
  repeated-index check (L248). So a set that disagrees still gets its existing error, and a surplus set with a repeated
  index gets `too many shares` (was `repeated index`). PROPOSED (architect confirms): k is defined only once all
  thresholds agree, and the count must come before the early return or R1 misses the target-is-input form.
- **R3 (threshold "0").** OPEN-1 (§3). Until it is ruled, the builder does not start.
- **R4 (Describe).** `codex32.Describe` (codex32/polish.go:26-58) maps `errTooManyShares` to **`too many shares`**
  (PROPOSED, architect confirms), next to `need more shares` (L54-55). The device cannot reach it (it collects exactly
  k); the case keeps `Describe` total over `Interpolate`'s sentinels so the defence-in-depth branch at
  `gui/codex32_polish.go:207` would not show the generic `invalid`.
- **R5 (biptool).** No biptool code change. `biptool interpolate` and `biptool derive` (any format) given more than k
  shares print nothing on stdout, stderr exactly `biptool: too many shares` and a newline, and exit **2** (main.go:88-90,
  the existing generic-failure branch; same shape as `biptool: insufficient shares`).
- **R6 (unchanged).** Exactly k shares: output unchanged (BIP-93 vectors 2 and 3, biptool `TestInterpolate`). Fewer than
  k: `insufficient shares` unchanged. A set whose target is one of its inputs and has at most k shares still returns
  that input (L221-229, untouched). `ConsistentShares` (codex32/polish.go:152) is unchanged: it validates a partial set
  as it is collected and does not take k.
- **R7 (doc comment).** The `Interpolate` doc comment (L188-190) gains one sentence: the set must hold at most k shares
  (exactly k to interpolate a new index), and more than k is refused, per BIP-93. Not holder-visible.

## 3. OPEN-1 (STOP: the rulings do not decide it)

A threshold-"0" string is an unshared secret (BIP-93 L73-74: its index must be `s`; L169-170: its threshold digit is
otherwise ignored). T34 says "more than k"; read literally with k = 0, every unshared secret is refused, which breaks
`biptool derive` of a single `ms10...` string (works at base, exit 0, seed `00000000000000000000000000000000` for the
first string above). At base, two different threshold-0 strings with the same id and length return the first one,
exit 0, with no error: the same silent-surplus fault as C1.

Options:
- **(a) k = 1 for threshold "0"** (one string is a complete set). One `ms10...` string works as today; two or more
  are refused with `too many shares` (R1's text, R2's position). The probe implements this.
- (b) Exempt threshold "0" from R1. One string works; two different secrets keep returning the first, exit 0.
- (c) Refuse two or more threshold-0 strings with the existing `repeated index` (they all have index `s`). Needs the
  repeated-index check moved before the early return for this case only.

**Recommend (a):** it closes the same fault in the same place with the same text, keeps the one-string path, and
matches BIP-93's reading that an unshared secret is a single string.

## 4. Changes (by file; for build ordering)

Source files this FIX changes: `codex32/codex32.go` (sentinel in the var block L23-38; count check after L217; doc
comment L188-190), `codex32/polish.go` (one `Describe` case after L55). New test files: `codex32/fix_sh_surplus_test.go`,
`cmd/biptool/fix_sh_surplus_test.go`. No other file. Not touched: `gui/`, `cmd/biptool/main.go`.
The author's probe is `design/FIX_sh_codex32_surplus_shares-probe.diff` (6 added lines; applies with `git apply --check`
on 25abe8e; it implements OPEN-1 option (a)). It is evidence, not the patch: the builder writes the doc comment (R7) too.

## 5. Tests (new; none existing change)

All in package `codex32` (`codex32/fix_sh_surplus_test.go`) unless named otherwise. Shares: A
`ms13casha320zyxwvutsrqpnmlkjhgfedca2a8d0zehn8a0t`, C `ms13cashcacdefghjklmnpqrstuvwxyz023949xq35my48dr`, D
`ms13cashd0wsedstcdcts64cd7wvy4m90lm28w4ffupqs7rm`, E `ms13casheekgpemxzshcrmqhaydlp6yhms3ws7320xyxsar9` (BIP-93 vector
3), X (above), N `ms12namea320zyxwvutsrqpnmlkjhgfedcaxrpp870hkkqrm` (BIP-93 vector 2 share A, lowercased; k = 2),
T0a and T0b (the two threshold-0 strings of §1). Each refusal row asserts: returned `String` is the zero value,
`err != nil`, `err.Error() == "too many shares"` (text, so the row runs at base without the new sentinel).

- `TestFixShCodex32SurplusInconsistent`: A C D X, target `S`, and X A C D, target `S`: refused.
- `TestFixShCodex32SurplusConsistent`: A C D E, target `S`: refused (T34: a consistent extra is refused too).
- `TestFixShCodex32SurplusTargetIsInput`: A C D X, targets `E` and `F`: refused.
- `TestFixShCodex32SurplusRepeatedIndex`: A C D D, target `S`: refused with `too many shares` (R2).
- `TestFixShCodex32SurplusMismatchFirst` (control, R2): A C D N, target `S`: error text `mismatched threshold`.
- `TestFixShCodex32ExactKUnchanged` (control, R6): A C D, target `S`: `ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln`,
  nil; A C, target `S`: `insufficient shares`; A C D, target `A`: returns A, nil.
- `TestFixShCodex32Threshold0` (R3; written once OPEN-1 is ruled; for option (a)): T0a alone, target `S`: T0a, nil;
  T0a T0b and T0b T0a, target `S`: refused.
- `TestFixShCodex32DescribeTooMany` (R4): `Describe` of the error from A C D E, target `S`, is `too many shares`.
- `cmd/biptool/fix_sh_surplus_test.go` `TestFixShBiptoolSurplusShares` (R5), through `execErr` (main_test.go:304):
  stdin A C D X (one per line) for `interpolate -idx s` and for `derive seed`: stdout empty, error text
  `too many shares`. Exit 2 and the `biptool: ` prefix are `main`'s existing generic branch (seen in §7's CLI rows).

Tests that change: none. With the probe applied, `go test ./codex32/ ./cmd/biptool/` and
`go test -run 'Codex32|codex32|Recover|Share' ./gui/` pass unchanged (seen). The triage test
`TestTriageBhShC1SurplusShare` (not in the repo) asserted that a consistent extra keeps the result; T34 overrides that
assertion, and the probe fails it as expected (`consistent extra changed result:  too many shares`).

## 6. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 count, every target | SurplusInconsistent, SurplusConsistent, SurplusTargetIsInput, BiptoolSurplusShares |
| R2 precedence | SurplusRepeatedIndex, SurplusMismatchFirst |
| R3 threshold 0 | Threshold0 (after OPEN-1) |
| R4 Describe | DescribeTooMany |
| R5 biptool text and exit | BiptoolSurplusShares; CLI rows §7 |
| R6 unchanged | ExactKUnchanged; BIP vectors 2, 3; biptool TestInterpolate; gui subset |
| R7 doc comment | none (not behaviour) |

## 7. RED rows (run by the author on base 25abe8e; "after" seen on the probe)

Library rows, as scratch tests in `codex32/` (removed after the run), `go test -p 2 -count=1 -v ./codex32/`:

| input, target | at base (seen) | after (seen on the probe) |
|---|---|---|
| A C D X, S (triage test) | FAIL `inconsistent surplus share accepted silently, result ms13cashskuq5jppqjsu6zaps4y06dh2r87hc0xg2kdyzghe (control ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln)` | `too many shares` |
| A C D X, E | `ms13casheqqqqqqqqqqqqqqqqqqqqqqqqqqa8zxete8lyl02`, nil | `too many shares` |
| A C D X, F | `ms13cashfe9jtwc7atzv2556rnhzrrqdvav7vw0ujqgtm4uu`, nil | `too many shares` |
| A C D D, S | `repeated index` | `too many shares` |
| T0a T0b, S / T0b T0a, S | T0a, nil / T0b, nil | `too many shares` (option (a)) |

CLI rows, `printf '%s\n' <shares> | biptool <verb>`, binaries built from base and from the probe:

| shares | verb | at base (seen) | after (seen) |
|---|---|---|---|
| A C D | interpolate -idx s; derive seed | `ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln`; `ffeeddccbbaa99887766554433221100`; exit 0 | same |
| A C D E | both | as A C D, exit 0 | `biptool: too many shares`, exit 2 |
| A C D X and X A C D | both | `ms13cashskuq5jppqjsu6zaps4y06dh2r87hc0xg2kdyzghe`; `b7014904209439a17430a91fa6dd433f`; exit 0 | `biptool: too many shares`, exit 2 |
| A C D X | interpolate -idx e / -idx f | X itself / `ms13cashfe9j...`, exit 0 | `biptool: too many shares`, exit 2 |
| A C D N | interpolate -idx s | `biptool: mismatched threshold`, exit 2 | same |
| A C | interpolate -idx s | `biptool: insufficient shares`, exit 2 | same |
| T0a | both | T0a; `00000000000000000000000000000000`; exit 0 | same |
| T0a T0b | both | T0a; `00000000000000000000000000000000`; exit 0 | `biptool: too many shares`, exit 2 (option (a)) |

(`derive seed` writes raw bytes; the author piped it through `xxd -p`.)

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`) ;
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ; `./scripts/test-32bit.sh` ;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI). `codex32` is linked into the firmware, so the TinyGo builds
matter here. Run by the author on the probe (go1.26.4): gofmt clean, `go vet ./codex32/ ./cmd/biptool/` ok, the three
`go test` lines of §5. Not run by the author: the rest of the gate. The builder runs the whole gate.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (no `mr`/`md-cli` text; the manual does not cover biptool or the seedhammer codex32 package).
- README/docs: no printed command passes more than k shares (grep of README.md and docs/ for biptool: no hits).
- Closes: bh-sh-C1 with the GREEN merge. seedhammer has no `design/FOLLOWUPS.md`; nothing to close there.
- Files: none.

## 10. Out of scope

M1 of the bug-hunt report (HRP/id case compared case-sensitively across shares) is a separate Minor; R2 keeps its
current behaviour. The device recovery flow is unchanged (it cannot reach R1).
