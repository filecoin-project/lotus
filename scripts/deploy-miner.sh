#!/usr/bin/env bash

HOST=$1

# upload binaries
# TODO: destroy

ssh "$HOST" 'systemctl stop lotus-storage-miner'
ssh "$HOST" 'systemctl stop lotus-daemon'

ssh "$HOST" 'mkdir -p .lotus .lotusstorage' &
scp "./lotus"  "$HOST:/usr/local/bin" &
scp "./lotus-storage-miner"  "$HOST:/usr/local/bin" &
scp -C scripts/daemon.service "${HOST}:/etc/systemd/system/lotus-daemon.service" &
scp -C scripts/sminer.service "${HOST}:/etc/systemd/system/lotus-storage-miner.service" &
wait

ssh "$HOST" 'systemctl daemon-reload'
ssh "$HOST" 'systemctl start lotus-daemon' &
ssh "$HOST" 'systemctl start lotus-storage-miner' &
wait

ssh "$HOST" 'lotus wallet new bls > addr'
ssh "$HOST" 'curl http://147.75.80.29:777/sendcoll?address=$(cat addr)' &
ssh "$HOST" 'curl http://147.75.80.29:777/sendcoll?address=$(cat addr)' &
ssh "$HOST" 'curl http://147.75.80.29:777/send?address=$(cat addr)' &
wait

echo "SYNC WAIT"

ssh "$HOST" 'lotus sync wait'
ssh "$HOST" 'lotus-storage-miner init --owner=$(cat addr)'
ssh "$HOST" 'systemctl start lotus-storage-miner' &

# setup miner actor
