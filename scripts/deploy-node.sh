#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'


HOST=$1

# upload binaries
# TODO: destroy

FILES_TO_SEND=(
	./lotus
	./lotus-miner
	scripts/lotus-daemon.service
	scripts/lotus-miner.service
)

rsync -P "${FILES_TO_SEND[@]}" "$HOST:~/lotus-stage/"

ssh "$HOST" 'bash -s' << 'EOF'
set -euo pipefail

systemctl stop lotus-miner
systemctl stop lotus-daemon
mkdir -p .lotus .lotusminer

cd "$HOME/lotus-stage/"
cp -f lotus lotus-miner /usr/local/bin
cp -f lotus-daemon.service /etc/systemd/system/lotus-daemon.service
cp -f lotus-miner.service /etc/systemd/system/lotus-miner.service

systemctl daemon-reload
systemctl start lotus-daemon
EOF


# setup miner actor
