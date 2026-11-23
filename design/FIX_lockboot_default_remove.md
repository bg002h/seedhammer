# FIX: the default build carries no `lock-boot` command

**Small track** (one Opus review round). Class: other (not a funds path; an irreversible OTP write). Grade I, one row:
r49 `lockboot-nfc-default-build` (coordinator `agent-reports/grade-security-sweep-engrave-1.md` L30, L100, L140, L162).
Sweep unit `lockboot-default-remove` (was `lockboot-default-confirm`).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-23. Base: seedhammer `origin/main` f36f786. Anchors are at that
base unless stated. Location: the fork keeps its FIX documents in `design/` beside a `-probe.diff`
(`design/FIX_sec-sh-sysw-plaintext-refuse.md`, `design/FIX_sh_uf2_truncation.md`, ...), so this one is
`design/FIX_lockboot_default_remove.md` with `design/FIX_lockboot_default_remove-probe.diff`, not `docs/`.

**P19 ruled by Brian 2026-11-23, "remove it"** (coordinator `status/sweep-product-rulings.md` L6-L7): option (b),
remove `command: lock-boot` from the fork's default build entirely, as the Refugium build already does; one shared
scan arm, so NFC and every other source lose it together. Context from Brian: "NFC is a second class feature. We
don't want to excise it but we aren't keen on expanding it as usb cable is by definition more secure." Secure boot
provisioning for fork owners stays the USB/picotool path of `docs/custom-firmware.md`. Out of scope, unchanged:
`FOREVERLAURA!` (the QA program) and every other debug-command behaviour.

## 1. The defect

`gui/debugcmd_default.go:33-39`: in the default build a scanned `command: lock-boot` tag calls
`ctx.Platform.LockBoot()` with no confirmation. On the device that is `cmd/controller/lockboot_default.go:17-25`:
`writeOTPValues(signKeyHash, boardVersion())` (white-label strings and SeedHammer's boot key into OTP), then
`otp.EnableSecureBoot()`, then `machine.CPUReset()`. The write is irreversible and is triggered by untrusted scanned
text. The Refugium build already links neither the command (`gui/debugcmd_refugium.go`) nor the writer
(`cmd/controller/lockboot_refugium.go`).

Callers of `Platform.LockBoot` at f36f786 (`git grep -n 'LockBoot()' -- '*.go'`, production files only): exactly one,
`gui/debugcmd_default.go:35`. The other hits are the interface declaration `gui/gui.go:3680`, the three
implementations (`cmd/controller/lockboot_default.go:17`, `cmd/controller/lockboot_refugium.go:13`,
`cmd/emu/platform.go:327`), the test platform `gui/gui_test.go:474`, and comments. No script, tool or other binary
in the repo calls it (`git grep -n -i 'lockboot\|lock-boot'` outside `*.go`: only `.github/workflows/test.yml:154`
and `scripts/refugium-elf-check.sh:49`, both naming the log-format marker, §4.4).

## 2. Rules (decided here; §7 maps each to its rows)

- **D1, behaviour.** In the default build a scanned `command: lock-boot` is an **unknown debug command**, exactly like
  `command: nope` today: `handleDebugCommand` takes its `default:` arm, logs `unknown debug command: "lock-boot"`
  (the existing format `unknown debug command: %q`, `gui/debugcmd_default.go:41`), and returns `debugPassThrough`;
  `StartScreen.Flow` returns `startScreenAction{scan: debugCommand{"lock-boot"}}`; `engraveObjectFlow` has no case
  for it and returns false; the main loop sets `scanUnknownFormat` (`gui/gui.go:2210-2211`), so the start screen's
  status line reads **"Unknown format"** for `scanStatusTimeout`. `Platform.LockBoot` is never called.
  - Why not the Refugium arm's behaviour (stay, log `debug commands are not part of this build`): in the default
    build that log text would be false (`FOREVERLAURA!` is part of it), and staying silently is a new behaviour with
    new text; D1 is the default build's existing rule for a command it does not know, reached by deleting the arm
    and adding nothing. In both builds the command does nothing; only the feedback differs, and each build keeps its
    own rule for all unrecognised commands.
- **D2, the writer is unlinked, not deleted.** Only the `case "lock-boot":` arm (seven lines) is deleted.
  `Platform.LockBoot` stays in the `gui.Platform` interface, and its three implementations stay as they are
  (`lockboot_default.go` keeps the OTP writer; `lockboot_refugium.go` and the emulator keep refusing). Why:
  - the upstream diff stays small: upstream's `gui.Platform` declares `LockBoot`, and its controller carries the
    writer; deleting them would make every upstream merge that touches the interface or `platform_sh2.go` conflict;
  - `writeOTPValues` keeps its host test (`cmd/controller/bughunt_sh2_39_test.go`), and the Refugium source gate's
    positive controls for `writeOTPValues`, `AddBootKey`, `EnableSecureBoot`, `WriteWhiteLabelAddr` and
    `WriteWhiteLabelString` (`gui/refugium_build_test.go:177-187`) still find their calls in the default controller set;
  - unreachability is pinned instead: no default-build file in `gui` or `cmd/controller` calls anything named
    `LockBoot` (R2), so a later command under another spelling cannot reach it unnoticed;
  - measured (§4.3): with no caller TinyGo links none of the writer into the default ELF, and the ELF check asserts
    that on every run (§4.4), so the shipped binary carries no OTP writer while the source stays.
- **D3, the emulator's refusal stays**, unchanged (`cmd/emu/platform.go:324-329`). It is now reachable from no UI
  path; it stays because `gui.Platform` declares the method.
- **D4, the gates follow the change.** `lock-boot` stays on the Refugium build's forbidden-literal list but is exempt
  from that list's default-build positive control (it is absent from both builds now). In the ELF check the four
  markers that the arm's removal takes out of the default ELF (§4.3: `lock-boot: %v`, the OTP redirect URL,
  `otp.writeECC`, `otp.writeOrRow`) move from present-in-default/absent-in-Refugium to absent-from-both (§4.4); every
  other marker is unchanged.
