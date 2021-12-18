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

MANAGED_FILES=(
	/etc/motd
)

snap install filecoin-lotus

snap alias lotus-filecoin.lotus lotus
snap alias lotus-filecoin.lotus-miner lotus-miner
snap alias lotus-filecoin.lotus-miner lotus-worker

# Setup firewall
yes | ufw enable
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh
