# SeedHammer II Firmware

This repository contains the source code to run the controller program for the
[SeedHammer II](https://seedhammer.com) engraving machine. The hardware is
[open source](https://github.com/seedhammer/hardware).

The [user manual](https://seedhammer.com/doc/manual) contains detailed instructions
for operating the machine.

## About this fork

This is a community fork of [seedhammer/seedhammer](https://github.com/seedhammer/seedhammer).
The `main` branch tracks upstream `main` plus two additive features, merged as
`dbb187a` and `e3c0c21` (the original feature branches are kept intact):

- **On-device CODEX32 seed entry** — re-enables the (upstream-disabled) CODEX32
  input flow. Upstream [PR #34](https://github.com/seedhammer/seedhammer/pull/34)
  (declined for now, pending UI polish).
- **BCH-validated `md1`/`mk1` engraving** — recognizes and engraves the `md1`
  (descriptor) and `mk1` (xpub) backup strings produced by the
  [mnemonic-engrave](https://github.com/bg002h/mnemonic-engrave) constellation of
  CLIs, verifying the BCH checksum before engraving. Upstream
  [PR #35](https://github.com/seedhammer/seedhammer/pull/35) (open).

These formats back up arbitrary wallet descriptors across multiple plates. The
`ms1` *secret* string is never accepted over NFC — it is hand-typed on the
air-gapped device via the CODEX32 flow above; only the public `md1`/`mk1` strings
are pushed and engraved.

> **Flashing a fork on retail hardware:** retail SeedHammer II units ship with
> secure boot **locked**, so running self-built firmware requires provisioning
> your own boot key into an OTP slot — an advanced and **irreversible** procedure.
> Consult the SeedHammer documentation before attempting it. The upstream install
> and reproducible-build steps below are otherwise unchanged.

## Installation

Press and hold the firmware upgrade button while connecting the machine to
a computer. Then, copy the firmware file to the USB drive that appears. The
installation is complete when the drive disappears.

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
