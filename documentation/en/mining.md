# Storage Mining

Here are instructions to learn how to perform storage mining. For hardware specifications please read [this](https://docs.lotu.sh/en+hardware-mining).

It is useful to [join the Testnet](https://docs.lotu.sh/en+join-testnet) prior to attempting storage mining for the first time.

## Note: Using the Lotus Miner from China

If you are trying to use `lotus-miner` from China. You should set this **environment variable** on your machine.

```sh
IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

## Get started

Please ensure that at least one **BLS address** (starts with `t3`) in your wallet exists with the following command:

```sh
lotus wallet list
```

If you do not have a bls address, create a new bls wallet:

```sh
lotus wallet new bls
```

With your wallet address:

- Visit the [faucet](https://faucet.testnet.filecoin.io)
- Click "Create Miner"
- DO NOT REFRESH THE PAGE. THIS OPERATION CAN TAKE SOME TIME.

The task will be complete when you see:

```sh
New miners address is: <YOUR_NEW_MINING_ADDRESS>
```

## Initialize the miner

In a CLI window, use the following command to start your miner:

```sh
lotus-miner init --actor=ACTOR_VALUE_RECEIVED --owner=OWNER_VALUE_RECEIVED
```

Example

```sh
lotus-miner init --actor=t01424 --owner=t3spmep2xxsl33o4gxk7yjxcobyohzgj3vejzerug25iinbznpzob6a6kexcbeix73th6vjtzfq7boakfdtd6a
```

You will have to wait some time for this operation to complete.

## Mining

To mine:

```sh
lotus-miner run
```

If you are downloading **Filecoin Proof Parameters**, the download can take some time.

Get information about your miner:

```sh
lotus-miner info
# example: miner id `t0111`
```

**Seal** random data to start producing **PoSts**:

```sh
lotus-miner sectors pledge
```

- Warning: On Linux configurations, this command will write data to `$TMPDIR` which is not usually the largest partition. You should point the value to a larger partition if possible.

Get **miner power** and **sector usage**:

```sh
lotus state power
# returns total power

lotus state power <miner>

lotus state sectors <miner>
```

## Performance tuning

### `FIL_PROOFS_MAXIMIZE_CACHING=1` Environment variable

This env var can be used with `lotus-miner`, `lotus-worker`, and `lotus-bench` to make the precommit1 step faster at the cost of some memory use (1x sector size)

### `FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1` Environment variable

This env var can be used with `lotus-miner`, `lotus-worker`, and `lotus-bench` to enable experimental precommit2 GPU acceleration
