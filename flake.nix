{
  description = "audioloader dev shell";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go gopls gotools gofumpt golangci-lint delve templ

            # make dev / make build
            air
            tailwindcss_4

            # track lookup, download and transcoding (see README's env vars)
            ffmpeg
            yt-dlp
            spotdl
            chromaprint # fpcalc, for AcoustID

            # mobile/ (Expo) and tests/js/*.test.mjs
            nodejs_22
          ];

          # The shell's own tools must win over whatever launched it. An
          # inherited LD_LIBRARY_PATH from another nixpkgs generation puts a
          # foreign libasound ahead of ffmpeg's, and ffmpeg then fails to
          # start at all ("version `GLIBC_2.43' not found"), which breaks the
          # trim tests. Nothing in this shell needs the variable: every
          # package here finds its libraries through its own RPATH.
          shellHook = ''
            unset LD_LIBRARY_PATH
          '';
        };
      });
    };
}
