# Seed XOR test vectors — provenance

`vectors.json` holds Coldcard-interop Seed XOR combine vectors used as the
cross-check for `seedxor.Combine`. The independent correctness anchor is the
`mnemonic-toolkit` G1 byte-pin / G2 round-trip relations (pure byte-XOR), which
`seedxor_test.go` re-derives directly (order-independence + round-trip); these
JSON vectors pin the same arithmetic against an external, authoritative source.

## Coldcard interop vectors

Source: Coldcard firmware `docs/seed-xor.md` (worked examples), fetched verbatim
from
<https://raw.githubusercontent.com/Coldcard/firmware/master/docs/seed-xor.md>
on 2026-06-18.

- `coldcard-24word-3part` — the published 24-word, 3-part example. The three
  parts XOR (on their BIP-39 entropy) to the combined phrase ending in
  `... primary indoor`.
- `coldcard-12word-3part` — the published 12-word, 3-part example, combining to
  the phrase ending in `... real trade`.

Each part and result is a checksum-valid BIP-39 mnemonic. Verified in-repo: the
bytewise XOR of the parts' `bip39.Entropy()` equals the result's entropy, and
`bip39.New(combined-entropy)` reproduces the result phrase exactly.

Seed XOR algorithm reference (Coldcard): bytewise XOR of the parts' raw BIP-39
entropy (the checksum word is recomputed on the result, not XORed). Coldcard's
deterministic SPLIT uses a `Batshitoshi`-tagged SHA256d; COMBINE — the only
direction this firmware implements — needs none of that, just XOR.
