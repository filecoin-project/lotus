#!/bin/bash

set -o errexit
set -o pipefail

set -e
set -x

err_report() {
    echo "Error on line $1"
}

trap 'err_report $LINENO' ERR

COMMIT=$1

# Validate required arguments
if [ -z "$COMMIT" ]
then
  echo -e "Please provider commit of Lotus to build against. For example: \`./build.sh 596ed33\`"
  exit 2
fi

my_dir="$(dirname "$0")"

docker build --build-arg LOTUS_VERSION=$COMMIT -t iptestground/oni-buildbase:$COMMIT -f $my_dir/Dockerfile.oni-buildbase $my_dir
docker build --build-arg LOTUS_VERSION=$COMMIT -t iptestground/oni-runtime:$COMMIT -f $my_dir/Dockerfile.oni-runtime $my_dir
