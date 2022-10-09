#!/bin/bash
rm -rf ~/.genesis-sectors

if [ $# -ne 1 ]
then
    echo "Provide the index of the validator to deploy as first argument. Starting from 0"
    exit 1
fi

INDEX=$1

# Config envs
export LOTUS_PATH=~/.lotus-local-net$INDEX
export LOTUS_MINER_PATH=~/.lotus-miner-local-net$INDEX
export LOTUS_SKIP_GENESIS_CHECK=_yes_
export CGO_CFLAGS_ALLOW="-D__BLST_PORTABLE__"
export CGO_CFLAGS="-D__BLST_PORTABLE__"

rm -rf ~/$LOTUS_PATH

# Uncomment to create a genesis template
# ./lotus-seed genesis new localnet.json
# ./lotus-seed pre-seal --sector-size 2KiB --num-sectors 2
# ./lotus-seed genesis add-miner localnet.json ~/.genesis-sectors/pre-seal-t01000.json

./lotus daemon --lotus-make-genesis=devgen.car --genesis-template=./scripts/mir/localnet.json --bootstrap=false --api=123$INDEX