- **D5, `FOREVERLAURA!` is unchanged** in both builds (TestDebugCommandQAProgramCharacterization, unchanged, still
  passes, and still fails if `LockBoot` is called).

## 3. Holder-visible changes (complete list)

| Build | Input | Before (f36f786) | After |
|---|---|---|---|
| default, device | `command: lock-boot` tag on the start screen | OTP written, secure boot enabled, reboot; on an OTP error, status "Scan error" and log `lock-boot: <err>` | status "Unknown format"; log `unknown debug command: "lock-boot"`; no OTP access |
| default, emulator | the same | status "Scan error", log `lock-boot: secure boot cannot be locked in the emulator` | status "Unknown format"; log `unknown debug command: "lock-boot"` |
| Refugium | any | unchanged (no tag is read; the arm stays and logs `debug commands are not part of this build`) | unchanged |

No other screen, text, exit or log line changes. `docs/custom-firmware.md` does not mention `lock-boot`
(`grep -n -i 'lock-boot\|lockboot' docs/custom-firmware.md`: no output) and stays true unchanged: provisioning is
USB/picotool.

## 4. Code changes (builder applies; reference implementation: `design/FIX_lockboot_default_remove-probe.diff`, f36f786 to GREEN)

### 4.1 `gui/debugcmd_default.go`
- Delete lines 33-39 (the `case "lock-boot":` arm, through its `return debugStay, startScreenAction{}`).
- Doc comment lines 20-28, old -> new:
  - old: `// replace it with debugcmd_refugium.go, which carries neither command. The` / `// behaviour is pinned by gui/debugcmd_characterization_test.go:` / `//` / `//   - FOREVERLAURA! returns the QA program;` / `//   - lock-boot calls Platform.LockBoot once and stays on the start screen,` / `//     reporting a failure in the status line;` / `//   - anything else is logged and passed through.`
  - new: `// replace it with debugcmd_refugium.go, which carries no command. The` / `// behaviour is pinned by gui/debugcmd_characterization_test.go:` / `//` / `//   - FOREVERLAURA! returns the QA program;` / `//   - anything else is logged and passed through, lock-boot included: this` / `//     build carries no lock-boot command and nothing here calls` / `//     Platform.LockBoot (Brian 2026-11-23, P19; design/FIX_lockboot_default_remove.md).`
- `log` stays imported (the `default:` arm uses it).

