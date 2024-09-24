# ChainIndexer Documentation for RPC Providers

## Introduction

We're shipping a new Indexer implementation in Lotus (`ChainIndexer`) to index Filecoin chain state such as tipsets, messages, events and ETH transactions for reliable and faster RPC responses. The `ChainIndexer` replaces the existing `MsgIndex`, `EthTxIndex` and `EventIndex` implementations in Lotus which suffer from a multitude of known problems documented [here](https://github.com/filecoin-project/lotus/issues/12293). 

**Note: If you are a Storage Provider or node operator who does not serve RPC requests, you can skip this document as the `ChainIndexer` is already disabled by default**. 

This document is aimed at RPC providers and node operators who serve RPC requests and aims to walk through the configuration changes, migration flow and operations/maintenance work needed to enable, backfill and maintain the `ChainIndexer`. 

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

### Garbage Collection

The `ChainIndexer` includes a garbage collection (GC) mechanism to manage the amount of historical data retained. By default, GC is disabled to preserve all indexed data.

To configure GC, use the `GCRetentionEpochs` parameter in the `ChainIndexer` section of your config.

The ChainIndexer periodically runs GC if `GCRetentionEpochs` is > 0 and removes indexed data for epochs older than `(current_head_height - GCRetentionEpochs)`.

```toml
[ChainIndexer]
  GCRetentionEpochs = X  # Replace X with your desired value
```

- Setting `GCRetentionEpochs` to 0 (**default**) disables GC.
- Any positive value enables GC and determines the number of epochs of historical data to retain.

#### Recommendations

1. **Archival Nodes**: **Keep GC disabled** (`GCRetentionEpochs` = 0) to retain all indexed data.

2. **Non-Archival Nodes**:  Set `GCRetentionEpochs` to match the amount of chain state your node retains 
(*for example:* if your node is configured to retain 2 days of Filecoin chain state with the Splitstore, set `GCRetentionEpochs` to (number of Filecoin epochs in a day *2) = 5760).

### Removed Options

**Note: The following config options no longer exist in Lotus and have been removed in favor of the `ChainIndexer` config options explained above:**

```toml
[Fevm]
EthTxHashMappingLifetimeDays = 0

[Events]
DisableHistoricFilterAPI = false
DatabasePath = ""
```


## Migration Guide

Migrating to the new `ChainIndexer` involves several steps to ensure a smooth transition:

1. **Backup Existing Index Databases**
   - Before restarting your Lotus node, create a backup of your existing index databases.
   - These files are located in the `{$LOTUS_PATH/sqlite}` directory.
   - While not used in the migration process, these backups are crucial for potential rollbacks.

2. **Remove Old Index Files**
   - After creating backups, remove the SQLite database files for `MsgIndex`, `EthTxIndex`, and `EventIndex` from the `{$LOTUS_PATH/sqlite}` directory.

