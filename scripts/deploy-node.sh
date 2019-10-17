#!/usr/bin/env bash

HOST=$1

# upload binaries
# TODO: destroy

ssh "$HOST" 'systemctl stop lotus-storage-miner'
ssh "$HOST" 'systemctl stop lotus-daemon'

ssh "$HOST" 'mkdir -p .lotus .lotusstorage' &
scp "./lotus"  "$HOST:/usr/local/bin" &
scp "./lotus-storage-miner"  "$HOST:/usr/local/bin" &
scp -C scripts/lotus-daemon.service "${HOST}:/etc/systemd/system/lotus-daemon.service" &
scp -C scripts/louts-miner.service "${HOST}:/etc/systemd/system/lotus-storage-miner.service" &
wait

ssh "$HOST" 'systemctl daemon-reload'
ssh "$HOST" 'systemctl start lotus-daemon' &
wait


# setup miner actor
