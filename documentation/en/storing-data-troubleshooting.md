# Storage Troubleshooting

```sh
WARN  main  lotus/main.go:72  routing: not found
```

- This miner is offline.

```sh
WARN  main  lotus/main.go:72  failed to start deal: computing commP failed: generating CommP: Piece must be at least 127 bytes
```

- There is a minimum file size of 127 bytes.

## Troubleshooting Sealing

Miners can check sealing progress with this command:

```sh
lotus-storage-miner sectors list
```

When sealing is complete, `pSet: NO` will become `pSet: YES`. From now on the
**Data CID** is [retrievable](https://docs.lotu.sh/en+retrieving-data) from the
**Lotus Storage Miner**.
