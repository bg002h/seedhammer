# md/testdata/vectors — vendored md-codec golden vectors

These are byte-exact cross-language golden vectors for the md1 encoder,
copied verbatim from the constellation source of truth:

- Source crate: `descriptor-mnemonic/crates/md-codec`
- Commit: `5a0a4f41` (md-codec v0.42.0, tag `md-cli-v0.13.0`)
- Path: `tests/vectors/<name>.{bytes.hex,phrase.txt,descriptor.json}`

Each vector has three files:

- `<name>.bytes.hex` — the exact `encode_payload` byte output (hex), the
  PRIMARY byte-parity gate for `encodePayload`.
- `<name>.phrase.txt` — the full md1 string. For single-string vectors this
  is the `encode_md1_string` output (the `encodeMD1String == .phrase.txt`
  gate). For the force-chunked vector (`wsh_multi_chunked`) it is a
  multi-chunk-format string prefixed with a `chunk-set-id:` header line, so
  it is EXCLUDED from the single-string parity table (R0-M3).
- `<name>.descriptor.json` — the AST used to build the Go `descriptor` input.

The original 10-entry MANIFEST (md-codec `src/test_vectors.rs`), including the
force-chunked `wsh_multi_chunked`, is vendored here. `chunked_md1_vector` (the
≥4-chunk 6-key wsh-sortedmulti from `me-cli/src/bundle.rs:547-585`) is NOT a
copyable file — it is hand-built in `chunk_test.go`.

Re-pinned 2026-08-14 (S0 D8, 0.36.0 → 0.42.0): all 30 files across these 10
vectors are byte-identical between the two commits — `cmp` confirms zero
drift in `.bytes.hex`, `.descriptor.json`, and `.phrase.txt` for every
vector. As of the new commit, `MANIFEST` in the primary is a canonical
**15-entry** corpus: the original 10 plus five Part-3 additions.

**All five additions are now vendored, and four of them are exercised.** D8 is
a *coverage* catch-up — the plan's own words are that the vendored set was "an
older, **smaller** sample, and a gate accepting them would prove agreement with
a subset of ourselves" — so re-pinning the provenance strings while leaving the
new vectors out would have delivered none of it. Note that
`md/testdata_test.go` enumerates vectors from a **hand-maintained list**, not a
directory scan, so copying files in without adding their names is a silent
no-op. Compare that list against the primary's `MANIFEST` on every re-pin.

| vector | state |
| --- | --- |
| `tr_with_leaf`, `nums_taproot`, `single_string_boundary` | exercised, single-string + byte parity |
| `wsh_sortedmulti_2chunk` | exercised, byte parity only — its `.phrase.txt` is a 3-line chunk-format string with a `chunk-set-id:` header, like `wsh_multi_chunked` |
| `sh_wpkh` | **vendored but NOT exercised.** This package refuses it: `md: missing explicit origin` (`md/md.go:893`), because its descriptor carries a pathless shared origin, `"path_decl":{"tag":"Shared","data":"m"}`. The primary gained pathless decode in the very release this re-pin points at. **The Go port is behind the Rust primary** — convergence work, tracked as F-166. Reproduce in one line: add `"sh_wpkh"` to `singleStringVectorNames` and run `go test ./md/`. |
