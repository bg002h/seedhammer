#!/usr/bin/env bash
#
# Run the word-size-sensitive packages at the DEVICE's word size.
#
# WHY THIS EXISTS. `int` is 64 bits on every builder and 32 bits on the
# SeedHammer II (Cortex-M33 via tinygo). A uint32 read off the wire and widened
# to `int` therefore behaves differently on the machine than in CI -- and it did:
# ParseHeader compared the section cap as `int(h.PubLen)`, so pub_len=0xFFFFFFFF
# became -1, slipped the cap, and produced TotalLen()==67 -- a plausible SMALL
# POSITIVE length. The device would have accepted a payload the host calls
# malformed. That is fixed; F-142 is about the blind spot that let it ship.
#
# The shared JSON vectors could never have caught it. They are evaluated at the
# BUILDER's word size, so no vector can observe a 32-bit wrap, however many
# vectors are added.
#
# CGO_ENABLED=0 is required, not cosmetic: with cgo on, `runtime/cgo` wants
# 32-bit glibc headers (gnu/stubs-32.h) that a 64-bit nix devshell does not
# ship. These packages are pure Go, so turning cgo off costs nothing here.
#
# PROVEN TO BITE, 2026-08-12: with the original `int(...)` comparison restored,
# the amd64 run passes (exit 0) and this script FAILS (exit 1). A gate that
# cannot fail is decoration; this one was checked against the real defect before
# being committed.
set -uo pipefail

PKGS=("${@:-./sysw/}")
fail=0

# 386 both builds AND runs here, which is what makes the assertion real.
CGO_ENABLED=0 GOARCH=386 go test "${PKGS[@]}"
rc=$?
echo "GOARCH=386 test:  exit $rc"
[ $rc -eq 0 ] || fail=1

# arm cannot RUN on this host (exec format error), so it is a BUILD check only.
# Kept because it is the device's actual architecture: a compile error that only
# 32-bit ARM produces would otherwise reach the machine.
CGO_ENABLED=0 GOARCH=arm go build "${PKGS[@]}"
rc=$?
echo "GOARCH=arm build: exit $rc"
[ $rc -eq 0 ] || fail=1

exit $fail
