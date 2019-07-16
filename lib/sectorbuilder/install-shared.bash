#!/usr/bin/env bash

download_release_tarball() {
    __resultvar=$1
    __submodule_path=$2
    __repo_name=$(echo $2 | cut -d '/' -f 1)
    __release_name="${__repo_name}-$(uname)"
    __release_sha1=$(git rev-parse @:"${__submodule_path}")
    __release_tag="${__release_sha1:0:16}"
    __release_tag_url="https://api.github.com/repos/filecoin-project/${__repo_name}/releases/tags/${__release_tag}"

    echo "acquiring release @ ${__release_tag}"

    __release_response=$(curl \
        --retry 3 \
        --location $__release_tag_url)

    __release_url=$(echo $__release_response | jq -r ".assets[] | select(.name | contains(\"${__release_name}\")) | .url")

    if [[ -z "$__release_url" ]]; then
        (>&2 echo "failed to download release (tag URL: ${__release_tag_url}, response: ${__release_response})")
        return 1
    fi

    __tar_path="/tmp/${__release_name}_$(basename ${__release_url}).tar.gz"

    if [[ ! -f "${__tar_path}" ]]; then
        __asset_url=$(curl \
            --head \
            --retry 3 \
            --header "Accept:application/octet-stream" \
            --location \
            --output /dev/null \
            -w %{url_effective} \
            "$__release_url")

        curl --retry 3 --output "${__tar_path}" "$__asset_url"
        if [[ $? -ne "0" ]]; then
            (>&2 echo "failed to download release asset (tag URL: ${__release_tag_url}, asset URL: ${__asset_url})")
            return 1
        fi
    fi

    eval $__resultvar="'$__tar_path'"
}

build_from_source() {
    __submodule_path=$1
    __submodule_sha1=$(git rev-parse @:"${__submodule_path}")
    __submodule_sha1_truncated="${__submodule_sha1:0:16}"

    echo "building from source @ ${__submodule_sha1_truncated}"

    if ! [ -x "$(command -v cargo)" ]; then
        (>&2 echo 'Error: cargo is not installed.')
        (>&2 echo 'Install Rust toolchain to resolve this problem.')
        exit 1
    fi

    if ! [ -x "$(command -v rustup)" ]; then
        (>&2 echo 'Error: rustup is not installed.')
        (>&2 echo 'Install Rust toolchain installer to resolve this problem.')
        exit 1
    fi

    pushd $__submodule_path

    cargo --version
    cargo update

    if [[ -f "./scripts/build-release.sh" ]]; then
        ./scripts/build-release.sh $(cat rust-toolchain)
    else
        cargo build --release --all
    fi

    popd
}
