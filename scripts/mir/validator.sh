#!/bin/bash

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

# Copy mir config and import keys
./lotus wallet import --as-default --format=json-lotus  ./scripts/mir/mir-config/node$INDEX/wallet.key
cp ./scripts/mir/mir-config/node$INDEX/* $LOTUS_PATH

# Set interceptor output
n=$(cat mir-event-logs/counter)
export MIR_INTERCEPTOR_OUTPUT="mir-event-logs/run-${n}"
echo $((n + 1)) > mir-event-logs/counter

# Compile and run validator
# make mir-validator
./mir-validator run --nosync 
