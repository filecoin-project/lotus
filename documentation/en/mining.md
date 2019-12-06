# Storage Mining

Here are instructions to learn how to perform storage mining. For hardware specifications please read [this](https://docs.lotu.sh/en+hardware-mining).

It is useful to [join the DevNet](https://docs.lotu.sh/en+join-devnet) prior to attempting storage mining for the first time.

## Get started

Please ensure that at least one **BLS address** (`t3..`) in your wallet exists with the following command:

```sh
lotus wallet list
```

With this address, go to the [faucet](https://lotus-faucet.kittyhawk.wtf/miner.html), and
click `Create Miner`

Await this response:

```sh
To initialize the storage miner run the following command
```

## Initialize the storage miner

```sh
lotus-storage-miner init --actor=ACTOR_VALUE_RECEIVED --owner=OWNER_VALUE_RECEIVED
```

Example

```sh
lotus-storage-miner init --actor=t01424 --owner=t3spmep2xxsl33o4gxk7yjxcobyohzgj3vejzerug25iinbznpzob6a6kexcbeix73th6vjtzfq7boakfdtd6a
```

This command will take 30-60 seconds.

## Mining

To mine:

```sh
lotus-storage-miner run
```

Get information about your miner:

```sh
lotus-storage-miner info
# example: miner id `t0111`
```

**Seal** random data to start producing **PoSts**:

```sh
lotus-storage-miner store-garbage
```

Get **miner power** and **sector usage**:

```sh
lotus-storage-miner state power
# returns total power

lotus-storage-miner state power <miner>

lotus-storage-miner state sectors <miner>
```

## Change nickname

Update `~/.lotus/config.toml` with:


```sh
[Metrics]
Nickname="snoopy"
```

## Troubleshooting

```sh
lotus-storage-miner info
# WARN  main  lotus-storage-miner/main.go:73  failed to get api endpoint: (/Users/myrmidon/.lotusstorage) %!w(*errors.errorString=&{API not running (no endpoint)}):
```

If you see this, that means your **Lotus Storage Miner** isn't ready yet.