{ pkgs ? (
    let
      inherit (builtins) fetchTree fromJSON readFile;
      inherit ((fromJSON (readFile ./flake.lock)).nodes) nixpkgs gomod2nix;
    in
    import (fetchTree nixpkgs.locked) {
      overlays = [
        (import "${fetchTree gomod2nix.locked}/overlay.nix")
      ];
    }
  )
  , lib
  , version
}:

pkgs.buildGoApplication rec {
  pname = "out-of-tree";

  inherit version;

  nativeBuildInputs = [ pkgs.makeWrapper ];

  src = ./.;
  pwd = ./.;

  doCheck = false;

  postFixup = ''
    wrapProgram $out/bin/out-of-tree \
      --prefix PATH : "${lib.makeBinPath [ pkgs.qemu pkgs.podman pkgs.openssl ]}"
  '';

  meta = with lib; {
    description = "kernel {module, exploit} development tool";
    homepage = "https://out-of-tree.io";
    maintainers = [ maintainers.dump_stack ];
    license = licenses.agpl3Plus;
  };
}
