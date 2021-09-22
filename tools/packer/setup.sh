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

export DEBIAN_FRONTEND=noninteractive
apt-add-repository -y ppa:protocollabs/lotus
apt -y install lotus

# Install systemd and other files.
# Because packer doesn't copy files with root permisison,
# files are in the home directory of the ssh user. Copy
# these files into the right position.
for i in "${MANAGED_FILES[@]}"
do
	fn=$(basename $i)
	install -o root -g root -m 644 $fn $i
	rm $fn
done

# Enable services
systemctl daemon-reload
systemctl enable lotus-daemon

# Setup firewall
yes | ufw enable
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh
ufw allow 5678   #libp2p

# This takes a while on AWS, so generate some output
while sleep 120
do
	echo still running
done &

# Digitalocean Cleanup Script
curl -s https://raw.githubusercontent.com/digitalocean/marketplace-partners/master/scripts/90-cleanup.sh | bash
