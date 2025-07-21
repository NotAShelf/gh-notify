{
  lib,
  buildGoModule,
  ...
}:
buildGoModule {
  pname = "gh-notify";
  version = "0.2.0";

  src = let
    fs = lib.fileset;
    s = ../.;
  in
    fs.toSource {
      root = s;
      fileset = fs.unions (map (dir: (s + /${dir})) [
        "main.go"
        "go.mod"
        "go.sum"
      ]);
    };

  vendorHash = "sha256-ORE8a3l9OzFRBgaaAGlaq1taTT78Zp26gbui2ZaTgbQ=";

  ldflags = ["-s" "-w"];

  meta = {
    description = "GitHub CLI extension to display GitHub notifications";
    homepage = "https://github.com/notashelf/gh-notify";
    license = lib.licenses.mpl20;
    mainProgram = "gh-notify";
    maintainers = [lib.maintainers.NotAShelf];
  };
}
