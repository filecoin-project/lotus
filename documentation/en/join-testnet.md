# Join Testnet

## Introduction

Anyone can set up a **Lotus Node** and connect to the **Lotus Testnet**. This is the best way to explore the current CLI and the **Filecoin Decentralized Storage Market**.

## Note: Using the Lotus Node from China

If you are trying to use `lotus` from China. You should set this **environment variable** on your machine:

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
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

Make sure that you have a reasonable "open files limit" set on your machine, such as 10000. If you're seeing a lower value, such as 256 (default on macOS), read our [troubleshooting notes](https://docs.lotu.sh/en+setup-troubleshooting) on how to update it prior to starting the Lotus daemon.

## Chain sync

While the daemon is running, the next requirement is to sync the chain. Run the command below to view the chain sync progress. To see current chain height, visit the [network stats page](https://stats.testnet.filecoin.io/).

```sh
lotus sync wait
```

- This step will take anywhere between a few hours to a couple of days.
- You will be able to perform **Lotus Testnet** operations after it is finished.

## Create your first address

Initialize a new wallet:

```sh
lotus wallet new
```

Sometimes your operating system may limit file name length to under 150 characters. You need to use a file system that supports long filenames.

Here is an example of the response:

```sh
t1aswwvjsae63tcrniz6x5ykvsuotlgkvlulnqpsi
```

- Visit the [faucet](https://faucet.testnet.filecoin.io) to add funds.
- Paste the address you created.
- Press the send button.

## Check wallet address balance

Wallet balances in the Lotus Testnet are in **FIL**, the smallest denomination of FIL is an **attoFil**, where 1 attoFil = 10^-18 FIL.

```sh
lotus wallet balance <YOUR_NEW_ADDRESS>
```

You will not see any attoFIL in your wallet if your **chain** is not fully synced.

## Send FIL to another wallet

To send FIL to another wallet from your default account, use this command:

```
lotus send <target> <amount>
```

## Configure your node's connectivity

To effectively accept incoming storage & retrieval deals, your Lotus node needs to be accessible to other nodes on the network. To improve your connectivity, be sure to: 

- [Set the multiaddresses for you miner to listen on](https://docs.filecoin.io/mine/connectivity/#setting-multiaddresses)
- [Maintain a healthy peer count](https://docs.filecoin.io/mine/connectivity/#checking-peer-count)
- [Enable port forwarding](https://docs.filecoin.io/mine/connectivity/#port-forwarding)
- [Configure your public IP address and port](https://docs.filecoin.io/mine/connectivity/#setting-a-public-ip-address)

## Monitor the dashboard

To see the latest network activity, including **chain block height**, **block height**, **blocktime**, **total network power**, largest **block producer miner**, check out the [monitoring dashboard](https://stats.testnet.filecoin.io).
