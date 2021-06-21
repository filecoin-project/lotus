#!/usr/bin/env bash

if [ ! -z DOCKER_LOTUS_IMPORT_SNAPSHOT ]; then
	GATE="$LOTUS_PATH"/date_initialized

	# Don't init if already initialized.
	if [ -f "GATE" ]; then
		echo lotus already initialized.
		exit 0
	fi

	echo importing minimal snapshot
	/usr/local/bin/lotus daemon --import-snapshot "$DOCKER_LOTUS_IMPORT_SNAPSHOT" --halt-after-import

	# Block future inits
	date > "$GATE"
fi

if [ ! -z DOCKER_LOTUS_WALLET_IMPORT ]; then
	mkdir $LOTUS_PATH/keystore
	cp "${DOCKER_LOTUS_WALLET_IMPORT}" "${LOTUS_PATH}/keystore"
fi

exec /usr/local/bin/lotus $@
