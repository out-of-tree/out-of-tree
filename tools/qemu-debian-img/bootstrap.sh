#!/bin/sh -eux
cd $(dirname $(realpath $0))

docker build -t gen-ubuntu2204-image .
docker run --privileged -v $(pwd):/shared -t gen-ubuntu2204-image
RUN="docker run -v $(pwd):/shared -t gen-ubuntu2204-image"
$RUN sh -c 'chmod 644 /boot/vmlinuz && cp /boot/vmlinuz /shared/ubuntu2204.vmlinuz'
$RUN sh -c 'cp /boot/initrd.img /shared/ubuntu2204.initrd'
$RUN sh -c 'cp $(find /lib/modules -name test_bpf.ko) /shared/ubuntu2204.ko'
