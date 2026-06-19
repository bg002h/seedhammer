# md/testdata/vectors — vendored md-codec golden vectors

These are byte-exact cross-language golden vectors for the md1 encoder,
copied verbatim from the constellation source of truth:

- Source crate: `descriptor-mnemonic/crates/md-codec`
- Commit: `c85cd49` (md-codec v0.36.0)
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

The 10-entry MANIFEST (md-codec `src/test_vectors.rs`) plus the force-chunked
`wsh_multi_chunked` are vendored here. `chunked_md1_vector` (the ≥4-chunk
6-key wsh-sortedmulti from `me-cli/src/bundle.rs:547-585`) is NOT a copyable
file — it is hand-built in `chunk_test.go`.
