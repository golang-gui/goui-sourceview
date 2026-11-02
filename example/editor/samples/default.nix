# Original Nix sample for SourceView.
{ pkgs ? import <nixpkgs> {} }:
let
  name = "sourceview";
  enabled = true;
  greeting = "Hello ${if enabled then name else "editor"}";
in {
  inherit name greeting;
  source = ./editor;
  script = ''
    echo "${greeting}"
    echo "''${SHELL_VARIABLE}"
  '';
}
