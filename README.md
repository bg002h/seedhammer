# SeedHammer II Firmware

This repository contains the source code to run the controller program for the
[SeedHammer II](https://seedhammer.com) engraving machine. The hardware is
[open source](https://github.com/seedhammer/hardware).

The [user manual](https://seedhammer.com/doc/manual) contains detailed instructions
for operating the machine.

## About this fork

This is a community fork of [seedhammer/seedhammer](https://github.com/seedhammer/seedhammer).
The `main` branch tracks upstream `main` plus a growing set of additive
features — the first two merged as `dbb187a` and `e3c0c21` (the original
feature branches are kept intact):

- **On-device CODEX32 seed entry** — re-enables the (upstream-disabled) CODEX32
  input flow. Upstream [PR #34](https://github.com/seedhammer/seedhammer/pull/34)
  (declined for now, pending UI polish).
- **BCH-validated `md1`/`mk1` engraving** — recognizes and engraves the `md1`
  (descriptor) and `mk1` (xpub) backup strings produced by the
  [mnemonic-engrave](https://github.com/bg002h/mnemonic-engrave) constellation of
  CLIs, verifying the BCH checksum before engraving. Upstream
  [PR #35](https://github.com/seedhammer/seedhammer/pull/35) (open).
- **SLIP-39 recovery — BIP-39-vs-verbatim choice** — recovering a SLIP-39 backup
  engraves the recovered seed as BIP-39 words, which is correct for backups made
  from a BIP-39 phrase or the `mnemonic` toolkit. For a Trezor or other SLIP-39
  wallet backup, choose **"Engrave shares"** at the post-recovery prompt to engrave
  your share words verbatim, or use the
  [mnemonic-toolkit](https://github.com/bg002h/mnemonic-toolkit) CLI to recover
  off-device.
- **BIP-39 Password** — a top-level program on the start screen that engraves a
  BIP-39 passphrase on its own plate, with two optional master-fingerprint fields (bare
  seed vs. passphrase-derived) and an opt-in QR carrying the passphrase and
  nothing else. For anyone running a passphrase-protected wallet who wants that
  passphrase backed up on steel instead of trusted to memory alone. Fork-side
  only; no upstream PR.

These formats back up arbitrary wallet descriptors across multiple plates. The
`ms1` *secret* string is never accepted over NFC — it is hand-typed on the
air-gapped device via the CODEX32 flow above; only the public `md1`/`mk1` strings
are pushed and engraved.

> **Flashing a fork on retail hardware — read the warning first.** This is
> **untested by humans in meat space and almost entirely AI generated.** You are
> expected to test its output yourself, and you have no excuse not to: proving
> the backup works *is* part of making the backup. Either a very useful tool or a
> cleverly designed foot gun — if you shoot yourself in the foot, that's on you,
> and I'm not your tech support staff.
>
> Retail SeedHammer II units ship with secure boot **locked**, so running
> self-built firmware means burning your own boot key into an OTP slot —
> advanced, **irreversible**, and it costs the device's `(UNLOCKED)`-free version
> line permanently. **[docs/custom-firmware.md](docs/custom-firmware.md)** has the
> full warning and the step-by-step, including the mandatory rehearsal on a
> disposable Pico 2. Not an official SeedHammer procedure; not supported by
> SeedHammer AB. The upstream install and reproducible-build steps below are
> unchanged.

## Installation

Press and hold the firmware upgrade button while connecting the machine to
a computer. Then, copy the firmware file to the USB drive that appears. The
installation is complete when the drive disappears.

### Installing *this fork*

That works for firmware signed by SeedHammer AB. It will **not** install a build
from this repository: retail units ship with secure boot locked to SeedHammer's
key, and the bootrom rejects anything else before a single pixel reaches the
screen. Running your own build means adding your own boot key to one of the
three free OTP slots, and that is **permanent** — see the warning above for what
it costs.

**[docs/custom-firmware.md](docs/custom-firmware.md) is the authoritative
procedure**; it explains why every check below exists and what each failure
looks like. What follows is the route in dependency order, so you can see the
whole shape before starting. The section references point into that document.
Do not run step 4 from this summary alone.

Both repos, side by side — the scripts live in the companion repo, the firmware
lives here:

```sh
git clone https://github.com/bg002h/seedhammer.git
git clone https://github.com/bg002h/mnemonic-engrave.git
cd seedhammer && nix develop
R=../mnemonic-engrave/scripts/pico2-bootkey-rehearsal.sh
```

You also need a udev rule numbered **below 73** so `picotool` can reach the
device without `sudo` (§3) — above it, `uaccess` is never applied.

**1 — Download your recovery image, first, while the machine still boots** (§3).
Flashing your own build destroys the copy in flash, so fetch the current
official release now and *prove* it is signed by slot 0's production key rather
than assuming it. An unverified download is not a safety net; it is a file.

**2 — Generate a secp256k1 key** (§4). Reversible; nothing has been written yet.

```sh
mkdir -p ~/.sh2 && chmod 700 ~/.sh2
openssl ecparam -name secp256k1 -genkey -noout -out ~/.sh2/sh2-boot-key.pem
chmod 600 ~/.sh2/sh2-boot-key.pem
openssl ec -in ~/.sh2/sh2-boot-key.pem -pubout -conv_form uncompressed \
  -outform DER | tail -c 64 | sha256sum
```

It must be secp256k1 — a P-256 key looks healthy right up until you have burned
its hash into a slot that can never boot anything. That last hash, the SHA-256
of the uncompressed 64-byte X‖Y with no `0x04` prefix and no DER wrapper, is the
value that goes into OTP. **Back the private key up offline, at least twice,
before going further.**

**3 — Rehearse on a $5 Raspberry Pi Pico 2** (§2). Same silicon, same OTP
mechanism, and this is where you find out whether you understand the procedure
while a mistake still costs $5. Phase 3 is the negative control — the same image
that boots in phase 5 must be *rejected* in phase 3 — so a board that was never
really sealed cannot hand you a green rehearsal that proved nothing.

```sh
$R --phase 0                # inventory, read-only
$R --phase 1 --execute      # seal the board — destructive
$R --phase 2                # verify sealed + page locks
$R --phase 3 --execute      # negative control: must FAIL to boot
$R --phase 4 --execute      # burn your key — destructive
$R --phase 5 --execute      # positive control: same image must now boot
$R --phase 6 --execute      # fallback control
```

Every phase after 0 pins the board by CHIPID and refuses to run against a
different device, including your SeedHammer II.

**4 — The OTP burn** (§5, §6). **Two writes, both permanent, no undo.** Put the
machine in BOOTSEL: hold its button while connecting USB.

```sh
$R --sh2-precheck                                       # read-only survey; records the CHIPID
$R --sh2-verify-slot 1 --key ~/.sh2/sh2-boot-key.pem    # MUST fail — slot 1 is blank
$R --make-otp-json --key ~/.sh2/sh2-boot-key.pem --slot 1 --out ~/.sh2/my-otp.json
#   now open my-otp.json yourself: exactly one top-level key, `bootkey1`,
#   holding your fingerprint — and NO `crit1`, NO `boot_flags1`.
picotool otp load ~/.sh2/my-otp.json                    # WRITE 1 — burns the key hash
$R --sh2-verify-slot 1 --key ~/.sh2/sh2-boot-key.pem    # must now pass. If it does not, STOP.
picotool otp set -s BOOT_FLAGS1.KEY_VALID 0x2           # WRITE 2 — marks the slot valid
$R --sh2-verify-valid 1 --key ~/.sh2/sh2-boot-key.pem   # 0x3, KEY_INVALID 0, all 3 copies agree
```

The blank-slot check is run *before* the burn on purpose: seeing it fail is how
you know it is a real check when it later passes. A slot marked valid whose
contents are wrong is strictly worse than a blank slot, which is why write 2 is
gated on the readback rather than bundled with write 1.

Two rules with no exceptions. Use `otp set -s` for the flags row and **never
`otp load`** — `load` rewrites the row wholesale and can clear slot 0's valid
bit. And **never set `KEY_INVALID`**, which revokes a slot irreversibly. Slot 0
stays valid throughout; official firmware keeps booting, and that is your way
back.

If verification reports a *low* value the write was interrupted: re-run the
identical `otp set -s`. It only sets bits, so repeating is safe.

**5 — Build, sign, flash** (§7). Freely retryable; no more slots at stake.

```sh
nix run .#build-firmware
../mnemonic-engrave/scripts/sign-firmware.sh <image>.uf2 ~/.sh2/sh2-boot-key.pem
picotool load --verify <image>.signed.uf2
```

`nix run .#build-firmware` produces an **unsigned** image — never flash it. It
cannot boot, and it overwrites the working firmware to get there.
`sign-firmware.sh` never modifies its input, proves the signature offline before
anything reaches the device, and refuses to sign an official SeedHammer release.
It cannot know the key is in *your* slot, though, so confirm that link yourself
(§7) before flashing.

**6 — Judge the result on the machine's own power supply**, not a laptop USB
cable. `monitorPowerSupply` runs before the LCD initialises and demands 20–28 V
from a USB-PD supply; without it the machine reboots straight back into BOOTSEL
and looks exactly like firmware that failed to boot. Give PD negotiation a
moment before concluding anything.

Success is the normal home screen with **`(UNLOCKED)`** on the version line.
That suffix is not a warning about a fault — it *is* the confirmation that the
bootrom found two valid keys and ran the one you signed.

If it does not boot, nothing is lost. Hold the button, plug USB back in, and
flash the recovery image from step 1.

### Building from source

To build a [UF2](https://github.com/microsoft/uf2) image, [Nix](https://nixos.org/) with flakes
enabled is required.

```sh
$ nix run .#build-firmware
```

### Reproducible builds

The build process is designed to be deterministic, that is, images produced with the above steps
should match the released images bit-for-bit, except for the signature. To copy the signature
from an official release to a locally built firmware:

```sh
$ nix run .#copy-signature <path/to/official/seedhammerii-vX.Y.Z.uf2> <path/to/your/seedhammerii-vX.Y.Z>
```

## Development

Connect a debugger to the debug and UART ports on the machine PCB. Then, build and flash a
firmware image:

```
$ nix run .#flash-firmware flash -tags debug
```

In debug mode, logging output from the controller is routed through the USB serial device.
Use

```
$ tinygo monitor
```

to show the log on your terminal.

### License

The files is this repository are in the public domain as described in the [LICENSE](LICENSE) file,
except files in directories with their own LICENSE files.

### Contributions

Contributors must agree to the [developer certificate of origin](https://developercertificate.org/),
to ensure their work is compatible with the the LICENSE. Sign your commits with
Signed-off-by statements to show your agreement with the `git commit --signoff` (or `-s`)
command.
