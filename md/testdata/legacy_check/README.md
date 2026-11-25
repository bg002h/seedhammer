# md/testdata/legacy_check — the legacy `Check(PkK)` / `Check(PkH)` wire

Bug hunt 2026-11-25, SH#1. mnemonic-toolkit before 0.55.0 (2026-06-12) and
md-cli before 0.30 wrote every `pk()`/`pkh()` as `Tag::Check` over a bare
`PkK`/`PkH` (the bare tag already means `c:pk_k`/`c:pk_h`), and such cards are
already engraved. The Rust primary renders `Check` over a bare key as the bare
key (md-codec `to_miniscript.rs`, "Check-idempotence"); these cards pin this
port to the same addresses.

`legacy_<vector>.md1.txt` is the corpus vector `<vector>` (`../vectors/` or
`../forkbuilt/`) re-encoded by this port with every `PkK`/`PkH` node wrapped in
`Check` — the shape the old encoders wrote. `md/legacy_check_test.go`
regenerates each one and requires the bytes to be identical, so the files cannot
drift from the vectors they came from.

`legacy_bughunt_sh1_or_d.md1.txt` is the bug report's own card B for
`wsh(or_d(pk(@0),and_v(v:pk(@1),older(144))))` (copied from the report).

Rust verdicts, taken 2026-11-25 with `md` 0.21.1 (md-codec 0.48.7) built from
descriptor-mnemonic `c8cde234`: `md decode` renders each card as its vector's
template, and `md address <card> --count 2 [--chain 1]` gives exactly the
vector's recorded addresses. Those Rust addresses are pinned in
`gui/legacy_check_address_test.go`, which drives the device's own address route.
