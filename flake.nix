{
  description = "A beautiful terminal UI for visualizing Git repository statistics";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      version = builtins.head (builtins.head (
        builtins.filter builtins.isList
          (builtins.split "pkgver=([^\n]+)" (builtins.readFile ./PKGBUILD))
      ));
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "gittop";
          inherit version;
          src = ./.;

          # Run `nix build` with this set to lib.fakeHash to get the real hash,
          # then replace it here.
          vendorHash = "sha256-f2B9vARgoZYqZa0P2HmsP+eHc1bZUSWSuHAr7AId6Lc=";

          postPatch = ''
            substituteInPlace go.mod --replace-fail 'go 1.26.1' 'go 1.25'
          '';

          ldflags = [ "-s" "-w" ];

          meta = with pkgs.lib; {
            description = "A beautiful terminal UI for visualizing Git repository statistics";
            homepage = "https://github.com/hjr265/gittop";
            license = licenses.bsd3;
            mainProgram = "gittop";
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            git
          ];
        };
      }
    );
}
