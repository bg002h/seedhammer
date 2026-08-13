# Running your own firmware on a SeedHammer II

> ## ⚠ READ THIS FIRST
>
> **This software is untested by humans in meat space and almost entirely AI
> generated.** You are expected to test any output of this software yourself,
> and you have no excuse for not testing — because a necessary part of making
> any backup is **proving that the backup works!**
>
> Because you must test the output yourself and prove that it works, it is
> perfectly acceptable if this software outputs complete garbage: your testing
> will reveal it, and you will know not to rely on its output.
>
> If you require a guarantee or a warranty of any kind before using this sort of
> backup software, **you have not met your own requirements by selecting this
> untested software.**
>
> Long story short: you have been handed either a very useful tool or a cleverly
> designed foot gun. If you shoot yourself in the foot, you have only yourself to
> blame, and I'm not your tech support staff.

> ## Who this is for
>
> People who are willing to own the outcome.
>
> The Raspberry Pi Pico 2 rehearsal in §2 is not hand-holding. **It is the
> filter.** It costs about $5 and twenty minutes, and it is where you find out
> whether you actually understand what you are about to do while a mistake is
> still worth $5 instead of a SeedHammer II. It is also where the author's own
> mistake surfaced — a check that demanded an all-zero page-lock value, which on
> a real RP2350 would have declared a perfectly good machine unusable. The
> rehearsal caught it. That is what it is for.
>
> If that step reads to you as an inconvenience to skip rather than the obvious
> thing to do, this firmware is not for you, and that is fine.
>
> Whatever you choose to do with this software is your own business. If you break
> your stuff with it, **you** broke your stuff — not me. There is no support, no
> warranty, and no obligation on anyone's part to help you put it back. It is
> dual-licensed MIT OR Unlicense precisely so that you are free to do as you
> like, entirely at your own risk.

---

This guide walks through burning your own secp256k1 boot key into a retail
SeedHammer II so the machine will boot firmware **you** built and signed.

It is written for people who are comfortable in a terminal and who understand
that **the central step is permanent**. There is no undo, no factory reset, and
no support path that puts the chip back the way it was.

> **Status of this document.** Every command here was executed end to end
> **once**, against **one** SeedHammer II (hardware v1.5 / RP2350B) on
> 2026-08-03, and against a disposable Raspberry Pi Pico 2 before that. That run
> completed successfully: the machine now boots self-signed firmware and displays
> `(UNLOCKED)`.
>
> Be precise about what that does and does not cover. **The boot-key procedure
> in this guide** has been run, once, by a human, on real hardware. **The
> firmware it lets you run has not been** — nobody has taken a plate engraved by
> a self-built build and checked the words on it against the seed they came
> from. That is the meat-space testing the warning above is about, and it is
> yours to do.
>
> A single successful run on a single machine is not testing in any sense that
> should reassure you. It means the procedure is not obviously wrong. It does not
> mean it is right for your unit, your host, or your firmware.

> **This is not an official SeedHammer procedure.** It is not endorsed by or
> supported by SeedHammer AB. You are modifying a device that holds bitcoin
> backup material; understand every step before running it.

---

## 0. What you need

