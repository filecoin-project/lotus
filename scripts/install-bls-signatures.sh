#!/usr/bin/env bash

set -Eeo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/install-shared.bash"

subm_dir="lib/bls-signatures/bls-signatures"

git submodule update --init --recursive $subm_dir

if download_release_tarball tarball_path "${subm_dir}"; then
    tmp_dir=$(mktemp -d)
    tar -C $tmp_dir -xzf $tarball_path

    cp -R "${tmp_dir}/include" lib/bls-signatures
    cp -R "${tmp_dir}/lib" lib/bls-signatures
else
    (>&2 echo "failed to find or obtain precompiled assets for ${subm_dir}, falling back to local build")
    build_from_source "${subm_dir}"

    mkdir -p lib/bls-signatures/include
    mkdir -p lib/bls-signatures/lib/pkgconfig

    find "${subm_dir}" -type f -name libbls_signatures.h -exec mv -- "{}" ./lib/bls-signatures/include/ \;
    find "${subm_dir}" -type f -name libbls_signatures_ffi.a -exec cp -- "{}" ./lib/bls-signatures/lib/ \;
    find "${subm_dir}" -type f -name libbls_signatures.pc -exec cp -- "{}" ./lib/bls-signatures/lib/pkgconfig/ \;
fi
