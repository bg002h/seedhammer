{
  description = "Seedhammer firmware";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      utils,
    }:
    utils.lib.eachDefaultSystem (
      system:
      let
        # Raspberry Pi openocd fork with rp2350 support.
        openocd-overlay =
          let
            version = "sdk-2.2.0";
          in
          final: prev: {
            version = "${version}";
            openocd = prev.openocd.overrideAttrs (old: {
              version = "${version}";
              pname = "openocd-rpi";
              src = final.fetchFromGitHub {
                owner = "raspberrypi";
                repo = "openocd";
                tag = "${version}";
                hash = "sha256-ZfbZVFVncHa1MvNJb4jbnU66vnlwVLBaOXPdgLqAneM=";
                # openocd disables the vendored libraries that use submodules and replaces them with nix versions.
                # this works out as one of the submodule sources seems to be flaky.
                fetchSubmodules = false;
              };
              nativeBuildInputs = old.nativeBuildInputs ++ [
                final.autoreconfHook
              ];
            });
          };
        # Newest TinyGo version.
        tinygo-overlay =
          let
            version = "0.41.1";
          in
          final: prev: {
            tinygo = prev.tinygo.overrideAttrs (prev: {
              version = "${version}";
              doCheck = false; # TinyGo tests are slow.
              src = final.fetchFromGitHub {
                owner = "tinygo-org";
                repo = "tinygo";

                tag = "v${version}";
                hash = "sha256-8Zpvhx+xgC/Cjdm3zSpntLKOT4HsBU7lPWdLumWeFyw=";

                fetchSubmodules = true;
                postFetch = ''
                  rm -r $out/lib/cmsis-svd/data/{SiliconLabs,Freescale}
                '';
              };
              vendorHash = "sha256-OO8o/s71jZIypfYZCLT6jwUPyQJ89AKg3DfzTrbrD/A=";
            });
          };
        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            tinygo-overlay
            openocd-overlay
          ];
        };
      in
      {
        formatter = pkgs.nixpkgs-fmt;
        packages = {
        }
        // (
          let
            tinygo-flags = "-target pico-plus2 -stack-size 16kb -gc precise -opt 2 -scheduler tasks";
            dummy_pem = pkgs.writeText "dummy.pem" ''
              -----BEGIN EC PARAMETERS-----
              BgUrgQQACg==
              -----END EC PARAMETERS-----
              -----BEGIN EC PRIVATE KEY-----
              MHQCAQEEIFSrFKF8udYXSJ0l6gPi4EY74G3dhrdOKSfQsN7yyUy6oAcGBSuBBAAK
              oUQDQgAEumLU1YRqzQosru/AaObMpw8q/LsRDVVM0XF84o9qAvSxktFcrtwMlNZr
              P20/GJHNoGFZmfh7zOmY42TQcasEdw==
              -----END EC PRIVATE KEY-----
            '';
          in
          {
            build-firmware = pkgs.writeShellScriptBin "build-firmware" ''
              set -eu -o pipefail

              # Fork builds are versioned v0.0.0-bg<sha>, NOT by `git describe
              # --tags`. The fork inherits upstream's release tags, so describe
              # yielded e.g. "v1.4.3-244-g86e0da9" -- which reads on the device
              # home screen as though this were official SeedHammer v1.4.3. It is
              # not. The -bg<sha> suffix keeps each build traceable to a commit.
              # Override with VERSION=... to stamp something else.
              #
              # WHY bg AND NOT g (2026-08-13, operator): `g` is what `git
              # describe` prefixes a SHA with, for "git". Borrowing it here made
              # our hand-built string look like describe output -- the exact
              # confusion the paragraph above exists to prevent, just one layer
              # in. `bg` is the fork owner, matches the bg002h remote, and is a
              # letter no tool will claim to have generated.
              if [ -z "''${VERSION:-}" ]; then
                SHA=$(${pkgs.git}/bin/git rev-parse --short HEAD)
                DIRTY=""
                ${pkgs.git}/bin/git diff --quiet HEAD || DIRTY="-dirty"
                VERSION="v0.0.0-bg''${SHA}''${DIRTY}"
              fi
              WORKDIR="$(mktemp -d)"
              OUTPUT="seedhammerii-$VERSION.uf2"

              ${pkgs.tinygo}/bin/tinygo build -o "$WORKDIR/firmware.uf2" -ldflags="-X main.Version=$VERSION" ${tinygo-flags} "$@" ./cmd/controller
              # Sign with a dummy key to convince picotool to create the necessary
              # file structure for signing.
              ${pkgs.picotool}/bin/picotool seal --sign --clear --quiet "$WORKDIR/firmware.uf2" "$WORKDIR/firmware.signed.uf2" "${dummy_pem}"
              # Clear public key and signature.
              ${pkgs.go}/bin/go run seedhammer.com/cmd/picosign sign -clear "$WORKDIR/firmware.signed.uf2"

              mv "$WORKDIR/firmware.signed.uf2" "$OUTPUT"
              rm -rf "$WORKDIR"
              echo "Built $OUTPUT"
            '';
            flash-firmware = pkgs.writeShellScriptBin "flash-firmware" ''
              set -eu -o pipefail

              VERB="$1"; shift
              if [ -z "''${VERSION:-}" ]; then
                SHA=$(${pkgs.git}/bin/git rev-parse --short HEAD)
                DIRTY=""
                ${pkgs.git}/bin/git diff --quiet HEAD || DIRTY="-dirty"
                # Same scheme as build-firmware above, deliberately -- a
                # tinygo flash/gdb build must stamp what build-firmware would,
                # or the home screen disagrees with the artifact name.
                VERSION="v0.0.0-bg''${SHA}''${DIRTY}"
              fi
              ${pkgs.tinygo}/bin/tinygo "$VERB" -ldflags="-X main.Version=$VERSION" ${tinygo-flags} \
                -programmer=cmsis-dap -ocd-commands "adapter speed 10000" \
                -serial=uart "$@" \
                ./cmd/controller
            '';
            copy-signature = pkgs.writeShellScriptBin "copy-signature" ''
              set -eu -o pipefail

              PUBKEY="03bcea00fedcd40ddbb67f4a3f2fafb59bb942e7b2130a76ffbed5568497477bfd"
              SRC="$1"; shift
              DST="$1"; shift
              SIGNATURE=$(${pkgs.go}/bin/go run seedhammer.com/cmd/picosign extract "$SRC")
              ${pkgs.go}/bin/go run seedhammer.com/cmd/picosign sign -pubkey "$PUBKEY" -sig "$SIGNATURE" "$DST"
            '';
          }
        );
        devShells =
          let
            selfpkgs = self.packages.${system};
          in
          {
            default = pkgs.mkShell {
              packages = with pkgs; [
                tinygo
                gcc-arm-embedded
                go
                openocd
                pioasm
                picotool
                selfpkgs.build-firmware
                selfpkgs.flash-firmware
                selfpkgs.copy-signature
              ];
            };
          };
      }
    );
}
