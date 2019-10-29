#!/usr/bin/env bash

############
## Settings
GENESIS_HOST=root@147.75.80.29
BOOTSTRAPPERS=( root@147.75.80.17 )

############

read -p "You are about to deploy new DevNet, killing bootstrap nodes. Proceed? (y/n)?" r
case "$r" in
  y|Y ) echo "Proceding";;
  n|N ) exit 0;;
  * ) exit 1;;
esac

log() {
  echo -e "\e[33m$1\e[39m"
}

rm -f build/bootstrap/*.pi

log '> Generating genesis'

make

GENPATH=$(mktemp -d)

log 'staring temp daemon'

./lotus --repo="${GENPATH}" daemon --lotus-make-random-genesis="${GENPATH}/devnet.car" &
GDPID=$!

sleep 3

log  'Extracting genesis miner prvate key'

ADDR=$(./lotus --repo="${GENPATH}" wallet list)
./lotus --repo="${GENPATH}" wallet export "$ADDR" > "${GENPATH}/wallet.key"

kill "$GDPID"

wait

log '> Creating genesis binary'
cp "${GENPATH}/devnet.car" build/genesis/devnet.car
rm -f build/bootstrap/*.pi

make

log '> Deploying and starting genesis miner'

ssh $GENESIS_HOST 'systemctl stop lotus-daemon' &
ssh $GENESIS_HOST 'systemctl stop lotus-storage-miner' &

wait

ssh $GENESIS_HOST 'rm -rf .lotus' &
ssh $GENESIS_HOST 'rm -rf .lotusstorage' &

scp -C lotus "${GENESIS_HOST}":/usr/local/bin/lotus &
scp -C lotus-storage-miner "${GENESIS_HOST}":/usr/local/bin/lotus-storage-miner &

wait

log 'Initializing genesis miner repo'

ssh $GENESIS_HOST 'systemctl start lotus-daemon'

scp scripts/bootstrap.toml "${GENESIS_HOST}:.lotus/config.toml" &
ssh < "${GENPATH}/wallet.key" $GENESIS_HOST '/usr/local/bin/lotus wallet import' &
wait

ssh $GENESIS_HOST 'systemctl restart lotus-daemon'

log 'Starting genesis mining'

ssh $GENESIS_HOST '/usr/local/bin/lotus-storage-miner init --genesis-miner --actor=t0101'
ssh $GENESIS_HOST 'systemctl start lotus-storage-miner'


log 'Getting genesis addr info'

ssh $GENESIS_HOST './lotus net listen' | grep -v '/10' | grep -v '/127' > build/bootstrap/root.pi

log '> Creating bootstrap binaries'
make


for host in "${BOOTSTRAPPERS[@]}"
do
  log "> Deploying bootstrap node $host"
  log "Stopping lotus daemon"

	ssh "$host" 'systemctl stop lotus-daemon' &
  ssh "$host" 'systemctl stop lotus-storage-miner' &

  wait

  ssh "$host" 'rm -rf .lotus' &
  ssh "$host" 'rm -rf .lotusstorage' &

  scp -C lotus "${host}":/usr/local/bin/lotus &
  scp -C lotus-storage-miner "${host}":/usr/local/bin/lotus-storage-miner &

  wait

  log 'Initializing repo'

  ssh "$host" 'systemctl start lotus-daemon'
  scp scripts/bootstrap.toml "${host}:.lotus/config.toml"
  ssh "$host" 'systemctl restart lotus-daemon'

  log 'Extracting addr info'

  ssh "$host" './lotus net listen' | grep -v '/10' | grep -v '/127' >> build/bootstrap/bootstrappers.pi
done