3. **Update Configuration**
   - Modify your Lotus configuration to enable the `ChainIndexer` as described in the [`ChainIndexer Config` section above](#chainindexer-config] .

4. **Restart Lotus Node**
   - Restart your Lotus node with the new configuration.
   - The `ChainIndexer` will begin indexing **real-time chain state changes** immediately.

### Backfilling
Once Lotus starts with the `ChainIndexer` enabled, it will begin indexing real-time chain state changes (i.e., new incoming tipsets). However, it will not index any historical chain state (i.e., any previously existing chain state). To index historical chain state (i.e., **"backfilling"**), you can use the following tools.

#### The `ChainValidateIndex` JSON RPC API

The `ChainValidateIndex` JSON RPC API serves a dual purpose: it validates/diagnoses the integrity of the index at a specific epoch (i.e., it ensures consistency between indexed data and actual chain state), while also providing the option to backfill the `ChainIndexer` if it does have data for the specified epoch. 

```go
// IndexValidation contains detailed information about the validation status of a specific chain epoch.
type IndexValidation struct {
	// TipSetKey is the key of the canonical tipset for this epoch.
	TipSetKey TipSetKey
	// Height is the epoch height at which the validation is performed.
	Height uint64
	// IndexedMessagesCount indicates the number of indexed messages for the canonical tipset at this epoch.
	IndexedMessagesCount uint64
	// IndexedEventsCount signifies the number of indexed events for the canonical tipset at this epoch.
	IndexedEventsCount uint64
	// Backfilled denotes whether missing data was successfully backfilled into the index during validation.
	Backfilled bool
	// IsNullRound indicates if the epoch corresponds to a null round and therefore does not have any indexed messages or events.
	IsNullRound bool
}

// ChainValidateIndex validates the integrity of the chain index at a specified epoch and also optionally backfills missing data.
//
// Parameters:
//   - epoch: The specific chain epoch for which to validate/backfill the index.
//   - backfill: A boolean flag indicating whether to attempt backfilling of missing data if the index does not have data for the 
//               specified epoch.
//
// Returns:
//   - *types.IndexValidation: A pointer to an IndexValidation struct containing the results of the validation/backfill.
//   - error: An error object if the validation/backfill fails. The error message will contain details about the index 
//            corruption if the call fails because of an incosistency between indexed data and the actual chain state.
//
// Note: The API returns an error if the index does not have data for the specified epoch and backfill is set to false.
func ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error)
```

The `ChainValidateIndex` API serves multiple purposes:

1. Validates the chain index at a specific epoch:
   - Ensures consistency between indexed data and actual chain state
   - Reports any errors found during validation

2. Optionally backfills missing data:
   - Backfills data if the index is missing information for the specified epoch
   - Backfilling only occurs when the `backfill` parameter is set to `true`

3. Detects "holes" in the index:
   - If `backfill` is `false` and the index lacks data for the specified epoch, the API returns an error indicating missing data

The `ChainValidateIndex` RPC API  is available for use once the Lotus daemon has started with [`ChainIndexer` enabled](#link-to-section). 

#### `lotus-shed chainindex validate-backfill` tool
The `lotus-shed chainindex validate-backfill` command is a tool for validating and optionally backfilling the chain index over a range of epochs since calling the API for a single epoch at a time can be cumbersome, especially when backfilling or validating the index over a range of historical epochs, such as during a migration. It wraps the `ChainValidateIndex` API to efficiently process multiple epochs.

**Note: This command can only be run when the Lotus daemon is already running with the [`ChainIndexer` enabled](#link-to-appropriate-sectoin) as it depends on the [`ChainValidateIndex` RPC API](#link-to-appropriate-section).**

#### Usage:
```
lotus-shed chainindex validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good]
```

#### Parameters:
- `--from` (required): The starting epoch (inclusive) for the validation range. Must be greater than 0.
- `--to` (required): The ending epoch (inclusive) for the validation range. Must be greater than 0 and less than or equal to `from`.
- `--backfill` (optional, default: true): Whether to backfill missing index entries during validation. 
- `--log-good` (optional, default: false): Whether to log details for tipsets that have no detected problems.

The command validates the chain index entries for each epoch in the specified range, checking for missing or inconsistent entries. If `--backfill` is enabled (which it is by default), it will attempt to backfill any missing entries using the `ChainValidateIndex` API.

#### Error conditions:
- If `from` or `to` are invalid (<=0 or `to` > `from`), an error is returned.
- If the `ChainValidateIndex` API returns an error for an epoch, indicating an inconsistency between the index and chain state, an error message is logged for that epoch.

#### Logging:
- **Progress is logged every 2880 epochs (1 day worth of epochs) processed during the validation process.**
- If `--log-good` is enabled, details are also logged for each epoch that has no detected problems. This includes:
  - Null rounds with no messages/events.
  - Epochs with a valid indexed entry.

#### Example usage:

To validate and backfill the chain index for the last 5760 epochs (2 days) and log details for all epochs:

```
lotus-shed chainindex validate-backfill --from 1000000 --to 994240 --log-good 
```

This command is useful for backfilling the chain index over a range of historical epochs during the migration to the new `ChainIndexer`. **It can also be run periodically to validate the index's integrity.**

## Need more help?
Please free to ask questions on `#fil-lotus-dev` on Filecoin Slack or create issues on Lotus [GitHub](https://github.com/filecoin-project/lotus/issues) for any questions/bugs/comments/concerns.