LOTUS_IMPORT_SNAPSHOT="https://forest-archive.chainsafe.dev/latest/mainnet/"
LOTUS_BINARY=$(dirname "$0")/lotus
GATE="$LOTUS_PATH"/date_initialized
if [ ! -f "$GATE" ]; then
	echo importing minimal snapshot
	$LOTUS_BINARY daemon --import-snapshot "$LOTUS_IMPORT_SNAPSHOT" --halt-after-import
	# Block future inits
	date > "$GATE"
fi
$LOTUS_BINARY daemon $ARGS
