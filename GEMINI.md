# GEMINI.md — SeedHammer II (`SH2`) Firmware (`bg002h/seedhammer`)

## 1. Instruction Loading & Repository Boundaries

- **Load ancestor and companion instructions explicitly:** Reading `GEMINI.md` does not auto-load `CLAUDE.md` or `AGENTS.md`. Before starting non-trivial work, read [CLAUDE.md](CLAUDE.md), `/scratch/code/CLAUDE.md` (when present in this workspace), and the companion `mnemonic-engrave` `CLAUDE.md` (at `/scratch/code/shibboleth/mnemonic-engrave/CLAUDE.md` or [on GitHub](https://github.com/bg002h/mnemonic-engrave/blob/a75719996304086e5f293aaf26812ff242a25244/CLAUDE.md)).
- **`seedhammer` (`bg002h/seedhammer`):** Maintained fork of SeedHammer firmware targeting the **SeedHammer II (`SH2`)** controller (Raspberry Pi RP2350, TinyGo target `pico-plus2`). Upstream is `seedhammer/seedhammer` (`upstream/main`).
- **`mnemonic-engrave` (`bg002h/mnemonic-engrave`):** Companion repository housing the `me` host CLI (`crates/me-cli`), `mnemonic-io-lib`, `seal`/`sysw` primary Rust implementations, flashing/OTP scripts (`scripts/`), and firmware design documents (`design/`).
  - Note: `mnemonic-engrave/third_party/seedhammer` is a pinned fork submodule (`7b6f2fbb`, lagging `seedhammer` `origin/main` `abdcb182` by 24 commits) used by the `me-preview` Go sidecar, **not** the active firmware checkout.
- **Constellation & Refugium boundaries:** Primary Rust codecs live in `descriptor-mnemonic` (`md-codec`, CLI `md`), `mnemonic-key` (`mk-codec`, CLI `mk`), `mnemonic-secret` (`ms-codec`, CLI `ms`), and `mnemonic-transaction` (`mt-codec`, CLI `mt`). Host bundle/word-card workflows live in `mnemonic-toolkit` (CLI `mnemonic`, library `wc-codec`), the `egui` utility overlay lives in `mnemonic-gui` (binaries `mnemonic-gui`, `gui-render`), the Refugium CLI/library lives in `mnemonic-refugium` (CLI `mr`), and the Slint Refugium GUI lives in `refugium-wallet` (`mr-gui`).

## 2. Canonical Documentation & Architecture References

In `/scratch/code/shibboleth`, the companion `mnemonic-engrave` checkout is at `/scratch/code/shibboleth/mnemonic-engrave` (and while the firmware-architecture branch is under review, in `/scratch/code/shibboleth/me-worktrees/firmware-architecture`):

- **Firmware Architecture Guide:** `mnemonic-engrave/design/firmware-architecture/ARCHITECTURE.md` ([GitHub branch](https://github.com/bg002h/mnemonic-engrave/blob/docs/firmware-architecture/design/firmware-architecture/ARCHITECTURE.md))
- **Source & Provenance Map:** `mnemonic-engrave/design/firmware-architecture/SOURCE_MAP.md` ([GitHub branch](https://github.com/bg002h/mnemonic-engrave/blob/docs/firmware-architecture/design/firmware-architecture/SOURCE_MAP.md))
- **Host-to-Device Interfaces:** `mnemonic-engrave/design/firmware-architecture/HOST_INTERFACES.md` ([GitHub branch](https://github.com/bg002h/mnemonic-engrave/blob/docs/firmware-architecture/design/firmware-architecture/HOST_INTERFACES.md))
- **Open Questions & Discrepancies:** `mnemonic-engrave/design/firmware-architecture/OPEN_QUESTIONS.md` ([GitHub branch](https://github.com/bg002h/mnemonic-engrave/blob/docs/firmware-architecture/design/firmware-architecture/OPEN_QUESTIONS.md)) and `mnemonic-engrave/design/FOLLOWUPS.md` ([GitHub baseline](https://github.com/bg002h/mnemonic-engrave/blob/a75719996304086e5f293aaf26812ff242a25244/design/FOLLOWUPS.md))
- **Flashing, Signing & OTP Runbooks:** [docs/custom-firmware.md](docs/custom-firmware.md), `mnemonic-engrave/FIRMWARE-QUICKSTART.txt` ([GitHub baseline](https://github.com/bg002h/mnemonic-engrave/blob/a75719996304086e5f293aaf26812ff242a25244/FIRMWARE-QUICKSTART.txt)), and `mnemonic-engrave/design/PICOTOOL_PIN.md` ([GitHub baseline](https://github.com/bg002h/mnemonic-engrave/blob/a75719996304086e5f293aaf26812ff242a25244/design/PICOTOOL_PIN.md))
- **Licensing & Font Provenance:** [PROVENANCE.md](PROVENANCE.md) (`font/constant` and `font/sh` are Unlicense public-domain single-stroke engraving fonts; `font/comfortaa` and `font/poppins` are SIL OFL 1.1 screen-only fonts and must never be imported into `backup/` or `engrave/`).

## 3. Rust-Primary vs. Go-Port Rule

- Constellation codecs and containers in this repo (`codex32/`, `md/`, `mk/`, `mt/`, `hashlock/`, `seal/`, `sysw/`) are **downstream ports** of their primary Rust crates (`mnemonic-engrave/CLAUDE.md:28`).
- **Normative changes** (wire format, identity/stub algorithms, validation, admission) must land **first in the primary Rust repository with test vectors**, and only then be ported to Go alongside updated `testdata/*.provenance.json` pins.
- **Permitted exceptions:** (a) **Go convergence fixes** that bring a Go port into conformance with already-correct Rust — provided you **always check the primary Rust implementation first** to confirm the defect does not exist in Rust (if it does, fix Rust first with a vector); (b) **fork-native firmware/GUI/UX/driver code** with no Rust counterpart.
- Do not rely on Go-only round-trip tests to establish cross-language conformance; verify against vendored Rust vectors and `./scripts/oracle-live.sh`.

## 4. Verified Build, Test, TinyGo & Emulator Commands

Host Go tests require **Go 1.26** (`/scratch/code/shibboleth/.toolchain/go/bin/go` or inside `nix develop`), while `go.mod` retains `go 1.25.10` for TinyGo `0.41.1` compatibility (`F-494`).

```bash
# Host unit tests (non-GUI packages; use Go 1.26)
export PATH="/scratch/code/shibboleth/.toolchain/go/bin:/nix/var/nix/profiles/default/bin:$PATH"
go test ./address/... ./backup/... ./bc/... ./bip32/... ./bip380/... ./bip39/... ./bip85/... \
  ./codex32/... ./engrave/... ./hashlock/... ./md/... ./mk/... ./mt/... ./nfc/... ./passphrase/... \
  ./picobin/... ./seal/... ./seedqr/... ./seedxor/... ./slip39/... ./stepper/... ./sysw/... ./txqr/... ./uf2/...

# Sharded GUI test suite (24 shards; asserts exhaustive test-list partition) & Refugium profile tests
/scratch/code/shibboleth/mnemonic-engrave/scripts/gui-shard-test.sh ./gui/ 24
./scripts/refugium-tagged-test.sh

# 32-bit int-width gate (matches Cortex-M33) & live Rust oracle differential gate
./scripts/test-32bit.sh
./scripts/oracle-live.sh

# TinyGo RP2350 unsigned firmware build (via flake build-firmware or inside `nix develop`)
nix run .#build-firmware
# Equivalent manual steps inside `nix develop` (seals load-map Clear 0x20000000->0x20082000, then zeroes dummy sig):
tinygo build -o build/firmware.uf2 -ldflags="-X main.Version=v0.0.0-bg$(git rev-parse --short HEAD)" \
  -target pico-plus2 -stack-size 16kb -gc precise -opt 2 -scheduler tasks ./cmd/controller
picotool seal --sign --clear --quiet build/firmware.uf2 build/firmware.signed.uf2 "$DUMMY_PEM"
go run ./cmd/picosign sign -clear build/firmware.signed.uf2

# Refugium profile TinyGo ELF build + forbidden-symbol ELF gate (inside `nix develop`)
tinygo build -tags refugium -size short -stack-size 16kb -gc precise -opt 2 -scheduler tasks -target pico-plus2 \
  -ldflags="-X main.Version=v0.0.0-bg$(git rev-parse --short HEAD)" -o build/controller-refugium.elf ./cmd/controller
./scripts/refugium-elf-check.sh build/controller-refugium.elf

# Browser WASM emulator (outputs cmd/emu/emu.wasm) & headless SVG/PNG plate preview
./cmd/emu/build.sh && python3 -m http.server -d cmd/emu 8080
go run ./cmd/plateview -plate bothproof -o build/preview.svg
```

## 5. Firmware Safety & Hardware Guardrails

- **Authorized scope:** Never trigger physical stepper motion, flash a device (`picotool load`), program OTP fuses (`picotool otp` / `LockBoot`), or sign/publish releases without explicit user instruction for that operation.
- **Automatic startup homing & USB-PD power gate (`cmd/controller/platform_sh2.go:267–282, 459–479`):** On boot, `Init()` negotiates 20–28 V USB-PD (`adjustSupplyVoltage`: 3 retries, 100 ms settle + 500 ms retry sleep). If 20–28 V is absent (e.g., on laptop USB), it calls `rebootIntoBOOTSEL()` before LCD init. **If 20–28 V is present, `Init()` immediately spawns `go home()` before LCD/touch init**, driving X/Y axes to endstops (`-homingDist`) and moving the needle to `(originX, originY)` concurrently with display startup and without GUI confirmation.
- **Touch-only hardware (`SH2`):** Physical units have no directional buttons (`ft6x36` touch only). Bind interactive controls with `Clickable`, not `ButtonFilter(Up/Down)` alone.
- **Screen & engraving fonts:** Keep all GUI strings printable ASCII (`gui/font_coverage_test.go`). Keep `.bin` spline goldens stable unless intentionally changing plate geometry.
- **Secret scrubbing:** Zero mutable secret `[]byte` and `ExtendedKey` buffers (`clear(...)`, `mk.Zero()`, `ctx.B.Scrub()`, `WipeSecretAt`) via `defer` on all return paths; preserve scrubbing test hooks.
- **Formatting baseline:** 5 files (`gui/transaction.go`, `gui/transaction_golden_test.go`, `gui/transaction_txrecord_test.go`, `mt/mt.go`, `mt/mt_test.go`) have pre-existing unformatted lines (`F-499`); diff `gofmt -l .` against this 5-file set rather than running repo-wide `gofmt -w .`.

## 6. Git, Worktree, Signing & Workflow Gates

- **Multi-agent workspace:** Inspect `git status` and `git branch` before editing; use isolated worktrees in `/scratch/code/shibboleth/sh-worktrees/<topic>` (never build or create worktrees in `/tmp`, which is a 32 GB `tmpfs`). Stage explicit paths only (`git add <file>`). For upstream PRs or signed branches, sign commits (`-S -s`) with no `Co-Authored-By:` trailers; push `main` via `/scratch/code/shibboleth/mnemonic-engrave/scripts/push-via-staging.sh`.
- **Risk-scoped review & gates (`/scratch/code/CLAUDE.md`, `mnemonic-engrave/CLAUDE.md:11–26`):**
  - **RISK SET (orchestrate + mandatory `0C/0I` gates):** (a) **irreversible or physically destructive actions** — OTP/fuse writes, firmware signing/flashing, tags/releases; (b) anything touching **funds, seeds, keys, or addresses**; (c) changes to **normative codec behavior** (wire format, identity/stub algorithms, validation, admission); (d) work spanning **many files or repos** where no single context holds the whole picture.
  - **Mandatory gates for risk-set work:** Spec/plan must converge to **0 Critical / 0 Important (`0C/0I`)** in independent R0 review before coding; implement test-first (TDD); persist every reviewer report verbatim in `design/agent-reports/` in its **own commit before the fold commit**; run the build/test gate on every fold; run a mandatory post-implementation independent whole-diff review to `0C/0I`. (Per operator ruling 2026-08-27, secret-handling leak findings are logged as non-gating follow-ups, whereas wrong results, data loss, funds-safety regressions, unmet guarantees, and false-PASS gates block.)
  - **Non-risk-set work runs solo:** Refactors, merges/upstream syncs, build/toolchain/environment work, docs, CI, debugging, and Q&A run solo and verify inline.
