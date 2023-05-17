[![Codacy Badge](https://api.codacy.com/project/badge/Grade/aba4aad2046b4d1a9a99cf98e22c018b)](https://app.codacy.com/app/jollheef/out-of-tree?utm_source=github.com&utm_medium=referral&utm_content=jollheef/out-of-tree&utm_campaign=Badge_Grade_Dashboard)
[![Go Report Card](https://goreportcard.com/badge/code.dumpstack.io/tools/out-of-tree)](https://goreportcard.com/report/code.dumpstack.io/tools/out-of-tree)
[![Documentation Status](https://readthedocs.org/projects/out-of-tree/badge/?version=latest)](https://out-of-tree.readthedocs.io/en/latest/?badge=latest)

# [out-of-tree](https://out-of-tree.io)

*out-of-tree* is the kernel {module, exploit} development tool.

*out-of-tree* was created to reduce the complexity of the environment for developing, testing and debugging Linux kernel exploits and out-of-tree kernel modules (hence the name "out-of-tree").

![Screenshot](https://cloudflare-ipfs.com/ipfs/Qmb88fgdDjbWkxz91sWsgmoZZNfVThnCtj37u3mF2s3T3T)

## Installation

### GNU/Linux (with [Nix](https://nixos.org/nix/))

    apt install podman || dnf install podman
    curl -L https://nixos.org/nix/install | sh
    # stable
    nix-env -iA nixpkgs.out-of-tree
    # latest
    nix build --extra-experimental-features 'nix-command flakes' git+https://code.dumpstack.io/tools/out-of-tree

### macOS

Note: case-sensitive FS is required for the ~/.out-of-tree directory.

    $ brew install podman
    $ podman machine stop || true
    $ podman machine rm || true
    $ podman machine init --cpus=4 --memory=4096 -v $HOME:$HOME
    $ podman machine start
    $ brew tap out-of-tree/repo
    $ brew install out-of-tree

Read [documentation](https://out-of-tree.readthedocs.io) for further info.

## Examples

Generate all Ubuntu 22.04 kernels:

    $ out-of-tree kernel genall --distro=Ubuntu --ver=22.04

Run tests based on .out-of-tree.toml definitions:

    $ out-of-tree pew

Test with a specific kernel:

    $ out-of-tree pew --kernel='Ubuntu:5.4.0-29-generic'

Run debug environment:

    $ out-of-tree debug --kernel='Ubuntu:5.4.0-29-generic'
