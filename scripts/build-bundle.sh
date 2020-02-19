#!/usr/bin/env bash
set -ex

ARCHS=(
    "darwin"
    "linux"
)

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

BINARIES=(
    "lotus"
    "lotus-storage-miner"
    "lotus-seal-worker"
)

export IPFS_PATH=`mktemp -d`
ipfs init
ipfs daemon &
PID="$!"
trap "kill -9 ${PID}" EXIT
sleep 30

for ARCH in "${ARCHS[@]}"
do
    mkdir -p "${ARCH}/lotus"
    pushd "${ARCH}"
    for BINARY in "${BINARIES[@]}"
    do
        cp "../../${ARCH}/${BINARY}" "lotus/"
        chmod +x "lotus/${BINARY}"
    done

    tar -zcvf "../lotus_${CIRCLE_TAG}_${ARCH}-amd64.tar.gz" lotus
    popd
    rm -rf "${ARCH}"

    sha512sum "lotus_${CIRCLE_TAG}_${ARCH}-amd64.tar.gz" | cut -d" " -f1 > "lotus_${CIRCLE_TAG}_${ARCH}-amd64.tar.gz.sha512"

    ipfs add "lotus_${CIRCLE_TAG}_${ARCH}-amd64.tar.gz" | cut -d" " -f2 > "lotus_${CIRCLE_TAG}_${ARCH}-amd64.tar.gz.cid"
done
popd
