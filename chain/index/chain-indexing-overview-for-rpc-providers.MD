# ChainIndexer Documentation for RPC Providers

# Table of Contents

- [ChainIndexer Documentation for RPC Providers](#chainindexer-documentation-for-rpc-providers)
  - [Introduction](#introduction)
  - [ChainIndexer Config](#chainindexer-config)
    - [Enablement](#enablement)
    - [Garbage Collection](#garbage-collection)
      - [Recommendations](#recommendations)
    - [Removed Options](#removed-options)
  - [Migration Guide](#migration-guide)
    - [Backfilling](#backfilling)
      - [The `ChainValidateIndex` JSON RPC API](#the-chainvalidateindex-json-rpc-api)
      - [`lotus-shed chainindex validate-backfill` CLI tool](#lotus-shed-chainindex-validate-backfill-cli-tool)
        - [Usage](#usage)
        - [Parameters](#parameters)
        - [Error conditions](#error-conditions)
        - [Logging](#logging)
        - [Example usage](#example-usage)
  - [Rollback](#rollback)
  - [Need more help?](#need-more-help)

## Introduction

This document is for RPC providers and node operators who serve RPC requests walking through the configuration changes, migration flow and operations/maintenance work needed to enable, backfill and maintain the `ChainIndexer`.

**Note: If you are a Storage Provider or node operator who does not serve RPC requests, you can skip this document as the `ChainIndexer` is already disabled by default**. 
 

## ChainIndexer Config
### Enablement

The following must be enabled on an RPC node before starting as they are disabled by default:

```toml
[ChainIndexer]
# Enable the ChainIndexer. 
  EnableIndexer = true 

[Fevm]
# Enable the ETH RPC APIs.
  EnableEthRPC = true

[Events]
# Enable the Actor Events APIs.
  EnableActorEventsAPI = true
```

You can learn more about these configuration options and other configuration options available for the `ChainIndexer` [here](https://github.com/filecoin-project/lotus/blob/master/documentation/en/default-lotus-config.toml).


### Garbage Collection

The `ChainIndexer` includes a garbage collection (GC) mechanism to manage the amount of historical data retained. By default, GC is disabled to preserve all indexed data.

To configure GC, use the `GCRetentionEpochs` parameter in the `ChainIndexer` section of your config.

The ChainIndexer [periodically runs](https://github.com/filecoin-project/lotus/blob/master/chain/index/gc.go#L15) GC if `GCRetentionEpochs` is > 0 and removes indexed data for epochs older than `(current_head_height - GCRetentionEpochs)`.

```toml
[ChainIndexer]
  GCRetentionEpochs = X  # Replace X with your desired value
```

- Setting `GCRetentionEpochs` to 0 (**default**) disables GC.
- Any positive value enables GC and determines the number of epochs of historical data to retain.

#### Recommendations

1. **Archival Nodes**: **Keep GC disabled** (`GCRetentionEpochs` = 0) to retain all indexed data.

2. **Non-Archival Nodes**:  Set `GCRetentionEpochs` to match the amount of chain state your node retains 
(*Example:* if your node is configured to retain 2 days of Filecoin chain state with the Splitstore, set `GCRetentionEpochs` to `retentionDays * epochsPerDay = 2 * 2880 = 5760`.)

### Removed Options

**Note: The following config options no longer exist in Lotus and have been removed in favor of the ChainIndexer config options explained above. They can be removed when upgrading to Lotus v1.31.0.**

```toml
[Fevm]
EthTxHashMappingLifetimeDays = 0

[Events]
DisableHistoricFilterAPI = false
DatabasePath = ""
```

## Migration Guide

Migrating to the new `ChainIndexer` involves several steps to ensure a smooth transition:

1. **Stop the Lotus Node**
   - Stop your Lotus node before starting the upgrade and migration process.

2. **Backup Existing Index Databases**
   - Back up the directory containing your existing index databases (`MsgIndex`, `EthTxIndex`, and `EventIndex`).
   - These databases are located in the `{$LOTUS_PATH/sqlite}` directory.
   - Use the following command to copy the entire directory:
     ```bash
     cp -r $LOTUS_PATH/sqlite {destination_path}
     ```
   - **Note: If you have configured a custom directory path for the Index databases using the `Events.DatabasePath` config option, replace `{$LOTUS_PATH/sqlite}` with your custom path.**
   - These backups are essential for potential rollbacks, even though they are not used in the migration process.

3. **Remove Old Index Files**
   - After creating backups, remove the `{$LOTUS_PATH/sqlite}` directory (*or your custom index database path*) using the following command:
     ```bash
     rm -rf $LOTUS_PATH/sqlite
     ```
   - **Warning: Please ensure and validate that you have made backups of your existing index databases before removing the directory.**

4. **Update Configuration**
   - Modify your Lotus configuration to enable the `ChainIndexer` as described in the [`ChainIndexer Config` section above](#chainindexer-config).

5. **Restart Lotus Node**
   - Restart your Lotus node with the new configuration.
   - The `ChainIndexer` will begin indexing **real-time chain state changes** immediately.
   - **However, it will not automatically index any historical chain state (i.e., any previously existing chain state). To perform backfilling, please see the [`Backfilling` section below](#backfilling).**

### Backfilling
To index historical chain state (i.e., **"backfilling"**), you can use the following tools:

#### `ChainValidateIndex` JSON RPC API

Please refer to the [Lotus API documentation](https://github.com/filecoin-project/lotus/blob/master/documentation/en/api-v1-unstable-methods.md) for detailed documentation of the `ChainValidateIndex` JSON RPC API.

The `ChainValidateIndex` JSON RPC API serves a dual purpose: it validates/diagnoses the integrity of the index at a specific epoch (i.e., it ensures consistency between indexed data and actual chain state), while also providing the option to backfill the `ChainIndexer` if it does not have data for the specified epoch. 

Here are some examples of how to use the `ChainValidateIndex` JSON RPC API for validating/ backfilling the index:


1) Validating the index for an epoch that is a NULL round:
 
 ```bash
 curl -X POST -H "Content-Type: application/json" --data '{
  "jsonrpc": "2.0",
  "method": "Filecoin.ChainValidateIndex",
  "params": [1954383, false],
  "id": 1
}' http://localhost:1234/rpc/v1 | jq .
```
```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "result": {
    "TipSetKey": [],
    "Height": 1954383,
    "IndexedMessagesCount": 0,
    "IndexedEventsCount": 0,
    "Backfilled": false,
    "IsNullRound": true
  }
}
```

2) Validating the Index for an epoch for which the Indexer has missing data with backfilling disabled:

```bash
curl -X POST -H "Content-Type: application/json" --data '{
  "jsonrpc": "2.0",
  "method": "Filecoin.ChainValidateIndex",
  "params": [1995103, false],
  "id": 1
}' http://localhost:1234/rpc/v1 | jq .
```
```json
{
  "error": {
    "code": 1,
    "message": "missing tipset at height 1995103 in the chain index, set backfill flag to true to fix"
  },
  "id": 1,
  "jsonrpc": "2.0"
}
```

3) Validating the Index for an epoch for which the Indexer has missing data with backfilling enabled:

```bash
curl -X POST -H "Content-Type: application/json" --data '{
  "jsonrpc": "2.0",
  "method": "Filecoin.ChainValidateIndex",
  "params": [1995103, true],
  "id": 1
}' http://localhost:1234/rpc/v1 | jq .
```
```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "result": {
    "TipSetKey": [
      {
        "/": "bafy2bzacebvzbpbdwxsclwyorlzclv6cbsvcbtq34sajow2sn7mnksy3wehew"
      },
      {
        "/": "bafy2bzacedgei4ve3spkfp3oou5oajwd5cogn7lljsuvoj644fgj3gv7luamu"
      },
      {
        "/": "bafy2bzacebbpcnjoi46obpaheylyxfy5y2lrtdsyglqw3hx2qg64quip5u76s"
      }
    ],
    "Height": 1995103,
    "IndexedMessagesCount": 0,
    "IndexedEventsCount": 0,
    "Backfilled": true,
    "IsNullRound": false
  }
}
```

The `ChainValidateIndex` RPC API  is available for use once the Lotus daemon has started with `ChainIndexer` [enabled](#enablement). 

#### `lotus-shed chainindex validate-backfill` CLI tool
The `lotus-shed chainindex validate-backfill` command is a tool for validating and optionally backfilling the chain index over a range of epochs since calling the `ChainValidateIndex` API for a single epoch at a time can be cumbersome, especially when backfilling or validating the index over a range of historical epochs, such as during a migration. It wraps the `ChainValidateIndex` API to efficiently process multiple epochs.

**Note: This command can only be run when the Lotus daemon is already running with the [`ChainIndexer` enabled](#enablement) as it depends on the `ChainValidateIndex` RPC API.**

#### Usage:
```
lotus-shed chainindex validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good]
```

The command validates the chain index entries for each epoch in the specified range, checking for missing or inconsistent entries(i.e. the indexed data does not match the actual chain state). If `--backfill` is enabled (which it is by default), it will attempt to backfill any missing entries using the `ChainValidateIndex` API.

#### Parameters:
- `--from` (required): The starting epoch (inclusive) for the validation range. Must be greater than 0.
- `--to` (required): The ending epoch (inclusive) for the validation range. Must be greater than 0 and less than or equal to `from`.
- `--backfill` (optional, default: true): Whether to backfill missing index entries during validation. 
- `--log-good` (optional, default: false): Whether to log details for tipsets that have no detected problems.

#### Error conditions:
- If `from` or `to` are invalid (<=0 or `to` > `from`), an error is returned.
- If the `ChainValidateIndex` API returns an error for an epoch, indicating an inconsistency between the index and chain state, an error message is logged for that epoch.

#### Logging:
- **Progress is logged every 2880 epochs (1 day worth of epochs) processed during the validation process.**
- If `--log-good` is enabled, details are also logged for each epoch that has no detected problems. This includes:
  - Null rounds with no messages/events.
  - Epochs with a valid indexed entry.

#### Example usage

To validate and backfill the chain index for the last 5760 epochs (2 days) and log details for all epochs:

```
lotus-shed chainindex validate-backfill --from 1000000 --to 994240 --log-good 
```

This command is useful for backfilling the chain index over a range of historical epochs during the migration to the new `ChainIndexer`. **It can also be run periodically to validate the index's integrity using system schedulers like `cron`.**

## Rollback

In case you need to rollback to the previous indexing system (`EthTxIndex`, `MsgIndex`, and `EventIndex`), follow these steps:

1. Stop your Lotus node.
2. Remove the current `${LOTUS_PATH}/sqlite` directory and replace it with the backup taken in the "**Backup Existing Index Databases**" section of the [Migration Guide](#migration-guide).
3. Build your Lotus binary for the rollback version which has the old `EthTxIndex`, `MsgIndex`, and `EventIndex` Indices.
4. Ensure that you've set the correct config for the existing `EthTxIndex`, `MsgIndex`, and `EventIndex` indices in the `config.toml` file.
5. Restart your Lotus node.
6. Backfill the `EthTxIndex`, `MsgIndex`, and `EventIndex` indices using the `lotus-shed index backfill-*` CLI tooling available in the previous indexing system for the epoch range in [epochs between the upgrade to `ChainIndexer` and the rollback of `ChainIndexer`].

## Need more help?
Please free to ask questions on `#fil-lotus-dev` on Filecoin Slack or create issues on [Lotus GitHub](https://github.com/filecoin-project/lotus/issues) for any questions/bugs/comments/concerns.