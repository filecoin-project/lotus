#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'


HOST=$1

# upload binaries
# TODO: destroy

FILES_TO_SEND=(
	./lotus
	./lotus-storage-miner
	scripts/lotus-daemon.service
	scripts/louts-miner.service
)

rsync -P "${FILES_TO_SEND[@]}" "$HOST:~/lotus-stage/"

ssh "$HOST" 'bash -s' << 'EOF'
set -euo pipefail

systemctl stop lotus-storage-miner
systemctl stop lotus-daemon
mkdir -p .lotus .lotusstorage

cd "$HOME/lotus-stage/"
cp -f lotus lotus-storage-miner /usr/local/bin
cp -f lotus-daemon.service /etc/systemd/system/lotus-daemon.service
cp -f lotus-miner.service /etc/systemd/system/lotus-storage-miner.service

systemctl daemon-reload
systemctl start lotus-daemon
EOF


# setup miner actor
