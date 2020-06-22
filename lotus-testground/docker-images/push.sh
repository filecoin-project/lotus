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
  echo -e "Please provider commit of Lotus to build against. For example: \`./push.sh 596ed33\`"
  exit 2
fi

docker push iptestground/oni-buildbase:$COMMIT
docker push iptestground/oni-runtime:$COMMIT
