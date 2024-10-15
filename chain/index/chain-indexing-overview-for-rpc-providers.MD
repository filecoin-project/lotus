TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1772125029
move this to lotus-docs

# ChainIndexer Documentation for RPC Providers
  
## Introduction

---
TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792731823
use different language than "RPC providers"
Some nuance is needed here - everyone running a Lotus node serves RPC requests, SPs use lotus as an RPC endpoint for their miner (lotus-miner or curio).

Something like "externally-available RPC" or "public API", or "RPC other than for internal consumption", or even better, target this document at those requiring: "high-performance RPC", which may include some SPs.

As we move forward, SPs are likely going to need some of this functionality anyway—consider PDP which is smart-contract mediated, they're going to need eth_getLogs and friends, so turning this on might start to make sense.

So, limiting this to "RPC providers" is probably not the best because (a) that term isn't precise enough and (b) there's grey area about who it might make sense to use this. You could even introduce a section that describes the functionality so it helps people decide for themselves.
---

This document is for RPC providers and node operators who serve RPC requests walking through the configuration changes, migration flow and operations/maintenance work needed to enable, backfill and maintain the [`ChainIndexer`](#chainindexer-indexing-system).

**Note: If you are a Storage Provider or node operator who does not serve RPC requests (i.e, if `Fevm.EnableEthRPC = false`), you can skip this document as the `ChainIndexer` is already disabled by default**. 

## ChainIndexer Config
### Enablement

The following must be enabled on an Lotus node before starting as they are disabled by default:

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

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1784677150
document when EnableEthRC == true && EnableActorEventsAPI == true but EnableIndexer == false

You can learn more about these configuration options and other configuration options available for the `ChainIndexer` [here](https://github.com/filecoin-project/lotus/blob/master/documentation/en/default-lotus-config.toml).


### Garbage Collection

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792736417
Can we give a very rough estimate of growth to help with informed decisions here? e.g.:
"At the time of writing, ChainIndexer will accumulate approximately 120MiB per day of data".

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
(*Example:* if your node is configured to retain 2 days of Filecoin chain state with the Splitstore, set `GCRetentionEpochs` to `retentionDays * epochsPerDay = 2 * 2880 = 5760`).
**Warning:** Setting this value below the chain state retention period will degrade RPC performance and reliability because the Index will lack data for epochs still present in the chain state.  

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792740160
> "if your node is configured to retain 2 days of Filecoin chain state with the Splitstore" isn't helpful because nobody configures it like this, so you need to say something here to align with how splitstore works

### Removed Options

**Note: The following config options no longer exist in Lotus and have been removed in favor of the ChainIndexer config options explained above. They can be removed when upgrading to Lotus v1.31.0.**

```toml
[Fevm]
EthTxHashMappingLifetimeDays = 0

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792743250
> see note in #12421, this one is duplicated with a moved redirect

[Events]
DisableHistoricFilterAPI = false
DatabasePath = ""
```

## Upgrade Steps

> **Note:** One can upgrade/downgrade between [pre-ChainIndexer](#previous-indexing-system) and [with-ChainIndexer](#chainindexer-indexing-system) Lotus versions without conflict because they persist state to different directories and don't rely on each other. No backup is necessary (but extra backups don't hurt).  

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792744673
> yeah .. but it's more complicated than this, because if you go back to a pre-ChainIndexer version then you'd need to backfill to fill in the holes you created in the period you were running the with-ChainIndexer version. I think you need to document this somewhere because you can't just hand-wave at this problem for people.

Upgrading to the new `ChainIndexer` involves these steps:

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1771226124
Time estimates for the migration for Snapshot synced and archival would also be good to add at the top level here.

1. **Stop the Lotus Node**
   - Stop your Lotus node before starting the upgrade and migration process.
2. **Update Configuration**
   - Modify your Lotus configuration to enable the `ChainIndexer` as described in the [`ChainIndexer Config` section above](#chainindexer-config).
3. **Restart Lotus Node**
   - Restart your Lotus node with the new configuration.
   - The `ChainIndexer` will begin indexing **real-time chain state changes** immediately in the `${LOTUS_PATH}/chainindex` directory.
   - **However, it will not automatically index any historical chain state (i.e., any previously existing chain state prior to the upgrade). To perform backfilling, please see the [`Backfill` section below](#backfill).**

TODO: from https://github.com/filecoin-project/lotus/pull/12450#discussion_r1771220740
Could be good to add some migration time estimates here when moving to the ChainIndexer?

> **Note:** It's recommended to keep the [pre-ChainIndexer](#previous-indexing-system) indexing database directory (`${LOTUS_PATH}/sqlite`) around until you've confirmed you don't need to [downgrade](#downgrade).  After sustained successful operations after the upgrade, the [pre-ChainIndexer](#previous-indexing-system) indexing database directory can be removed to reclaim disk space.  

## Backfill
There is no automated migration from [pre-ChainIndexer indices](#previous-indexing-system) to the [ChainIndex](#chainindexer-indexing-system).  Instead one needs to index historical chain state (i.e., backfill) using one of the following tools.

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792801381
Add more infor here

> **Note:** The ChainIndex will consume ~10GB of storage per month of tipsets (e.g., ~86400 epochs).  Ensure you have sufficient disk space before proceeding.

### `ChainValidateIndex` JSON RPC API

TODO: https://github.com/filecoin-project/lotus/pull/12450#discussion_r1792818109
Move this out

Please refer to the [Lotus API documentation](https://github.com/filecoin-project/lotus/blob/master/documentation/en/api-v1-unstable-methods.md) for detailed documentation of the `ChainValidateIndex` JSON RPC API.

The `ChainValidateIndex` JSON RPC API serves a dual purpose: it validates/diagnoses the integrity of the index at a specific epoch (i.e., it ensures consistency between indexed data and actual chain state), while also providing the option to backfill the `ChainIndexer` if it does not have data for the specified epoch. 

The `ChainValidateIndex` RPC API is available for use once the Lotus daemon has started with `ChainIndexer` [enabled](#enablement). 

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

### `lotus-shed chainindex validate-backfill` CLI tool
The `lotus-shed chainindex validate-backfill` command is a tool for validating and optionally backfilling the chain index over a range of epochs since calling the `ChainValidateIndex` API for a single epoch at a time can be cumbersome, especially when backfilling or validating the index over a range of historical epochs, such as during a migration. It wraps the `ChainValidateIndex` API to efficiently process multiple epochs.

**Note: This command can only be run when the Lotus daemon is already running with the [`ChainIndexer` enabled](#enablement) as it depends on the `ChainValidateIndex` RPC API.**

TODO: from https://github.com/filecoin-project/lotus/pull/12450#discussion_r1771217907
Let us provide some basic on resource usage here, some potential questions from users I foresee is:
How long does it take to backfill XX epochs.
How much CPU / RAM does it require (i.e do we need to shift traffic to other nodes when running the backfill)


#### Usage:
```
lotus-shed chainindex validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good]
```

The command validates the chain index entries for each epoch in the specified range, checking for missing or inconsistent entries(i.e. the indexed data does not match the actual chain state). If `--backfill` is enabled (which it is by default), it will attempt to backfill any missing entries using the `ChainValidateIndex` API.

You can learn about how to use the tool with `lotus-shed chainindex validate-backfill -h`.

Note: If you are using a non-standard Lotus repo directory then you can run the command with `lotus-shed -repo /path/to/lotus/repo chainindex validate-backfill ...`, or by setting the `LOTUS_PATH` environment variable.

## Downgrade

In case you need to downgrade to the [previous indexing system](#previous-indexing-system), follow these steps:

1. Stop your Lotus node.
2. Download or build a Lotus binary for the rollback version which has the implementation of the old `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices.
4. Ensure that you've set the correct config for the existing `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices in the `config.toml` file.
5. Restart your Lotus node.
6. Backfill the `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices using the `lotus-shed index backfill-*` CLI tooling available in the [previous indexing system](#previous-indexing-system) for the epoch range in [epochs between the upgrade to `ChainIndexer` and the rollback of `ChainIndexer`].

## Terminology
### Previous Indexing System
* This corresponds to the indexing system used in Lotus versions before v1.31.0.  
* It has been replaced by the [ChainIndex](#chainindexer-indexing-system).
* It was composed of three indexers using three separate databases: [`EthTxHashLookup`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/ethhashlookup/eth_transaction_hash_lookup.go), [`MsgIndex`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/index/msgindex.go), and [`EventIndex`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/events/filter/index.go).
* It persisted state to the [removed option](#removed-options) for `Events.DatabasePath`, which defaulted to `${LOTUS_PATH}/sqlite`.
* It had CLI backfill tooling: `lotus-shed index backfill-*`

### ChainIndexer Indexing System
* This corresponds to the indexing system used in Lotus versions v1.31.0 onwards.  
* It replaced the [previous indexing system](#previous-indexing-system).
* It is composed of a single indexer, [`ChainIndexer`](https://github.com/filecoin-project/lotus/blob/master/chain/index/indexer.go), using a [single database for transactions, messages, and events](https://github.com/filecoin-project/lotus/blob/master/chain/index/ddls.go).
* It persists state to `${LOTUS_HOME}/chainindex`.
* It has this CLI backfill tooling: [`lotus-shed chainindex validate-backfill`](#lotus-shed-chainindex-validate-backfill-cli-tool)
