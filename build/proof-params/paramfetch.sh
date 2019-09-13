#!/usr/bin/env bash

set -euo pipefail

IPGET_PARAMS="--node=spawn -p=/ip4/138.201.67.219/tcp/4002/ws/ipfs/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA -p=/ip4/138.201.67.218/tcp/4002/ws/ipfs/QmbVWZQhCGrS7DhgLqWbgvdmKN7JueKCREVanfnVpgyq8x -p=/ip4/94.130.135.167/tcp/4002/ws/ipfs/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE -p=/ip4/138.201.68.74/tcp/4001/ipfs/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR -p=/ip4/138.201.67.220/tcp/4001/ipfs/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i"
OUT_DIR="/var/tmp/filecoin-proof-parameters"
PARAMS="build/proof-params/parameters.json"

mkdir -p $OUT_DIR
jq '. | to_entries | map("'$OUT_DIR'/\(.key) \(.value.cid) \(.value.digest)") | .[]' --raw-output $PARAMS | \
	while read -r dest cid digest; do
		if [[ -f "$dest" ]]; then
			b2=$(b2sum "$dest" | head -c 32)
			if [[ "$digest" == "$b2" ]]; then
				echo "$dest exists and has correct hash"
				continue
			else
				echo "$dest has incorrect hash"
				rm -f "$dest"
			fi
		fi
		echo "downloading $dest"
		./bin/ipget $IPGET_PARAMS -o "$dest" "$cid"
	done
