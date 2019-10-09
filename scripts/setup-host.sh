#!/usr/bin/env bash

HOST=$1

scp scripts/daemon.service "${HOST}:/etc/systemd/system/lotus-daemon.service"
scp scripts/sminer.service "${HOST}:/etc/systemd/system/lotus-storage-miner.service"
