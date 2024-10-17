#!/usr/bin/env python3

import os
import subprocess

script_dir = os.path.dirname(os.path.realpath(__file__))
os.chdir(script_dir)

releases = [
    ('12.04', 'precise', 'http://old-releases.ubuntu.com/ubuntu'),
    ('14.04', 'trusty', 'http://archive.ubuntu.com/ubuntu'),
    ('16.04', 'xenial', 'http://archive.ubuntu.com/ubuntu'),
    ('18.04', 'bionic', 'http://archive.ubuntu.com/ubuntu'),
    ('20.04', 'focal', 'http://archive.ubuntu.com/ubuntu'),
    ('22.04', 'jammy', 'http://archive.ubuntu.com/ubuntu'),
    ('24.04', 'noble', 'http://archive.ubuntu.com/ubuntu')
]

template = '''
FROM ubuntu:{version}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update
RUN apt install -y debootstrap qemu-utils
RUN apt install -y linux-image-generic

ENV TMPDIR=/tmp/ubuntu
ENV IMAGEDIR=/tmp/image
ENV IMAGE=/shared/out_of_tree_ubuntu_{img_version}.img
ENV REPOSITORY={repository}
ENV RELEASE={codename}

RUN mkdir $IMAGEDIR

# Must be executed with --privileged because of /dev/loop
CMD debootstrap --include=openssh-server,policykit-1 \
	$RELEASE $TMPDIR $REPOSITORY && \
	/shared/setup.sh $TMPDIR && \
	qemu-img create $IMAGE 2G && \
	mkfs.ext4 -F $IMAGE && \
	mount -o loop $IMAGE $IMAGEDIR && \
	cp -a $TMPDIR/* $IMAGEDIR/ && \
	umount $IMAGEDIR
'''

def run_cmd(cmd):
    print(f"+ {cmd}")
    subprocess.run(cmd, shell=True, check=True, executable='/bin/bash')

for version, codename, repository in releases:
    numeric_version = version.replace('.', '')
    img_version=version.replace(".","__")

    dockerfile_content = template.format(
        version=version,
        img_version=img_version,
        codename=codename,
        repository=repository,
        numeric_version=numeric_version)

    os.makedirs(str(version), exist_ok=True)
    with open(f"{version}/Dockerfile", "w") as dockerfile:
        dockerfile.write(dockerfile_content)

    run_cmd(f"podman build -t gen-ubuntu{numeric_version}-image {version}")
    run_cmd(f"rm -rf {version}")

    run_cmd(f"podman run --privileged -v {os.getcwd()}:/shared -t gen-ubuntu{numeric_version}-image")

    run_cmd(f"tar -Szcf out_of_tree_ubuntu_{img_version}.img.tar.gz out_of_tree_ubuntu_{img_version}.img")

