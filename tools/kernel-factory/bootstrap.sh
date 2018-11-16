#!/bin/sh -eux
mkdir -p output
echo > output/kernels.toml
find | grep Docker | sed 's/Dockerfile//' | while read DOCKER; do
    CONTAINER_NAME=$(echo $DOCKER | sed -e 's;/;;g' -e 's;\.;;g' -e 's;\(.*\);\L\1;')
    docker build -t ${CONTAINER_NAME} ${DOCKER}
    docker run ${CONTAINER_NAME} bash -c 'ls /boot'
    CONTAINER_ID=$(docker ps -a | grep ${CONTAINER_NAME} | awk '{print $1}' | head -n 1)
    docker cp ${CONTAINER_ID}:/boot/. output/
    DISTRO_NAME=$(echo $DOCKER | cut -d '/' -f 2)
    DISTRO_VER=$(echo $DOCKER | cut -d '/' -f 3)

    BOOT_FILES="$(docker run $CONTAINER_NAME ls /boot)"
    for KERNEL_RELEASE in $(docker run $CONTAINER_NAME ls /lib/modules); do
	echo '[[Kernels]]' >> output/kernels.toml
	echo 'distro_type =' \"$DISTRO_NAME\" >> output/kernels.toml
	echo 'distro_release =' \"$DISTRO_VER\" >> output/kernels.toml
	echo 'kernel_release =' \"$KERNEL_RELEASE\" >> output/kernels.toml
	echo 'container_name =' \"$CONTAINER_NAME\" >> output/kernels.toml
	KERNEL_PATH=$(echo $BOOT_FILES | sed  's/ /\n/g' | grep $KERNEL_RELEASE | grep vmlinuz)
	echo 'kernel_path =' \"$(realpath output/$KERNEL_PATH)\" >> output/kernels.toml
	INITRD_PATH=$(echo $BOOT_FILES | sed  's/ /\n/g' | grep $KERNEL_RELEASE | grep init)
	echo 'initrd_path =' \"$(realpath output/$INITRD_PATH)\" >> output/kernels.toml
	ROOTFS_PATH=$(realpath $DOCKER/Image)
	echo 'root_f_s =' \"$ROOTFS_PATH\" >> output/kernels.toml
	echo >> output/kernels.toml
    done
done
rm -rf output/grub
