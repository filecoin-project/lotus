#!/usr/bin/env bash

set -euo pipefail

sed "s/{{PARAMSJSON}}/$(base64 build/proof-params/parameters.json -w 0)/g" build/proof-params/paramfetch.sh.template > ./build/paramfetch.sh
chmod +x ./build/paramfetch.sh
