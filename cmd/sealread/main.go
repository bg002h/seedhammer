// Command sealread verifies Plan B Phase A Task 7 Step 4 on real RP2350
// silicon: that the XIP read at 0x10E00000 works at all, and that it is bounded
// by RegionLen.
//
// WHY THIS EXISTS. There is no precedent anywhere in this repo for reading N
// bytes from a fixed XIP address — every other unsafe.Pointer site takes the
// address of an already-typed peripheral register struct field, and
// driver/otp/otp_rp2350.go is a cgo bootrom CALL, not a flash read. So whether
// TinyGo 0.41.1 compiles seal.XIPReader into a correct read is an
// implementation-time question that only hardware answers (§10.1 says so).
//
// WHERE TO RUN IT. A Pico 2 / Pico Plus 2, NEVER the SeedHammer II.
// `tinygo flash` targets whatever RP2350 is in BOOTSEL, so before flashing:
//
//  1. Physically disconnect the SeedHammer II.
//
//  2. Confirm exactly ONE RP2350 is present and it is the Pico:
//     lsusb | grep 2e8a          # 2e8a:000f == bootrom in BOOTSEL
//     picotool info -a           # chipid must be 0x66d3d60ff20abf2f
//
//     THREE boards on this bench answer to 2e8a:000f. Full table in
//     mnemonic-engrave/design/HARDWARE_INVENTORY.md:
//     0x66d3d60ff20abf2f  Pico 2, rehearsal keys  <- the one you want
//     0x77c483b745abf55c  SeedHammer II           <- STOP
//     0xb3d19289d3ec3f0e  Pico 2 W                <- WRONG BOARD
//
//     The Pico 2 W is the easy mistake: same form factor, same 4 MB, sat next
//     to the rehearsal Pico. It has secure boot: 0, so it runs UNSIGNED images
//     — a sealread that boots there proves nothing about the signing path. Its
//     LED is also not where a Pico 2's is, so a dark LED is not evidence of a
//     failed boot. On 2026-08-07 it and the SH2 were in BOOTSEL *at the same
//     time*, which is exactly what step 2 exists to catch.
//
//  3. Replug holding BOOTSEL — a running TinyGo app has NO reset interface,
//     so picotool reboot -f cannot get you here ("Unable to locate reset
//     interface on the device", measured 2026-08-07).
//
// Build, sign and flash:
//
//	nix develop --command tinygo build -target pico2 -o /tmp/sealread.uf2 ./cmd/sealread
//	<mnemonic-engrave>/scripts/sign-firmware.sh /tmp/sealread.uf2 \
//	    <mnemonic-engrave>/rehearsal-work/my-key.pem
//	nix develop --command picotool load -x /tmp/sealread.signed.uf2
//
// The Pico's OTP holds the REHEARSAL keys, not ~/.sh2/sh2-boot-key.pem — that
// one is the SH2's and the Pico does not trust it.
//
// Then read the output. This kernel has no cdc_acm, so there is no
// /dev/ttyACM*: use scripts/cdcread.py from mnemonic-engrave.
//
// LIKE cmd/kdfbench, THIS IS TINYGO-ONLY. It imports `machine`, which is not in
// the host Go standard library, so `go build`, `go vet` and `go test ./...`
// cannot touch it — they report "package machine is not in std [setup failed]".
// That means the repo's sanctioned-green baseline is now TWO setup failures,
// kdfbench and sealread, not one. Only `tinygo build -target pico2` compiles it.
//
// It prints in a LOOP on purpose. TinyGo's CDC drops output when no host is
// attached, so a one-shot print is simply lost.
package main

import (
	"fmt"
	"machine"
	"time"
	"unsafe"

	"seedhammer.com/seal"
)

