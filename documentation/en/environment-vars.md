# Lotus Environment Variables

## Building

## Common

The environment variables are common across most lotus binaries.

### `LOTUS_FD_MAX`

Sets the file descriptor limit for the process. This should be set high (8192
or higher) if you ever notice 'too many open file descriptor' errors.

### `LOTUS_JAEGER`

This can be set to enable jaeger trace reporting. The value should be the url
of the jaeger trace collector, the default for most jaeger setups should be
`localhost:6831`.

### `LOTUS_DEV`

If set to a non-empty value, certain parts of the application will print more
verbose information to aid in development of the software. Not recommended for
end users.

## Lotus Daemon

### `LOTUS_PATH`

Sets the location for the lotus daemon on-disk repo. If left empty, this defaults to `~/.lotus`.

### `LOTUS_SKIP_GENESIS_CHECK`

Can be set to `_yes_` if you wish to run a lotus network with a different
genesis than the default one built into your lotus binary.

### `LOTUS_CHAIN_TIPSET_CACHE`

Sets the cache size for the chainstore tipset cache. The default value is 8192,
but if your usage of the lotus API involves frequent arbitrary tipset lookups,
you may want to increase this.

### `LOTUS_CHAIN_INDEX_CACHE`

Sets the cache size for the chainstore epoch index cache. The default value is 32768,
but if your usage of the lotus API involves frequent deep chain lookups for
block heights that are very far from the current chain height, you may want to
increase this.


### `LOTUS_BSYNC_MSG_WINDOW`

Set the initial maximum window size for message fetching blocksync requests. If
you have a slower internet connection and are having trouble syncing, you might
try lowering this down to 10-20 for a 'poor' internet connection.

## Lotus Miner

A number of environment variables are respected for configuring the behavior of the filecoin proving subsystem. For more details on those [see here](https://github.com/filecoin-project/rust-fil-proofs/#settings).

### `LOTUS_MINER_PATH`

Sets the location for the lotus miners on-disk repo. If left empty, this defaults to `~/.lotusminer`.


