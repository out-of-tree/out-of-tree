#!/usr/bin/env bash

set -eu

df -h

sudo systemd-run --wait rm -rf \
     /usr/share/az* \
     /usr/share/dotnet \
     /usr/share/gradle* \
     /usr/share/miniconda \
     /usr/share/swift \
     /var/lib/gems \
     /var/lib/mysql \
     /var/lib/snapd

sudo fstrim /

df -h