func main() {
	// Let the CDC endpoint come up before the first line.
	time.Sleep(3 * time.Second)

	// SECOND READ, at an address that EXISTS on a 4 MB Pico 2.
	//
	// PayloadAddr (0x10E00000) is 14 MB into flash and only exists on a 16 MB
	// part -- the SH2, or a Pico PLUS 2. On a plain Pico 2 (4 MB, measured:
	// "flash size: 4096K") an XIP read there ALIASES to 0x10200000, so the
	// first read below returns a plausible "no payload" for the wrong reason.
	//
	// This exercises the IDENTICAL expression at testAddr, which is inside this
	// board's flash and clear of sealread's own image (which ends ~0x10029600).
	// It proves the read MECHANISM -- that a fixed-address unsafe.Slice returns
	// the real flash bytes -- which is what Phase A actually needs from
	// hardware. The normative address itself is fixed by §5's arithmetic and by
	// the SH2's 16 MB part, not by this board.
	const testAddr = 0x10300000

	for {
		probe := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(testAddr))), 64)
		fmt.Printf("sealread: probe @%#08x first 16: % x\n", testAddr, probe[:16])
		if string(probe[:8]) == "MNEMBLOB" {
			fmt.Printf("sealread: probe MAGIC PRESENT — parsing header at test address\n")
			if h, herr := seal.ParseHeader(probe[:]); herr != nil {
				fmt.Printf("sealread: probe header: %v\n", herr)
			} else {
				fmt.Printf("sealread: probe header OK — pub_len=%d ct_len=%d sealed=%v\n",
					h.PubLen, h.CtLen, h.Sealed())
			}
		}

		b, err := seal.XIPReader{}.Read()

		switch {
		case err == seal.ErrNoPayload:
			// Expected on a board that has never been loaded: the region is
			// erased flash and the magic does not match.
			fmt.Printf("sealread: no payload at %#08x (magic absent) — this is the CLEAN state\n",
				seal.PayloadAddr)
		case err != nil:
			fmt.Printf("sealread: read error: %v\n", err)
		default:
			// Step 4 sub-step 5: print the byte count actually read and check
			// it against the region bound. A read longer than RegionLen means
			// clampRegion is not doing its job on the target, which the host
			// test cannot observe — read_tinygo.go is behind //go:build tinygo.
			ok := "OK"
			if len(b) > seal.RegionLen {
				ok = "OVER-LONG — clampRegion FAILED on target"
			}
			fmt.Printf("sealread: read %d bytes at %#08x (bound %d) %s\n",
				len(b), seal.PayloadAddr, seal.RegionLen, ok)
			fmt.Printf("sealread: first 8 bytes: % x\n", b[:8])

			// If a real payload is present, parse its header — that exercises
			// §6.2 on target, not just on the host.
			if h, herr := seal.ParseHeader(b); herr != nil {
				fmt.Printf("sealread: header: %v\n", herr)
			} else {
				fmt.Printf("sealread: header OK — pub_len=%d ct_len=%d sealed=%v iterations=%d\n",
					h.PubLen, h.CtLen, h.Sealed(), h.Iterations)
			}
		}

		fmt.Printf("sealread: (looping — CDC drops output with no host attached)\n\n")
		time.Sleep(2 * time.Second)
		_ = machine.Serial
	}
}

// ON-TARGET RESULT — 2026-08-07, Pico 2, chipid 0x66d3d60ff20abf2f
//
// NEGATIVE PATH VERIFIED. Flashed signed with rehearsal-work/my-key.pem and
// booted (secure boot: 1, so booting at all proves the OTP trusts that key --
// and picotool showed the PREVIOUS image carried the same pubkey 9EC00C33...).
// Output, read with scripts/cdcread.py:
//
//	sealread: no payload at 0x10e00000 (magic absent) — this is the CLEAN state
//
// So seal.XIPReader's unsafe.Slice over a fixed XIP address COMPILES under
// TinyGo 0.41.1, EXECUTES on RP2350 silicon, and reads erased flash correctly.
// That was Phase A's one claim with no precedent anywhere in this repo.
//
// POSITIVE PATH VERIFIED -- at testAddr, NOT at PayloadAddr.
//
// Loaded a real `me seal` payload (3 public md1 records) and read it back:
//
//	probe @0x10300000 first 16: 4d 4e 45 4d 42 4c 4f 42 01 00 00 00 00 00 00 00
//	probe MAGIC PRESENT — parsing header at test address
//	probe header OK — pub_len=203 ct_len=0 sealed=false
//
// Byte-exact against what the host wrote, and pub_len=203 is independently
// right: 3 records x 67 chars + 2 LF separators. So the read MECHANISM and
// ParseHeader both work on silicon.
//
// WHAT REMAINS UNVERIFIED, and it is not a small caveat: the read at the
// NORMATIVE PayloadAddr (0x10E00000). That address is 14 MB into flash and this
// Pico 2 has 4 MB ("flash size: 4096K", measured), so the region does not exist
// here. picotool refuses the load outright --
//
//	ERROR: File size 0x100 starting at 0xe00000 is too big to fit in
//	       flash size 0x400000
//
// -- and an XIP read there silently ALIASES to 0x10200000. The FIRST read in
// main() therefore reports a plausible "no payload" for the WRONG REASON on
// this board. Do not read that negative result as evidence about 0x10E00000.
//
// Closing that gap needs a 16 MB part: a Pico PLUS 2 (which is why the fork's
// own build target is pico-plus2) or the SH2 itself. The PBKDF2 rate on an
// RP2350B is likewise still open -- this board is an RP2350A.
