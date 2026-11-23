# FIX: oracle-live.sh finds and runs every oraclelive test, per package

**Small track** (one Opus review round). Grade I. Test infrastructure only: no firmware, no device text, no funds or
secret path.
Key: bughunt-1119 `bh-sh-I11` (consultant report `design/agent-reports/bug-hunt-2026-11-19.md` I11, L102-112).
Table row 57, coordinator `agent-reports/architect-bughunt-1119-questions.md` L169 ("none" blocked-by, no T-ruling).
Grader: `agent-reports/bughunt-1119/grade-sh.md` L40 (CONFIRMED, static decider) and proposal 9 (L72), triage
`triage-sh-t3.md` L12.
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors at that
base (scripts/oracle-live.sh is unchanged since 9aec7a8, where the bug hunt cited it).

## 1. The defect

- `scripts/oracle-live.sh` exists to run every `//go:build oraclelive` test and to fail if any of them does not run
  (its "VACUITY CHECK", L148-168). But it searches only a hand list of three packages at L129
  (`grep -rlE ... ./oracle/ ./gui/ ./sysw/`), and it gives `go test` the same hand list at L144.
  `cmd/emu/sysw_composer_payload_live_test.go` (`//go:build oraclelive`, `TestSyswComposerPayloadDigestAgreesWithMe`
  at L34) is in neither. The script prints `discovered 8 tagged test(s) from source`, but the tree holds 9 tagged tests
  in 4 packages. It never runs the cross-air-gap payload digest check and still prints `live checks: PASS (exit 0)`.
  CI's tagged step (`.github/workflows/test.yml:103`) compiles with `-run '^$'`, so neither entry point runs that test.
  A gate that cannot fail is the defect class here.
- Second defect, of the same class: the vacuity check compares test NAMES only (L160-161). If one tagged name ran in
  one package, it covers the same name in another package that never ran. The consultant asks for package-plus-name
  reconciliation (L106: "Reconcile package plus test name against actual executions").
- Stale claim: the comment at L74-76 says the tagged files are checked by `go vet -tags oraclelive ./oracle/ ./gui/
  ./sysw/`. CI really runs `go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` (test.yml:103).

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (discovery is module-wide).** The script searches every `.go` file under the repo root, in the directories
  `go test ./...` would visit. It skips any directory named `testdata` or `vendor`, and any whose name starts with `.`
  or `_` (the root `.` itself is searched). A file is tagged when a line matches the existing constraint regex
  `^//go:build\b.*\boraclelive\b` (unchanged, including its stated handling of `!oraclelive`). There is no hand list of
  packages anywhere in the check path. The `-update` path (L89-114) is unchanged and stays `./gui/` only.
- **R2 (tests are keyed by package).** Each discovered top-level test is recorded as `<pkg> <Test>`, where `<pkg>` is
  the file's directory with a trailing `/` (for example `./cmd/emu/`). `TestMain` is still excluded. Zero tagged files
  and zero tagged tests keep their existing `::error::` lines and exit 1 (L130-139).
- **R3 (one `go test` per package).** For each package, in sorted order, the script runs
  `CGO_ENABLED=0 go test -tags oraclelive -count=1 -v -run "^(<that package's names>)\$" <pkg> "$@"` and prints its
  output as before. The script's exit code is the exit code of the first package run that failed, or 0 if none
  failed. Extra arguments are passed to every package run, where before they were passed to the one run.
- **R4 (vacuity check by package and name).** A test counts as run only when a `=== RUN   <Test>` line (top level, no
  `/`) appears in the output of its own package's run. Every `<pkg> <Test>` that did not run is listed and makes the
  exit 1.
- **R5 (regression test in every push).** A new untagged test file, `oracle/live_runner_test.go`, runs the real script
  with a stub `go` first on PATH. It therefore runs in CI's `go test ./...` and needs no pinned binaries. It checks R1-R4
  against an independent discovery: Go's own `go/build/constraint` and `go/parser`, walking the tree by Go's directory
  rules.
- **R6 (comments).** The script's header comment says that the packages are derived from the source (citing this FIX
  and bh-sh-I11), names the new test, and corrects the stale CI command (L74-76) to the real `go test -tags oraclelive
  -run '^$'` step, noting that that package list is kept by hand (see FOLLOWUPS).

