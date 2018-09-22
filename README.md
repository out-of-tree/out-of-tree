# go-qemu-kernel

Qemu wrapper for kernel-related CI tasks. Supports *GNU/Linux* and *macOS*.

Features:
* Uses upstream virtualization -- KVM in GNU/Linux and Hypervisor.framework in macOS.
* Run files and kernel modules directly from local filesystem. No need to copy byself!
* Run commands inside qemu virtual machine at the same way as you run in it locally.

## Installation

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

## Usage

    $ go get github.com/jollheef/go-qemu-kernel

Minimal example:

	kernel := qemu.Kernel{
		Name:       "Some kernel name",
		KernelPath: "/path/to/vmlinuz",
		InitrdPath: "/path/to/initrd", // if required
	}
	q, err := qemu.NewQemuSystem(qemu.X86_64, kernel, "/path/to/qcow2")
	if err != nil {
		log.Fatal(err)
	}

	if err = q.Start(); err != nil {
		log.Fatal(err)
	}
	defer q.Stop()

	output, err = q.Command("root", "echo Hello, World!")
	if err != nil {
		log.Fatal(err)
	}

	// output == "Hello, World!\n"

More information and list of all functions see at go documentation project, or just run locally:

    $ godoc github.com/jollheef/go-qemu-kernel