- A **SeedHammer II** and its own power supply (not just a laptop USB cable)
- A **Raspberry Pi Pico 2** to rehearse on — treat this as mandatory, see §2
- [Nix](https://nixos.org/) with flakes enabled
- This firmware repo, plus the tooling repo that implements the procedure:

```sh
git clone https://github.com/bg002h/seedhammer.git
git clone https://github.com/bg002h/mnemonic-engrave.git
```

Everything below assumes those two live side by side. The scripts
(`pico2-bootkey-rehearsal.sh`, `sign-firmware.sh`) are in **mnemonic-engrave**;
the firmware you are going to build and sign is in **this** repo.

---

## 1. What this actually does, and what it costs

The RP2350 in the SeedHammer II has secure boot permanently enabled from the
factory. It holds **four boot-key slots**; each stores the SHA-256 of a
secp256k1 public key. The bootrom will run any image signed by a key in a slot
that is marked *valid*.

Your machine ships with **slot 0** holding SeedHammer AB's production key. The
other three are blank. You are going to fill one of them.

**What you gain:** the machine boots your builds.

**What you give up, permanently:**

- **The version screen will read `(UNLOCKED)` forever.** The firmware reports a
  locked device only when secure boot is on *and* SeedHammer's key is the sole
  valid key (`isSecureBootEnabled()` requires `nvalid == 1`). Two valid keys
  means that attestation is gone. You cannot restore it — you can only revoke a
  key, never un-validate one.
- **One of your four slots.** A botched burn (a mismatched readback) consumes
  the slot with nothing to show for it.
- **Any resale or warranty story you might have had.** Assume the machine is
  yours now in every sense.

**What you do *not* give up:** secure boot stays fully enforced — the device
simply trusts two keys instead of one. And **slot 0 stays valid**, which means
official SeedHammer firmware still boots. That is your recovery path, and
nothing in this guide touches it.

> **Never set `KEY_INVALID`.** That field revokes a slot, irreversibly.
> Revoking slot 0 destroys your only route back to official firmware. There is
> no reason to touch it, ever.

---

## 2. Rehearse on a $5 board first

**Do not make your first attempt on the engraver.** Buy a Raspberry Pi Pico 2
(an RP2350 board — the Pico 2 W works too, but its LED is behind the Wi-Fi chip,
so plain Pico 2 is easier). It is the same silicon and the same OTP mechanism.

The rehearsal script runs a proof structure that is worth understanding, because
it is the difference between "it blinked" and "I proved the mechanism works":

```
phase 3   an image signed by YOUR key is REJECTED  (your key isn't burned yet)
phase 4   burn your key                             <-- the only thing that changes
phase 5   the SAME image is now ACCEPTED
```

Without phase 3, a board that was never really sealed would blink in phase 5 and
you would record a green rehearsal that proved nothing. **Do not skip phase 3.**

```sh
cd /path/to/seedhammer && nix develop
R=/path/to/mnemonic-engrave/scripts/pico2-bootkey-rehearsal.sh

$R --phase 0                # inventory + prep (read-only, safe)
$R --phase 1 --execute      # seal the board — DESTRUCTIVE, enables secure boot
$R --phase 2                # verify sealed + page locks
$R --phase 3 --execute      # NEGATIVE control: must FAIL to boot
$R --phase 4 --execute      # burn your key — DESTRUCTIVE
$R --phase 5 --execute      # POSITIVE control: same image must now boot
$R --phase 6 --execute      # fallback control
```

The script is dry-run by default; `--execute` arms writes, and destructive
phases additionally demand a typed confirmation. Every phase after 0 pins the
board by CHIPID and **refuses to run against a different device — including your
SeedHammer II.** That refusal is deliberate, and it is why the SH2 steps below
are separate read-only modes.

The rehearsal board is consumed. That is the point: you spend a cheap board to
buy certainty about an expensive one.

---

## 3. Toolchain

Everything runs inside this repo's Nix devshell, which pins the exact `picotool`
and `tinygo` versions used here:

```sh
cd /path/to/seedhammer
nix develop
```

You also need a udev rule so `picotool` can talk to the device without `sudo`.
**Number it below 73**, or `uaccess` never gets applied (`73-seat-late.rules`
runs first and the tag is ignored):

```sh
printf 'SUBSYSTEM=="usb", ATTRS{idVendor}=="2e8a", MODE="0666", TAG+="uaccess"\n' \
  | sudo tee /etc/udev/rules.d/60-picotool.rules
sudo udevadm control --reload && sudo udevadm trigger
```

**Getting into BOOTSEL:** hold the SeedHammer's button while plugging in USB,
then release. The machine enumerates as an RP2350 mass-storage device.

### Download your recovery image *first*

Your machine ships with official firmware in flash. The moment you flash your
own build, that copy is gone — so fetch the official release **before** you
start, while you still have a working machine and a browser:

```sh
mkdir -p ~/.sh2/recovery && cd ~/.sh2/recovery
curl -LO https://github.com/seedhammer/seedhammer/releases/download/v1.4.3/seedhammerii-v1.4.3.uf2
```

Then **prove it is actually a recovery image** rather than assuming it. It only
helps you if it is signed by the key in slot 0:

```sh
picotool info -a seedhammerii-v1.4.3.uf2 | grep -iE 'public key|signature'
# hash the embedded key and compare against slot 0:
picotool info -a seedhammerii-v1.4.3.uf2 \
  | awk '/[Pp]ublic key:/{print $NF}' | xxd -r -p | sha256sum
```

That hash must equal `c8314536d6af61ac2e62e5991e3e4711629c54696ba8c4af08965a1d319a473b`
— SeedHammer's production key — and `picotool` must say `signature: verified`.
An unverified download is not a safety net; it is a file.

Two things that look like faults and are not:

- USB reports **`Raspberry Pi / RP2350 Boot`**, not SeedHammer. Correct — the
  factory white-label provisioning sets the SCSI strings (`SH` / `SHII`), never
  the USB descriptor strings.
- On some units, **every** plug-in logs `device descriptor read/64, error -71`
  before succeeding. On the unit used to write this guide that happened on 5 of
  5 enumerations across three different host ports, while a Pico 2 on the same
  port was clean 7 times out of 7. It is a property of the machine's USB front
  end — most likely the RP2350 coming up while the USB-PD sink is still
  settling. It retries and succeeds. **Don't chase it by swapping cables.**

---

## 4. Generate your key

```sh
mkdir -p ~/.sh2 && chmod 700 ~/.sh2
openssl ecparam -name secp256k1 -genkey -noout -out ~/.sh2/sh2-boot-key.pem
chmod 600 ~/.sh2/sh2-boot-key.pem
```

It **must** be secp256k1. The RP2350 bootrom accepts nothing else, and a P-256
key looks perfectly healthy right up until you have burned its hash into a slot
that can never boot anything.

Print the fingerprint — this is the 32-byte value that goes into OTP:

```sh
openssl ec -in ~/.sh2/sh2-boot-key.pem -pubout -conv_form uncompressed \
  -outform DER | tail -c 64 | sha256sum
```

Note the shape of that command: it is the SHA-256 of the **uncompressed 64-byte
X‖Y** public key — no `0x04` prefix, no DER wrapper. Getting this wrong is the
single easiest way to burn a slot to a value nothing will ever match.

**Back the private key up now, offline, at least twice.** If you lose it you
cannot sign new firmware, and you will have spent a slot for nothing. If someone
else gets it, they can sign firmware your machine will boot — treat it like a
seed.

---

## 5. Read-only reconnaissance

Put the SeedHammer in BOOTSEL and survey it. Nothing here can write:

```sh
$R --sh2-precheck
```

This confirms the device is a genuine SeedHammer II (slot 0 must hold the
production key hash `c8314536…`), records its CHIPID so later steps can refuse a
different machine, and checks that the OTP pages are still writable. It also
reads the redundant `CRIT1` (×8) and `BOOT_FLAGS1` (×3) rows individually, so a
partial earlier write can't hide behind a majority vote.

**On page locks:** you will see `PAGE1_LOCK1 = 0x040404`. That is the RP2350
**factory default**, not a problem — a blank Pico 2 reads identically. It
decodes to `LOCK_S=0` (Secure may write), `LOCK_BL=0`, `LOCK_NS=1` (non-secure
read-only). Only `LOCK_S` or `LOCK_BL` being non-zero would mean no further key
can ever be added. An early version of this tooling demanded all-zero here and
would have declared a perfectly good engraver permanently unusable.

Now run the gate that is *supposed to fail*:

```sh
$R --sh2-verify-slot 1 --key ~/.sh2/sh2-boot-key.pem
```

Expect `SLOT 1 READBACK MISMATCH` — expected your hash, read all zeros. Slot 1
is blank; that is correct. Seeing this fail first is how you know the check is
real when it later passes.

---

## 6. The two irreversible writes

Build the OTP payload (this touches no hardware):

```sh
$R --make-otp-json --key ~/.sh2/sh2-boot-key.pem --slot 1 --out ~/.sh2/my-otp.json
```

Before loading it, **open the file and check it yourself.** It should contain
exactly one top-level key, `bootkey1`, holding 32 bytes that match your
fingerprint from step 4 — and **no** `crit1` and **no** `boot_flags1` entries.
That absence is what makes the next command safe in isolation: it writes the key
hash and nothing else. Your machine keeps booting official firmware.

### Write 1 — burn the hash

```sh
picotool otp load ~/.sh2/my-otp.json
```

This is a roughly one-second window with no recovery. If it is interrupted, rows
`0..k` burn and the rest stay blank, and **re-running cannot repair it** — the
bootrom refuses to rewrite a programmed ECC row. That costs you the slot; you
would move to slot 2 with one spare left. Don't bump the cable, don't let the
machine sleep, let it finish.

Then verify — the same command that just failed must now pass:

```sh
$R --sh2-verify-slot 1 --key ~/.sh2/sh2-boot-key.pem
```

All 16 rows must match a hash derived independently from your key file. **If
this does not pass, stop.** Do not set the valid bit. A slot marked valid whose
contents are wrong is strictly worse than a blank slot.

### Write 2 — mark it valid

```sh
picotool otp set -s BOOT_FLAGS1.KEY_VALID 0x2
```

`-s` means set-bits: it ORs `0x2` into the existing `0x1`, giving `0x3`. Use
`otp set -s` here and **never** `otp load` — `load` would rewrite the row
wholesale and could clear slot 0's valid bit, taking your recovery path with it.

Then close the loop:

```sh
$R --sh2-verify-valid 1 --key ~/.sh2/sh2-boot-key.pem
```

Expect `KEY_VALID is 0x3`, `KEY_INVALID 0`, and **all three redundant
`BOOT_FLAGS1` copies agreeing**. That last check is why this mode exists: a bare
`picotool otp get` prints `3` even for a degraded 2-of-3 majority write, which
reads as success while sitting one bit-flip from trouble.

If it reports a *low* value, the write was interrupted — just re-run the
identical `otp set -s`. It only sets bits, so repeating is safe. Do not burn
another slot and do not start re-signing.

**The OTP work is now complete.** Everything after this point is freely
retryable and costs no slots.

---

## 7. Build, sign, flash

```sh
# from this repo (needs Nix with flakes)
nix run .#build-firmware
# -> seedhammerii-<version>.uf2, named v0.0.0-bg<sha> by flake.nix -- NOT by
#    `git describe`, which would inherit upstream's tags and read as official

# sign it
/path/to/mnemonic-engrave/scripts/sign-firmware.sh <image>.uf2 ~/.sh2/sh2-boot-key.pem
```

`sign-firmware.sh` writes `<image>.signed.uf2` and **never modifies its input**.
It proves the signature offline before anything reaches the device: it asserts
the digest is unchanged after the signature is embedded (i.e. the signature
really is outside the hashed region), verifies with `openssl`, checks the
embedded bytes match the DER r‖s conversion, requires exactly two metadata
blocks, and requires `picotool` to independently report `signature: verified`.

It also **refuses to sign an official SeedHammer release** — that image is your
recovery path, and signing over it would be silent and green.

### One extra check before you flash

`sign-firmware.sh` is device-agnostic — it proves the signature is valid, but it
cannot know whether that key is in *your* slot. Confirm the link yourself:

```sh
picotool info -a <image>.signed.uf2 \
  | awk '/[Pp]ublic key:/{print $NF}' | head -1 | xxd -r -p | sha256sum
```

That must equal the fingerprint you burned in step 4. If it doesn't, the image
will not boot no matter how valid its signature is.

### Flash

```sh
picotool load --verify <image>.signed.uf2
```

**Judge the result on machine power, not on USB.** The controller's
`monitorPowerSupply` runs *before* the LCD initialises and demands 20–28 V from
a USB-PD supply; if it doesn't get that it reboots straight back into BOOTSEL.
A tethered laptop cable will look exactly like a firmware that failed to boot,
so don't bother with `picotool reboot` — unplug USB, connect the machine's own
power supply, and switch it on.

**Success looks like** the normal home screen with `(UNLOCKED)` on the version
line. That suffix is not a warning about a problem; it *is* the confirmation
that the bootrom found two valid keys and ran the one you signed.

### If it doesn't boot

Nothing is lost. Slot 0 is still valid. Hold the button while plugging USB back
in to reach BOOTSEL, then:

```sh
picotool load --verify ~/.sh2/recovery/seedhammerii-v1.4.3.uf2 && picotool reboot
```

That is the whole reason slot 0 is left alone.

---

## 8. Keep a record

Write down, in a file you will still have in two years:

- The device CHIPID and BOOTSEL serial (these corroborate each other — the
  serial is the same four 16-bit words in reverse row order)
- Which slot you used, and the fingerprint you burned into it
- Which slots remain free
- Where the private key is backed up

All of that is public information — key fingerprints and chip IDs, no secrets —
so it is safe to keep alongside your notes.

---

## 9. Questions people ask

**Can I add another key later?** Yes — two slots remain after this, and adding a
key never removes one. The bootrom checks signatures against every valid slot.

**Can I rotate keys?** Not really. "Rotation" requires revoking the old key,
which is irreversible and reduces your slot count permanently. Plan on adding,
not replacing.

**Can I undo the `(UNLOCKED)` label?** No. It follows from having more than one
valid key, and valid bits cannot be cleared.

**Does this weaken secure boot?** Not mechanically — the bootrom still refuses
unsigned images. What changes is the trust set: your key is now as powerful as
SeedHammer's on this machine. The security of the device now depends on the
security of your private key.

---

## Tooling reference

In [`bg002h/mnemonic-engrave`](https://github.com/bg002h/mnemonic-engrave):

| Path | What it is |
|---|---|
| `scripts/pico2-bootkey-rehearsal.sh` | Rehearsal phases + the read-only SH2 modes |
| `scripts/sign-firmware.sh` | Signs a UF2 and proves it offline |
| `scripts/test/run-e2e.sh` | Hardware-free regression harness for the above |
| `design/RUNBOOK_custom_boot_key.md` | The terse operator runbook |
| `design/REHEARSAL_RESULT_2026-08-03.md` | The completed Pico 2 rehearsal log |
