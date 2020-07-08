#!/usr/bin/env bash

HOST=$1

ssh "$HOST" '[ -e ~/.lotusminer/token ]' && exit 0

ssh "$HOST" 'lotus wallet new bls > addr'
ssh "$HOST" 'curl http://147.75.80.29:777/sendcoll?address=$(cat addr)' &
ssh "$HOST" 'curl http://147.75.80.29:777/sendcoll?address=$(cat addr)' &
ssh "$HOST" 'curl http://147.75.80.29:777/send?address=$(cat addr)' &
wait

echo "SYNC WAIT"
sleep 30

ssh "$HOST" 'lotus sync wait'
ssh "$HOST" 'lotus-miner init --owner=$(cat addr)'
ssh "$HOST" 'systemctl start lotus-miner' &
