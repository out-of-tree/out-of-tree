[![Ubuntu](https://github.com/out-of-tree/out-of-tree/actions/workflows/ubuntu.yml/badge.svg)](https://github.com/out-of-tree/out-of-tree/actions/workflows/ubuntu.yml)
[![E2E](https://github.com/out-of-tree/out-of-tree/actions/workflows/e2e.yml/badge.svg)](https://github.com/out-of-tree/out-of-tree/actions/workflows/e2e.yml)
[![Documentation Status](https://readthedocs.org/projects/out-of-tree/badge/?version=latest)](https://out-of-tree.readthedocs.io/en/latest/?badge=latest)

# [out-of-tree](https://out-of-tree.io)

*out-of-tree* is the kernel {module, exploit} development tool.

*out-of-tree* was created to reduce the complexity of the environment for developing, testing and debugging Linux kernel exploits and out-of-tree kernel modules (hence the name "out-of-tree").

## Installation

### GNU/Linux (with [Nix](https://nixos.org/nix/))

    sudo apt install podman || sudo dnf install podman

    curl -L https://nixos.org/nix/install | sh
    mkdir -p ~/.config/nix
    echo "experimental-features = nix-command flakes" >> ~/.config/nix/nix.conf

    # stable
    nix profile install nixpkgs#out-of-tree

    # latest
    nix profile install git+https://code.dumpstack.io/tools/out-of-tree

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
