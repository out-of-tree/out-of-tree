# Copyright 2018 Mikhail Klementev. All rights reserved.
# Use of this source code is governed by a AGPLv3 license
# (or later) that can be found in the LICENSE file.
#
# Usage:
#
#     $ docker build -t gen-ubuntu1804-image .
#     $ docker run --privileged -v $(pwd):/shared -t gen-ubuntu1804-image
#
# centos7.img will be created in current directory. You can change $(pwd) to
# different directory to use different destination for image.
#
FROM ubuntu:18.04

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update
RUN apt install -y debootstrap qemu
RUN apt install -y linux-image-generic

ENV TMPDIR=/tmp/ubuntu
ENV IMAGEDIR=/tmp/image
ENV IMAGE=/shared/ubuntu1804.img
ENV REPOSITORY=http://archive.ubuntu.com/ubuntu
ENV RELEASE=bionic

RUN mkdir $IMAGEDIR

# Must be runned with --privileged because of /dev/loop
CMD debootstrap --include=openssh-server $RELEASE $TMPDIR $REPOSITORY && \
	/shared/setup.sh $TMPDIR && \
	qemu-img create $IMAGE 2G && \
	mkfs.ext4 -F $IMAGE && \
	mount -o loop $IMAGE $IMAGEDIR && \
	cp -a $TMPDIR/* $IMAGEDIR/ && \
	umount $IMAGEDIR
