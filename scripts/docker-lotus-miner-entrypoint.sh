#!/usr/bin/env bash

if [ ! -z $DOCKER_LOTUS_MINER_INIT ]; then
	GATE="${LOTUS_MINER_PATH}/date_initialized"

	# Don't init if already initialized.
	if [ ! -f "${GATE}" ]; then
		echo starting init
		eval "/usr/local/bin/lotus-miner init ${DOCKER_LOTUS_MINER_INIT_ARGS}"
		if [ $? == 0 ]
			then
				echo lotus-miner init successful
				date > "$GATE"
			else
				echo lotus-miner init unsuccessful
				exit 1
		fi
	else
		echo lotus-miner already initialized.
	fi

fi

exec /usr/local/bin/lotus-miner $@
