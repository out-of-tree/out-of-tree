# [out-of-tree](https://out-of-tree.io)

out-of-tree kernel {module, exploit} development tool

![Screenshot](https://cloudflare-ipfs.com/ipfs/Qmb88fgdDjbWkxz91sWsgmoZZNfVThnCtj37u3mF2s3T3T)

## Installation

    $ go get github.com/jollheef/out-of-tree
    $ out-of-tree bootstrap

Then you can check it on kernel module example:

    $ cd $GOPATH/github.com/jollheef/out-of-tree/examples/kernel-module
    $ out-of-tree kernel autogen # generate kernels based on .out-of-tree.toml
    $ out-of-tree pew

## Generate all kernels (does not required if you dont need to use `--guess`)

    $ cd $GOPATH/src/github.com/jollheef/out-of-tree/tools/kernel-factory
    $ ./bootstrap.sh # more than 6-8 hours for all kernels

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

## Development

Read [Qemu API](qemu/README.md).

### Generate images

    $ cd $GOPATH/src/github.com/jollheef/out-of-tree/tools/qemu-debian-img/
    $ docker run --privileged -v $(pwd):/shared -e IMAGE=/shared/ubuntu1404.img -e RELEASE=trusty -t gen-ubuntu1804-image
    $ docker run --privileged -v $(pwd):/shared -e IMAGE=/shared/ubuntu1604.img -e RELEASE=xenial -t gen-ubuntu1804-image
