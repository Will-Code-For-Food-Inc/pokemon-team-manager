{ lib, buildGoModule, fetchFromGitHub }:

buildGoModule {
  pname = "ptm";
  version = "unstable";

  src = ./.;

  vendorHash = null;

  CGO_ENABLED = "0";

  ldflags = [ "-s" "-w" ];

  meta = {
    description = "Pokemon Team Manager — VGC team builder MCP server and web UI";
    homepage = "https://github.com/Will-Code-For-Food-Inc/pokemon-team-manager";
    license = lib.licenses.mit;
    mainProgram = "ptm";
  };
}
