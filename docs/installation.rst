Installation (from source)
============

OS/Distro-specific
==================

Ubuntu
------

Install dependencies::

  $ sudo snap install go --classic
  $ sudo snap install docker
  $ sudo apt install qemu-system-x86 build-essential gdb

macOS
-----

Install dependencies::

  $ brew install go qemu
  $ brew cask install docker

NixOS
-----

There's a minimal configuration that you need to apply::

  #!nix
  { config, pkgs, ... }:
  {
    virtualisation.docker.enable = true;
    virtualisation.libvirtd.enable = true;
    environment.systemPackages = with pkgs; [
      go git
    ];
  }

Common
======

Setup Go environment::

  $ echo 'export GOPATH=$HOME' >> ~/.bashrc
  $ echo 'export PATH=$PATH:$HOME/bin' >> ~/.bashrc
  $ source ~/.bashrc

Build *out-of-tree*::

  $ go get -u code.dumpstack.io/tools/out-of-tree

.. note::
  On a GNU/Linux you need to add your user to docker group if you want
  to use *out-of-tree* without sudo. Note that this has a **serious**
  security implications. Check *Docker* documentation for more
  information.

Test that everything works::

  $ cd $GOPATH/src/code.dumpstack.io/tools/out-of-tree/examples/kernel-exploit
  $ out-of-tree kernel autogen --max=1
  $ out-of-tree pew --max=1

Enjoy!
