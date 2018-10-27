#!/bin/sh -eux
docker build -t gen-ubuntu1804-image .
docker run --privileged -v $(pwd):/shared -t gen-ubuntu1804-image
RUN="docker run -v $(pwd):/shared -t gen-ubuntu1804-image"
$RUN sh -c 'chmod 644 /vmlinuz && cp /vmlinuz /shared/ubuntu1804.vmlinuz'
$RUN sh -c 'cp /initrd.img /shared/ubuntu1804.initrd'
$RUN sh -c 'cp $(find /lib/modules -name test_static_key_base.ko) /shared/ubuntu1804.ko'
