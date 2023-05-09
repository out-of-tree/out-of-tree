#!/bin/sh -eux

for version in 6 7 8 9; do
    mkdir $version
    sed "s/_VERSION_/${version}/" Dockerfile.template >> $version/Dockerfile
    if [[ $version -eq 6 ]]; then
	sed -i 's/baseos_latest/u10_base/' $version/Dockerfile
    fi
    if [[ $version -eq 7 ]]; then
	sed -i 's/baseos_latest/u9_base/' $version/Dockerfile
    fi
    docker build -t gen-oraclelinux${version}-image $version
    rm -rf $version
    docker run --privileged -v $(pwd):/shared -t gen-oraclelinux${version}-image
done
