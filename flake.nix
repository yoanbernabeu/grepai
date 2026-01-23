{
  description = "grepai - AI-powered semantic code search tool";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      version = "0.18.0";

      mkGrepai = pkgs: pkgs.buildGoModule {
        pname = "grepai";
        inherit version;
        src = ./.;

        vendorHash = "sha256-B3E/Faqe4CkrVDsOsvYo8cxpGUn7pwJrmI1pCMBaEJk=";

        ldflags = [
          "-s"
          "-w"
          "-X main.version=${version}"
        ];

        meta = with pkgs.lib; {
          description = "AI-powered semantic code search tool";
          homepage = "https://github.com/yoanbernabeu/grepai";
          license = licenses.mit;
          mainProgram = "grepai";
        };
      };
    in
    {
      packages = forAllSystems (system: {
        grepai = mkGrepai nixpkgs.legacyPackages.${system};
        default = self.packages.${system}.grepai;
      });

      overlays.default = final: prev: {
        grepai = mkGrepai final;
      };

      devShells = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [ go gopls gotools go-tools ];
            shellHook = ''
              echo "grepai dev shell - Go $(go version | cut -d' ' -f3)"
            '';
          };
        }
      );
    };
}
