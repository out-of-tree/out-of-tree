#!/usr/bin/env bash

set -eux

cd $(dirname $(realpath $0))

for version in 7 8 9 10 11; do
    if [[ $version -eq 7 ]]; then
	release=wheezy
	last_version=7.11.0
    fi
    if [[ $version -eq 8 ]]; then
	release=jessie
	last_version=8.11.1
    fi
    if [[ $version -eq 9 ]]; then
	release=stretch
	last_version=9.13.0
    fi
    if [[ $version -eq 10 ]]; then
	release=buster
	last_version=10.13.0
    fi
    if [[ $version -eq 11 ]]; then
	release=bullseye
	last_version=11.6.0
    fi

    mkdir $version

    sed "s/_VERSION_/${version}/" Dockerfile.template >> $version/Dockerfile
    sed -i "s/_RELEASE_/${release}/" $version/Dockerfile

    repository=$(wget -q -O - https://cdimage.debian.org/mirror/cdimage/archive/${last_version}/amd64/jigdo-bd/debian-${last_version}-amd64-BD-1.jigdo | gunzip | awk -F= '/snapshot.debian.org/ {print $2}' | cut -d ' ' -f 1)

    sed -i "s;_REPOSITORY_;${repository};" $version/Dockerfile

    docker build -t gen-debian${version}-image $version
    rm -rf $version

    docker run --privileged -v $(pwd):/shared -t gen-debian${version}-image

    tar -Szcf out_of_tree_debian_${version}.img.tar.gz out_of_tree_debian_${version}.img
done
