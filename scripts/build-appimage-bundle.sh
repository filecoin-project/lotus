#!/usr/bin/env bash
set -ex

REQUIRED=(
    "ipfs"
    "sha512sum"
)
for REQUIRE in "${REQUIRED[@]}"
do
    command -v "${REQUIRE}" >/dev/null 2>&1 || echo >&2 "'${REQUIRE}' must be installed"
done

mkdir bundle
pushd bundle

export IPFS_PATH=`mktemp -d`
ipfs init
ipfs daemon &
PID="$!"
trap "kill -9 ${PID}" EXIT
sleep 30

cp "../appimage/Lotus-${CIRCLE_TAG}-x86_64.AppImage" .
sha512sum "Lotus-${CIRCLE_TAG}-x86_64.AppImage" > "Lotus-${CIRCLE_TAG}-x86_64.AppImage.sha512"
ipfs add -q "Lotus-${CIRCLE_TAG}-x86_64.AppImage" > "Lotus-${CIRCLE_TAG}-x86_64.AppImage.cid"
popd
