#!/usr/bin/env bash

log() {
  echo -e "\e[33m$1\e[39m"
}

host=$1

log "> Deploying bootstrap node $host"
log "Stopping lotus daemon"

ssh "$host" 'systemctl stop lotus-daemon' &
ssh "$host" 'systemctl stop lotus-miner' &

wait

ssh "$host" 'rm -rf .lotus' &
ssh "$host" 'rm -rf .lotusminer' &

scp -C lotus "${host}":/usr/local/bin/lotus &
scp -C lotus-miner "${host}":/usr/local/bin/lotus-miner &

wait

log 'Initializing repo'

ssh "$host" 'systemctl start lotus-daemon'
scp scripts/bootstrap.toml "${host}:.lotus/config.toml"
ssh "$host" "echo -e '[Metrics]\nNickname=\"Boot-$host\"' >> .lotus/config.toml"
ssh "$host" 'systemctl restart lotus-daemon'

log 'Extracting addr info'

ssh "$host" 'lotus net listen' | grep -v '/10' | grep -v '/127' >> build/bootstrap/bootstrappers.pi
