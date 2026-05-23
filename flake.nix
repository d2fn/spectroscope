{
  description = "spectroserver — in-process spectrogram view of metrics";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "spectroserver";
          version = "0.1.0";
          src = ./.;

          # Nix fetches every module listed in go.sum and assembles a vendor
          # tree inside the build sandbox — your source tree stays clean.
          # On first build (or after go.sum changes) leave this as fakeHash;
          # Nix will fail and print the correct hash to paste in.
          vendorHash = pkgs.lib.fakeHash;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
          ];
        };
      });
}
