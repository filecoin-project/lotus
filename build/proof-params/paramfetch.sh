#!/usr/bin/env bash

set -euo pipefail

die() {
	echo "$@" >&2
	exit 1
}

have_binary() {
	type "$1" > /dev/null 2> /dev/null
}

check_writable() {
	printf "" > "$1" && rm "$1"
}

try_download() {
	url="$1"
	output="$2"
	command="$3"
	util_name="$(set -- $command; echo "$1")"

	if ! have_binary "$util_name"; then
		return 1
	fi

	printf '==> Using %s to download "%s" to "%s"\n' "$util_name" "$url" "$output"
	if eval "$command"; then
		echo "==> Download complete!"
		return
	else
		echo "error: couldn't download with $util_name ($?)"
		return 1
	fi
}

download() {
	dl_url="$1"
	dl_output="$2"

	test "$#" -eq "2" || die "download requires exactly two arguments, was given $@"

	if ! check_writable "$dl_output"; then
		die "download error: cannot write to $dl_output"
	fi

	try_download "$dl_url" "$dl_output" "wget '$dl_url' -O '$dl_output'" && return
	try_download "$dl_url" "$dl_output" "curl --silent --fail --output '$dl_output' '$dl_url'" && return
	try_download "$dl_url" "$dl_output" "fetch '$dl_url' -o '$dl_output'" && return
	try_download "$dl_url" "$dl_output" "http '$dl_url' > '$dl_output'" && return
	try_download "$dl_url" "$dl_output" "ftp -o '$dl_output' '$dl_url'" && return

	die "Unable to download $dl_url. exiting."
}

fetch_ipget() {
  local dest="$1"
  local cid="$2"

  IPGET_PARAMS="--node=spawn -p=/ip4/138.201.67.219/tcp/4002/ws/ipfs/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA -p=/ip4/138.201.67.218/tcp/4002/ws/ipfs/QmbVWZQhCGrS7DhgLqWbgvdmKN7JueKCREVanfnVpgyq8x -p=/ip4/94.130.135.167/tcp/4002/ws/ipfs/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE -p=/ip4/138.201.68.74/tcp/4001/ipfs/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR -p=/ip4/138.201.67.220/tcp/4001/ipfs/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i"

  ./bin/ipget $IPGET_PARAMS -o "$dest" "$cid"
}

fetch_gateway() {
  local dest="$1"
  local cid="$2"

  local url="http://198.211.99.118/ipfs/$cid"

  download "$url" "$dest"
}

OUT_DIR="/var/tmp/filecoin-proof-parameters"
PARAMS="build/proof-params/parameters.json"

mkdir -p $OUT_DIR
jq '. | to_entries | map("'$OUT_DIR'/\(.key) \(.value.cid) \(.value.digest)") | .[]' --raw-output $PARAMS | \
	while read -r dest cid digest; do
		if [[ -f "$dest" ]]; then
			b2=$(b2sum "$dest" | head -c 32)
			if [[ "$digest" == "$b2" ]]; then
				echo "$dest exists and has correct hash"
				continue
			else
				echo "$dest has incorrect hash"
				rm -f "$dest"
			fi
		fi
		echo "downloading $dest"

		fetch_gateway "$dest" "$cid"
	done
