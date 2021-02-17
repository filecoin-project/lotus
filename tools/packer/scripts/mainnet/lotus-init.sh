#!/usr/bin/env bash

# This script sets up an initial configuraiton for the lotus daemon and miner
# It will only run once.

GATE="$LOTUS_PATH"/date_initialized

# Don't init if already initialized.
if [ -f "GATE" ]; then
	echo lotus already initialized.
	exit 0
fi

echo importing minimal snapshot
lotus daemon --import-snapshot https://fil-chain-snapshots-fallback.s3.amazonaws.com/mainnet/minimal_finality_stateroots_latest.car --halt-after-import

# Block future inits
date > "$GATE"
