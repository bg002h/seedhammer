# `seal/testdata` — provenance

## `vectors.json`

**Generated. Never hand-edited.**

The canonical `MNEMBLOB` test vectors (SPEC §11.4, vectors A–G). They are
produced by the **normative Rust implementation** in `mnemonic-engrave` and
copied here verbatim; this package's tests bind to them byte-for-byte.

| | |
| --- | --- |
| Source repo | `mnemonic-engrave` |
| Source commit | `4df43d6` on `feat/emit-seal-vectors` |
| Source path | `crates/me-cli/testdata/seal_vectors.json` |
| Generating command | `ME_EMIT_VECTORS=1 cargo test -p mnemonic-engrave --lib seal::tests::emit_vectors` |
| sha256 | `333ac47e7f61d031c995b85510565bfffd86cd1992f09b0230c1484fffd4d4bc` |

### Why it is not hand-edited

Per the project's Rust-primary rule, `crates/me-cli/src/seal/` is normative and
this Go package is a behaviour-faithful port that may never lead. If a Go test
disagrees with a value in this file, **the Go code is wrong** until proven
otherwise. The fix is never to edit the JSON.

A change to normative wire behaviour lands in Rust first, with a vector; the
exporter is then re-run, this file is re-copied, and the table above is updated.
The exporter also asserts the committed Rust-side copy still matches on every
`cargo test`, so drift in either direction fails that suite.

### What the vectors carry

Per vector: the inputs (`passphrase`, `iterations`, `salt_hex`, `iv_hex`, and
the `public`/`secret` record lists), the section lengths (`pub_len`, `ct_len`)
**as declared by the sealing header**, and the outputs (`blob_hex`,
`blob_sha256`, `header_hex`, `derived_key_hex`, `tag_hex`, `pubhash_sealed`,
`pubhash_unsealed`).

`pub_len`/`ct_len` come from the `Header` the exporter built from the encoded
sections — they are **not** re-read from `header_hex` or `blob_hex`. That is
what makes `TestParseHeaderMatchesTheVectors` a real check rather than a
tautology.

`passphrase`, `derived_key_hex` and `tag_hex` are `null` for vector E, the
unsealed shape: it has no key and no tag. `pubhash_sealed`/`pubhash_unsealed`
are `null` whenever `pub_len == 0` (§10.2 step 3 displays nothing then).
