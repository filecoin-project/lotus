# Join Lotus Devnet

## Node CLI setup

If you have run lotus before and want to remove all previous data: `rm -rf ~/.lotus ~/.lotusstorage`

## Genesis & Bootstrap

The current lotus build will automatically join the lotus Devnet using the genesis and bootstrap files in the `build/` directory. No configuration is needed.

## Start Daemon

```sh
$ lotus daemon
```

In another window check that you are connected to the network:

```sh
$ lotus net peers | wc -l
2 # number of peers
```

Wait for the chain to finish syncing:

```sh
$ lotus sync wait
```

You can view latest block height along with other network metrics at the https://lotus-metrics.kittyhawk.wtf/chain.

## Basics

Create a new address:

```sh
$ lotus wallet new bls
t3...
```

Grab some funds from faucet - go to https://lotus-faucet.kittyhawk.wtf/, paste the address
you just created, and press Send.

Check the wallet balance (balance is listed in attoFIL, where 1 attoFIL = 10^-18 FIL):

```sh
$ lotus wallet balance [optional address (t3...)]
```

If you see an error like `actor not found` after executing this command, it means that either your node isn't fully synced or there are no transactions to this address yet on chain. If the latter, using the faucet should 'fix' this.

## Make a deal

It is possible for a Client to make a deal with a Miner on the same lotus Node.

```sh
# List all miners in the system. Choose one to make a deal with.

$ lotus state list-miners

# List asks proposed by a miner

$ lotus client query-ask <miner>

# Propose a deal with a miner. Price is in attoFIL/byte/block. Duration is # of blocks.

$ lotus client deal <Data CID> <miner> <price> <duration>
```

For example `\$ lotus client deal bafkre...qvtjsi t0111 36000 12` proposes a deal to store CID `bafkre...qvtjsi` with miner `t0111` at price `36000` for a duration of `12` blocks. If successful, the `client deal` command will return a deal CID.

## Monitoring Dashboard

To see the latest network activity, including chain block height, blocktime, total network power, largest miners, and more, check out the [monitoring dashboard](https://lotus-metrics.kittyhawk.wtf).
