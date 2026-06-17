# CLAUDE.md — bg002h/seedhammer fork notes

This file is auto-loaded by Claude Code when starting a session in this repository. It is a fork-local working note for our maintained fork of `seedhammer/seedhammer`. It lives on `main`; feature/PR branches branch off `upstream/main`, so it does not appear in upstream PR diffs.

## What this is

Our public-domain (Unlicense) fork of the SeedHammer II firmware. `main` tracks upstream `main` plus our additive features (on-device CODEX32 seed entry; `md1`/`mk1` BCH-validated engraving) — see the README "About this fork". Firmware planning/design docs and architect reviews live in the sibling `mnemonic-engrave/design/`; this tree is kept close to upstream for clean, reviewable PRs.

## Conventions

- **Default to ultracode (multi-agent workflow orchestration).** Standing user directive (2026-06-17), project-wide across the m-format constellation and the seedhammer fork — does NOT require the per-turn `ultracode` keyword. For any *substantial* task (research, design, implementation, review — anything beyond a conversational answer or a trivial mechanical edit), author and run a **Workflow** by default: decompose, fan out parallel subagents, **adversarially verify** findings before relying on them, and prefer exhaustiveness over token thrift; chain several workflows for multi-phase work. Token cost is not a constraint. Plain Q&A and trivial one-line edits still run solo. Composes with — does not replace — the R0 gate.
- **R0 gate before code:** brainstorm specs and implementation plans pass an opus architect R0 review and converge to 0 Critical / 0 Important before implementation; planning docs + reviews persist in `mnemonic-engrave/design/` (this tree kept clean).
- Upstream PRs branch off `upstream/main`; commits signed + DCO, authored Brian Goss; keep PRs small and focused. Host-test `gui`/`codex32`/`bip39` with `go test`; full firmware build is TinyGo/Nix.
