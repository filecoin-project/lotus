#!/bin/bash

set -e

devnet=/lotus/tools/dockers/docker-examples/devnet

echo "Waiting for services to start up ..."
s6-svwait -u -t 10000 "$devnet/miner" "$devnet/seed"
lotus sync wait
echo ""
echo "Seed Peer (up=$(s6-svstat -u "$devnet/seed"))"
lotus_token=$(lotus auth create-token --perm=admin)
echo "FULLNODE_API_INFO=\"${lotus_token}:/ip4/127.0.0.1/tcp/1234/http\""
echo "# P2P Connection: /ip4/127.0.0.1/tcp/5678"
peer_id=$(lotus net id)
echo "# Net ID: ${peer_id})"
echo "# multiaddr: /ip4/127.0.0.1/tcp/5678/${peer_id}"
echo ""
echo "Miner (up=$(s6-svstat -u "$devnet/miner"))"
storage_token=$(lotus-storage-miner auth create-token --perm=admin)
echo "STORAGE_API_INFO=\"${storage_token}:/ip4/127.0.0.1/tcp/2345/http\""
