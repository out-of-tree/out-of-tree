FROM ubuntu:14.04

RUN apt-get update

# for linux-image-3*-generic, so... CRUTCHES!
# E: Unable to locate package linux-image-3*-generic
ENV DEBIAN_FRONTEND=noninteractive
RUN apt search linux-image | grep -e linux-image-3 -e linux-image-4 | grep generic | cut -d '/' -f 1 | xargs apt install -y
RUN apt search linux-headers | grep -e linux-headers-3 -e linux-headers-4 | grep generic | cut -d '/' -f 1 | xargs apt install -y
