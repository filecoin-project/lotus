# Mining Troubleshooting

```sh
lotus-storage-miner info
# WARN  main  lotus-storage-miner/main.go:73  failed to get api endpoint: (/Users/myrmidon/.lotusstorage) %!w(*errors.errorString=&{API not running (no endpoint)}):
```

If you see this, that means your **Lotus Storage Miner** isn't ready yet.

```sh
CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up
```

If you see this, that means your computer is too slow and your blocks are not included in the chain, and you will not receive any rewards.