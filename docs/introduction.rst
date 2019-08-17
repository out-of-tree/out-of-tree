Introduction
============

*out-of-tree* is written in *Go*, it uses *Docker* for generating
kernel/filesystem images and *Qemu* for virtualization.

Also it possible to generate kernels from the host system and use the
custom one.

*out-of-tree* supports *GNU/Linux* (usually it's tested on NixOS and
latest Ubuntu LTS) and *macOS*. Technically all systems that supported
by Go, Docker, and Qemu must work well. Create the issue if you'll
notice any issue in integration for your operating system.

All *Qemu* interaction is stateless.

*out-of-tree* is allow and require metadata (``.out-of-tree.toml``)
for work. TOML (Tom's Obvious, Minimal Language) is used for kernel
module/exploit description.

``.out-of-tree.toml`` is mandatory, you need to have in the current
directory (usually, it's a project of kernel module/exploit) or use
the ``--path`` flag.

Files
-----

All data is stored in ``~/.out-of-tree/``.

- *db.sqlite* contains logs related to run with ``out-of-tree pew``,
  debug mode (``out-of-tree debug``) is not store any data.

- *images* used for filesystem images (rootfs images that used for
  ``qemu -hda ...``) that can be generated with the
  ``tools/qemu-*-img/...``.

- *kernels* stores all kernel ``vmlinuz/initrd/config/...`` files that
  generated previously with a some *Docker magic*.

- *kernels.toml* contains metadata for generated kernels. It's not
  supposed to be edited by hands.

- *kernels.user.toml* is default path for custom kernels definition.

- *Ubuntu* (or *Centos*/*Debian*/...) is the Dockerfiles tree
  (DistroName/DistroVersion/Dockerfile). Each Dockerfile contains a
  base layer and incrementally updated list of kernels that must be
  installed.

Overview
---------

*out-of-tree* creating debugging environment based on **defined** kernels::

    $ out-of-tree debug --kernel 'Ubuntu:4.15.0-58-generic'
    [*] KASLR SMEP SMAP
    [*] gdb runned on tcp::1234
    [*] build result copied to /tmp/exploit

    ssh -o StrictHostKeyChecking=no -p 29308 root@127.133.45.236
    gdb /usr/lib/debug/boot/vmlinux-4.15.0-58-generic -ex 'target remote tcp::1234'

    out-of-tree> help
    help    : print this help message
    log     : print qemu log
    clog    : print qemu log and cleanup buffer
    cleanup : cleanup qemu log buffer
    ssh     : print arguments to ssh command
    quit    : quit
    out-of-tree>

*out-of-tree* uses three stages for automated runs:

- Build

  - Inside the docker container (default).
  - Binary version (de facto skip stage).
  - On host.

- Run

  - Insmod for the kernel module.
  - This step is skipped for exploits.

- Test

  - Run the test.sh script on the target machine.
  - Test script is run from *root* for the kernel module.
  - Test script is run from *user* for the kernel exploit.
  - Test script for the kernel module is fully custom (only return
    value is checked).
  - Test script for the kernel exploit receives two parameters:

    - Path to exploit
    - Path to file that must be created with root privileges.

Security
--------

*out-of-tree* is not supposed to be used on multi-user systems or with
an untrusted input.

Meanwhile, all modern hypervisors are supporting nested
virtualization, which means you can use it for isolating *out-of-tree*
if you want to work with an untrusted input (e.g. with a mass-scale
testing public proofs-of-concept).
