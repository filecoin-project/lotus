#!/usr/bin/env bash

set -xeo

NUM_SECTORS=2
SECTOR_SIZE=2KiB


sdt0111=$(mktemp -d)
sdt0222=$(mktemp -d)
sdt0333=$(mktemp -d)

staging=$(mktemp -d)

make debug

./lotus-seed --sector-dir="${sdt0111}" pre-seal --miner-addr=t0111 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS} &
./lotus-seed --sector-dir="${sdt0222}" pre-seal --miner-addr=t0222 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS} &
./lotus-seed --sector-dir="${sdt0333}" pre-seal --miner-addr=t0333 --sector-offset=0 --sector-size=${SECTOR_SIZE} --num-sectors=${NUM_SECTORS} &

wait

./lotus-seed aggregate-manifests "${sdt0111}/pre-seal-t0111.json" "${sdt0222}/pre-seal-t0222.json" "${sdt0333}/pre-seal-t0333.json" > "${staging}/genesis.json"

lotus_path=$(mktemp -d)

./lotus --repo="${lotus_path}" daemon --lotus-make-random-genesis="${staging}/devnet.car" --genesis-presealed-sectors="${staging}/genesis.json" --bootstrap=false &
lpid=$!

sleep 3

kill "$lpid"

wait

cp "${staging}/devnet.car" build/genesis/devnet.car

make debug

ldt0111=$(mktemp -d)
ldt0222=$(mktemp -d)
ldt0333=$(mktemp -d)

sdlist=( "$sdt0111" "$sdt0222" "$sdt0333" )
ldlist=( "$ldt0111" "$ldt0222" "$ldt0333" )

for (( i=0; i<${#sdlist[@]}; i++ )); do
  preseal=${sdlist[$i]}
  fullpath=$(find ${preseal} -type f -iname 'pre-seal-*.json')
  filefull=$(basename ${fullpath})
  filename=${filefull%%.*}
  mineraddr=$(echo $filename | sed 's/pre-seal-//g')

  wallet_raw=$(jq -rc ".${mineraddr}.Key" < ${preseal}/${filefull})
  wallet_b16=$(./lotus-shed base16 "${wallet_raw}")
  wallet_adr=$(./lotus-shed keyinfo --format="{{.Address}}" "${wallet_b16}")
  wallet_adr_enc=$(./lotus-shed base32 "wallet-${wallet_adr}")

  mkdir -p "${ldlist[$i]}/keystore"
  cat > "${ldlist[$i]}/keystore/${wallet_adr_enc}" <<EOF
${wallet_raw}
EOF

  chmod 0700 "${ldlist[$i]}/keystore/${wallet_adr_enc}"
done

pids=()
for (( i=0; i<${#ldlist[@]}; i++ )); do
  repo=${ldlist[$i]}
  ./lotus --repo="${repo}" daemon --api "3000$i" --bootstrap=false &
  pids+=($!)
done

sleep 10

boot=$(./lotus --repo="${ldlist[0]}" net listen)

for (( i=1; i<${#ldlist[@]}; i++ )); do
  repo=${ldlist[$i]}
  ./lotus --repo="${repo}" net connect ${boot}
done

sleep 3

mdt0111=$(mktemp -d)
mdt0222=$(mktemp -d)
mdt0333=$(mktemp -d)

env LOTUS_PATH="${ldt0111}" LOTUS_MINER_PATH="${mdt0111}" ./lotus-miner init --genesis-miner --actor=t0111 --pre-sealed-sectors="${sdt0111}" --pre-sealed-metadata="${sdt0111}/pre-seal-t0111.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true
env LOTUS_PATH="${ldt0111}" LOTUS_MINER_PATH="${mdt0111}" ./lotus-miner run --nosync &
mpid=$!

env LOTUS_PATH="${ldt0222}" LOTUS_MINER_PATH="${mdt0222}" ./lotus-miner init                 --actor=t0222 --pre-sealed-sectors="${sdt0222}" --pre-sealed-metadata="${sdt0222}/pre-seal-t0222.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true
env LOTUS_PATH="${ldt0333}" LOTUS_MINER_PATH="${mdt0333}" ./lotus-miner init                 --actor=t0333 --pre-sealed-sectors="${sdt0333}" --pre-sealed-metadata="${sdt0333}/pre-seal-t0333.json" --nosync=true --sector-size="${SECTOR_SIZE}" || true

kill $mpid
wait $mpid

for (( i=0; i<${#pids[@]}; i++ )); do
  kill ${pids[$i]}
done

wait

rm -rf $mdt0111
rm -rf $mdt0222
rm -rf $mdt0333

rm -rf $ldt0111
rm -rf $ldt0222
rm -rf $ldt0333

rm -rf $sdt0111
rm -rf $sdt0222
rm -rf $sdt0333

rm -rf $staging
