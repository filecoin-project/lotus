#!/usr/bin/env bash


__PWD=$PWD
__DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
__PARENT_DIR=$(dirname $__DIR)
LOTUS_PATH="${1:-$__PARENT_DIR}"
LOTUS_PURGE_LOCALDIR=${LOTUS_PURGE_LOCALDIR:-false}
LOTUS_COMPILE=${LOTUS_COMPILE:-false}


LOTUS_BIN="$LOTUS_PATH/lotus"
LOTUS_SEED="$LOTUS_PATH/lotus-seed"
LOTUS_MINER="$LOTUS_PATH/lotus-miner"


##### Functions

configure_lotus() {

  echo -e "\nDownloading the 2048 byte parameters:\n"
  $LOTUS_BIN fetch-params 2048

  echo -e "\nPre-seal some sectors:\n"
  $LOTUS_SEED pre-seal --sector-size 2KiB --num-sectors 2
#  $LOTUS_SEED pre-seal --sector-size 2KiB --num-sectors 2 --miner-addr t01001

#  export ROOT=$(cat ~/.genesis-sectors/pre-seal-t01001.json | jq -r '.t01001 | .Owner')
  export MINER=$(cat ~/.genesis-sectors/pre-seal-t01000.json | jq -r '.t01000 | .Owner')

  echo -e "\nCreate the genesis block and start up the first node:\n"
  $LOTUS_SEED genesis new $LOTUS_PATH/localnet.json
  $LOTUS_SEED genesis add-miner $LOTUS_PATH/localnet.json ~/.genesis-sectors/pre-seal-t01000.json
  # $LOTUS_SEED genesis add-msigs $LOTUS_PATH/localnet.json test.csv
  jq --arg root $MINER '. + {VerifregRootKey: {Meta: {Signers: [$root], Threshold: 1 }, Type: "multisig", Balance: "50000000000000000000000000"}}' ../localnet.json > $LOTUS_PATH/localnet2.json
  # cp $LOTUS_PATH/localnet.json $LOTUS_PATH/localnet2.json
  # jq --arg root $ROOT '. + {RootKey: $root}' ../localnet.json > $LOTUS_PATH/localnet2.json
  # jq --arg root $MINER '.Accounts |= . + [{Type: "multisig", Balance: "50000000000000000000000000", Meta: { Signers: [$root], Threshold: 1 }}]' ../localnet.json > $LOTUS_PATH/localnet2.json

  tmux new-session -s lotus -n script -d bash balances.sh

  sleep 5
  tmux new-window -t lotus:1 -n daemon -d $LOTUS_BIN daemon --lotus-make-genesis=dev.gen --genesis-template=$LOTUS_PATH/localnet2.json --bootstrap=false
  # $LOTUS_BIN daemon --lotus-make-genesis=dev.gen --genesis-template=$LOTUS_PATH/localnet2.json --bootstrap=false || exit

  sleep 5
  echo -e "\nImporting the genesis miner key:\n"
  $LOTUS_BIN wallet import ~/.genesis-sectors/pre-seal-t01000.key
#  $LOTUS_BIN wallet import ~/.genesis-sectors/pre-seal-t01001.key

  sleep 3
  echo -e "\nSetting up the genesis miner:\n"
  $LOTUS_MINER init --genesis-miner --actor=t01000 --sector-size=2KiB \
    --pre-sealed-sectors=~/.genesis-sectors \
    --pre-sealed-metadata=~/.genesis-sectors/pre-seal-t01000.json --nosync

  sleep 3
  echo -e "\nStarting up the miner:\n"
  tmux new-window -t lotus:2 -n miner -d $LOTUS_MINER run --nosync
}



purge_local_dirs() {
  echo -e "\nRemoving local state\n"
  rm -rf ~/.lotus || true
  rm -rf ~/.lotusstorage || true
  rm -rf ~/.lotusminer || true
  rm -rf ~/.genesis-sectors || true
}


compile_lotus() {
  echo -e "\nCompiling lotus binary for devnet usage\n"
  make clean && make 2k all
  sudo make install

}

main() {

  echo -e "\nUsing Lotus Path: $LOTUS_PATH \n"

  if [[ "$LOTUS_PURGE_LOCALDIR" = true ]]; then
    echo -e "Purging"
    purge_local_dirs
  fi


  if [[ "$LOTUS_COMPILE" = true ]]; then
    compile_lotus
  fi


  if [[ ! -f $LOTUS_BIN ]]; then
    echo -e "Lotus binary doesn't exist"
    echo -e "Please visit https://docs.lotu.sh/en+getting-started for compiling Lotus"
    echo -e "Please remember to compile lotus with the 2k flag:"
    echo -e "make 2k"
    exit 1
  fi

  configure_lotus
}

#### Main

main
