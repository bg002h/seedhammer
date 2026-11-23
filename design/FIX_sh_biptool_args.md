# FIX: biptool refuses a misplaced or leftover argument in every verb, and a seed of the wrong length for the word count

**Small track** (one Opus review round). Grade I. Secret handling (bh-sh-I4: a backup mnemonic not derived from the
supplied seed alone; `SECRET id=bh1119-sh-i4-biptool-short-seed-padded`; R5 also closes `SECRET id=b34-sh-s1-biptool-verb-leftover-secret`).
Keys: bughunt-1119 `bh-sh-I3` and `bh-sh-I4`. Table row 34, coordinator `agent-reports/architect-bughunt-1119-questions.md`
L146. No T-ruling names this row. Product ruling B9 (biptool is not a supported recovery path) keeps the grade at I and
does not change the fix. Grader fix line (grade-sh.md L67): "Reject arguments left after the format, and check
`len(seed) >= entLen`. It also closes the deferred `sh-biptool-short-seed`."
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e (FIX-sh-go-cli-output-sink
merged). All anchors are `cmd/biptool/main.go` at that base.

Scope points ruled (architect-bh-batch34 #34): S1 in (rule R5, grade I, part of bh-sh-I3), S2 in (R3 widened, grade M). R1 and R2 texts confirmed.

## 1. The defect

- **I3.** `derive` (L116-120) parses its options with `deriveFlags`, and Go's `flag` package stops at the first
  non-flag argument. Everything after the format word is then ignored by the formats with no options of their own
  (xpub L237, xprv L248, pubkey L258, privkey L269), and bip39 (L202) and seed (L221) ignore whatever is left after
  their own `-words` / `-n`. Seen on base (§6): with the codex32 share of bytes `00..0f`,
  `derive -path m/83696968h/32h/0h xpub` prints the child `...cLhwaALp6H`, but `derive xpub -path m/83696968h/32h/0h`
  exits 0 with the **master** `...W1EGMcet8`; `derive bip39 extra -words 12` exits 0 with 24 words;
  `derive -path m seed extra -n 8` exits 0 with 16 bytes.
- **I4.** `derive bip39` (L215-216) slices `seed[:entLen]` without checking `len(seed)`. When the codex32 seed is
  shorter than the word count's entropy, the slice reaches into spare capacity of the decoded buffer: a 31-zero-byte
  seed with `-words 24` exits 0 with 23 x `abandon` + `art`, the same mnemonic as 32 zero bytes (also 19/15, 23/18,
  27/21 words). With no spare capacity it panics (16-byte seed and `-words 24`; an xprv input with `-path m`, which
  has no seed at all): the October sh2 #28 case, deferred as `sh-biptool-short-seed`.

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (leftover argument).** After the format word, and after bip39's `-words` / seed's `-n` options, any argument
  left is refused before any output: `derive` returns
  `derive: unexpected argument "<first leftover>" after format "<format>"` (Go `%q` for both), so `main` prints
  `biptool: derive: unexpected argument "-path" after format "xpub"` and exits **2** (the existing failure path,
  L88-90). Only the first leftover is named. **Confirmed (architect-bh-batch34)** text.
  Order: for xpub/xprv/pubkey/privkey the check is the first thing in the case (before the `isXprvDeriv` path
  check); for bip39 and seed it runs right after that format's own `Parse`, before every other check. An unknown
  format keeps `derive: unknown format "<f>"` (the default case is not reached by R1). The stdin read and path
  checks before the format dispatch (L131-194) keep their place, so a bad path or bad stdin is still reported first.
- **R2 (no seed).** `derive bip39` with no seed (an extended key on stdin with no BIP-85 path, so `seed` is nil)
  returns the existing seed-format text `derive: no seed provided` (L232): `biptool: derive: no seed provided`,
  exit 2. Was: a Go panic (exit 2, stack trace).
- **R3 (seed length).** `derive bip39` with a non-empty seed
  returns `derive: seed is <len> bytes; -words <n> needs exactly <entLen> bytes` when `len(seed) < entLen`, or when
  there is no BIP-85 path and `len(seed) != entLen`; e.g. `biptool: derive: seed is 31 bytes; -words 24 needs exactly 32
  bytes` and `biptool: derive: seed is 32 bytes; -words 12 needs exactly 16 bytes`, exit 2.
  R2 and R3 run after the existing `-words` range check (L206-208) and bip39 path check (L209-214), immediately before
  `bip39.New`.
- **R4 (unchanged).** Every documented spelling (options before the format; bip39/seed options after it) keeps its
  output byte for byte; every other failure keeps its text and exit 2; a seed at least `entLen` long keeps the existing
  behaviour (the BIP-85 path always yields 64 bytes; on the BIP-85 path the first entLen of its 64 bytes are taken, per BIP-85).
- **R5 (other verbs; S1).** `rand`, `seed` and `interpolate` refuse any argument left after their own
  Parse, before reading stdin or writing output: `<verb>: unexpected argument "<arg>"` (Go `%q`), e.g. `biptool: rand:
  unexpected argument "extra"`, exit 2, first leftover named.

## 3. Holder-visible changes (complete list)

| command | condition | before | after |
|---|---|---|---|
| `biptool derive {xpub,xprv,pubkey,privkey} <anything>` | any argument after the format | exit 0, output for the options given before the format (e.g. master xpub) | exit 2, `biptool: derive: unexpected argument "<arg>" after format "<f>"`, no stdout |
| `biptool derive bip39 [-words N] <anything>` | argument left after bip39's options | exit 0, mnemonic for the default/earlier `-words` | same refusal, format `"bip39"` |
| `biptool derive seed [-n N] <anything>` | argument left after seed's options | exit 0, seed bytes for the default/earlier `-n` | same refusal, format `"seed"` |
| `biptool derive bip39 -words N` | codex32 seed shorter than the entropy N words need | exit 0 with a padded mnemonic, or a panic | exit 2, `biptool: derive: seed is <len> bytes; -words <N> needs exactly <entLen> bytes` |
| `biptool derive bip39` | extended key on stdin, no BIP-85 path | panic | exit 2, `biptool: derive: no seed provided` |
| `biptool derive bip39 -words N` (no BIP-85 path) | raw codex32 seed longer than the entropy N words take | exit 0, mnemonic of the first entLen bytes | exit 2, same R3 text |
| `biptool {rand,seed,interpolate} ... <anything>` | argument left after the verb's options | exit 0; `interpolate extra -idx D` prints the S secret, `seed ... extra -threshold 2 -idx A` an unshared `ms10` string, `rand extra -n 64` 32 bytes | exit 2, `biptool: <verb>: unexpected argument "<arg>"`, no stdout |

No flag, verb or type is added. No README, manual or doc text mentions biptool's argument order or exit codes
(`git grep -n biptool -- '*.md' ':!design/agent-reports/*' ':!design/FIX_*'`: nothing; toolkit manual
`40-cli-reference/`: nothing).

## 4. Code changes (builder applies)

All in `cmd/biptool/main.go`; the scratch version is `design/FIX_sh_biptool_args-probe.diff` (evidence, not a patch to
copy blindly). The probe predates the architect-bh-batch34 fold: its R3 text and ShortSeed expectations are superseded by R3 and §5, and it has no R5, no S2 check and no OtherVerbsLeftover/LongSeed rows.

1. A helper (probe name `noArgsLeft(format string, rest []string) error`) returning nil for an empty `rest`, else
   R1's error naming `rest[0]`. Comment: the flag package stops at the first non-flag, so a misplaced option would be
   dropped silently; names this FIX and bh-sh-I3.
2. Cases xpub, xprv, pubkey, privkey: first statement `if err := <helper>(f, args); err != nil { return err }`.
3. Case bip39: after `bip39Flags.Parse(args)` (L202-204), the helper with `bip39Flags.Args()`. After `entLen`
   (L215): `if len(seed) == 0` -> R2's error; `if len(seed) < entLen || (len(path) == 0 && len(seed) != entLen)` -> R3's error; then `bip39.New(seed[:entLen])`.
4. Case seed: after `deriveSeedFlags.Parse(args)` (L221-223), the helper with `deriveSeedFlags.Args()`.
5. In `run`, after randFlags/seedFlags/interpolateFlags `.Parse(args)`: `if rest := <fs>.Args(); len(rest) > 0 { return fmt.Errorf("<verb>: unexpected argument %q", rest[0]) }` (R5).

## 5. Tests

New file `cmd/biptool/fix_sh_biptool_args_test.go` (in the probe diff; test function names are the TRACE row names).
Every row runs the real `main` in a child process through the existing `TestMain` / `fixShRunChild`
(`fix_sh_output_sink_test.go`), because biptool's flag sets are process-global and an in-process run would carry
`-path` / `-words` / `-n` from one row into the next. Inputs are test entropy only (no real key; biptool is mainnet-only,
so a signet input is not available): the codex32 share of bytes `00..0f`
(`ms10testsqqqsyqcyq5rqwzqfpg9scrgwpupj0pvk8uetl6l`), shares of N zero bytes made in-process with
`seed -id test -seedlen N -threshold 1 -idx S -hrp ms`, and the BIP-85 test xprv already in `TestDerive`.
Each refusal row asserts exit 2, empty stdout and stderr exactly the R1/R2/R3 line plus `\n`.

- `TestFixShBiptoolArgsMisplacedPath` (R1): `derive <f> -path m/83696968h/32h/0h` for f in xpub, xprv, pubkey,
  privkey -> `biptool: derive: unexpected argument "-path" after format "<f>"`.
- `TestFixShBiptoolArgsLeftoverArgument` (R1): `derive xpub extra` (format "xpub"); on the 32-zero-byte share
  `derive -path m bip39 extra -words 12` and `derive -path m bip39 -words 12 extra` (format "bip39");
  `derive -path m seed extra -n 8` and `derive -path m seed -n 8 extra` (format "seed"); each names `"extra"`.
- `TestFixShBiptoolArgsShortSeed` (R2, R3): 16-byte share `-path m bip39 -words 24` -> `seed is 16 bytes; -words 24 needs exactly 32 bytes`;
  zero shares 31/24 -> `needs exactly 32 bytes`, 19/15 -> `needs exactly 20 bytes`, 23/18 -> `needs exactly 24 bytes`, 27/21 -> `needs exactly 28 bytes`;
  xprv `-path m bip39 -words 12` -> `biptool: derive: no seed provided`.
- `TestFixShBiptoolArgsOtherVerbsLeftover` (R5): `rand extra -n 64`, `rand -n 64 extra`, `seed -id test -seedlen 16
  extra -threshold 2 -idx A` (stdin 16 zero bytes), `interpolate extra -idx D` and `interpolate -idx D extra` (stdin BIP-93
  vector 2 shares A, C); each refused with its verb's R5 line naming "extra".
- `TestFixShBiptoolArgsLongSeed` (R3, S2): 32-zero-byte share `derive -path m bip39 -words 12` -> `biptool: derive: seed is 32 bytes; -words 12 needs exactly
  16 bytes`; 20-zero-byte share `derive -path m bip39` -> `biptool: derive: seed is 20 bytes; -words 24 needs exactly 32
  bytes`.
- `TestFixShBiptoolArgsControls` (R4, pass at base): `derive -path m/83696968h/32h/0h xpub` on the 00..0f share ->
  `xpub661MyMwAqRbcGYtxqsn5fkBerXLHYPhqTY36WeRoyARM8EQogL2Gw4a18TRg7gkfnQSYDDKzZRNQWMND4Wdo3ojq7Coiu96S4cLhwaALp6H\n`;
  `derive -path m bip39 -words 12` on it -> `abandon amount liar amount expire adjust cage candy arch gather drum buyer\n`;
  `derive -path m seed -n 8` -> bytes `00..07`; 32-zero-byte share `derive -path m bip39 -words 24` -> 23 x `abandon ` +
  `art\n`. Exit 0, empty stderr.

**Tests that change:** `cmd/biptool/bughunt_sh2_s5_test.go` `TestBughuntSh228Bip39ShortSeed`: delete the line
`t.Skip("deferred-minor: sh-biptool-short-seed")` (L23); nothing else. Its two cases become errors (R2, R3) instead
of panics. The existing `TestDerive`, `TestDeriveSeed` and the output-sink tests pass unchanged.

## 6. RED rows (run on base 25abe8e with the §5 tests added and the skip removed)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixShBiptoolArgs|TestBughuntSh228' ./cmd/biptool/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestFixShBiptoolArgsMisplacedPath | FAIL x4: e.g. `biptool derive xpub -path m/83696968h/32h/0h: exit 0 stdout "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8\n" stderr ""` (the master) | PASS |
| TestFixShBiptoolArgsLeftoverArgument | FAIL x5: `derive xpub extra` exit 0 master; `bip39 extra -words 12` exit 0 with 24 words; `bip39 -words 12 extra` exit 0; `seed extra -n 8` exit 0 with 16 bytes; `seed -n 8 extra` exit 0 | PASS |
| TestFixShBiptoolArgsShortSeed | FAIL x6: 16-byte/24 and xprv rows exit 2 with `panic: runtime error: slice bounds out of range [:32] with capacity 17` / `[:16] with capacity 0`; 31/24, 19/15, 23/18, 27/21 exit 0 with a padded mnemonic | PASS |
| TestBughuntSh228Bip39ShortSeed (skip removed) | FAIL: both cases `panicked: runtime error: slice bounds out of range` | PASS |
| TestFixShBiptoolArgsControls | PASS (control) | PASS |
| TestFixShBiptoolArgsOtherVerbsLeftover | FAIL x5: rand rows exit 0 with 32/64 random bytes; seed row exit 0 `ms10testsqqqqqqqqqqqqqqqqqqqqqqqqqqs75svv7jal8p5`; `interpolate extra -idx D` exit 0 `MS12NAMES6XQGUZTTXKEQNJSJZV4JV3NZ5K3KWGSPHUH6EVW`; `interpolate -idx D extra` exit 0 the D share | PASS |
| TestFixShBiptoolArgsLongSeed | FAIL x2: `-words 12` exit 0 `abandon` x11 `about`; 20-byte row `panic: runtime error: slice bounds out of range [:32] with capacity 20` | PASS |

Mutations seen on the scratch fix: removing the `len(seed) < entLen` check fails TestFixShBiptoolArgsShortSeed;
removing the xpub case's argument check fails TestFixShBiptoolArgsMisplacedPath and
TestFixShBiptoolArgsLeftoverArgument.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 leftover argument refused, first named, exit 2 | TestFixShBiptoolArgsMisplacedPath (4 flagless formats); TestFixShBiptoolArgsLeftoverArgument (xpub, bip39 x2, seed x2) |
| R2 no seed | TestFixShBiptoolArgsShortSeed (xprv row); TestBughuntSh228Bip39ShortSeed |
| R3 seed length | TestFixShBiptoolArgsShortSeed (5 rows); TestFixShBiptoolArgsLongSeed (2 rows); TestBughuntSh228Bip39ShortSeed |
| R4 unchanged | TestFixShBiptoolArgsControls; existing TestDerive, TestDeriveSeed, output-sink tests |
| R5 other verbs | TestFixShBiptoolArgsOtherVerbsLeftover (5 rows) |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`) ;
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ; `./scripts/test-32bit.sh` ;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI). Measured on the scratch fix by the author (go1.26.7,
/scratch/code/shibboleth/.toolchain/go): `go test -v ./cmd/biptool/` ok (18 top-level tests PASS);
`GOARCH=386 go test ./cmd/biptool/` ok; `go vet ./cmd/biptool/` ok; gofmt clean. Not run by the author: the rest of
`./...` and the other jobs; no other package imports `cmd/biptool`. The builder runs the whole gate.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (no `mr`/`md-cli` text; the manual never mentions biptool).
- Closes: bh-sh-I3 and bh-sh-I4 with the GREEN merge; the secret-list row `bh1119-sh-i4-biptool-short-seed-padded`
  closes with it; the secret-list row `b34-sh-s1-biptool-verb-leftover-secret` (R5) closes with it; and the deferred mnemonic-engrave `design/FOLLOWUPS.md` entry `sh-biptool-short-seed` (its RED row
  `TestBughuntSh228Bip39ShortSeed` is un-skipped here). seedhammer has no `design/FOLLOWUPS.md`.
- Files: none (S1 and S2 ruled into this row, architect-bh-batch34).

## 10. Scope points (ruled S1 (b), S2 (b): architect-bh-batch34)

- **S1 (same root in the other verbs).** `rand`, `seed` and `interpolate` parse with their own flag sets and also drop
  everything after the first non-flag. Seen on base: `rand extra -n 64` writes 32 bytes, exit 0;
  `interpolate extra -idx D` (BIP-93 vector 2 shares A and C) prints the S share
  `MS12NAMES6XQGUZTTXKEQNJSJZV4JV3NZ5K3KWGSPHUH6EVW` instead of D, exit 0. (The architect ran the seed row; §3 and §6 carry its output.) Options:
  (a) out of this row; file a FOLLOWUPS line (text in the report); (b) extend R1 here: after each verb's `Parse`,
  a leftover argument returns `<verb>: unexpected argument "<arg>"` (e.g. `biptool: rand: unexpected argument "extra"`),
  exit 2, one RED row `TestFixShBiptoolArgsOtherVerbsLeftover` with those three verbs. **Recommend (b):** same root,
  same two-line check, a wrong-share/short-entropy result with exit 0 is the I3 class; no new flag. **Ruled (b).**
- **S2 (raw seed longer than the word count).** With `-path m` (no BIP-85), `derive bip39 -words 12` on a 32-byte
  codex32 seed uses the first 16 bytes and exits 0 (seen: 32 zero bytes -> `abandon x11 about`), so the mnemonic does
  not carry the whole seed. On the BIP-85 path the truncation is the BIP-85 rule (entropy = first `entLen` bytes) and
  is correct. Not in the bug report. Options: (a) keep (raw conversion takes a prefix); (b) refuse when there is no
  BIP-85 path and `len(seed) != entLen`, text as R3. **Recommend (b)** as a separate Minor or folded here if the architect prefers: a holder converting a
  codex32 backup to words would otherwise get a different secret with exit 0. **Ruled (b).**

## 11. Out of scope

biptool's flag-set globals (test isolation handled by the child-process rows); the
`flag.ExitOnError` usage path for an undefined option (already exit 2 with usage, e.g.
`derive bip39 -words 12 -path ...` -> `flag provided but not defined: -path`).

## 12. How this was checked

RED and GREEN rows run by the author as in §6; the base binary rows in §1 and §10 run with a build of 25abe8e
(`go build ./cmd/biptool`). Probe diff `design/FIX_sh_biptool_args-probe.diff` sha256
`89ce4f026cf4c37144718a223c141a8cddd1621f987b70b39699b53af92d9684` applies with `git apply --check` on 25abe8e; the
scratch fix was reverted. Error and panic texts are captured from the runs, not written by hand.
