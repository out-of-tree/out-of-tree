FROM centos:8

RUN sed -i 's/enabled=1/enabled=0/' /etc/yum.repos.d/* || true
RUN echo -e '[8.5.2111-os]\nbaseurl=https://vault.centos.org/8.5.2111/BaseOS/$basearch/os/\ngpgcheck=0' >> /etc/yum.repos.d/oot.repo
RUN echo -e '[8.5.2111-appstream]\nbaseurl=https://vault.centos.org/8.5.2111/AppStream/$basearch/os/\ngpgcheck=0' >> /etc/yum.repos.d/oot.repo

RUN yum -y update
RUN yum -y groupinstall "Development Tools"
RUN yum -y install qemu-img e2fsprogs

ENV TMPDIR=/tmp/centos

RUN yum --installroot=$TMPDIR \
        --releasever=8 \
        -y groupinstall Base

RUN rm $TMPDIR/etc/yum.repos.d/*
RUN cp /etc/yum.repos.d/* $TMPDIR/etc/yum.repos.d/

RUN yum --installroot=$TMPDIR \
        --releasever=8 \
        -y install openssh-server openssh-clients yum dnf

RUN chroot $TMPDIR /bin/sh -c 'useradd -m user'
RUN sed -i 's/root:\*:/root::/' $TMPDIR/etc/shadow
RUN sed -i 's/user:!!:/user::/' $TMPDIR/etc/shadow
RUN sed -i '/PermitEmptyPasswords/d' $TMPDIR/etc/ssh/sshd_config
RUN echo PermitEmptyPasswords yes >> $TMPDIR/etc/ssh/sshd_config
RUN sed -i '/PermitRootLogin/d' $TMPDIR/etc/ssh/sshd_config
RUN echo PermitRootLogin yes >> $TMPDIR/etc/ssh/sshd_config

# network workaround
RUN chmod +x $TMPDIR/etc/rc.local
RUN echo 'dhclient' >> $TMPDIR/etc/rc.local

ENV IMAGEDIR=/tmp/image
ENV IMAGE=/shared/out_of_tree_centos_8.img

RUN mkdir $IMAGEDIR

# Must be executed with --privileged because of /dev/loop
CMD qemu-img create $IMAGE 2G && \
	mkfs.ext4 -F $IMAGE && \
	mount -o loop $IMAGE $IMAGEDIR && \
	cp -a $TMPDIR/* $IMAGEDIR/ && \
	umount $IMAGEDIR
