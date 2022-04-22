#!/usr/bin/env bash

set -xeo

NUM_SECTORS=2
SECTOR_SIZE=2KiB


if ./lotus --version | grep -v debug; then
  make debug
fi

sdt01000=$(mktemp -d)
sdt01001=$(mktemp -d)
sdt01002=$(mktemp -d)

staging=$(mktemp -d)

./lotus-seed --sector-dir="${sdt01000}" pre-seal --fake-sectors --miner-addr=t01000 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS}
./lotus-seed --sector-dir="${sdt01001}" pre-seal --fake-sectors --miner-addr=t01001 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS}
./lotus-seed --sector-dir="${sdt01002}" pre-seal --fake-sectors --miner-addr=t01002 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS}

./lotus-seed genesis new --network-name debugnet "${staging}/genesis.json"
./lotus-seed genesis set-network-version "${staging}/genesis.json"


cat "${sdt01000}/pre-seal-t01000.json"


jq --arg MinerId "t01000" --arg VerifiedDeal "true" '
    .[$MinerId].Sectors[] |= (.Deal.Label = .CommR."/")
' < "${sdt01000}/pre-seal-t01000.json" > "${sdt01000}/pre-seal-t01000.json.updated" && mv "${sdt01000}/pre-seal-t01000.json.updated" "${sdt01000}/pre-seal-t01000.json"

jq --arg MinerId "t01001" --arg VerifiedDeal "true" '
    .[$MinerId].Sectors[] |= (.Deal.Label = .CommR."/")
' < "${sdt01000}/pre-seal-t01000.json" > "${sdt01000}/pre-seal-t01000.json.updated" && mv "${sdt01000}/pre-seal-t01000.json.updated" "${sdt01000}/pre-seal-t01000.json"

jq --arg MinerId "t01002" --arg VerifiedDeal "true" '
    .[$MinerId].Sectors[] |= (.Deal.Label = .CommR."/")
' < "${sdt01000}/pre-seal-t01000.json" > "${sdt01000}/pre-seal-t01000.json.updated" && mv "${sdt01000}/pre-seal-t01000.json.updated" "${sdt01000}/pre-seal-t01000.json"

./lotus-seed genesis add-miner "${staging}/genesis.json" "${sdt01000}/pre-seal-t01000.json"
./lotus-seed genesis add-miner "${staging}/genesis.json" "${sdt01001}/pre-seal-t01001.json"
./lotus-seed genesis add-miner "${staging}/genesis.json" "${sdt01002}/pre-seal-t01002.json"

./lotus-seed genesis car --out "${staging}/devnet.car" "${staging}/genesis.json"
cp "${staging}/devnet.car" build/genesis/devnet.car

ldt01000=$(mktemp -d)
ldt01001=$(mktemp -d)
ldt01002=$(mktemp -d)

sdlist=( "$sdt01000" "$sdt01001" "$sdt01002" )
ldlist=( "$ldt01000" "$ldt01001" "$ldt01002" )

for (( i=0; i<${#sdlist[@]}; i++ )); do
  preseal=${sdlist[$i]}
  fullpath=$(find ${preseal} -type f -iname 'pre-seal-*.json')
  filefull=$(basename ${fullpath})
  filename=${filefull%%.*}
  mineraddr=$(echo $filename | sed 's/pre-seal-//g')

  wallet_file="${preseal}/${filename}.key"
  mkdir -p "${ldlist[$i]}/keystore"
  LOTUS_PATH="${ldlist[$i]}" ./lotus-shed keyinfo import "${wallet_file}"
done

pids=()
for (( i=0; i<${#ldlist[@]}; i++ )); do
  repo=${ldlist[$i]}
  ./lotus --repo="${repo}" daemon --api "3000$i" --bootstrap=false --genesis build/genesis/devnet.car &
  pids+=($!)
done

sleep 10

boot=$(./lotus --repo="${ldlist[0]}" net listen)

for (( i=1; i<${#ldlist[@]}; i++ )); do
  repo=${ldlist[$i]}
  ./lotus --repo="${repo}" net connect ${boot}
done

sleep 3

mdt01000=$(mktemp -d)
mdt01001=$(mktemp -d)
mdt01002=$(mktemp -d)

env LOTUS_PATH="${ldt01000}" LOTUS_MINER_PATH="${mdt01000}" ./lotus-miner init --genesis-miner --actor=t01000 --pre-sealed-sectors="${sdt01000}" --pre-sealed-metadata="${sdt01000}/pre-seal-t01000.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true
env LOTUS_PATH="${ldt01000}" LOTUS_MINER_PATH="${mdt01000}" ./lotus-miner run --nosync &
mpid=$!

env LOTUS_PATH="${ldt01001}" LOTUS_MINER_PATH="${mdt01001}" ./lotus-miner init                 --actor=t01001 --pre-sealed-sectors="${sdt01001}" --pre-sealed-metadata="${sdt01001}/pre-seal-t01001.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true
env LOTUS_PATH="${ldt01002}" LOTUS_MINER_PATH="${mdt01002}" ./lotus-miner init                 --actor=t01002 --pre-sealed-sectors="${sdt01002}" --pre-sealed-metadata="${sdt01002}/pre-seal-t01002.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true

kill $mpid
wait $mpid

for (( i=0; i<${#pids[@]}; i++ )); do
  kill ${pids[$i]}
done

wait

rm -rf $mdt01000
rm -rf $mdt01001
rm -rf $mdt01002

rm -rf $ldt01000
rm -rf $ldt01001
rm -rf $ldt01002

rm -rf $sdt01000
rm -rf $sdt01001
rm -rf $sdt01002

rm -rf $staging
