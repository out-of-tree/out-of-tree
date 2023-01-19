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
    $ nix-env -iA nixpkgs.out-of-tree

Note that adding a user to group *docker* has serious security implications. Check Docker documentation for more information.

### macOS

    $ brew cask install docker
    $ open --background -a Docker && sleep 1m
    $ brew tap jollheef/repo
    $ brew install out-of-tree

Read [documentation](https://out-of-tree.readthedocs.io) for further info.

## Examples

Run by absolute path

    $ out-of-tree --path /path/to/exploit/directory pew

Test only with one kernel:

    $ out-of-tree pew --kernel='Ubuntu:5.4.0-29-generic

Run debug environment:

    $ out-of-tree debug --kernel='Ubuntu:5.4.0-29-generic

Test binary module/exploit with implicit defined test ($BINARY_test)

    $ out-of-tree pew --binary /path/to/exploit

Test binary module/exploit with explicit defined test

    $ out-of-tree pew --binary /path/to/exploit --test /path/to/exploit_test

Guess work kernels:

    $ out-of-tree pew --guess

Use custom kernels config

    $ out-of-tree --kernels /path/to/kernels.toml pew

Generate all kernels

    $ out-of-tree kernel genall --distro Ubuntu --ver 22.04

## Development

Read [Qemu API](qemu/README.md).
