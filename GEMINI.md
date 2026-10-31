# GEMINI.md — SeedHammer II (`SH2`) Firmware (`bg002h/seedhammer`)

## 1. Project Purpose & Repository Boundaries

- **`seedhammer` (`bg002h/seedhammer`):** Maintained fork of SeedHammer firmware targeting the **SeedHammer II (`SH2`)** controller (Raspberry Pi RP2350, TinyGo target `pico-plus2`). Upstream is `seedhammer/seedhammer` (`upstream/main`).
- **`../mnemonic-engrave` (`bg002h/mnemonic-engrave`):** Companion repository housing the `me` host CLI, `seal`/`sysw` primary Rust crates, flashing/OTP scripts (`scripts/`), and all firmware design documents (`design/`).
  - Note: `../mnemonic-engrave/third_party/seedhammer` is a pinned fork submodule (`7b6f2fbb`, lagging `origin/main` by 24 commits) for `me-preview`, **not** the active firmware checkout.
- **Constellation boundary:** Primary Rust codecs live in `../descriptor-mnemonic` (`md1`), `../mnemonic-key` (`mk1`), `../mnemonic-secret` (`ms1`/hashlock), and `../mnemonic-transaction` (`mt1`). Host desktop/appliance UIs live in `../mnemonic-toolkit`, `../mnemonic-gui`, and `../mnemonic-refugium`.

## 2. Canonical Documentation & Architecture References

- **Firmware Architecture Guide:** [`../mnemonic-engrave/design/firmware-architecture/ARCHITECTURE.md`](../mnemonic-engrave/design/firmware-architecture/ARCHITECTURE.md)
- **Source & Provenance Map:** [`../mnemonic-engrave/design/firmware-architecture/SOURCE_MAP.md`](../mnemonic-engrave/design/firmware-architecture/SOURCE_MAP.md)
- **Host-to-Device Interfaces:** [`../mnemonic-engrave/design/firmware-architecture/HOST_INTERFACES.md`](../mnemonic-engrave/design/firmware-architecture/HOST_INTERFACES.md)
- **Open Questions & Discrepancies:** [`../mnemonic-engrave/design/firmware-architecture/OPEN_QUESTIONS.md`](../mnemonic-engrave/design/firmware-architecture/OPEN_QUESTIONS.md) and [`../mnemonic-engrave/design/FOLLOWUPS.md`](../mnemonic-engrave/design/FOLLOWUPS.md)
- **Flashing, Signing & OTP Runbooks:** [`docs/custom-firmware.md`](docs/custom-firmware.md), [`../mnemonic-engrave/FIRMWARE-QUICKSTART.txt`](../mnemonic-engrave/FIRMWARE-QUICKSTART.txt), and [`../mnemonic-engrave/design/PICOTOOL_PIN.md`](../mnemonic-engrave/design/PICOTOOL_PIN.md)
- **Licensing & Font Provenance:** [`PROVENANCE.md`](PROVENANCE.md) (`font/constant` and `font/sh` are Unlicense public-domain single-stroke engraving fonts; `font/comfortaa` and `font/poppins` are SIL OFL 1.1 screen-only fonts and must never be imported into `backup/` or `engrave/`).

## 3. Rust-Primary vs. Go-Port Rule

- All Shibboleth constellation codecs in this repo (`codex32/`, `md/`, `mk/`, `mt/`, `hashlock/`, `seal/`, `sysw/`) are **downstream ports** of their primary Rust crates.
- Never invent or alter wire formats, checksums, or classification rules solely in Go. Changes must land in the Rust primary first, emit updated test vectors, and then update the Go package alongside its `testdata/*.provenance.json` pin.
- Do not rely on Go-only round-trip tests to claim cross-language conformance; verify against vendored Rust vectors and `./scripts/oracle-live.sh`.

## 4. Verified Build, Test, TinyGo & Emulator Commands

