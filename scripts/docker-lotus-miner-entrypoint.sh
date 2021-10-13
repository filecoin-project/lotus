#!/usr/bin/env bash

if [ ! -z $DOCKER_LOTUS_MINER_INIT ]; then
	GATE="$LOTUS_PATH"/date_initialized

	# Don't init if already initialized.
	if [ -f "$GATE" ]; then
		echo lotus-miner already initialized.
		exit 0
	fi

	echo starting init
	/usr/local/bin/lotus-miner init

	# Block future inits
	date > "$GATE"
fi

exec /usr/local/bin/lotus-miner $@
