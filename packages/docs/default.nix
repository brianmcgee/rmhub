{ pkgs, ... }:
let
  inherit (pkgs) lib;
  site = pkgs.stdenvNoCC.mkDerivation {
    name = "rmhub-docs-site";

    src = lib.fileset.toSource {
      root = ../..;
      fileset = lib.fileset.unions [
        ../../book.toml
        ../../docs
        ../../theme
      ];
    };

    nativeBuildInputs = [ pkgs.mdbook ];

    buildPhase = ''
      runHook preBuild
      mdbook build --dest-dir "$out"
      runHook postBuild
    '';

    dontInstall = true;

    meta = {
      description = "The rmhub documentation as a static site";
      license = lib.licenses.agpl3Plus;
    };
  };
in
# `nix run .#docs` serves the site. `nix build .#docs.site` builds only
# the static files, for GitHub Pages.
pkgs.writeShellApplication {
  name = "rmhub-docs";

  runtimeInputs = [ pkgs.python3 ];

  text = ''
    port="''${1:-8000}"
    echo "The rmhub documentation is on http://127.0.0.1:$port/"
    exec python3 -m http.server "$port" --bind 127.0.0.1 --directory ${site}
  '';

  passthru = { inherit site; };

  meta = {
    description = "Serve the rmhub documentation on a local port";
    license = lib.licenses.agpl3Plus;
  };
}
