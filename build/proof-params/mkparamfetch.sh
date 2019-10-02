#!/usr/bin/env bash

set -euo pipefail

function b64() {
	f="$1"
	case `uname` in
	Darwin)
		base64 -i "$f"
	;;
	Linux)
		base64 "$f" -w 0
	;;
	esac
	printf "unsupported system" 1>&2
	exit 1
}

sed "s/{{PARAMSJSON}}/$(b64 build/proof-params/parameters.json)/g" build/proof-params/paramfetch.sh.template > ./build/paramfetch.sh
chmod +x ./build/paramfetch.sh
