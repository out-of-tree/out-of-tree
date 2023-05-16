{ pkgs ? import <nixpkgs> {} }:

with pkgs; mkShell {
  packages = [ go gcc qemu podman ];
}
