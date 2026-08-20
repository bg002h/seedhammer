# CLAUDE.md — bg002h/seedhammer fork notes

This file is auto-loaded by Claude Code when starting a session in this repository. It is a fork-local working note for our maintained fork of `seedhammer/seedhammer`. It lives on `main`; feature/PR branches branch off `upstream/main`, so it does not appear in upstream PR diffs.

## What this is

Our public-domain (Unlicense) fork of the SeedHammer II firmware. `main` tracks upstream `main` plus our additive features (on-device CODEX32 seed entry; `md1`/`mk1` BCH-validated engraving) — see the README "About this fork". Firmware planning/design docs and architect reviews live in the sibling `mnemonic-engrave/design/`; this tree is kept close to upstream for clean, reviewable PRs.

## Conventions

- **Default to ultracode (multi-agent orchestration) — refined policy** (2026-06-17, after an architect panel; verdict: keep default-ON, refine per-phase). Standing user directive, project-wide across the m-format constellation + seedhammer fork; does NOT require the per-turn `ultracode` keyword. **Default ON for every *substantial* task; token cost is not a constraint.** Trivial one-line/mechanical edits, version bumps, and plain Q&A run solo. **Per-phase pattern:** (1) **research/recon** — fan out parallel subagents; any agent handling **external protocol facts** (BIP-39, BCH/codec semantics, NDEF, RP2350 OTP, SDK behavior) MUST verify them against **authoritative source text**, not just the draft doc (guards against false-consensus on plausible-but-wrong facts — the "1 valid last word" class). (2) **design/spec/plan** — single author + the mandatory R0 loop. (3) **implementation** — a *single* subagent executes the GREEN plan in a worktree (NOT parallel re-implementations); TDD. (4) **post-implementation** — a **mandatory, non-deferrable** independent adversarial execution review over the whole diff (R0 = plan correctness; this catches implementation-introduced regressions TDD misses). (5) if Agent-API dispatch fails mid-session, **flag it explicitly** and defer the formal review to API recovery — never silently substitute inline self-review. Composes with — does not replace — the R0 gate; verbatim agent reports persist to `design/agent-reports/`.
- **R0 gate before code:** brainstorm specs and implementation plans pass an opus architect R0 review and converge to 0 Critical / 0 Important before implementation; planning docs + reviews persist in `mnemonic-engrave/design/` (this tree kept clean).
- Upstream PRs branch off `upstream/main`; commits signed + DCO, authored Brian Goss; keep PRs small and focused. Host-test `gui`/`codex32`/`bip39` with `go test`; full firmware build is TinyGo/Nix.

## Parallel execution — this machine has 24 CPU cores

**Standing directive (2026-08-19): consider parallel execution for ALL tests,
cache generation and long calculations.** The defaults use almost none of the
box. Measured constellation-wide the same day: **824s → 204s (~4×)**.

- **Rust — `cargo nextest run --locked`**, not `cargo test`. `cargo test` runs
  each test *binary* serially; nextest spreads them over all cores. Per-repo
  measurements: mnemonic-toolkit 256s→49s, descriptor-mnemonic 40s→27s,
  mnemonic-engrave 33s→16s, mnemonic-secret 2s→0.3s. `cargo-nextest` 0.9.140 is
  installed.
- **Go — shard the package.** `-parallel` does NOTHING unless tests call
  `t.Parallel()`; the fork's `gui` package has 886 test funcs and zero of them.
  `mnemonic-engrave/scripts/gui-shard-test.sh <pkg> 24` took `./gui/` from 493s
  to 112s. It enumerates its partition from `go test -list` and **asserts the
  union is exhaustive before running**, so it cannot silently drop a test — any
  replacement must do the same.
- **Long independent work** — cache/corpus generation, fixture derivation, batch
  rendering — is a candidate too. Ask whether it is CPU-bound and independent
  before running it in a loop.

**Speed WITHOUT dropping debug_assertions.** Do NOT reach for `--release` to
speed tests up — it drops `debug_assertions` and overflow checks, so mutation
tests and invariant panics stop detecting things while still reporting green.
Raise the optimisation level instead and keep them:

```toml
[profile.test]
opt-level = 2

[profile.dev]
opt-level = 2
```

`debug-assertions` defaults to **true** on both profiles, so this is pure gain.
Measured on descriptor-mnemonic: execution **25.4s → 0.765s**, versus 0.775s for
`--release` — the same speed, with the checks intact. Verified empirically, not
inferred: at `opt-level = 2` both `cfg!(debug_assertions)` and an
`attempt to add with overflow` panic still fire. Cost is a slower first build of
dependencies, cached thereafter.

**Check what `/tmp` is before building there.** On this box it is a 32 GB tmpfs,
and a scratch worktree's `target/` filled it and killed a running test.

**Never run the same suite twice** to collect counts and failures separately.
Capture once to a file, then grep it — otherwise every measurement costs double.
