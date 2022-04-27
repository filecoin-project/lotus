#!/bin/bash
set -e

cd "$(dirname "$0")"

. bundles.env

die() {
    echo "$1"
    exit 1
}

fetch() {
    ver=$1
    rel=$2

    if [ ! -e $ver ]; then
        mkdir $ver
    fi

    if [ -e $ver/release ]; then
       cur=$(cat $ver/release)
       if [ $cur == $rel ]; then
           return 0
       fi
    fi

    for net in mainnet caterpillarnet butterflynet calibrationnet devnet testing testing-fake-proofs; do
        fetch_bundle $ver $rel $net
    done

    # remember the current release so that we don't have to hit github unless we have modified it
    echo $rel > $ver/release
}

fetch_bundle() {
    ver=$1
    rel=$2
    net=$3

    target=builtin-actors-$net.car
    hash=builtin-actors-$net.sha256

    pushd $ver

    # fetch the hash first and check if it matches what we (may) already have
    curl -L --retry 3 https://github.com/filecoin-project/builtin-actors/releases/download/$rel/$hash -o $hash || die "error fetching hash for $ver/$net"
    if [ -e $target ]; then
        if (shasum -a 256 --check $hash); then
            popd
            return 0
        fi
    fi

    # we don't have the (correct) bundle, fetch it
    curl -L --retry 3 https://github.com/filecoin-project/builtin-actors/releases/download/$rel/$target -o $target || die "error fetching bundle for $ver/$net"
    # verify
    shasum -a 256 --check $hash || die "hash mismatch"
    # all good
    popd
}

touch_bundles() {
    ver=$1

    if [ ! -e $ver ]; then
        mkdir $ver
    fi

    for net in mainnet caterpillarnet butterflynet calibrationnet devnet testing testing-fake-proofs; do
        touch $ver/builtin-actors-$net.car
    done
}

if [ -n "$actors7_release" ]; then
    fetch v7 "$actors7_release"
else
    touch_bundles v7
fi

if [ -n "$actors8_release" ]; then
    fetch v8 "$actors8_release"
else
    touch_bundles v8
fi
