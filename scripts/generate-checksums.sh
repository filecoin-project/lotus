#!/usr/bin/env bash
set -exo

REQUIRED=(
    "sha512sum"
    "ipfs"
)
for REQUIRE in "${REQUIRED[@]}"
do
    command -v "${REQUIRE}" >/dev/null 2>&1 || echo >&2 "'${REQUIRE}' must be installed"
done

# start ipfs
export IPFS_PATH=`mktemp -d`
ipfs init
ipfs daemon &
PID="$!"
trap "kill -9 ${PID}" EXIT

# generate checksums
for FILE in dist/*.tar.gz
do
  sha512sum "${FILE}" > "${FILE}.sha512"
  until ipfs add -q "${FILE}" > "${FILE}.cid"
  do
    echo "Waiting for ipfs daemon to start..."
    sleep 2
  done
done
