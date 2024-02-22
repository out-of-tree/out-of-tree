#!/usr/bin/env bash

set -eu

id

df -h

sudo systemd-run --wait rm -rf \
     /usr/share/az* \
     /usr/share/dotnet \
     /usr/share/gradle* \
     /usr/share/miniconda \
     /usr/share/swift \
     /var/lib/gems \
     /var/lib/mysql \
     /var/lib/snapd \
     /opt/hostedtoolcache/CodeQL \
     /opt/hostedtoolcache/Java_Temurin-Hotspot_jdk

sudo fstrim /

df -h
