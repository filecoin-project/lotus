# Storage Mining

Here are instructions to learn how to perform storage mining. For hardware specifications please read [this](https://lotu.sh/en+hardware-mining).

It is useful to [join the Testnet](https://lotu.sh/en+join-testnet) prior to attempting storage mining for the first time.

## Note: Using the Lotus Miner from China

If you are trying to use `lotus-miner` from China. You should set this **environment variable** on your machine.

```sh
export IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
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

- Visit the [faucet](http://spacerace.faucet.glif.io/)
- Paste the address you created under REQUEST.
- Press the Request button.
- Run `/lotus-miner init --owner=<blsAddress> --worker=<blsAddress>`

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

### Setting multiaddresses

Set multiaddresses for the miner to listen on in a miner's `config.toml` file
(by default, it is located at `~/.lotusminer/config.toml`). The `ListenAddresses` in this file should be interface listen addresses (usually `/ip4/0.0.0.0/tcp/PORT`), and the `AnnounceAddresses` should match the addresses being passed to `set-addrs`.

The addresses passed to `set-addrs` parameter in the commands below should be currently active and dialable; confirm they are before entering them.

Once the config file has been updated, set the on-chain record of the miner's listen addresses:

```
lotus-miner actor set-addrs <multiaddr_1> <multiaddr_2> ... <multiaddr_n>
```

This updates the `MinerInfo` object in the miner's actor, which will be looked up
when a client attempts to make a deal. Any number of addresses can be provided.

Example:

```
lotus-miner actor set-addrs /ip4/123.123.73.123/tcp/12345 /ip4/223.223.83.223/tcp/23456
```

# Separate address for windowPoSt messages

WindowPoSt is the mechanism through which storage is verified in Filecoin. It requires miners to submit proofs for all sectors every 24h, which require sending messages to the chain.

Because many other mining related actions require sending messages to the chain, and not all of those are "high value", it may be desirable to use a separate account to send PoSt messages from. This allows for setting lower GasFeeCaps on the lower value messages without creating head-of-line blocking problems for the PoSt messages in congested chain conditions

To set this up, first create a new account, and send it some funds for gas fees:
```sh
lotus wallet new bls
t3defg...

lotus send t3defg... 100
```

Next add the control address
```sh
lotus-miner actor control set t3defg...
Add t3defg...
Pass --really-do-it to actually execute this action
```

Now actually set the addresses
```sh
lotus-miner actor control set --really-do-it t3defg...
Add t3defg...
Message CID: bafy2..
```

Wait for the message to land on chain
```sh
lotus state wait-msg bafy2..
...
Exit Code: 0
...
```

Check miner control address list to make sure the address was correctly setup
```sh
lotus-miner actor control list
name       ID      key           use    balance
owner      t01111  t3abcd...  other  300 FIL
worker     t01111  t3abcd...  other  300 FIL
control-0  t02222  t3defg...  post   100 FIL
```
