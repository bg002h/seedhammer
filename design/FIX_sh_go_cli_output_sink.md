# FIX: biptool and picosign report a failed stdout write instead of exiting 0

**Small track** (one Opus review round). Grade I. Secret handling (bh-sh-I5: biptool backup output).
Keys: bughunt-1119 `bh-sh-I5` (secret-listed, `SECRET id=bh1119-sh-i5-biptool-unchecked-backup-write`) and `bh-sh-I6`.
Table row 16, coordinator `agent-reports/architect-bughunt-1119-questions.md` L128.
Ruling (decided, not re-ruled here): **T6** (same report L42), the failed-write contract every tool takes from md's
published text (toolkit manual `docs/manual/src/40-cli-reference/42-md.md` "When stdout fails", origin/master L797-799):
a closed pipe only stops the output and the exit code is the verb's own; any other stdout write error prints
`<tool>: cannot write the output: <error>` and exits 1; a verb already failing keeps its code. Grade by T2/P6 (I).
B9 (biptool is not a supported recovery path) does not change the grade or the fix.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 2c5b53b. All anchors are at that
base (the lines in the bug-hunt report, made at 9aec7a8, are unchanged in these two files).

## 1. The defect

- biptool (cmd/biptool/main.go) drops the write result at all eight payload writers: L192 (derive bip39), L207 (derive
  seed), L216 (derive xpub), L224 (derive xprv), L233 (derive pubkey), L242 (derive privkey), L318 (seed), L394
  (interpolate). With stdout on `/dev/full` each exits 0 with empty stderr: a pipeline can mark an undelivered backup
  complete. `rand` (L291-299) does return the `io.Copy` error, but as a generic failure: exit 2,
  `biptool: write /dev/stdout: no space left on device`.
- picosign (cmd/picosign/main.go) drops it at L90 (`extract`, `fmt.Printf`) and L115 (`hash`, `os.Stdout.Write`): both
  exit 0 on `/dev/full`.
- Both tools, as Go programs that do not handle SIGPIPE, are killed by the signal when the reader has closed the pipe
  (the shell sees 141), so the exit code is not the verb's own as T6 requires (seen: §6 ClosedPipe rows).

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (both tools).** Every write of a verb's output to stdout is checked. On error the verb stops at once (nothing
  after the failed write runs; for `seed` the unengraveable warning on stderr is not printed) and the error reaches
  `main` marked as an output failure.
- **R2 (full disk and any other non-EPIPE write error).** `main` prints exactly one stderr line and exits **1**:
  `biptool: cannot write the output: <error>` / `picosign: cannot write the output: <error>`, where `<error>` is the
  Go write error as returned (for `/dev/full`: `write /dev/stdout: no space left on device`). The prefix text is T6's
  template; no new wording.
