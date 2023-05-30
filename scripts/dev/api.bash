#!/bin/bash
# vim: set expandtab ts=2 sw=2:

_lotus_token=$(./lotus auth create-token --perm admin)

runAPI() {
  curl -X POST \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","id":2,"method":"Filecoin.'"$1"'","params":'"${2:-null}"'}' \
    'http://127.0.0.1:1234/rpc/v0?token='"$_lotus_token"
}
