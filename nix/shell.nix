{
  mkShellNoCC,
  go,
  gopls,
  delve,
}:
mkShellNoCC {
  name = "go";
  packages = [
    delve
    go
    gopls
  ];
}