- **R3 (closed pipe).** SIGPIPE is ignored for the process, so a write to a closed pipe returns EPIPE. An output
  failure whose error is EPIPE prints nothing and exits **0** (every payload write is the verb's last act, so the
  verb's own code is success). This is T6's first clause.
- **R4 (`rand`).** `rand -n N` reads N bytes from the system random source first (a read failure stays a verb failure:
  `biptool: <error>`, exit 2), then writes them in one write that R1-R3 govern. A full disk becomes exit 1 with R2's
  text (was exit 2, `biptool: write ...`).
- **R5 (unchanged).** Every other failure keeps its text and exit 2 (`biptool: <error>` / `picosign: <error>`);
  successful output bytes are unchanged. "A verb already failing keeps its code" has no instance here: no verb fails
  after it has written output.
- **R6 (scope).** stderr writes (usage text from the `flag` sets, `seed`'s unengraveable warning, the error line
  itself) are not checked: they carry no artifact and the exit code carries the result (T6's stderr clause).
  `picosign sign` writes the image file in place, not stdout or an `--out`: its existing error path (exit 2) is
  unchanged.

## 3. Holder-visible changes (complete list)

| command | condition | before | after |
|---|---|---|---|
| biptool derive {seed,bip39,xpub,xprv,pubkey,privkey}, seed, interpolate | stdout write fails (not EPIPE) | exit 0, no stderr | exit 1, `biptool: cannot write the output: <error>` |
| biptool rand | stdout write fails (not EPIPE) | exit 2, `biptool: <error>` | exit 1, `biptool: cannot write the output: <error>` |
| biptool, every payload verb incl. rand | reader closed the pipe | killed by SIGPIPE (shell 141) | exit 0, no stderr |
| picosign hash, extract | stdout write fails (not EPIPE) | exit 0, no stderr | exit 1, `picosign: cannot write the output: <error>` |
| picosign hash, extract | reader closed the pipe | killed by SIGPIPE | exit 0, no stderr |

No flag, verb or type a holder sees is added. Neither tool has a manual page or README text that states exit codes
(`git grep -i 'biptool\|picosign' -- '*.md'` outside `design/agent-reports/` finds nothing; toolkit manual: none).

## 4. Code changes (builder applies)

Both `main.go` files get the same shape (the scratch version is `design/FIX_sh_go_cli_output_sink-probe.diff`; it is
evidence, not a patch to copy blindly):

1. A package sentinel `errOutput` (`errors.New("cannot write the output")`) and a helper that returns nil for nil and
   otherwise wraps the write error as `fmt.Errorf("%w: %w", errOutput, err)`, so `Error()` is R2's text after the
   `<tool>: ` prefix and `errors.Is` sees both the sentinel and `syscall.EPIPE`. Comment: names this FIX and T6.
2. `main` calls `signal.Ignore(syscall.SIGPIPE)` first (comment: a closed pipe returns EPIPE instead of killing the
   process, so the exit code stays the verb's own), then classifies the verb's error: nil -> 0; output failure with
   EPIPE -> 0, silent; output failure -> R2 line, `os.Exit(1)`; anything else -> the existing line, `os.Exit(2)`.
   picosign: the existing `if err != nil` block at L63-66 becomes this classification; the two early `os.Exit(2)`
   paths (L37-40 no command, L59-61 unknown command) are unchanged.
3. biptool: each of L192, L207, L216, L224, L233, L242, L318, L394 becomes `if _, err := <the same call>; err != nil
   { return <helper>(err) }`. L318 returns before `warnUnengraveable` (R1).
4. biptool `genRand` L296-298: replace the `io.LimitReader` + `io.Copy` with `io.ReadFull` of `rand.Reader` into an
   N-byte buffer (error returned as is), then one `stdout.Write` whose error goes through the helper (R4).
5. picosign: L90 becomes `_, err = fmt.Printf(...)`, L115 `_, err = os.Stdout.Write(hash[:])`, each followed by
   `return <helper>(err)` in place of `return nil`.

## 5. Tests

New (test function names are the TRACE row names), in the probe diff:
- cmd/biptool/fix_sh_output_sink_test.go: a `TestMain` that runs `main()` when `BIPTOOL_CHILD=1` (re-exec of the test
  binary as biptool). The verb set is all nine payload verbs (`rand -n 16`, `seed -id test -seedlen 16`,
  `interpolate`, `derive seed`, `derive -path m bip39 -words 12`, `derive xpub`, `derive xprv`, `derive pubkey`,
  `derive privkey`) on a codex32 share of all-zero test entropy (no real key; biptool is mainnet-only, so a signet
  input is not available).
  - `TestFixShBiptoolWriteFailureReturnsError`: in-process `run` with a writer that rejects every write (ENOSPC) and
    one that accepts one byte then fails (the consultant's prefix writer); each verb, working-writer control first;
    want error text exactly `cannot write the output: no space left on device`.
  - `TestFixShBiptoolFullDiskExit`: subprocess, stdout `/dev/full`: exit 1, stderr exactly
    `biptool: cannot write the output: write /dev/stdout: no space left on device\n`.
  - `TestFixShBiptoolClosedPipeExit`: subprocess, stdout a pipe whose read end is closed first: exit 0 (not killed
    by a signal), empty stderr.
  - `TestFixShBiptoolOtherErrorUnchanged` (control): `derive bogus` exit 2, stderr `biptool: derive: unknown format "bogus"\n`.
- cmd/picosign/fix_sh_output_sink_test.go: `TestMain` with `PICOSIGN_CHILD=1`; fixture: picobin/testdata/signed.bin.gz
  wrapped in 256-byte UF2 payloads (triage-sh-t2's builder, the consultant's recipe).
  - `TestFixShPicosignOutputControl` (control): `hash` digest hex
    `15cf016da39866e8d1c0dff1aaa29fd0429876f9b55a290c1fc6fce819783557` (the consultant's value), `extract` prints
    `28510e83c2ab21039c023c4c10967405d6efd7ec15bcd8ed92b92bd029a9da4721404e4bd1424bb2fbc1faf102dd63d1797f54be4c872c53c1a63a6ad4305281\n`
    (129 bytes, as the consultant measured); exit 0, empty stderr.
  - `TestFixShPicosignFullDiskExit`: hash and extract to `/dev/full`: exit 1, stderr exactly
    `picosign: cannot write the output: write /dev/stdout: no space left on device\n`.
  - `TestFixShPicosignClosedPipeExit`: hash and extract into a closed pipe: exit 0, empty stderr.
  - `TestFixShPicosignOtherErrorUnchanged` (control): `hash <missing file>` exit 2, stderr `picosign: open <path>: no such file or directory\n`.

Tests that change: none. picosign had no tests; biptool's existing tests call `run` with a `bytes.Buffer` and pass
unchanged (§8).

## 6. RED rows (run on base 2c5b53b with the §5 tests added)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixSh' ./cmd/biptool/ ./cmd/picosign/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestFixShBiptoolWriteFailureReturnsError | FAIL, 18 lines: `run with a full stdout returned <nil>; want "cannot write the output: no space left on device"` and the same for `prefix`, 8 verbs each; rand: `returned no space left on device; want ...` | PASS (9 subtests) |
| TestFixShBiptoolFullDiskExit | FAIL: 8 verbs `exit 0 stderr ""`; rand `exit 2 stderr "biptool: write /dev/stdout: no space left on device\n"`; want exit 1 and R2's line | PASS (9) |
| TestFixShBiptoolClosedPipeExit | FAIL: 9 verbs `signal: broken pipe (killed by a signal, not an exit code)` | PASS (9) |
| TestFixShPicosignFullDiskExit | FAIL: `picosign hash > /dev/full: exit 0 stderr ""` and the same for extract | PASS (2) |
| TestFixShPicosignClosedPipeExit | FAIL: hash and extract `signal: broken pipe` | PASS (2) |
| TestFixShBiptoolOtherErrorUnchanged, TestFixShPicosignOutputControl, TestFixShPicosignOtherErrorUnchanged | PASS (controls) | PASS |

Mutation seen on the scratch fix: dropping biptool's `signal.Ignore(syscall.SIGPIPE)` fails
TestFixShBiptoolClosedPipeExit.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 every payload write checked, verb stops | TestFixShBiptoolWriteFailureReturnsError (all 9 verbs, both writers; covers L224/L233/L242, which the triage did not); TestFixShPicosignFullDiskExit |
| R2 exit 1 and exact line | TestFixShBiptoolFullDiskExit; TestFixShPicosignFullDiskExit |
| R3 closed pipe exit 0, silent | TestFixShBiptoolClosedPipeExit; TestFixShPicosignClosedPipeExit |
| R4 rand | rand subtests of the three biptool rows above |
| R5 other failures and output unchanged | TestFixShBiptoolOtherErrorUnchanged; TestFixShPicosignOtherErrorUnchanged; TestFixShPicosignOutputControl; the existing biptool suite |
| R6 scope | none (no behaviour change) |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`) ;
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ; `./scripts/test-32bit.sh` ;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI). Measured on the scratch fix by the author (go1.27.3, the only
Go on this host; CI pins 1.26): `go test ./cmd/...` all ok; `GOARCH=386 go test ./cmd/biptool/ ./cmd/picosign/` ok;
`GOARCH=arm go build` of both ok; `go vet` of both ok; gofmt clean. Not run by the author: the rest of `./...` and the
other jobs; no other package imports these two `main` packages. The builder runs the whole gate.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (no `mr`/`md-cli` text; the manual never mentions biptool or picosign).
- Closes: bh-sh-I5 and bh-sh-I6 with the GREEN merge; the secret-list row
  `bh1119-sh-i5-biptool-unchecked-backup-write` closes with it. seedhammer has no `design/FOLLOWUPS.md`; nothing tracked
  there to close.
- Files (proposed, verbatim, for direct-push after the merge; target file is the coordinator's choice):

```
- [sh-go-devtools-output-sink-audit] status=open grade=N kind=tooling owner=none filed=2026-11-22 src=seedhammer:design/FIX_sh_go_cli_output_sink.md repo=sh :: T6 failed-write contract applied to biptool and picosign only; cmd/glyphtrace (main.go:84), cmd/plateview (main.go:50), cmd/policyprobe (main.go:119 bufio stdout) still drop stdout write errors (dev tools, public output).
```

## 10. Out of scope

stderr write errors (R6); `picosign sign`'s in-place file write (R6); the other Go dev tools (filed above); the
firmware (`cmd/controller`) has no stdout.

## 11. How this was checked

RED and GREEN rows run by the author as in §6 (probe diff sha256 recorded in the report); the probe diff applies with
`git apply --check` on 2c5b53b and was reverted after the run. Errors text `write /dev/stdout: no space left on device`
is Go's real message, captured from the run, not written by hand.
