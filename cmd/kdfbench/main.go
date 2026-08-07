// Command kdfbench measures PBKDF2-HMAC-SHA256 throughput on real RP2350
// silicon, so the encrypted-payload iteration count (SPEC §7.1 / §12 item 1)
// rests on a measurement rather than an estimate.
//
// WHY THIS EXISTS. The only on-device figure the project had was
// SPEC_seedhammer_slip39_recovery.md §5.6 — "10000·2^e software SHA-256
// blocks: e=0/1 ≈ 0.5–1.9 s" — which is a design-time estimate on a range,
// and whose units ("blocks") disagree with the reading later used to derive
// ~15,000 iterations/s. This program replaces both with a number.
//
// WHERE TO RUN IT. A Pico 2 / Pico Plus 2, NOT the SeedHammer II:
//   - identical RP2350 and identical TinyGo target, so cpuFreq = 150 MHz
//     (machine_rp2_2350.go:12) is the same on both;
//   - it runs on plain USB 5 V, whereas the SH2 app demands a 20–28 V USB-PD
//     contract before Init() will even configure the LCD;
//   - nothing is at risk if it hangs.
//
// The measured rate transfers because PBKDF2 here is compute-bound with a
// working set of a few hundred bytes — it lives in cache, so the RP2350A
// (Pico 2) and RP2350B (SH2) differences (pin count, flash size) do not bear on it.
//
// Build and flash (from the fork root, board held in BOOTSEL):
//
//	nix develop --command tinygo flash -target pico2 ./cmd/kdfbench
//
// then read the results:
//
//	picocom -b 115200 /dev/ttyACM0     # or: screen /dev/ttyACM0 115200
//
// Use -target pico-plus2 for a Pico Plus 2.
package main

import (
	"crypto/sha256"
	"fmt"
	"machine"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// Matches the firmware's real call shape in slip39/feistel.go:50 —
// pbkdf2.Key(pw, salt, iters, half, sha256.New). `half` is 8 or 16 there and
// 32 for the sealed-payload key; all three are one PBKDF2 block (<= 32 bytes),
// so the per-iteration cost is identical. Both are measured anyway, so the
// claim is checked rather than asserted.
var (
	passphrase = []byte("beef beef beef beef beef beef beef beef beef beef beef beef")
	salt       = []byte{
		0xbe, 0xef, 0xbe, 0xef, 0xbe, 0xef, 0xbe, 0xef,
		0xbe, 0xef, 0xbe, 0xef, 0xbe, 0xef, 0xbe, 0xef,
	}
)

func measure(iters, dkLen int) (time.Duration, float64) {
	start := time.Now()
	out := pbkdf2.Key(passphrase, salt, iters, dkLen, sha256.New)
	elapsed := time.Since(start)
	// Consume the result so nothing can be optimised away.
	if len(out) != dkLen {
		panic("short derived key")
	}
	return elapsed, float64(iters) / elapsed.Seconds()
}

type result struct {
	iters, dkLen int
	elapsed      time.Duration
	rate         float64
}

func main() {
	// Measure once, then REPRINT FOREVER. The host cannot rely on attaching
	// within any particular window — on this workstation the kernel has no
	// cdc_acm driver at all, so the reader is a libusb client that may attach
	// long after boot. Printing once would lose the run.
	time.Sleep(2 * time.Second)

	// Warm up: the first run pays one-time init and XIP cache fills.
	measure(1000, 32)

	var results []result
	var last float64
	for _, dkLen := range []int{16, 32} {
		for _, iters := range []int{10_000, 50_000, 100_000, 200_000} {
			elapsed, rate := measure(iters, dkLen)
			results = append(results, result{iters, dkLen, elapsed, rate})
			if dkLen == 32 {
				last = rate
			}
		}
	}

	for {
		fmt.Printf("\r\n=== PBKDF2-HMAC-SHA256 on RP2350 @ %d Hz ===\r\n", machine.CPUFrequency())
		fmt.Printf("%-10s %-8s %-12s %-14s\r\n", "iters", "dkLen", "elapsed", "iters/sec")
		fmt.Printf("%-10s %-8s %-12s %-14s\r\n", "-----", "-----", "-------", "---------")
		for _, r := range results {
			fmt.Printf("%-10d %-8d %-12s %-14.0f\r\n",
				r.iters, r.dkLen, r.elapsed.Round(time.Millisecond), r.rate)
		}
		fmt.Printf("\r\n--- SPEC 7.1 ---\r\n")
		fmt.Printf("measured rate (dkLen=32): %.0f iterations/sec\r\n", last)
		fmt.Printf("estimate this replaces  : 15000 iterations/sec\r\n")
		fmt.Printf("iterations for  5s      : %.0f\r\n", last*5)
		fmt.Printf("iterations for 15s      : %.0f\r\n", last*15)
		fmt.Printf("iterations for 30s      : %.0f   <-- the spec target\r\n", last*30)
		fmt.Printf("plan default (450000)   : %.1f s on device\r\n", 450_000/last)
		fmt.Printf("bounds [100000,2000000] : %.1f s .. %.1f s\r\n",
			100_000/last, 2_000_000/last)
		fmt.Printf("=== END ===\r\n")
		time.Sleep(3 * time.Second)
	}
}
