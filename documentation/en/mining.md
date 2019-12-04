# Getting started

Ensure that at least one **BLS address** (`t3..`) in your wallet exists

```sh
$ lotus wallet list
t3...
```

With this address, go to the [faucet](https://lotus-faucet.kittyhawk.wtf/miner.html), and
click `Create Miner`

Wait for a page telling you the address of the newly created **Lotus Storage Miner** to appear.

The screen should show: `New storage miners address is: t0..`

## Initialize

```sh
$ lotus-storage-miner init --actor=t01.. --owner=t3....
```

This command should return successfully after **Lotus Storage Miner** is setup on **chain**. It usually takes 30 to 60 seconds.

## Start mining

```sh
$ lotus-storage-miner run
```

To view the miner id used for deals:

```sh
$ lotus-storage-miner info
```

e.g. miner id `t0111`

**Seal** random data to start producing **PoSts**:

```sh
$ lotus-storage-miner store-garbage
```

You can check **miner power** and **sector** usage with the miner id:

```sh
# Total power of the network
$ lotus-storage-miner state power

$ lotus-storage-miner state power <miner>

$ lotus-storage-miner state sectors <miner>
```

## Assign a nickname for your node

In the `.lotus` folder, modify `config.toml` with:

```sh
[Metrics]
Nickname="snoopy"
```
