[![Codacy Badge](https://api.codacy.com/project/badge/Grade/aba4aad2046b4d1a9a99cf98e22c018b)](https://app.codacy.com/app/jollheef/out-of-tree?utm_source=github.com&utm_medium=referral&utm_content=jollheef/out-of-tree&utm_campaign=Badge_Grade_Dashboard)
[![Go Report Card](https://goreportcard.com/badge/code.dumpstack.io/tools/out-of-tree)](https://goreportcard.com/report/code.dumpstack.io/tools/out-of-tree)
[![Documentation Status](https://readthedocs.org/projects/out-of-tree/badge/?version=latest)](https://out-of-tree.readthedocs.io/en/latest/?badge=latest)

# [out-of-tree](https://out-of-tree.io)

out-of-tree kernel {module, exploit} development tool

out-of-tree is for automating some routine actions for creating development environments for debugging kernel modules and exploits, generating reliability statistics for exploits, and also provides the ability to easily integrate into CI (Continuous Integration).

![Screenshot](https://cloudflare-ipfs.com/ipfs/Qmb88fgdDjbWkxz91sWsgmoZZNfVThnCtj37u3mF2s3T3T)

## Installation

### GNU/Linux (with [Nix](https://nixos.org/nix/))

    $ curl -fsSL https://get.docker.com | sh
	$ sudo usermod -aG docker user && newgrp docker
    $ curl -L https://nixos.org/nix/install | sh
    $ nix-env -iA nixpkgs.out-of-tree # Note: may not be up to date immediately, in this case consider installing from source

Note that adding a user to group *docker* has serious security implications. Check Docker documentation for more information.

### macOS

Note: case-insensitive FS is required for the ~/.out-of-tree directory.

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
