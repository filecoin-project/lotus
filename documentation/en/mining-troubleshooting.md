# Mining Troubleshooting

## Config: Filecoin Proof Parameters directory

If you want to put the **Filecoin Proof Parameters** in a different directory, use the following environment variable:

```sh
FIL_PROOFS_PARAMETER_CACHE
```

## Error: Can't acquire bellman.lock

The **Bellman** lockfile is created to lock a GPU for a process. This bug can occur when this file isn't properly cleaned up:

```sh
mining block failed: computing election proof: github.com/filecoin-project/lotus/miner.(*Miner).mineOne
```

This bug occurs when the storage miner can't acquire the `bellman.lock`. To fix it you need to stop the `lotus-storage-miner` and remove `/tmp/bellman.lock`.

## Error: Failed to get api endpoint

```sh
lotus-storage-miner info
# WARN  main  lotus-storage-miner/main.go:73  failed to get api endpoint: (/Users/myrmidon/.lotusstorage) %!w(*errors.errorString=&{API not running (no endpoint)}):
```

If you see this, that means your **Lotus Storage Miner** isn't ready yet. You need to finish [syncing the chain](https://docs.lotu.sh/en+join-testnet).

## Error: Your computer may not be fast enough

```sh
CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up
```

If you see this, that means your computer is too slow and your blocks are not included in the chain, and you will not receive any rewards.

## Error: No space left on device

```sh
lotus-storage-miner sectors pledge
# No space left on device (os error 28)
```

If you see this, that means `pledge-sector` wrote too much data to `$TMPDIR` which by default is the root partition (This is common for Linux setups). Usually your root partition does not get the largest partition of storage so you will need to change the environment variable to something else.

## Error: GPU unused

If you suspect that your GPU is not being used, first make sure it is properly configured as described in the [testing configuration page](hardware-mining.md). Once you've done that (and set the `BELLMAN_CUSTOM_GPU` as appropriate if necessary) you can verify your GPU is being used by running a quick lotus-bench benchmark.

First, to watch GPU utilization run `nvtop` in one terminal, then in a separate terminal, run:

```sh
make bench
./bench sealing --sector-size=2KiB
```

This process uses a fair amount of GPU, and generally takes ~4 minutes to complete. If you do not see any activity in nvtop from lotus during the entire process, it is likely something is misconfigured with your GPU.

## Checking Sync Progress

You can use this command to check how far behind you are on syncing:

```sh
date -d @$(./lotus chain getblock $(./lotus chain head) | jq .Timestamp)
```
