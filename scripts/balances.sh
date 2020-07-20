#!/bin/bash

sleep 20

lotus wait-api

lotus chain head

export MAIN=$(cat ../localnet2.json | jq -r '.Accounts | .[0] | .Meta .Owner')

# Send funds to root key
lotus send --from $MAIN $ROOT 5000000

export VERIFIER=$(lotus wallet new)
export CLIENT=$(lotus wallet new)

# Send funds to verifier
lotus send --from $MAIN $VERIFIER 5000000

# Send funds to client
lotus send --from $MAIN $CLIENT 5000000

while [ "5000000 FIL" != "$(lotus wallet balance $VERIFIER)" ]
do
 sleep 1
 lotus wallet balance $ROOT
done

# export PARAM=$(lotus-shed verifreg add-verifier --dry t01001 100000000000000000000000000000000000000000)
export PARAM=824300e907440076adf1

# Add verifier
# lotus-shed verifreg add-verifier --from $ROOT t01001 100000000000000000000000000000000000000000

lotus msig propose --from $MAIN t080 t06 0 2 $PARAM
lotus msig inspect t080

lotus-shed verifreg list-verifiers

# Add verified client
lotus-shed verifreg verify-client --from $VERIFIER $CLIENT 10000
lotus-shed verifreg list-clients

# Remove verifier datacap
lotus-shed verifreg add-verifier --from $ROOT t01001 0
lotus-shed verifreg list-verifiers

export DATA=$(lotus client import dddd | awk '{print $NF}')

lotus client local

# Client can make a verified deal
lotus client deal --verified-deal --from $CLIENT $DATA t01000 0.005 100000

while [ "3" != "$(lotus-miner sectors list | wc -l)" ]
do
 sleep 10
 lotus-miner sectors list
done

curl -H "Content-Type: application/json" -H "Authorization: Bearer $(cat ~/.lotusminer/token)" -d '{"id": 1, "method": "Filecoin.SectorStartSealing", "params": [2]}' localhost:2345/rpc/v0

lotus-miner info

lotus-miner sectors list

while [ "3" != "$(lotus-miner sectors list | grep Proving | wc -l)" ]
do
 sleep 5
 lotus-miner sectors list | tail -n 1
 lotus-miner info | grep "Actual Power"
done

sleep 300000
