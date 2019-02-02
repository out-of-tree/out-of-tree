# out-of-tree/qemu

Qemu wrapper for kernel-related CI tasks. Supports *GNU/Linux* and *macOS*.

Features:
* Uses upstream virtualization -- KVM in GNU/Linux and Hypervisor.framework in macOS.
* Run files and kernel modules directly from local filesystem. No need to copy byself!
* Run commands inside qemu virtual machine at the same way as you run in it locally.

## Installation

    $ go get code.dumpstack.io/tools/out-of-tree/qemu

### Generate root image

First of all we need to generate rootfs for run qemu.

#### Install qemu and docker

##### GNU/Linux

    $ sudo apt install -y qemu docker

##### macOS

Note: qemu on macOS since v2.12 (24 April 2018) supports Hypervisor.framework.

    $ brew install qemu
    $ brew cask install docker

#### Generate image

    $ cd $GOPATH/src/code.dumpstack.io/tools/out-of-tree/tools/qemu-debian-img
    $ ./bootstrap.sh

### Fill configuration file

    $ $EDITOR $GOPATH/src/code.dumpstack.io/tools/out-of-tree/qemu/test.config.go

### Run tests

    $ go test -v

## Usage

    $ go get code.dumpstack.io/tools/out-of-tree/qemu

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

    $ godoc code.dumpstack.io/tools/out-of-tree/qemu
