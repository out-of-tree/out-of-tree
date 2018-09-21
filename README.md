# go-qemu-kernel

Qemu wrapper for kernel-related CI tasks

## Usage

TODO

## Development

    $ go get github.com/jollheef/go-qemu-kernel

### Generate root image

First of all we need to generate rootfs for run qemu.

#### GNU/Linux

    $ sudo apt install -y debootstrap qemu
    $ sudo qemu-debian-img generate sid.img

#### macOS

Note: qemu on macOS since v2.12 (24 April 2018) supports Hypervisor.framework.

    $ brew install qemu

Because it's a very complicated to debootstrap qemu images from macOS,
preferred way is to use Vagrant with any hypervisor.

    $ brew cask install vagrant
    $ cd $GOPATH/src/github.com/jollheef/go-qemu-kernel/tools/qemu-debian-image
    $ vagrant up && vagrant destroy -f

bionic.img and bionic-vmlinuz will be created in current directory.

### Fill configuration file

    $ $EDITOR $GOPATH/src/github.com/jollheef/go-qemu-kernel/test.config.go

### Run tests

    $ go test -v
