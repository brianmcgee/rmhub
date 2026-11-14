{
  flake,
  inputs,
  pkgs,
  ...
}:
let
  treefmtEval = inputs.treefmt-nix.lib.evalModule pkgs {
    projectRootFile = "flake.nix";

    programs.nixfmt.enable = true;
    programs.deadnix.enable = true;
    programs.statix.enable = true;
    programs.gofumpt.enable = true;
    programs.goimports.enable = true;
  };
  formatter = treefmtEval.config.build.wrapper;
in
formatter
// {
  passthru = formatter.passthru // {
    tests.check = treefmtEval.config.build.check flake;
  };
}
