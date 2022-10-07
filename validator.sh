#!/bin/bash

# Config envs
export LOTUS_PATH=~/.lotus-local-net
export LOTUS_MINER_PATH=~/.lotus-miner-local-net
export LOTUS_SKIP_GENESIS_CHECK=_yes_
export CGO_CFLAGS_ALLOW="-D__BLST_PORTABLE__"
export CGO_CFLAGS="-D__BLST_PORTABLE__"

# Copy mir config and import keys
./lotus wallet import --as-default --format=json-lotus  f1cp4q4lqsdhob23ysywffg2tvbmar5cshia4rweq.key
cp mir-config/* $LOTUS_PATH

# Set interceptor output
n=$(cat mir-event-logs/counter)
export MIR_INTERCEPTOR_OUTPUT="mir-event-logs/run-${n}"
echo $((n + 1)) > mir-event-logs/counter

# Compile and run validator
make mir-validator
./mir-validator run --nosync 
