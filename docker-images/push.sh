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
  echo -e "Please provide a tag for the push. For example: \`./push.sh v3\`"
  exit 2
fi

docker push "iptestground/oni-buildbase:$TAG"
docker push "iptestground/oni-runtime:$TAG"
