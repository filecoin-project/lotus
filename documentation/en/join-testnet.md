# Join Testnet

## Introduction

Anyone can set up a **Lotus Node** and connect to the **Lotus Testnet**. This is the best way to explore the current CLI and the **Filecoin Decentralized Storage Market**.

If you have installed older versions, you may need to clear existing chain data, stored wallets and miners if you run into any errors. You can use this command:

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

In order to connect to the network, you need to be connected to at least 1 peer. If you’re seeing 0 peers, read our [troubleshooting notes](https://docs.lotu.sh/en+setup-troubleshooting).

## Chain sync

While the daemon is running, the next requirement is to sync the chain. Run the command below to start the chain sync progress. To see current chain height, visit the [network stats page](http://stats.testnet.filecoin.io/).

```sh
lotus sync wait
```

- This step will take anywhere between 30 minutes to a few hours.
- You will be able to perform **Lotus Testnet** operations after it is finished.

## Create your first address

Initialize a wallet using BLS signature formats:

```sh
lotus wallet new bls
```

Here is an example of the response:

```sh
t3vhfme4qfvegqaz7m7q6o6afjcs67n6kpzv7t2eozio4chwpafwa2y4l7zhwd5eom7jmihzdg4s52dpvnclza
```

- Visit the [faucet](https://lotus-faucet.kittyhawk.wtf/funds.html) to add funds.
- Paste the address you created.
- Press the send button.

## Check wallet address balance

Wallet balances in the Lotus Testnet are in **FIL**, the smallest denomination of FIL is an **attoFil**, where 1 attoFil = 10^-18 FIL.

```sh
lotus wallet balance <YOUR_NEW_ADDRESS>
```

You will not see any attoFIL in your wallet if your **chain** is not fully synced.

## Send FIL to another wallet

To send FIL to another wallet, use this command:

```
lotus send <target> <amount>
```

## Monitor the dashboard

To see the latest network activity, including **chain block height**, **block height**, **blocktime**, **total network power**, largest **block producer miner**, check out the [monitoring dashboard](https://stats.testnet.filecoin.io).
