#!/usr/bin/env bash

set -Eeo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/install-shared.bash"

subm_dir="lib/rust-fil-sector-builder"

git submodule update --init --recursive $subm_dir

if download_release_tarball tarball_path "${subm_dir}"; then
    tmp_dir=$(mktemp -d)
    tar -C $tmp_dir -xzf $tarball_path

    cp -R "${tmp_dir}/include" lib/sectorbuilder
    cp -R "${tmp_dir}/lib" lib/sectorbuilder
else
    echo "failed to find or obtain precompiled assets for ${subm_dir}, falling back to local build"
    build_from_source "${subm_dir}"

    mkdir -p lib/sectorbuilder/include
    mkdir -p lib/sectorbuilder/lib/pkgconfig

    find "${subm_dir}" -type f -name sector_builder_ffi.h -exec mv -- "{}" ./lib/sectorbuilder/include/ \;
    find "${subm_dir}" -type f -name libsector_builder_ffi.a -exec cp -- "{}" ./lib/sectorbuilder/lib/ \;
    find "${subm_dir}" -type f -name sector_builder_ffi.pc -exec cp -- "{}" ./lib/sectorbuilder/pkgconfig/ \;
fi