### 4.2 Comments that named the removed path (text only)
- `cmd/controller/lockboot_default.go:11-15`, old: `// The OTP writer behind the `command: lock-boot` debug command` / `// (gui/debugcmd_default.go), moved out of platform_sh2.go unchanged so the` / `// Refugium build links none of it (Refugium plan F7 §4.1). Its twin,` / `// lockboot_refugium.go, refuses. signKeyHash stays in platform_sh2.go because` / `// isSecureBootEnabled still reads it.`
  new: `// The OTP writer that the `command: lock-boot` debug command used to call,` / `// moved out of platform_sh2.go unchanged so the Refugium build links none of it` / `// (Refugium plan F7 §4.1). Nothing calls it: the default build dropped that` / `// command too (Brian 2026-11-23, P19; design/FIX_lockboot_default_remove.md),` / `// and gui/refugium_build_test.go's TestDefaultBuildCarriesNoLockBootCommand` / `// keeps it that way, so TinyGo links it into neither ELF` / `// (scripts/refugium-elf-check.sh). It is kept, not deleted, to keep the` / `// upstream diff small. Its twin, lockboot_refugium.go, refuses. signKeyHash` / `// stays in platform_sh2.go because isSecureBootEnabled still reads it.`
- `cmd/controller/lockboot_refugium.go:10-12`, old: `// Nothing in this build calls it -- gui's debug-command arm recognises no` / `// command here (gui/debugcmd_refugium.go) -- and it fails closed if anything` / `// ever does.`
  new: `// Nothing in either build calls it -- gui's debug-command arm recognises no` / `// lock-boot command in either (design/FIX_lockboot_default_remove.md) -- and it` / `// fails closed if anything ever does.`
- `seal/record.go:213-218`, old: `// cmdPrefix mirrors gui/scan.go's. A decrypted plaintext of` / `// "command: lock-boot" would reach gui/debugcmd_default.go's handleDebugCommand` / `// and call Platform.LockBoot, which does writeOTPValues -> otp.EnableSecureBoot` / `// -> machine.CPUReset (cmd/controller/lockboot_default.go). The Refugium build` / `// carries neither the command nor the writer (gui/debugcmd_refugium.go,` / `// cmd/controller/lockboot_refugium.go). This prefix is the only gate, and the`
  new: `// cmdPrefix mirrors gui/scan.go's. A decrypted plaintext of "command: ..."` / `// would reach gui/debugcmd_default.go's handleDebugCommand, which in the default` / `// build still starts the FOREVERLAURA! QA program. ("command: lock-boot", which` / `// once called Platform.LockBoot -> writeOTPValues -> otp.EnableSecureBoot, is in` / `// neither build since design/FIX_lockboot_default_remove.md; the Refugium build` / `// carries no command at all, gui/debugcmd_refugium.go.) This prefix is the only` / `// gate, and the`
  (lines 219-220, `// wire format is normative and public, ...`, unchanged).
- `gui/refugium_profile_test.go:103`, old: `// and the start screen stays. Control: TestDebugCommandLockBootCharacterization.`
  new: `// and the start screen stays. Control:` / `// TestDebugCommandLockBootIsUnknownInTheDefaultBuild (the default build reads` / `// the same tag and hands it on).`
- `.github/workflows/test.yml:154-158`, old: `# The device half of the absence gate: every marker (the lock-boot log` / `# format, the OTP redirect URL, the proof triggers, Poller.Read,` / `# qaEngraveFlow, the OTP write primitives) PRESENT in default.elf and` / `# ABSENT in refugium.elf, and the profile's own notices the other way` / `# round. Allocated sections only, so DWARF cannot match.`
  new: `# The device half of the absence gate: every marker (the proof triggers,` / `# Poller.Read, qaEngraveFlow) PRESENT in default.elf and ABSENT in` / `# refugium.elf, the profile's own notices the other way round, and the` / `# removed lock-boot arm's log format, the OTP redirect URL and the OTP` / `# write primitives ABSENT from both. Allocated sections only, so DWARF` / `# cannot match.`
  This is a comment-only `.github/` edit inside the reviewed PR; the PR description says so.

