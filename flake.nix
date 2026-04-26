{
  description = "ptm — Pokemon Team Manager";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f {
        pkgs = nixpkgs.legacyPackages.${system};
        inherit system;
      });
    in {
      packages = forAllSystems ({ pkgs, ... }: {
        default = pkgs.callPackage ./default.nix { };
        ptm     = pkgs.callPackage ./default.nix { };
      });

      apps = forAllSystems ({ pkgs, system }: {
        default = {
          type = "app";
          program = "${self.packages.${system}.ptm}/bin/ptm";
        };
      });

      devShells = forAllSystems ({ pkgs, ... }: {
        default = pkgs.mkShell {
          packages = with pkgs; [ go gopls gotools ];
        };
      });

      # NixOS module for the web service
      nixosModules.ptm = { config, lib, pkgs, ... }:
        let
          cfg = config.services.ptm;
          ptmPkg = self.packages.${pkgs.system}.ptm;
        in {
          options.services.ptm = {
            enable = lib.mkEnableOption "ptm Pokemon Team Manager web UI";
            user   = lib.mkOption { type = lib.types.str; default = "alex"; };
            addr   = lib.mkOption { type = lib.types.str; default = "0.0.0.0:9133"; };
            dbPath = lib.mkOption { type = lib.types.str; };
          };

          config = lib.mkIf cfg.enable {
            systemd.user.services.ptm = {
              description = "ptm Pokemon Team Manager web UI";
              wantedBy = [ "default.target" ];
              after    = [ "default.target" ];
              serviceConfig = {
                ExecStart = "${ptmPkg}/bin/ptm web --addr ${cfg.addr} --db ${cfg.dbPath}";
                Restart    = "on-failure";
                RestartSec = 5;
              };
            };
          };
        };
    };
}
