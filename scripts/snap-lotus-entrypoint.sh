LOTUS_IMPORT_SNAPSHOT="https://fil-chain-snapshots-fallback.s3.amazonaws.com/mainnet/minimal_finality_stateroots_latest.car"
LOTUS_BINARY=$(dirname "$0")/lotus
GATE="$LOTUS_PATH"/date_initialized
if [ ! -f "$GATE" ]; then
	echo importing minimal snapshot
	$LOTUS_BINARY daemon --import-snapshot "$LOTUS_IMPORT_SNAPSHOT" --halt-after-import
	# Block future inits
	date > "$GATE"
fi
$LOTUS_BINARY daemon $ARGS
