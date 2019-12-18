# Storage Mining

Here are instructions to learn how to perform storage mining. For hardware specifications please read [this](https://docs.lotu.sh/en+hardware-mining).

It is useful to [join the Testnet](https://docs.lotu.sh/en+join-testnet) prior to attempting storage mining for the first time.

## Note: Filecoin Proof Parameters

If you are trying to download the **Filecoin Proof Parameters** from within China, you can download them from [JDCloud CDN](https://s3.cn-south-1.jdcloud-oss.com/proof-parameters/filecoin-proof-parameters-v20-testnet.tar.gz). Once you do you will need to extract them to:

```sh
# put the .param and .vk files in the directory below
/var/tmp/filecoin-proof-parameters
```

## Get started

Please ensure that at least one **BLS address** in your wallet exists with the following command:

```sh
lotus wallet list
```

With your wallet address:

- Visit the [faucet](https://lotus-faucet.kittyhawk.wtf/miner.html)
- Click "Create Miner"
- DO NOT REFRESH THE PAGE. THIS OPERATION CAN TAKE SOME TIME.

The task will be complete when you see:

```sh
New storage miners address is: <YOUR_NEW_MINING_ADDRESS>
```

## Initialize the storage miner

In a CLI window, use the following command to start your miner:

```sh
lotus-storage-miner init --actor=ACTOR_VALUE_RECEIVED --owner=OWNER_VALUE_RECEIVED
```

Example

```sh
lotus-storage-miner init --actor=t01424 --owner=t3spmep2xxsl33o4gxk7yjxcobyohzgj3vejzerug25iinbznpzob6a6kexcbeix73th6vjtzfq7boakfdtd6a
```

You will have to wait some time for this operation to complete.

## Mining

To mine:

```sh
lotus-storage-miner run
```

If you are downloading **Filecoin Proof Parameters**, the download can take some time.

Get information about your miner:

```sh
lotus-storage-miner info
# example: miner id `t0111`
```

**Seal** random data to start producing **PoSts**:

```sh
lotus-storage-miner pledge-sector
```

- Warning: On Linux configurations, this command will write data to `$TMPDIR` which is not usually the largest partition. You should point the value to a larger partition if possible.

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
Nickname="fun"
```
