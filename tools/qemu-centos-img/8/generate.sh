#!/bin/sh

set -eux

cd "$(dirname "$0")"

sudo podman build -t gen-centos8-image .
sudo podman run --privileged -v $(pwd):/shared -t gen-centos8-image
tar -Szcf out_of_tree_centos_8.img.tar.gz out_of_tree_centos_8.img
