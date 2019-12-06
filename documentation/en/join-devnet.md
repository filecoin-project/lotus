# Join DevNet

## Introduction

Anyone can set up a **Lotus Node** and connect to the **Lotus DevNet**. This is the best way to explore the current CLI and the **Filecoin Decentralized Storage Market**.

If you have run Lotus before, you may need to clear existing data if you encounter errors.

```sh
rm -rf ~/.lotus ~/.lotusstorage
```

## Get started

Start the **daemon** using the default configuration in `./build`:

```sh
lotus daemon
```

In another terminal window, check your connection with peers:

```sh
lotus net peers | wc -l
```

Synchronize the **chain**:

```sh
lotus sync wait
```

Congrats! Now you can perform **Lotus DevNet** operations.

## Exploring the chain

View **chain block height** along with other network metrics at our [chain explorer](https://lotus-metrics.kittyhawk.wtf/chain).

## Create a new address

```sh
lotus wallet new bls
t3...
```

- Visit the [faucet](https://lotus-faucet.kittyhawk.wtf/funds.html)
- Paste the address you created.
- Press the send button.

## Check wallet address balance

Wallet balances for the DevNet are in attoFIL. 1 attoFIL = 10^-18 FIL

```sh
lotus wallet balance [optional address (t3...)]
```

You will not see any attoFIL in your wallet if your **chain** is not fully synced.

## Monitoring Dashboard

To see the latest network activity, including **chain block height**, **blocktime**, **total network power**, largest **block producer miner**, check out the [monitoring dashboard](https://lotus-metrics.kittyhawk.wtf).
