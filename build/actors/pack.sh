#!/bin/bash

set -e

if [[ $# -ne 2 ]]; then
    echo "expected two arguments, an actors version (e.g., v8) and an actors release"
    exit 1
fi

VERSION="$1" # actors version
RELEASE="$2" # actors release name
NETWORKS=(devnet mainnet caterpillarnet butterflynet testing testing-fake-proofs calibrationnet)

echo "Downloading bundles for actors version ${VERSION}, release ${RELEASE}"

TARGET_FILE="$(pwd)/${VERSION}.tar.zst"
WORKDIR=$(mktemp -d -t "actor-bundles-${VERSION}.XXXXXXXXXX")
trap 'rm -rf -- "$WORKDIR"' EXIT

pushd "${WORKDIR}"
encoded_release="$(jq -rn --arg release "$RELEASE" '$release | @uri')"
for network in "${NETWORKS[@]}"; do
    wget "https://github.com/filecoin-project/builtin-actors/releases/download/${encoded_release}/builtin-actors-${network}"{.car,.sha256}
done

echo "Checking the checksums..."

sha256sum -c -- *.sha256


echo "Packing..."

rm -f -- "$TARGET_FILE"
tar -cf "$TARGET_FILE" --use-compress-program "zstd -19" -- *.car
popd

echo "Generating metadata..."

make -C ../../ bundle-gen
