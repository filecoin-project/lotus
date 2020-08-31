# Storage Troubleshooting

## Error: Routing: not found

```
WARN  main  lotus/main.go:72  routing: not found
```

This error means that the miner is offline.

## Error: Failed to start deal

```sh
WARN  main  lotus/main.go:72  failed to start deal: computing commP failed: generating CommP: Piece must be at least 127 bytes
```

This error means that there is a minimum file size of 127 bytes.

## Error: 0kb file response during retrieval

This means that the file to be retrieved may have not yet been sealed and is thus, not retrievable yet.

Miners can check sealing progress with this command:

```sh
lotus-miner sectors list
```

When sealing is complete, `pSet: NO` will become `pSet: YES`.

