# out-of-tree

out-of-tree kernel {module, exploit} development tool

![Screenshot](https://cloudflare-ipfs.com/ipfs/QmUmfCPWjW83xboSwSbKq1YhtAPBWpVqZAeGk3UCJemvmU)

## Installation

Read [Qemu API](qemu/README.md).

### Generate kernels

    cd $GOPATH/src/github.com/jollheef/out-of-tree/tools/kernel-factory
    ./bootstrap.sh # more than 6-8 hours for all kernels

### "I just want to see how it works"

If you already have Go, Qemu, Vagrant and Docker installed, there's cross-platform installation checklist:

    $ go get github.com/jollheef/out-of-tree
    $ cd $GOPATH/src/github.com/jollheef/out-of-tree/tools/qemu-debian-img/
    $ ./bootstrap.sh
    $ cd ../kernel-factory
    $ rm -rf {Debian,CentOS,Ubuntu/{14.04,18.04}} # speed up :)
    $ ./bootstrap.sh
    $ # wait several hours...
    $ cd ../../examples/kernel-exploit
    $ # test kernel exploit
    $ out-of-tree
    $ cd ../kernel-module
    $ # test kernel module
    $ out-of-tree
