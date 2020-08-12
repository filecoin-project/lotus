#!/usr/bin/env bash

HOST=$1

scp scripts/lotus-daemon.service "${HOST}:/etc/systemd/system/lotus-daemon.service"
scp scripts/lotus-miner.service "${HOST}:/etc/systemd/system/lotus-miner.service"
