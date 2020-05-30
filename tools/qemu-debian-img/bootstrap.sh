#!/bin/sh -eux
cd $(dirname $(realpath $0))

docker build -t gen-ubuntu2004-image .
docker run --privileged -v $(pwd):/shared -t gen-ubuntu2004-image
RUN="docker run -v $(pwd):/shared -t gen-ubuntu2004-image"
$RUN sh -c 'chmod 644 /boot/vmlinuz && cp /boot/vmlinuz /shared/ubuntu2004.vmlinuz'
$RUN sh -c 'cp /boot/initrd.img /shared/ubuntu2004.initrd'
$RUN sh -c 'cp $(find /lib/modules -name test_static_key_base.ko) /shared/ubuntu2004.ko'
