#!/usr/bin/env bash

set -eux

cd $(dirname $(realpath $0))

for version in 6 7 8 9; do
    mkdir $version

    sed "s/_VERSION_/${version}/" Dockerfile.template >> $version/Dockerfile

    if [[ $version -eq 6 ]]; then
	sed -i 's/baseos_latest/u10_base/' $version/Dockerfile
	sed -i '/appstream/d' $version/Dockerfile
    fi
    if [[ $version -eq 7 ]]; then
	sed -i 's/baseos_latest/u9_base/' $version/Dockerfile
	sed -i '/appstream/d' $version/Dockerfile
    fi

    podman build -t gen-oraclelinux${version}-image $version
    rm -rf $version

    podman run --privileged -v $(pwd):/shared -t gen-oraclelinux${version}-image

    tar -Szcf out_of_tree_oraclelinux_${version}.img.tar.gz out_of_tree_oraclelinux_${version}.img
done