### 4.3 Measured: what TinyGo links once the arm is gone
TinyGo builds of `./cmd/controller` with CI's flags (`-target pico-plus2 -stack-size 16kb -gc precise -opt 2
-scheduler tasks`, `-size short`), ELFs in `/scratch/cache/lockboot-elf/`: `base-default.elf` (f36f786),
`base-refugium.elf` (f36f786, `-tags refugium`), `probe-default.elf` (f36f786 with only the arm deleted). The Refugium
ELF is the same before and after: every changed production file is `!refugium` or comment-only.

| marker (refugium-elf-check.sh) | base default | probe default | Refugium |
|---|---|---|---|
| `lock-boot: %v` | 1 | 0 | 0 |
| `seedhammer.com/doc/?d=SHII` (OTP redirect URL) | 1 | 0 | 0 |
| `seedhammer.com/driver/otp.writeECC` | 1 | 0 | 0 |
| `seedhammer.com/driver/otp.writeOrRow` | 1 | 0 | 0 |
| the 7 other markers (proof triggers, Poller.Read, qaEngraveFlow) | present | present | 0 |

Flash: base default 1760032 bytes, probe default 1756476 (3556 fewer); Refugium 1688872. So with its only caller gone,
TinyGo links the OTP writer into **neither** ELF: D2 keeps the source (small upstream diff) while the shipped default
binary, like the Refugium one, carries no OTP writer. §4.4 asserts that on every CI run.

### 4.4 `scripts/refugium-elf-check.sh`
The unchanged script FAILS on the GREEN default ELF (`./scripts/refugium-elf-check.sh probe-default.elf
base-refugium.elf`, rc=1; `/scratch/cache/lockboot-elf/elf-probe-oldscript.txt`): `lock-boot: %v`, the redirect URL,
`otp.writeECC` and `otp.writeOrRow` each `CONTROL FAILED: absent from the default ELF`. Change (probe diff):
- Move `b"lock-boot: %v"` and `b"seedhammer.com/doc/?d=SHII"` out of `LITERALS` into a new `ABSENT_LITERALS`, and
  `otp.writeECC` and `otp.writeOrRow` out of `SYMBOLS` into a new `ABSENT_SYMBOLS`; each such row (labelled
  `absent: <marker>`) must count 0 in BOTH ELFs, else `PRESENT in the default ELF` / `PRESENT in the Refugium ELF`
  and `RESULT: FAIL`. `LITERALS`, `SYMBOLS` and `PROFILE` keep their other entries and checks unchanged.
- The ok line becomes `RESULT: ok -- 7 markers present in the default ELF and absent in the Refugium ELF; 4 OTP-writer
  markers absent from both; 3 profile notices the other way round` (counts computed from the lists, as today).
- The header gains the both-absent rule and says why it has no positive control (the names were measured PRESENT,
  count 1 each, in f36f786's default ELF, the last build that called the writer); exact text in the probe diff.
Run: new script on probe-default/base-refugium rc=0 with that ok line (`elf-probe-newscript.txt`); new script on
base-default/base-refugium rc=1, the four `absent:` rows `PRESENT in the default ELF` (`elf-base-newscript.txt`), so
the new rows can fail; old script on base ELFs rc=0, `RESULT: ok -- 11 markers ...` (`elf-base.txt`).

## 5. Tests

Changed file `gui/debugcmd_characterization_test.go` (`//go:build !refugium`):
- `TestDebugCommandLockBootCharacterization` (old: a lock-boot tag calls `LockBoot` exactly once, for a nil and an
  error return, and `Flow` stays on the start screen) is **replaced** by **`TestDebugCommandLockBootIsUnknownInTheDefaultBuild`**
  (R1): a test platform whose `lockBoot` hook counts calls; drive `command: lock-boot` (stop as soon as the hook is
  called); want 0 calls (`LockBoot called %d times for a lock-boot tag, want 0`), `Flow` returned
  (`StartScreen.Flow did not return on a lock-boot tag; want it handed on as an unknown command`), the action equal to
  `startScreenAction{scan: debugCommand{"lock-boot"}}`, and `engraveObjectFlow` on that object false (the "Unknown
  format" path). The `errors` import goes (no other user in the file).
- The file's header comment (lines 17-19) gains the P19 sentence (probe diff).
- `TestDebugCommandQAProgramCharacterization` and `TestDebugCommandUnknownCharacterization`: unchanged.

Changed file `gui/refugium_build_test.go` (untagged; runs in both suites):
- New **`TestDefaultBuildCarriesNoLockBootCommand`** (R2): over the default file sets the Refugium gate already reads
  (`./gui` with tags `""` and `tinygo,rp`; `./cmd/controller` with `tinygo,rp`): no string literal containing
  `lock-boot`, no call named `LockBoot`. Positive control first: the same sets carry a `FOREVERLAURA!` literal and a
  `handleDebugCommand` call, else `t.Fatal("POSITIVE CONTROL: ...")`.
- `TestRefugiumFileSetsCarryNoForbiddenLiteralOrCall` (R3, changed, stays GREEN at every step): `lock-boot` is exempt
  from the default-build positive control only (comment cites R2); its Refugium-absence check is unchanged.

Mutants each new row catches (run, §6):
- M-A, the arm kept (today's code): R1 and R2 FAIL.
- M-B, the arm kept but inert, Refugium-style (`case "lock-boot": return debugStay, startScreenAction{}`): R1 FAILs
  `did not return`, R2 FAILs on the literal.
- M-C, the writer reached under another spelling (`case "LOCKBOOT!": _ = ctx.Platform.LockBoot()`): R1 passes, R2
  FAILs `the default build calls LockBoot, in [debugcmd_default.go]`.

Unchanged and still green: `cmd/controller/bughunt_sh2_39_test.go` (writeOTPValues), `gui/refugium_profile_test.go`
(Refugium rows; comment only, §4.2), `seal/*` rows that use `command: lock-boot` as a fixture string (it is still a
`ClassDebugCommand` record and still refused in a sealed section; those rows test the classifier, not the arm).

## 6. RED and GREEN rows (go1.26.4 at `~/.local/go/bin`, `GOTOOLCHAIN=local CGO_ENABLED=0`, GOTMPDIR outside the worktree)

The command throughout (CI's `go test ./...` restricted to the rows):
`go test ./gui/ -run '^(TestDebugCommandLockBootIsUnknownInTheDefaultBuild|TestDefaultBuildCarriesNoLockBootCommand|TestDebugCommandQAProgramCharacterization|TestDebugCommandUnknownCharacterization|TestRefugiumFileSetsCarryNoForbiddenLiteralOrCall|TestRefugiumPairsAreWellFormed)$' -count=1 -v`

- **RED on f36f786 plus the test changes** (production code unmodified), rc=1:
  - `debugcmd_characterization_test.go:67: LockBoot called 1 times for a lock-boot tag, want 0`
  - `debugcmd_characterization_test.go:70: StartScreen.Flow did not return on a lock-boot tag; want it handed on as an unknown command`
  - `--- FAIL: TestDebugCommandLockBootIsUnknownInTheDefaultBuild`
  - `refugium_build_test.go:207: the default build carries a string literal containing "lock-boot", in [debugcmd_default.go]`
  - `refugium_build_test.go:210: the default build calls LockBoot, in [debugcmd_default.go]`
  - `--- FAIL: TestDefaultBuildCarriesNoLockBootCommand`
  - PASS: the two unchanged characterization rows. (Run with the first four names; the R3 rows pass on f36f786 with or without R3's change.)
- **Ordering RED** (why R3 is in the same commit): the arm deleted and R1/R2 added but R3's exemption not yet applied,
  rc=1: `refugium_build_test.go:170: POSITIVE CONTROL: no default-build file has a string literal containing "lock-boot", so its absence from the Refugium build proves nothing`.
- **GREEN** (the probe diff applied), rc=0: 6 PASS; R1 logs `unknown debug command: "lock-boot"`.
- **Mutants on GREEN** (rc=1 each): M-B `debugcmd_characterization_test.go:70: StartScreen.Flow did not return on a lock-boot tag ...` and `refugium_build_test.go:210: the default build carries a string literal containing "lock-boot"`; M-C `refugium_build_test.go:213: the default build calls LockBoot, in [debugcmd_default.go]` with R1 PASS. (Line numbers are on the GREEN file; the RED run's are 3 lower, before R3's comment.)

## 7. Coverage map (rule -> rows)

| Rule | Rows |
|---|---|
| D1 lock-boot is an unknown command: no LockBoot call, handed on, "Unknown format" | R1 TestDebugCommandLockBootIsUnknownInTheDefaultBuild (RED; M-A, M-B) |
| D2 no default-build caller of LockBoot, any spelling; writer kept | R2 TestDefaultBuildCarriesNoLockBootCommand (RED; M-C); TestBughuntSh239LockBootStopsOnLabelAddrError and the Refugium call controls still find the writer (existing) |
| D3 emulator refusal stays | no code change; `GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` (gate) |
| D4 gates follow | R3 TestRefugiumFileSetsCarryNoForbiddenLiteralOrCall (ordering RED); refugium-elf-check.sh: old script RED on the GREEN default ELF, new script ok on it and RED on the base default ELF (§4.4) |
| D5 FOREVERLAURA! unchanged | TestDebugCommandQAProgramCharacterization (existing, unchanged) |
| Refugium unchanged | the tagged suite `./scripts/refugium-tagged-test.sh`, TestRefugiumDebugCommandsAreInert, TestRefugiumStartScreenReadsNoTag (existing) |

## 8. Gate (the repo's CI jobs, `.github/workflows/test.yml`; run once each to a file and grep it)

The builder runs on the GREEN head: `CGO_ENABLED=0 go test -timeout 20m ./...` (or
`mnemonic-engrave/scripts/gui-shard-test.sh ./gui/ 24` for gui plus `go test ./...` for the rest);
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`; `./scripts/test-32bit.sh`;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and with `-tags refugium`; `./scripts/refugium-tagged-test.sh`; both TinyGo
builds with CI's flags and `./scripts/refugium-elf-check.sh default.elf refugium.elf`; gofmt on the changed files.

## 9. Lockstep and records

- mnemonic-engrave (separate change, PR track there; not in this PR): `design/RUNBOOK_custom_boot_key.md:234-237`,
  old: `> **Never send the NFC `lock-boot` debug command to this unit.** It is the` / `> factory provisioning path (`gui/gui.go:1626` → `LockBoot()`). It cannot waste` / `> a boot-key slot — `AddBootKey` returns the existing slot on an exact match —` / `> but it burns white-label OTP rows for no benefit.`
  new: `> **Never send the NFC `lock-boot` debug command to this unit.** On SeedHammer's retail firmware it is the` / `> factory provisioning path (`LockBoot()`); the fork's builds, default and Refugium, carry no such command` / `> (seedhammer `design/FIX_lockboot_default_remove.md`). On retail firmware it cannot waste` / `> a boot-key slot — `AddBootKey` returns the existing slot on an exact match —` / `> but it burns white-label OTP rows for no benefit.`
  `RUNBOOK_custom_boot_key.md:32-34` (§0, "Retail units are sealed at the factory by an NFC-delivered `lock-boot`")
  describes retail firmware and stays true: unchanged.
- mnemonic-engrave `design/FOLLOWUPS.md` L984-990 (the free-text hazard note) stays true: `command: ` still dispatches
  to `debugCommand` (`FOREVERLAURA!`), so the allow-list must still not be undone; unchanged (FOLLOWUPS is
  append-only). L20536-20539 (F7, Refugium) is still accurate for the Refugium build; unchanged. One new line,
  direct-pushed after this FIX's PR merges (report lists it verbatim).
- The F7 plan (`design/IMPLEMENTATION_PLAN_fork_refugium_F5_F7.md` L140-146, L288) is a historical plan record of
  what F7 built; unchanged.
- Toolkit manual: not affected (no `mr`/`md-cli` flag or text).

## 10. Out of scope

`FOREVERLAURA!` and the typed QA proof triggers; the Refugium build (no change); deleting the OTP writer or the
interface method (D2); the `seal` classifier's treatment of `command: ` records (unchanged; still refused).

## 11. How this was checked

Every RED, GREEN, ordering and mutant row in §6 was run on f36f786 in `/scratch/cache/wt-sh-lockboot-remove` (RED)
and `/scratch/cache/wt-sh-lockboot-probe` (the probe diff); the TinyGo builds in §4.3 were run with CI's flags
(`-size short` for all three; the probe ELF was built after the arm deletion and before the comment-only edits).
Also run on the probe tree, rc=0 (`/scratch/cache/lockboot-elf/probe-broad.log`): `go test -count=1 ./cmd/... ./seal/`
(all ok); `go test -run 'Debug|Refugium|LockBoot|NFC' ./gui/` untagged and `-tags refugium` (ok); `GOOS=js GOARCH=wasm
go vet ./cmd/emu/` untagged and `-tags refugium` (ok); gofmt -l on the changed Go files (no output). Not run here: the
full gui suite, the full tagged suite and oraclelive (the build owes them, §8).
