#!/bin/sh
cd "$(dirname "$0")"

sudo docker build -t gen-centos8-image .
sudo docker run --privileged -v $(pwd):/shared -t gen-centos8-image
tar -Szcf out_of_tree_centos_8.img.tar.gz out_of_tree_centos_8.img
