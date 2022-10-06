#!/bin/bash
rm -rf ~/.genesis-sectors
rm -rf ~/.lotus*

export LOTUS_PATH=~/.lotus-local-net
export LOTUS_MINER_PATH=~/.lotus-miner-local-net
export LOTUS_SKIP_GENESIS_CHECK=_yes_
export CGO_CFLAGS_ALLOW="-D__BLST_PORTABLE__"
export CGO_CFLAGS="-D__BLST_PORTABLE__"

# ./lotus-seed genesis new localnet.json
# ./lotus-seed pre-seal --sector-size 2KiB --num-sectors 2
# ./lotus-seed genesis add-miner localnet.json ~/.genesis-sectors/pre-seal-t01000.json
./lotus daemon --lotus-make-genesis=devgen.car --genesis-template=localnet.json --bootstrap=false
# ./lotus wallet import --as-default --format=json-lotus f1..
# cp mir-config/* $LOTUS_PATH
