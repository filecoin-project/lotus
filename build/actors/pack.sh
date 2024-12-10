#!/bin/bash

NETWORKS=(devnet mainnet caterpillarnet butterflynet testing testing-fake-proofs calibrationnet)

set -e

if [[ $# -lt 2 ]]; then
    echo "Usage: $0 VERSION RELEASE [LOCAL_DIR] [NETWORK=RELEASE_OVERRIDE]..." >&2
    echo "expected at least two arguments, an actors version (e.g., v8), an actors release, an optional local directory, and any number of release overrides." >&2
    exit 1
fi

VERSION="$1" # actors version
RELEASE="$2" # actors release name
LOCAL_DIR=""
if [[ $# -ge 3 && ! "${3}" =~ "=" ]]; then
    LOCAL_DIR="$3"
    RELEASE_OVERRIDES=("${@:4}")
else
    RELEASE_OVERRIDES=("${@:3}")
fi

if [[ ! "$VERSION" =~ ^v?[0-9]+$ ]]; then
    echo "Error: VERSION must be an integer (got: ${VERSION})" >&2
    exit 1
fi
if [[ ! "$VERSION" =~ ^v ]]; then
    VERSION="v${VERSION}"
fi


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
    if [[ -n "$LOCAL_DIR" ]]; then
        if [[ -f "${LOCAL_DIR}/builtin-actors-${network}.car" ]]; then
            echo "Fetching $release for network $network from local directory."
            cp "${LOCAL_DIR}/builtin-actors-${network}.car" .
        else
            echo "Error: File ${LOCAL_DIR}/builtin-actors-${network}.car not found in local directory."
            exit 1
        fi
    else
        echo "Downloading $release for network $network."
        wget "https://github.com/filecoin-project/builtin-actors/releases/download/${encoded_release}/builtin-actors-${network}"{.car,.sha256}
    fi
done

if [[ -z "$LOCAL_DIR" ]]; then
    echo "Checking the checksums..."
    sha256sum -c -- *.sha256
else
    echo "Skipping checksum verification for local files."
fi

echo "Packing..."

rm -f -- "$TARGET_FILE"
tar -cf "$TARGET_FILE" --use-compress-program "zstd -19" -- *.car
popd

echo "Generating metadata..."

make -C ../../ VERSION="$VERSION" RELEASE="$RELEASE" RELEASE_OVERRIDES="${RELEASE_OVERRIDES[*]}" bundle-gen