Edge cases, decided here (none is visible to a holder; these are maintainer-script behaviours):
- A tagged file at the repo root gets the package `./`, which `go test` accepts.
- A tagged file inside a nested module, or in a directory `go test` cannot load, makes that package's `go test`
  fail, so the script fails loudly. It is never silently dropped.
- A tagged file whose path holds whitespace is refused with the `::error::a tagged file's path holds whitespace`
  line (§3) and exit 1, before any `go test` runs (Go cannot load such a package: `malformed import path`). No nested module exists at the base
  (`find . -name go.mod` finds only the root).
- The same test name in two packages counts as two tests (R2 and R4).
- Discovery uses `find ... -prune`, not `grep -r --exclude-dir='.*'`. Seen on GNU grep 3.12: that pattern also
  matches the starting `.`, so grep searches nothing and the script would stop at "no //go:build oraclelive files
  found". The comment says why.

## 3. Printed-text changes (maintainer script; complete list)

| where | before | after |
|---|---|---|
| discovery line | `   discovered <N> tagged test(s) from source` | `   discovered <N> tagged test(s) in <P> package(s) from source` **PROPOSED (architect confirms)** |
| each never-run entry under `::error::discovered <N> tagged test(s); these never executed:` | `         <Test>` | `         <pkg> <Test>` (e.g. `         ./gui/ TestSame`) **PROPOSED (architect confirms)** |
| whitespace refusal (new) | none (the file was dropped and the run could PASS) | `::error::a tagged file's path holds whitespace; go cannot load it and this script cannot run it:` then each such path with nine spaces of indent; exit 1 **PROPOSED (architect confirms)** |
| go output | one `go test` block for 3 packages | one block per package (4 at base: `./cmd/emu/`, `./gui/`, `./oracle/`, `./sysw/`) |

Unchanged: every other line, including `live checks: PASS (exit 0)` / `live checks: FAIL (exit <rc>)`, both
`::error::` lines for an empty discovery, the closing `a live check that does not run is ...` line, and the whole
`-update` branch. No flag, command or type is added. Nothing in the repo or the toolkit manual quotes the two changed
lines (`git grep 'tagged test(s)'` outside `design/agent-reports/` finds only the script; the manual never mentions
oracle-live.sh).

## 4. Code changes (builder applies)

The scratch version is `design/FIX_sh_oracle_live_discovery-probe.diff`. It is evidence, not a patch to copy blindly;
the builder writes the final version.

Files changed (for build ordering): `scripts/oracle-live.sh`; new `oracle/live_runner_test.go`. No other file.

1. `scripts/oracle-live.sh` L129: replace the hand-listed grep with `find . -type d \( -name testdata -o -name vendor
   -o -name '.?*' -o -name '_*' \) -prune -o -type f -name '*.go' -print0 | sort -z | xargs -0 -r grep -lE '<same regex>'`
   (R1).
1a. After the existing empty-discovery check, refuse a tagged path that holds whitespace (R1; review r1 I1):

       if grep -q '[[:space:]]' <<<"$tagged_files"; then
         echo "::error::a tagged file's path holds whitespace; go cannot load it and this script cannot run it:"
         grep '[[:space:]]' <<<"$tagged_files" | sed 's/^/         /'
         exit 1
       fi

2. L134: build `tagged_tests` as `<pkg> <Test>` lines, sorted and unique, with `<pkg>` = `${f%/*}/` (R2). Derive the
   package list from it, then print R3's discovery line.
3. L144-146: loop over the packages, one `go test` each, with that package's own `-run` filter. Keep the first
   non-zero exit code, and collect `<pkg> <Test>` from each run's top-level `=== RUN` lines (R3).
4. L158-165: `comm -23` of the discovered pairs against the ran pairs. Print each missing pair with nine spaces of
   indent (R4).
5. Comments per R6, at L69-76 and above the new find.

## 5. Tests

