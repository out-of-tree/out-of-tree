# Copyright 2018 Mikhail Klementev. All rights reserved.
# Use of this source code is governed by a AGPLv3 license
# (or later) that can be found in the LICENSE file.
#
# Usage:
#
#     $ docker build -t gen-centos7-image .
#     $ docker run --privileged -v $(pwd):/shared -t gen-centos7-image
#
# centos7.img will be created in current directory. You can change $(pwd) to
# different directory to use different destination for image.
#
FROM centos:7

RUN yum -y groupinstall "Development Tools"
RUN yum -y install qemu-img e2fsprogs

ENV TMPDIR=/tmp/centos

RUN yum --installroot=$TMPDIR \
        --releasever=7 \
        --disablerepo='*' \
        --enablerepo=base \
        -y groupinstall Base
RUN yum --installroot=$TMPDIR \
        --releasever=7 \
        --disablerepo='*' \
        --enablerepo=base \
        -y install openssh-server

RUN chroot $TMPDIR /bin/sh -c 'useradd -m user'
RUN sed -i 's/root:\*:/root::/' $TMPDIR/etc/shadow
RUN sed -i 's/user:!!:/user::/' $TMPDIR/etc/shadow
RUN sed -i '/PermitEmptyPasswords/d' $TMPDIR/etc/ssh/sshd_config
RUN echo PermitEmptyPasswords yes >> $TMPDIR/etc/ssh/sshd_config
RUN sed -i '/PermitRootLogin/d' $TMPDIR/etc/ssh/sshd_config
RUN echo PermitRootLogin yes >> $TMPDIR/etc/ssh/sshd_config

# network workaround
# FIXME kernel module compatibility issues
RUN chmod +x $TMPDIR/etc/rc.local
RUN echo 'find /lib/modules | grep e1000.ko | xargs insmod -f' >> $TMPDIR/etc/rc.local
RUN echo 'dhclient' >> $TMPDIR/etc/rc.local

ENV IMAGEDIR=/tmp/image
ENV IMAGE=/shared/centos7.img

RUN mkdir $IMAGEDIR

# Must be runned with --privileged because of /dev/loop
CMD qemu-img create $IMAGE 2G && \
	mkfs.ext4 -F $IMAGE && \
	mount -o loop $IMAGE $IMAGEDIR && \
	cp -a $TMPDIR/* $IMAGEDIR/ && \
	umount $IMAGEDIR
