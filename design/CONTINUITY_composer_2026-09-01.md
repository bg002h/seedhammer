
## S2 review r0 folded (2026-09-02)

- `composer-S2-exec-review-r0` (opus, 1C/1I/4M/1N) persisted bce5ba1. Both
  blocking findings were host<->device lockstep breaks in the key: path
  grammar. Folded RUST FIRST: engrave master c05074f1 (host refuses `+`-signed
  components; fixture 47 rows, sha 5b3960ca…; CHANGELOG [Unreleased] -- this
  is a post-0.8.0 host change, so me 0.8.1 is owed at some point); fork
  `composer-s2` fold commit (the range check for indices >= 2^31, the
  re-vendored 47-row fixture with provenance = c05074f1, M-1..M-4, N-1). Plan
  record a8bf04e. Gates green on both sides (gui 1059/1059).
- NEXT: sonnet fold verification (`composer-S2-exec-review-r1-fold-verification.md`)
  → merge `composer-s2` --no-ff into fork main (brief: scratchpad/push-brief-s2.md,
  message scratchpad/s2-merge.msg) → push, watch test.yml.
