[![Codacy Badge](https://api.codacy.com/project/badge/Grade/aba4aad2046b4d1a9a99cf98e22c018b)](https://app.codacy.com/app/jollheef/out-of-tree?utm_source=github.com&utm_medium=referral&utm_content=jollheef/out-of-tree&utm_campaign=Badge_Grade_Dashboard)
[![Build Status](https://travis-ci.com/jollheef/out-of-tree.svg?branch=master)](https://travis-ci.com/jollheef/out-of-tree)
[![Go Report Card](https://goreportcard.com/badge/code.dumpstack.io/tools/out-of-tree)](https://goreportcard.com/report/code.dumpstack.io/tools/out-of-tree)
[![Documentation Status](https://readthedocs.org/projects/out-of-tree/badge/?version=latest)](https://out-of-tree.readthedocs.io/en/latest/?badge=latest)
[![Donate](https://img.shields.io/badge/donate-paypal-green.svg)](https://www.paypal.com/cgi-bin/webscr?cmd=_s-xclick&hosted_button_id=R8W2UQPZ5X5JE&source=url)
[![Donate](https://img.shields.io/badge/donate-bitcoin-green.svg)](https://blockchair.com/bitcoin/address/bc1q23fyuq7kmngrgqgp6yq9hk8a5q460f39m8nv87)

# [out-of-tree](https://out-of-tree.io)

out-of-tree kernel {module, exploit} development tool

out-of-tree is for automating some routine actions for creating development environments for debugging kernel modules and exploits, generating reliability statistics for exploits, and also provides the ability to easily integrate into CI (Continuous Integration).

![Screenshot](https://cloudflare-ipfs.com/ipfs/Qmb88fgdDjbWkxz91sWsgmoZZNfVThnCtj37u3mF2s3T3T)

## Requirements

[Qemu](https://www.qemu.org), [docker](https://docker.com) and [golang](https://golang.org) is required.

Also do not forget to set GOPATH and PATH e.g.:

    $ echo 'export GOPATH=$HOME' >> ~/.bashrc
    $ echo 'export PATH=$PATH:$HOME/bin' >> ~/.bashrc
    $ source ~/.bashrc

### Gentoo

    # emerge app-emulation/qemu app-emulation/docker dev-lang/go

### macOS

    $ brew install go qemu
    $ brew cask install docker

### Fedora

    $ sudo dnf install go qemu moby-engine

Also check out [docker post-installation steps](https://docs.docker.com/install/linux/linux-postinstall/).

## Build from source

    $ go get -u code.dumpstack.io/tools/out-of-tree

Then you can check it on kernel module example:

    $ cd $GOPATH/src/code.dumpstack.io/tools/out-of-tree/examples/kernel-module
    $ out-of-tree kernel autogen # generate kernels based on .out-of-tree.toml
    $ out-of-tree pew

## Examples

Run by absolute path

    $ out-of-tree --path /path/to/exploit/directory pew

Test only with one kernel:

    $ out-of-tree pew --kernel='Ubuntu:4.10.0-30-generic'

Run debug environment:

    $ out-of-tree debug --kernel='Ubuntu:4.10.0-30-generic'

Test binary module/exploit with implicit defined test ($BINARY_test)

    $ out-of-tree pew --binary /path/to/exploit

Test binary module/exploit with explicit defined test

    $ out-of-tree pew --binary /path/to/exploit --test /path/to/exploit_test

Guess work kernels:

    $ out-of-tree pew --guess

Use custom kernels config

    $ out-of-tree --kernels /path/to/kernels.toml pew

Generate all kernels

    $ out-of-tree kernel genall --distro Ubuntu --ver 16.04


## Troubleshooting

If anything happens that you cannot solve -- just remove `$HOME/.out-of-tree`.

But it'll be better if you'll write the bug report.

## Development

Read [Qemu API](qemu/README.md).

### Generate images

    $ cd $GOPATH/src/code.dumpstack.io/tools/out-of-tree/tools/qemu-debian-img/
    $ docker run --privileged -v $(pwd):/shared -e IMAGE=/shared/ubuntu1404.img -e RELEASE=trusty -t gen-ubuntu1804-image
    $ docker run --privileged -v $(pwd):/shared -e IMAGE=/shared/ubuntu1604.img -e RELEASE=xenial -t gen-ubuntu1804-image