```bash
# Host unit tests (non-GUI packages)
go test ./address/... ./backup/... ./bc/... ./bip32/... ./bip380/... ./bip39/... ./bip85/... \
  ./codex32/... ./engrave/... ./hashlock/... ./md/... ./mk/... ./mt/... ./passphrase/... \
  ./picobin/... ./seal/... ./seedqr/... ./seedxor/... ./slip39/... ./stepper/... ./sysw/... ./txqr/... ./uf2/...

# Sharded GUI test suite (avoids single-binary timeout) & Refugium profile tests
../mnemonic-engrave/scripts/gui-shard-test.sh
./scripts/refugium-tagged-test.sh

# 32-bit int-width gate (matches Cortex-M33) & live Rust oracle differential gate
./scripts/test-32bit.sh
./scripts/oracle-live.sh

# TinyGo RP2350 firmware build & UF2 seal (run inside `nix develop`)
tinygo build -size short -stack-size 16kb -gc precise -opt 2 -scheduler tasks -target pico-plus2 \
  -ldflags="-X main.Version=v0.0.0-bg$(git rev-parse --short=8 HEAD)" -o build/controller.elf ./cmd/controller
picotool seal --quiet --hash --sign --clear build/controller.elf build/controller.uf2 <key.pem> --no-overwrite

# Refugium profile TinyGo build + forbidden-symbol ELF gate
tinygo build -tags refugium -size short -stack-size 16kb -gc precise -opt 2 -scheduler tasks -target pico-plus2 \
  -ldflags="-X main.Version=v0.0.0-bg$(git rev-parse --short=8 HEAD)" -o build/controller-refugium.elf ./cmd/controller
./scripts/refugium-elf-check.sh build/controller-refugium.elf

# Browser WASM emulator & headless SVG/PNG plate preview
./cmd/emu/build.sh && python3 -m http.server -d cmd/emu 8080
go run ./cmd/plateview -out build/preview
```

## 5. Firmware Safety & Hardware Guardrails

- **Authorized scope:** Never trigger physical stepper motion, flash a device (`picotool load`), program OTP fuses (`picotool otp` / `LockBoot`), or sign/publish releases without explicit user instruction for that operation.
- **Touch-only hardware (`SH2`):** Physical units have no directional buttons (`ft6x36` touch only). Bind interactive controls with `Clickable`, not `ButtonFilter(Up/Down)` alone.
- **Screen & engraving fonts:** Keep all GUI strings printable ASCII (`gui/font_coverage_test.go`). Keep `.bin` spline goldens stable unless intentionally changing plate geometry.
- **Secret scrubbing:** Zero mutable secret `[]byte` and `ExtendedKey` buffers (`clear(...)`, `mk.Zero()`, `ctx.B.Scrub()`, `WipeSecretAt`) via `defer` on all return paths; preserve scrubbing test hooks.
- **Power-gate boot behavior:** `cmd/controller/platform_sh2.go` reboots into BOOTSEL before LCD init if 20–28 V USB-PD (`driver/ap33772s`) is absent (e.g., on laptop USB).
- **Formatting baseline:** 5 files (`gui/transaction.go`, `gui/transaction_golden_test.go`, `gui/transaction_txrecord_test.go`, `mt/mt.go`, `mt/mt_test.go`) have pre-existing unformatted lines (`F-499`); format only your touched hunks.

## 6. Git, Worktree, Signing & Workspace Hygiene

- **Multi-agent workspace:** Other agents may be active across `/scratch/code/shibboleth`. Inspect `git status` and `git branch` before editing; use isolated worktrees in `/scratch/code/shibboleth/sh-worktrees/<topic>` (never build or create worktrees in `/tmp`, where `tmpfs` exhausts quota).
- **Commits & pushes:** Stage explicit file paths only (`git add <file>`). Sign all commits with SSH key `~/.ssh/id_ed25519` and DCO sign-off (`git commit -S -s`). Push to `origin` (`bg002h/seedhammer`), or via `../mnemonic-engrave/scripts/push-via-staging.sh` when pushing from restricted subagents.

## 7. Reconciled Workflow Guidance

- **Direct implementation by default:** `../mnemonic-engrave/CLAUDE.md` (2026-07-26) supersedes `seedhammer/CLAUDE.md` (2026-06-17): implement docs, tests, mechanical ports, and localized UI changes directly with focused verification. Reserve heavy multi-agent review (`/ultracode` or subagent audits) for high-risk changes: crypto/secret scrubbing, `seal`/`sysw` wire parsers, OTP/bootrom/flash layout, stepper motion safety, and cross-repo codec changes.
