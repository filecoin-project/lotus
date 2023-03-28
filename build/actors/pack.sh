#!/bin/bash

NETWORKS=(devnet mainnet caterpillarnet butterflynet testing testing-fake-proofs calibrationnet)

set -e

if [[ $# -lt 2 ]]; then
    echo "Usage: $0 VERSION RELEASE [NETWORK=RELEASE_OVERRIDE]..." >&2
    echo "expected at least two arguments, an actors version (e.g., v8), an actors release, and any number of release overrides." >&2
    exit 1
fi

VERSION="$1" # actors version
RELEASE="$2" # actors release name
RELEASE_OVERRIDES=("${@:3}")

echo "Downloading bundles for actors version ${VERSION} release ${RELEASE}"
echo "With release overrides ${RELEASE_OVERRIDES[*]}"

TARGET_FILE="$(pwd)/${VERSION}.tar.zst"
WORKDIR=$(mktemp -d -t "actor-bundles-${VERSION}.XXXXXXXXXX")
trap 'rm -rf -- "$WORKDIR"' EXIT

encode_release() {
    jq -rn --arg release "$1" '$release | @uri'
}

pushd "${WORKDIR}"
for network in "${NETWORKS[@]}"; do
    release="$RELEASE"
    # Ideally, we'd use an associative array (map). But that's not supported on macos.
    for override in "${RELEASE_OVERRIDES[@]}"; do
        if [[ "${network}" = "${override%%=*}" ]]; then
            release="${override#*=}"
            break
        fi
    done
    encoded_release="$(encode_release "$release")"
    echo "Downloading $release for network $network."
    wget "https://github.com/filecoin-project/builtin-actors/releases/download/${encoded_release}/builtin-actors-${network}"{.car,.sha256}
done

echo "Checking the checksums..."

sha256sum -c -- *.sha256

echo "Packing..."

rm -f -- "$TARGET_FILE"
tar -cf "$TARGET_FILE" --use-compress-program "zstd -19" -- *.car
popd

echo "Generating metadata..."

make -C ../../ VERSION="$VERSION" RELEASE="$RELEASE" RELEASE_OVERRIDES="${RELEASE_OVERRIDES[*]}" bundle-gen
