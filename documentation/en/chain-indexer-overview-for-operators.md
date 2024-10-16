# ChainIndexer Documentation for RPC Providers <!-- omit in toc -->

- [Introduction](#introduction)
- [ChainIndexer Config](#chainindexer-config)
  - [Enablement](#enablement)
  - [Garbage Collection](#garbage-collection)
    - [Recommendations](#recommendations)
  - [Removed Options](#removed-options)
- [Upgrade](#upgrade)
  - [Part 1: Preparation](#part-1-preparation)
  - [Part 2: Create a Backfilled ChainIndexer `chainindex.db`](#part-2-create-a-backfilled-chainindexer-chainindexdb)
  - [Part 3: Create a copyable `chainindex.db`](#part-3-create-a-copyable-chainindexdb)
  - [Part 4: Update Other Nodes](#part-4-update-other-nodes)
  - [Part 5: Cleanup](#part-5-cleanup)
- [Backfill](#backfill)
  - [Backfill Timing](#backfill-timing)
  - [Backfill Disk Space Requirements](#backfill-disk-space-requirements)
  - [`lotus-shed chainindex validate-backfill` CLI tool](#lotus-shed-chainindex-validate-backfill-cli-tool)
    - [Usage](#usage)
- [Downgrade Steps](#downgrade-steps)
- [Terminology](#terminology)
  - [Previous Indexing System](#previous-indexing-system)
  - [ChainIndexer Indexing System](#chainindexer-indexing-system)
- [Appendix](#appendix)
  - [Wny isn't there an automated migration from the previous indexing system to the ChainIndexer indexing system?](#wny-isnt-there-an-automated-migration-from-the-previous-indexing-system-to-the-chainindexer-indexing-system)
  - [`ChainValidateIndex` RPC API](#chainvalidateindex-rpc-api)

## Introduction

This document is for externally-available and/or high-performance RPC providers.  It walks through the configuration changes, migration flow and operations/maintenance work needed to enable, backfill and maintain the [`ChainIndexer`](#chainindexer-indexing-system).  The justification for and benefits of the `ChainIndexer` are documented [here](https://github.com/filecoin-project/lotus/issues/12453). 

The ChainIndexer is now also required if you enable Ethereum (`eth_*`) APIs using the `EnableEthRPC` Lotus configuration option.

**Note: If you are a Storage Provider or node operator who does not serve public RPC requests or does not need Ethereum APIs enabled (i.e, if `Fevm.EnableEthRPC = false`), you can skip this document as the `ChainIndexer` is already disabled by default**. 

## ChainIndexer Config
### Enablement

The following must be enabled on an Lotus node before starting as they are disabled by default:

```toml
[Fevm]
# Enable the ETH RPC APIs.
# This is not required for ChainIndexer support, but ChainIndexer is required if you enable this.
  EnableEthRPC = true

[Events]
# Enable the Actor Events APIs.
# This is not required for ChainIndexer support, but ChainIndexer is required if you enable this.
  EnableActorEventsAPI = true

[ChainIndexer]
# Enable the ChainIndexer, which is required for the ETH RPC APIs and Actor Events APIs.
# If they are enabled, but the ChainIndexer is not, Lotus will exit during startup.
# (ChainIndexer needs to be explicitly enabled to signal to the node operator the extra supporting functionalithy that will now be running.)
  EnableIndexer = true 
```

You can learn more about these configuration options and other configuration options available for the `ChainIndexer` [here](https://github.com/filecoin-project/lotus/blob/master/documentation/en/default-lotus-config.toml).


### Garbage Collection

The `ChainIndexer` includes a garbage collection (GC) mechanism to manage the amount of historical data retained.  See the [ChainIndexer size requirements](#backfill-disk-space-requirements).

By default, GC is disabled to preserve all indexed data.

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

**Example:** if your node is configured to retain Filecoin chain state with a Splitstore Hotstore that approximates 2 days of epochs, set `GCRetentionEpochs` to at least `retentionDays * epochsPerDay = 2 * 2880 = 5760`).

**Warning:** Setting this value below the chain state retention period may degrade RPC performance and reliability because the ChainIndexer will lack data for epochs still present in the chain state.

**Note:** `Chainstore.Splitstore` is configured in terms of bytes (not epochs) and `ChainIndexer.GCRetentionEpochs` is in terms of epochs (not bytes).  For the purposes of this discussion, we're assuming operators have determined `Chainstore.Splitstore.HotStoreMaxSpaceTarget` and `Chainstore.Splitstore.HotStoreMaxSpaceThreshold` values that approximate a certain number days of storage in the Splitstore Hotstore.  The guidance here is to make sure this approximation exceeds `ChainIndexer.GCRetentionEpochs`.

### Removed Options

**Note: The following config options no longer exist in Lotus and have been removed in favor of the ChainIndexer config options explained above. They can be removed when upgrading to Lotus v1.31.0.**

```toml
[Fevm]
EthTxHashMappingLifetimeDays
DisableHistoricFilterAPI

[Events]
DisableHistoricFilterAPI
DatabasePath

[Index]
EnableMsgIndex
```

The previously deprecated `Fevm.Events` options are now also all removed:
* `Fevm.Events.DatabasePath`
* `Fevm.Events.DisableRealTimeFilterAPI`
* `Fevm.Events.DisableHistoricFilterAPI`
* `Fevm.Events.FilterTTL`
* `Fevm.Events.MaxFilters`
* `Fevm.Events.MaxFilterResults`
* `Fevm.Events.MaxFilterHeightRange`

## Upgrade

### Part 1: Preparation
> **Note:** One can upgrade/downgrade between [pre-ChainIndexer](#previous-indexing-system) and [with-ChainIndexer](#chainindexer-indexing-system) Lotus versions without conflict because they persist state to different directories and don't rely on each other. No backup is necessary (but extra backups don't hurt). There is still a [backfilling step though when downgrading](#downgrade-steps).

These upgrade steps assume one has multiple nodes in their fleet and can afford to have a node not handling traffic, potentially for days per [backfill timing below](#backfill-timing).

One should also check to ensure they have [sufficient disk space](#backfill-disk-space-requirements).

### Part 2: Create a Backfilled ChainIndexer `chainindex.db`
1. **Route traffic away from an initial node**
   - Example: prevent a load balancer from routing traffic to a designated node.
2. **Stop the designated Lotus Node**
   - Stop the designated Lotus node before starting the upgrade and backfill process.
3. **Update Configuration**
   - Modify the Lotus configuration to enable the `ChainIndexer` as described in the [`ChainIndexer Config` section above](#chainindexer-config). 
4. **Restart Lotus Node**
   - Restart the Lotus node with the new configuration.
   - The `ChainIndexer` will begin indexing **real-time chain state changes** immediately in the `${LOTUS_PATH}/chainindex` directory.
   - *However, it will not automatically index any historical chain state (i.e., any previously existing chain state prior to the upgrade).*
5. **Backfill**
   - See the ["Backfill" section below](#backfill).
   - This could potentially take days per [Backfill Timing](#backfill-timing).
6. **Ensure node health**
   - Perform whatever steps are usually done to validate a node's health before handling traffic (e.g., log scans, smoke tests)
7. **Route traffic to the backfilled node that is now using ChainIndexer**
8. **Ensure equal or better correctness and performance**
   - ChainIndexer-using nodes should have full correctness and better performance when compared to [pre-ChainIndexer](#previous-indexing-system) nodes.

### Part 3: Create a copyable `chainindex.db`
[Part 4 below](#part-4-update-other-nodes) is going to use the backfilled `chainindex.db` from above with other nodes so they don't have to undergo as long of a backfill process.  That said, this backfilled `chaindex.db` shouldn't be done while the updated-and-backfilled node is running.  Options include :
1.  Stop the updated-and-backfilled node before copying it.
  * `cp ${LOTUS_PATH}/chainindex/chainindex.db /copy/destination/path/chainindex.db`
2.  While the node is running, use the `sqlite3` CLI utility (which should be at least version 3.37) to clone it. 
  * `sqlite3 ${LOTUS_PATH}/chainindex/chainindex.db '.clone /copy/destination/path/chainindex.db'`
Both of these will result in a file `/copy/destination/path/chainindex.db` that can be copied around in part 4 below.

### Part 4: Update Other Nodes
Now that one has a `${LOTUS_PATH}/chainindex/chainindex.db` from a trusted node, it can be copied to additional nodes to expedite bootstrapping.
1. **Route traffic away from the next node to upgrade**
2. **Stop the Lotus Node**
3. **Update Configuration**
   - Modify the Lotus configuration to enable the `ChainIndexer` as described in the [`ChainIndexer Config` section above](#chainindexer-config). 
4. **Copy `/copy/destination/path/chainindex.db` from the trusted node in [part 3 above](#part-3-create-a-copyable-chainindexdb)**
4. **Restart Lotus Node**
   - Restart your Lotus node with the new configuration.
   - The `ChainIndexer` will begin indexing **real-time chain state changes** immediately in the `${LOTUS_PATH}/chainindex` directory.
   - *However, it will not automatically index the chain state from where the copied-in `chainindex.db` ends.  This will need to be done manually.*
5. **Backfill the small data gap from after the copied-in `chainindex.db`**
   - See the [`Backfill` section below](#backfill).
   - This should be quick since this gaps is presumably on the order of epoch minutes, hours, or days rather than months.
6. **Ensure node health**
   - Perform whatever steps are usually done to validate a node's health before handling traffic (e.g., log scans, smoke tests)
7. **Route traffic to this newly upgraded ChainIndexer-enabled node**
8. **Repeat for other nodes that need to upgrade**

### Part 5: Cleanup
It's recommended to keep the [pre-ChainIndexer](#previous-indexing-system) indexing database directory (`${LOTUS_PATH}/sqlite`) around until you've confirmed you don't need to [downgrade](#downgrade).  After sustained successful operations after the upgrade, the [pre-ChainIndexer](#previous-indexing-system) database directory can be removed to reclaim disk space.  

## Backfill
There is no automated migration from [pre-ChainIndexer indices](#previous-indexing-system) to the [ChainIndex](#chainindexer-indexing-system).  Instead one needs to index historical chain state (i.e., backfill), if RPC access to that historical state is required. (If curious, [read why](#wny-isnt-there-an-automated-migration-from-the-previous-indexing-system-to-the-chainindexer-indexing-system).]

### Backfill Timing

Backfilling the new `ChainIndexer` was [benchmarked to take approximately ~12 hours per month of epochs on a sample archival node doing no other work](https://github.com/filecoin-project/lotus/issues/12453#issuecomment-2405306468). Your results will vary depending on hardware, network, and competing processes.  This means if one is upgrading a FEVM archival node, they should plan on the node being out of production service for ~10 days.  Additional nodes to update don't need to go throuh the same time-intensive process though.  They can get a `${LOTUS_PATH}/chainindex/chainindex.db` copied from a trusted node per the [upgrade steps](#upgrade).  

### Backfill Disk Space Requirements

As of 202410, ChainIndexer will accumulate approximately ~340 MiB per day of data, or 10 GiB per month (see [here](https://github.com/filecoin-project/lotus/issues/12453)).

### `lotus-shed chainindex validate-backfill` CLI tool
The `lotus-shed chainindex validate-backfill` command is a tool for validating and optionally backfilling the chain index over a range of epochs since calling the [`ChainValidateIndex` API](#chainvalidateindex-rpc-api) for a single epoch at a time can be cumbersome, especially when backfilling or validating the index over a range of historical epochs, such as during a backfill. This tool wraps the `ChainValidateIndex` API to efficiently process multiple epochs.

**Note: This command can only be run when the Lotus daemon is already running with the [`ChainIndexer` enabled](#enablement) as it depends on the `ChainValidateIndex` RPC API.**

#### Usage

```
lotus-shed chainindex validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good]
```

The command validates the chain index entries for each epoch in the specified range, checking for missing or inconsistent entries (i.e. the indexed data does not match the actual chain state). If `--backfill` is enabled (which it is by default), it will attempt to backfill any missing entries using the `ChainValidateIndex` API.

You can learn about how to use the tool with `lotus-shed chainindex validate-backfill -h`.

Note: If you are using a non-standard Lotus repo directory then you can run the command with `lotus-shed -repo /path/to/lotus/repo chainindex validate-backfill ...`, or by setting the `LOTUS_REPO` environment variable.

## Downgrade Steps

In case you need to downgrade to the [previous indexing system](#previous-indexing-system), follow these steps:

1. Prevent the node from receiving traffic.
2. Stop your Lotus node.
3. Download or build a Lotus binary for the rollback version which has the implementation of the old `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices.
4. Ensure that you've set the correct config for the existing `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices in the `config.toml` file.
5. Restart your Lotus node.
6. Backfill the `EthTxHashLookup`, `MsgIndex`, and `EventIndex` indices using the `lotus-shed index backfill-*` CLI tooling available in the [previous indexing system](#previous-indexing-system) for the range of epochs between the upgrade to `ChainIndexer` and the rollback of `ChainIndexer`.
7. Route traffic back to the node.  

## Terminology
### Previous Indexing System
* This corresponds to the indexing system used in Lotus versions before v1.31.0.  
* It has been replaced by the [ChainIndexer](#chainindexer-indexing-system).
* It was composed of three indexers using three separate databases: [`EthTxHashLookup`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/ethhashlookup/eth_transaction_hash_lookup.go), [`MsgIndex`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/index/msgindex.go), and [`EventIndex`](https://github.com/filecoin-project/lotus/blob/v1.31.0/chain/events/filter/index.go).
* It persisted state to the [removed option](#removed-options) for `Events.DatabasePath`, which defaulted to `${LOTUS_PATH}/sqlite`.
* It had CLI backfill tooling: `lotus-shed index backfill-*`

### ChainIndexer Indexing System
* This corresponds to the indexing system used in Lotus versions v1.31.0 onwards.  
* It replaced the [previous indexing system](#previous-indexing-system).
* It is composed of a single indexer, [`ChainIndexer`](https://github.com/filecoin-project/lotus/blob/master/chain/index/indexer.go), using a [single database for transactions, messages, and events](https://github.com/filecoin-project/lotus/blob/master/chain/index/ddls.go).
* It persists state to `${LOTUS_PATH}/chainindex`.
* It has this CLI backfill tooling: [`lotus-shed chainindex validate-backfill`](#lotus-shed-chainindex-validate-backfill-cli-tool)
* **Storage requirements:** See the [backfill disk space requirements](#backfill-disk-space-requirements).
* **Backfil times:** See the [backfill timing](#backfill-timing).

## Appendix

### Wny isn't there an automated migration from the [previous indexing system](#previous-indexing-system) to the [ChainIndexer indexing system](#chainindexer-indexing-system)?

The decision not invest here ultimately comes down to the development-time cost vs. benefit ratio.

For achival nodes, we don't have the confidence that the [previous indexing system](#previous-indexing-system) has the correct data to bootstrap from. In 2024, Lotus maintainers have fixed multiple bugs in the [previous indexing system](#previous-indexing-system), but they still see reports of missing data, mismatched event index counts, etc.  Investing here in a migration isn't guaranteed to yield a correct index. As a result, one would still need to perform the [backfill steps](#backfill) to validate and correct the data anyway.  While this should be faster having partially correct data than no data, it would still require an archival node to take an outage on the order of days which isn't good enough.

The schemas of [the old fragmented Indices](#previous-indexing-system) don't naturally map to the schema of the [ChainIndexer](#chainindexer-indexing-system). There would be additional data wrangling work to ultimately get this right.

[Backfilling](#backfill) is a one time cost. If an operator provider is running multiple nodes, they only need to do it on one node and can then simply copy over the Index to the other node per [the upgrade steps](#upgrade-steps). The new `chainindex.db` copy can also be shared among operators if there is a trust relationship.

Note that this lack of an automated migration is primarily a concern for the relatively small-in-number archival nodes.  It isn't as much of a concern for snapshot-synced nodes. For snapshot-synced nodes with only a portion of the chain state because they only serve queries going back a few days can expect the backfill take closer to an hour per [backfill timing](#backfill-timing).

### `ChainValidateIndex` RPC API

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