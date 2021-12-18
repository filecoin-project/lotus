#!/usr/bin/env bash

# This script is executed by packer to setup the image.
# When this script is run, packer will have already copied binaries into the home directory of
# whichever user it has access too. This script is executed from within the home directory of that
# user. Bear in mind that different cloud providers, and different images on the same cloud
# provider will have a different initial user account.

set -x

# Become root, if we aren't already.
# Docker images will already be root. AMIs will have an SSH user account.
if [ x$UID != x0 ]
then
	printf -v cmd_str '%q ' "$0" "$@"
	exec sudo su -c "$cmd_str"
fi

set -e

MANAGED_FILES=(
	/etc/motd
)

# this is required on digitalocean, which does not have snap seeded correctly at this phase.
apt update
apt reinstall snapd

snap install lotus-filecoin

snap alias lotus-filecoin.lotus lotus
snap alias lotus-filecoin.lotus-miner lotus-miner
snap alias lotus-filecoin.lotus-miner lotus-worker

#snap stop lotus-filecoin.lotus-daemon

# Setup firewall
yes | ufw enable
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh

set +e

curl -L https://raw.githubusercontent.com/digitalocean/marketplace-partners/master/scripts/90-cleanup.sh | bash
