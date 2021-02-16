#!/usr/bin/env bash

# This script is executed by packer to setup the image.
# When this script is run, packer will have already copied binaries into the home directory of
# whichever user it has access too. This script is executed from within the home directory of that
# user. Bear in mind that different cloud providers, and different images on the same cloud
# provider will have a different initial user account.

set -x

# Become root, if we aren't already.
# Docker images will already be root. AMIs will have an SSH user account.
UID=$(id -u)
if [ x$UID != x0 ]
then
	printf -v cmd_str '%q ' "$0" "$@"
	exec sudo su -c "$cmd_str"
fi

MANAGED_BINS=( lotus lotus-miner lotus-init.sh )
MANAGED_SYSTEMD=( lotus-daemon.service lotus-miner.service lotus-init.service )

# install libs.
apt update
apt -y install libhwloc15 ocl-icd-libopencl1
ln -s /usr/lib/x86_64-linux-gnu/libhwloc.so.15 /usr/lib/x86_64-linux-gnu/libhwloc.so.5

# Create lotus user
useradd -c "lotus system account" -r fc
install -o fc -g fc -d /var/lib/lotus
install -o fc -g fc -d /var/lib/lotus-miner

# Install software
for i in "${MANAGED_BINS[@]}"
do
	install -o root -g root -m 755 -t /usr/local/bin $i
	rm $i
done

# Install systemd units
for i in "${MANAGED_SYSTEMD[@]}"
do
	install -o root -g root -m 644 -t /lib/systemd/system $i
	rm $i
done

# Enable services
systemctl daemon-reload
systemctl enable lotus-daemon
