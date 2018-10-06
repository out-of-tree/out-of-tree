#!/bin/sh
mkdir output
find | grep Docker | sed 's/Dockerfile//' | while read DOCKER; do
    CONTAINER_NAME=$(echo $DOCKER | sed -e 's;/;;g' -e 's;\.;;g' -e 's;\(.*\);\L\1;')
    docker build -t ${CONTAINER_NAME} ${DOCKER}
    CONTAINER_ID=$(docker ps -a | grep ${CONTAINER_NAME} | awk '{print $1}' | head -n 1)
    docker cp ${CONTAINER_ID}:/boot/. output/
done
find output -type f | grep  -v init | grep -v '/vmlinuz' | xargs rm
find output/* -type d | xargs rm -rf