New (test function names are the TRACE row names), in `oracle/live_runner_test.go` (package `oracle`, untagged). A
`goStub` bash script, written to a temp dir and placed first on PATH, parses `-run '^(A|B)$'` and the `./`-package
arguments. For each package and name it logs `<pkg>\t<name>` to `$STUB_LOG` and prints `=== RUN   <name>` /
`--- PASS` unless `<pkg>:<name>` is in `$STUB_SKIP`. It fails a package (`FAIL\t<pkg>`, exit 1) if that package is in
`$STUB_FAIL`. bash missing is `t.Fatalf`, not a skip.
- `TestFixShOracleLiveRunsEveryTaggedTestInTheTree`: runs the real script on the real tree. Its oracle is a
  `filepath.WalkDir` over the repo by Go's directory rules, taking files whose `//go:build` line evaluates true with
  `oraclelive` and false without it (`go/build/constraint`), and their top-level `Test*` functions other than `TestMain`
  (`go/parser`). It pins that the oracle contains `./cmd/emu/ TestSyswComposerPayloadDigestAgreesWithMe`. Wants exit 0
  and stub pairs equal to the oracle's pairs: nothing missing, nothing extra.
- `TestFixShOracleLiveFailsOnAFailingNewPackage`: synthetic tree (a copy of the script, plus tagged `oracle/`, `gui/`,
  `sysw/` and `newpkg/`). The stub fails `./newpkg/`. Wants a non-zero exit, `./newpkg/ TestN` asked for, and
  `live checks: FAIL (exit 1)` (the consultant's "deliberately failing synthetic tagged test in an additional package").
- `TestFixShOracleLiveReconcilesByPackage`: `oracle/` and `gui/` each declare tagged `TestSame`. The stub skips
  `./gui/:TestSame`. Wants exit 1 and the line `\n         ./gui/ TestSame\n`.
- `TestFixShOracleLiveSkipsDirsGoIgnores`: tagged tests in `oracle/`, `oracle/testdata/`, `.hidden/`, `_skip/` and
  `vendor/v/`. Wants exit 0, stub pairs exactly `["./oracle/ TestO"]`, and
  `   discovered 1 tagged test(s) in 1 package(s) from source\n`.
- `TestFixShOracleLiveRefusesAPathWithWhitespace`: tagged `oracle/live_test.go` (TestO) and `a b/live_test.go`
  (TestSpace). Wants exit 1 and the whitespace `::error::` line followed by `         ./a b/live_test.go`.

Tests that change: none. The tagged tests themselves are untouched. The new file also compiles under
`-tags oraclelive` (seen: CI's tagged compile step is ok with it).

## 6. RED rows (run on base 25abe8e with only the §5 test file added)

Command (from the repo root, Go 1.26.4, `GOTOOLCHAIN=local`):
`CGO_ENABLED=0 go test -count=1 -v -run 'TestFixShOracleLive' ./oracle/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestFixShOracleLiveRunsEveryTaggedTestInTheTree | FAIL: `oracle-live.sh exit 0` / `never asked for: ["./cmd/emu/ TestSyswComposerPayloadDigestAgreesWithMe"]`, and extra pairs (every name asked in each of the 3 hand-listed packages); output shows `discovered 8 tagged test(s) from source` | PASS |
| TestFixShOracleLiveFailsOnAFailingNewPackage | FAIL: `a failing tagged test in ./newpkg/: exit 0, asked for it: false` | PASS |
| TestFixShOracleLiveRefusesAPathWithWhitespace | FAIL: `exit 0; want exit 1 and the line ...` (also FAIL on the probe as first written: `live checks: PASS (exit 0)`) | PASS |
| TestFixShOracleLiveReconcilesByPackage | FAIL: `./gui/ TestSame never ran: exit 0; want exit 1 ...` | PASS |
| TestFixShOracleLiveSkipsDirsGoIgnores | FAIL: `exit 0, asked for ["./gui/ TestO" "./gui/ TestTestdata" "./oracle/ TestO" "./oracle/ TestTestdata" "./sysw/ TestO" "./sysw/ TestTestdata"]` (base searches `oracle/testdata/`) | PASS |

Fixed script, real tree, stub `go` (seen): `discovered 9 tagged test(s) in 4 package(s) from source`; `ok` for
`./cmd/emu/`, `./gui/`, `./oracle/`, `./sysw/`; `live checks: PASS (exit 0)`. Reconcile case (seen):
`::error::discovered 2 tagged test(s); these never executed:` / `         ./gui/ TestSame` / `live checks: FAIL (exit 1)`.

Mutation seen on the scratch fix: dropping `./cmd/` paths from the find output fails
TestFixShOracleLiveRunsEveryTaggedTestInTheTree with `never asked for: ["./cmd/emu/ TestSyswComposerPayloadDigestAgreesWithMe"]`.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 module-wide discovery, Go's directory rules | TestFixShOracleLiveRunsEveryTaggedTestInTheTree; TestFixShOracleLiveFailsOnAFailingNewPackage; TestFixShOracleLiveSkipsDirsGoIgnores; TestFixShOracleLiveRefusesAPathWithWhitespace |
| R2 tests keyed by package | TestFixShOracleLiveRunsEveryTaggedTestInTheTree (pairs exact); TestFixShOracleLiveReconcilesByPackage |
| R3 one run per package, first failure's exit code | TestFixShOracleLiveFailsOnAFailingNewPackage; TestFixShOracleLiveRunsEveryTaggedTestInTheTree (no extra pairs) |
| R4 vacuity by package and name, the listed line | TestFixShOracleLiveReconcilesByPackage |
| R5 runs in `go test ./...` | all five (untagged file in ./oracle/) |
| R6 comments | none (review reads it) |
| §3 discovery line | TestFixShOracleLiveSkipsDirsGoIgnores |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`) ;
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ; `./scripts/test-32bit.sh` ;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI) ; `bash -n scripts/oracle-live.sh` ; gofmt.
Measured on the scratch fix by the author (go1.26.4): `go test ./oracle/` ok; the tagged compile step above ok;
`GOARCH=386 go test -run TestFixShOracleLive ./oracle/` ok; `gofmt -l oracle/` empty; `go vet ./oracle/` reports
nothing for the new file; `bash -n` ok. The author did not run the rest of `./...` or the other jobs; no other package
is touched. The builder runs the whole gate.

