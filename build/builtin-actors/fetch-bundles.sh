#!/bin/bash
set -e

cd $(dirname "$0")

actors7_cid=""
actors7_hash=""
actors8_cid="bafybeictmywrut5tprz5fnoti6adfwuvixvrfardhqwldxosmdsfavc56e"
actors8_hash="687b38f59b0c32800f55a8f1f303de214ec173c90e653984d67f393bc41c1416"

die() {
    echo "$1"
    exit 1
}

check() {
    file=$1
    hash=$2
    if [ -e "$file" ]; then
        file_hash=$(sha256sum "$file" | cut -d ' ' -f 1)
        if [ "$file_hash" == "$hash" ]; then
            return 0
        else
            return 1
        fi
    else
        return 1
    fi
}

fetch() {
    output=$1
    cid=$2
    hash=$3
    if (check $output $hash); then
        return 0
    else
        echo "fetching $cid to $output"
        curl -k "https://dweb.link/ipfs/$cid" -o $output
        check $output $hash || die "hash mismatch"
    fi
}

if [ ! -z "$actors7_cid" ]; then
    fetch builtin-actors-v7.car "$actors7_cid" "$actors7_hash"
else
    touch builtin-actors-v7.car
fi

if [ ! -z "$actors8_cid" ]; then
    fetch builtin-actors-v8.car "$actors8_cid" "$actors8_hash"
else
    touch builtin-actors-v8.car
fi
