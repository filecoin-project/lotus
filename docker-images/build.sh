#!/bin/bash

set -o errexit
set -o pipefail

set -e
set -x

err_report() {
    echo "Error on line $1"
}

trap 'err_report $LINENO' ERR

TAG=$1

# Validate required arguments
if [ -z "$TAG" ]
then
  echo -e "Please provide a tag for the build. For example: \`./build.sh v3\`"
  exit 2
fi

dir="$(dirname "$0")"

docker build -t "iptestground/oni-buildbase:$TAG" -f "$dir/Dockerfile.oni-buildbase" "$dir"
docker build -t "iptestground/oni-runtime:$TAG" -f "$dir/Dockerfile.oni-runtime" "$dir"