Not covered by any row: a real live run (`./scripts/oracle-live.sh` with the pinned `me`/md/mk/ms from
`oracle/pins.json` at `~/.cargo/bin` and the sibling checkouts). The rows prove which packages and names the script
asks for, and how it judges the result; they do not prove the oracles agree. The builder runs the real script once if
the pinned binaries are installed, and records the result (the consultant saw the emu test pass alone against `me`
0.13.0, and saw the script exit 1 for other, already-tracked reasons). Never with `-update`.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (no `mr`/`md-cli` text; the manual does not mention oracle-live.sh).
- `.github/`: not edited. The CI tagged-compile list already includes `./cmd/emu/`.
- Closes: bh-sh-I11 with the GREEN merge. seedhammer has no `design/FOLLOWUPS.md`; nothing tracked there to close.
- Files (proposed, verbatim, for direct-push after the merge; the coordinator chooses the target file):

```
- [sh-oraclelive-ci-compile-list] status=open grade=M kind=tooling owner=none filed=2026-11-22 src=seedhammer:design/FIX_sh_oracle_live_discovery.md repo=sh :: .github/workflows/test.yml:103 compiles the oraclelive files from a hand package list (./oracle/ ./gui/ ./sysw/ ./cmd/emu/); a tagged file in a new package is not compiled on push until someone adds it. `-tags oraclelive -run '^$' ./...` or a derived list closes it.
- [sh-oraclelive-typecheck-comments] status=open grade=N kind=docs owner=none filed=2026-11-22 src=seedhammer:design/FIX_sh_oracle_live_discovery.md repo=sh :: Tagged files claim `go vet -tags oraclelive ./...` type-checks them on every push (gui/chain_fixture_live_test.go:25, gui/multisig_build_oracle_live_test.go:25, sysw/vendored_vectors_live_test.go:24, oracle/live_test.go:74); CI runs `go test -tags oraclelive -run '^$'` over a hand list instead.
```

## 10. Out of scope

The CI step's hand list and the stale comments in the tagged files (filed above). The `!oraclelive` regex caveat
(L127-128) is unchanged. No such file exists, and one would show up as a never-run pair. The `-update` mint path.

## 11. How this was checked

The author ran the RED and GREEN rows as in §6. The probe diff's sha256 is recorded in the report. The probe diff
applies with `git apply --check` on 25abe8e, and was reverted after the run. The "seen" texts are copied from the
runs, not written by hand.
