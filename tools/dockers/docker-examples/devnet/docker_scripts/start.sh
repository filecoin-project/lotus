#!/bin/bash

config () {
    echo "[API]"
    echo "ListenAddress = \"/ip4/0.0.0.0/tcp/1234/http\""
    echo "[Libp2p]"
    echo "ListenAddresses = [\"/ip4/0.0.0.0/tcp/5678\", \"/ip6/::/tcp/5678\"]"
}

mkdir -p     /root/.lotus/
chmod -f u+w /root/.lotus/config.toml
config   >   /root/.lotus/config.toml
chmod -f  -w /root/.lotus/config.toml

set -e

echo "---"
echo "Fetching proving params"
echo "---"

lotus fetch-params \
  --proving-params=1024

echo "---"
echo "Presealing sectors"
echo "---"

mkdir -p /mnt/genesis-sectors
lotus-seed \
  --sectorbuilder-dir=/mnt/genesis-sectors \
  pre-seal \
  --sector-size=1024 \
  --num-sectors=2

echo "---"
echo "Starting services"
echo "---"

exec s6-svscan /devnet
