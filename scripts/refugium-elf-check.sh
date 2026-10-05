#!/usr/bin/env bash
# refugium-elf-check.sh -- the device half of the Refugium build's absence gate.
#
#   scripts/refugium-elf-check.sh DEFAULT.elf REFUGIUM.elf
#
# Refugium plan F7 §4.5. Both ELFs are TinyGo builds of ./cmd/controller with
# the flake's flags (-target pico-plus2 -stack-size 16kb -gc precise -opt 2
# -scheduler tasks), the second with -tags refugium. The check:
#
#   every marker is PRESENT in DEFAULT.elf   (positive control: the scan can
#                                             see the thing it is looking for)
#   every marker is ABSENT  in REFUGIUM.elf
#   the profile's own notices: absent in DEFAULT.elf, present in REFUGIUM.elf
#                             (reverse control: the second file IS the
#                             Refugium build, not a default one missing bits)
#
# Two kinds of marker, both measured on the default ELF (plan §4.5; the
# measurement is repeated by this script on every run):
#
#   data literals  searched as bytes in the ALLOCATED sections only (SHF_ALLOC),
#                  because the ELF also carries DWARF, whose file names and
#                  identifiers would match a source-level name forever;
#   symbols        defined symbols whose section is allocated, matched by
#                  exact name.
#
# writeOTPValues, AddBootKey and EnableSecureBoot have no symbols even in the
# default ELF (inlined), and FOREVERLAURA! occurs 0 times there, so they cannot
# be markers: the OTP writer's absence rests on otp.writeECC/otp.writeOrRow, the
# redirect URL that only writeOTPValues carried, and gui's source-level call
# check (gui/refugium_build_test.go).
#
# It also prints the st25r3916 driver symbols present in each ELF, for the
# record of what the reader's removal took with it; those are reported, not
# asserted, beyond the markers above.
#
# Needs python3 only (no pyelftools): the ELF32 little-endian header, section
# table and symbol table are parsed here.
set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: $0 DEFAULT.elf REFUGIUM.elf" >&2
	exit 2
fi

exec python3 - "$1" "$2" <<'PY'
import struct, sys

LITERALS = [
    b"lock-boot: %v",              # the debug-command arm's log format (gui)
    b"seedhammer.com/doc/?d=SHII", # the OTP redirect URL writeOTPValues wrote
    b"PASSPROOF",                  # ppPassProofKeep* help strings + trigger
    b"TEXTPROOF",
    b"CONSTPROOF",
    b"BOTHPROOF",
    b"SIZEPROOF",
]
SYMBOLS = [
    "(*seedhammer.com/nfc/poller.Poller).Read",
    "seedhammer.com/gui.qaEngraveFlow",
    "seedhammer.com/driver/otp.writeECC",
    "seedhammer.com/driver/otp.writeOrRow",
]
# The REVERSE control: the profile's own notices, present only in the Refugium
# ELF. Without it, two copies of a default ELF that merely lost the markers
# some other way (a stripped build, the wrong file) would not be told apart
# from the Refugium build.
PROFILE = [
    b"NFC could not be turned off",            # gui/nfc_fault.go
    b"This build takes no BIP-39 passphrase",  # gui/passphrase_off.go
    b"This build reads no cards over NFC",     # gui/nfc_scan.go
]
SHF_ALLOC = 0x2
SHT_SYMTAB, SHT_NOBITS = 2, 8


def load(path):
    data = open(path, "rb").read()
    if data[:4] != b"\x7fELF" or data[4] != 1 or data[5] != 1:
        sys.exit(f"{path}: not an ELF32 little-endian file")
    shoff, = struct.unpack_from("<I", data, 0x20)
    shentsize, shnum, shstrndx = struct.unpack_from("<HHH", data, 0x2E)
    secs = []
    for i in range(shnum):
        name, typ, flags, addr, off, size, link, info, align, entsize = \
            struct.unpack_from("<IIIIIIIIII", data, shoff + i * shentsize)
        secs.append(dict(name=name, type=typ, flags=flags, off=off, size=size,
                         link=link, entsize=entsize))
    shstr = secs[shstrndx]

    def cstr(base, off):
        end = data.index(b"\0", base + off)
        return data[base + off:end].decode("utf-8", "replace")

    for s in secs:
        s["sname"] = cstr(shstr["off"], s["name"])
    alloc = b"".join(data[s["off"]:s["off"] + s["size"]] for s in secs
                     if s["flags"] & SHF_ALLOC and s["type"] != SHT_NOBITS)
    syms = set()
    for s in secs:
        if s["type"] != SHT_SYMTAB:
            continue
        strtab = secs[s["link"]]
        for j in range(s["size"] // s["entsize"]):
            nm, val, sz, info, other, shndx = struct.unpack_from(
                "<IIIBBH", data, s["off"] + j * s["entsize"])
            if shndx == 0 or shndx >= len(secs):
                continue  # undefined, or ABS/COMMON
            if not secs[shndx]["flags"] & SHF_ALLOC:
                continue
            syms.add(cstr(strtab["off"], nm))
    if not syms:
        sys.exit(f"{path}: no symbol table -- refusing to report absence")
    return alloc, syms


def table(path):
    alloc, syms = load(path)
    row = {}
    for lit in LITERALS:
        row[lit.decode()] = alloc.count(lit)
    for sym in SYMBOLS:
        row[sym] = 1 if sym in syms else 0
    for lit in PROFILE:
        row["profile: " + lit.decode()] = alloc.count(lit)
    st = sorted(s for s in syms if s.startswith("(*seedhammer.com/driver/st25r3916.Device)."))
    return row, st, len(alloc)


dpath, rpath = sys.argv[1], sys.argv[2]
drow, dst, dlen = table(dpath)
rrow, rst, rlen = table(rpath)
print(f"allocated bytes scanned: default {dlen}, refugium {rlen}")
print(f"{'marker':<45} {'default':>8} {'refugium':>9}")
fail = False
for k in drow:
    verdict = ""
    if k.startswith("profile: "):
        if drow[k] != 0:
            verdict = "  PRESENT in the default ELF"
            fail = True
        if rrow[k] == 0:
            verdict += "  REVERSE CONTROL FAILED: absent from the Refugium ELF"
            fail = True
        print(f"{k:<45} {drow[k]:>8} {rrow[k]:>9}{verdict}")
        continue
    if drow[k] == 0:
        verdict = "  CONTROL FAILED: absent from the default ELF"
        fail = True
    if rrow[k] != 0:
        verdict += "  PRESENT in the Refugium ELF"
        fail = True
    print(f"{k:<45} {drow[k]:>8} {rrow[k]:>9}{verdict}")
print("st25r3916 Device methods, default :", ", ".join(s.split(").")[1] for s in dst))
print("st25r3916 Device methods, refugium:", ", ".join(s.split(").")[1] for s in rst) or "(none)")
gone = sorted(set(s.split(").")[1] for s in dst) - set(s.split(").")[1] for s in rst))
print("gone in the Refugium ELF          :", ", ".join(gone) or "(none)")
if fail:
    print("RESULT: FAIL")
    sys.exit(1)
print(f"RESULT: ok -- {len(LITERALS) + len(SYMBOLS)} markers present in the default ELF and absent "
      f"in the Refugium ELF; {len(PROFILE)} profile notices the other way round")
PY
