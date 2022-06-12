# Groups
* [](#)
  * [Closing](#Closing)
  * [Discover](#Discover)
  * [Session](#Session)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Actor](#Actor)
  * [ActorAddress](#ActorAddress)
  * [ActorAddressConfig](#ActorAddressConfig)
  * [ActorSectorSize](#ActorSectorSize)
* [Auth](#Auth)
  * [AuthNew](#AuthNew)
  * [AuthVerify](#AuthVerify)
* [Check](#Check)
  * [CheckProvable](#CheckProvable)
* [Compute](#Compute)
  * [ComputeDataCid](#ComputeDataCid)
  * [ComputeProof](#ComputeProof)
  * [ComputeWindowPoSt](#ComputeWindowPoSt)
* [Create](#Create)
  * [CreateBackup](#CreateBackup)
* [Dagstore](#Dagstore)
  * [DagstoreGC](#DagstoreGC)
  * [DagstoreInitializeAll](#DagstoreInitializeAll)
  * [DagstoreInitializeShard](#DagstoreInitializeShard)
  * [DagstoreListShards](#DagstoreListShards)
  * [DagstoreLookupPieces](#DagstoreLookupPieces)
  * [DagstoreRecoverShard](#DagstoreRecoverShard)
  * [DagstoreRegisterShard](#DagstoreRegisterShard)
* [Deals](#Deals)
  * [DealsConsiderOfflineRetrievalDeals](#DealsConsiderOfflineRetrievalDeals)
  * [DealsConsiderOfflineStorageDeals](#DealsConsiderOfflineStorageDeals)
  * [DealsConsiderOnlineRetrievalDeals](#DealsConsiderOnlineRetrievalDeals)
  * [DealsConsiderOnlineStorageDeals](#DealsConsiderOnlineStorageDeals)
  * [DealsConsiderUnverifiedStorageDeals](#DealsConsiderUnverifiedStorageDeals)
  * [DealsConsiderVerifiedStorageDeals](#DealsConsiderVerifiedStorageDeals)
  * [DealsImportData](#DealsImportData)
  * [DealsList](#DealsList)
  * [DealsPieceCidBlocklist](#DealsPieceCidBlocklist)
  * [DealsSetConsiderOfflineRetrievalDeals](#DealsSetConsiderOfflineRetrievalDeals)
  * [DealsSetConsiderOfflineStorageDeals](#DealsSetConsiderOfflineStorageDeals)
  * [DealsSetConsiderOnlineRetrievalDeals](#DealsSetConsiderOnlineRetrievalDeals)
  * [DealsSetConsiderOnlineStorageDeals](#DealsSetConsiderOnlineStorageDeals)
  * [DealsSetConsiderUnverifiedStorageDeals](#DealsSetConsiderUnverifiedStorageDeals)
  * [DealsSetConsiderVerifiedStorageDeals](#DealsSetConsiderVerifiedStorageDeals)
  * [DealsSetPieceCidBlocklist](#DealsSetPieceCidBlocklist)
* [I](#I)
  * [ID](#ID)
* [Indexer](#Indexer)
  * [IndexerAnnounceAllDeals](#IndexerAnnounceAllDeals)
  * [IndexerAnnounceDeal](#IndexerAnnounceDeal)
* [Log](#Log)
  * [LogAlerts](#LogAlerts)
  * [LogList](#LogList)
  * [LogSetLevel](#LogSetLevel)
* [Market](#Market)
  * [MarketCancelDataTransfer](#MarketCancelDataTransfer)
  * [MarketDataTransferDiagnostics](#MarketDataTransferDiagnostics)
  * [MarketDataTransferUpdates](#MarketDataTransferUpdates)
  * [MarketGetAsk](#MarketGetAsk)
  * [MarketGetDealUpdates](#MarketGetDealUpdates)
  * [MarketGetRetrievalAsk](#MarketGetRetrievalAsk)
  * [MarketImportDealData](#MarketImportDealData)
  * [MarketListDataTransfers](#MarketListDataTransfers)
  * [MarketListDeals](#MarketListDeals)
  * [MarketListIncompleteDeals](#MarketListIncompleteDeals)
  * [MarketListRetrievalDeals](#MarketListRetrievalDeals)
  * [MarketPendingDeals](#MarketPendingDeals)
  * [MarketPublishPendingDeals](#MarketPublishPendingDeals)
  * [MarketRestartDataTransfer](#MarketRestartDataTransfer)
  * [MarketRetryPublishDeal](#MarketRetryPublishDeal)
  * [MarketSetAsk](#MarketSetAsk)
  * [MarketSetRetrievalAsk](#MarketSetRetrievalAsk)
* [Mining](#Mining)
  * [MiningBase](#MiningBase)
* [Net](#Net)
  * [NetAddrsListen](#NetAddrsListen)
  * [NetAgentVersion](#NetAgentVersion)
  * [NetAutoNatStatus](#NetAutoNatStatus)
  * [NetBandwidthStats](#NetBandwidthStats)
  * [NetBandwidthStatsByPeer](#NetBandwidthStatsByPeer)
  * [NetBandwidthStatsByProtocol](#NetBandwidthStatsByProtocol)
  * [NetBlockAdd](#NetBlockAdd)
  * [NetBlockList](#NetBlockList)
  * [NetBlockRemove](#NetBlockRemove)
  * [NetConnect](#NetConnect)
  * [NetConnectedness](#NetConnectedness)
  * [NetDisconnect](#NetDisconnect)
  * [NetFindPeer](#NetFindPeer)
  * [NetLimit](#NetLimit)
  * [NetPeerInfo](#NetPeerInfo)
  * [NetPeers](#NetPeers)
  * [NetPing](#NetPing)
  * [NetProtectAdd](#NetProtectAdd)
  * [NetProtectList](#NetProtectList)
  * [NetProtectRemove](#NetProtectRemove)
  * [NetPubsubScores](#NetPubsubScores)
  * [NetSetLimit](#NetSetLimit)
  * [NetStat](#NetStat)
* [Pieces](#Pieces)
  * [PiecesGetCIDInfo](#PiecesGetCIDInfo)
  * [PiecesGetPieceInfo](#PiecesGetPieceInfo)
  * [PiecesListCidInfos](#PiecesListCidInfos)
  * [PiecesListPieces](#PiecesListPieces)
* [Pledge](#Pledge)
  * [PledgeSector](#PledgeSector)
* [Return](#Return)
  * [ReturnAddPiece](#ReturnAddPiece)
  * [ReturnDataCid](#ReturnDataCid)
  * [ReturnFetch](#ReturnFetch)
  * [ReturnFinalizeReplicaUpdate](#ReturnFinalizeReplicaUpdate)
  * [ReturnFinalizeSector](#ReturnFinalizeSector)
  * [ReturnGenerateSectorKeyFromData](#ReturnGenerateSectorKeyFromData)
  * [ReturnMoveStorage](#ReturnMoveStorage)
  * [ReturnProveReplicaUpdate1](#ReturnProveReplicaUpdate1)
  * [ReturnProveReplicaUpdate2](#ReturnProveReplicaUpdate2)
  * [ReturnReadPiece](#ReturnReadPiece)
  * [ReturnReleaseUnsealed](#ReturnReleaseUnsealed)
  * [ReturnReplicaUpdate](#ReturnReplicaUpdate)
  * [ReturnSealCommit1](#ReturnSealCommit1)
  * [ReturnSealCommit2](#ReturnSealCommit2)
  * [ReturnSealPreCommit1](#ReturnSealPreCommit1)
  * [ReturnSealPreCommit2](#ReturnSealPreCommit2)
  * [ReturnUnsealPiece](#ReturnUnsealPiece)
* [Runtime](#Runtime)
  * [RuntimeSubsystems](#RuntimeSubsystems)
* [Sealing](#Sealing)
  * [SealingAbort](#SealingAbort)
  * [SealingSchedDiag](#SealingSchedDiag)
* [Sector](#Sector)
  * [SectorAbortUpgrade](#SectorAbortUpgrade)
  * [SectorAddPieceToAny](#SectorAddPieceToAny)
  * [SectorCommitFlush](#SectorCommitFlush)
  * [SectorCommitPending](#SectorCommitPending)
  * [SectorGetExpectedSealDuration](#SectorGetExpectedSealDuration)
  * [SectorGetSealDelay](#SectorGetSealDelay)
  * [SectorMarkForUpgrade](#SectorMarkForUpgrade)
  * [SectorMatchPendingPiecesToOpenSectors](#SectorMatchPendingPiecesToOpenSectors)
  * [SectorPreCommitFlush](#SectorPreCommitFlush)
  * [SectorPreCommitPending](#SectorPreCommitPending)
  * [SectorRemove](#SectorRemove)
  * [SectorSetExpectedSealDuration](#SectorSetExpectedSealDuration)
  * [SectorSetSealDelay](#SectorSetSealDelay)
  * [SectorStartSealing](#SectorStartSealing)
  * [SectorTerminate](#SectorTerminate)
  * [SectorTerminateFlush](#SectorTerminateFlush)
  * [SectorTerminatePending](#SectorTerminatePending)
* [Sectors](#Sectors)
  * [SectorsList](#SectorsList)
  * [SectorsListInStates](#SectorsListInStates)
  * [SectorsRefs](#SectorsRefs)
  * [SectorsStatus](#SectorsStatus)
  * [SectorsSummary](#SectorsSummary)
  * [SectorsUnsealPiece](#SectorsUnsealPiece)
  * [SectorsUpdate](#SectorsUpdate)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
  * [StorageAttach](#StorageAttach)
  * [StorageBestAlloc](#StorageBestAlloc)
  * [StorageDeclareSector](#StorageDeclareSector)
  * [StorageDropSector](#StorageDropSector)
  * [StorageFindSector](#StorageFindSector)
  * [StorageGetLocks](#StorageGetLocks)
  * [StorageInfo](#StorageInfo)
  * [StorageList](#StorageList)
  * [StorageLocal](#StorageLocal)
  * [StorageLock](#StorageLock)
  * [StorageReportHealth](#StorageReportHealth)
  * [StorageStat](#StorageStat)
  * [StorageTryLock](#StorageTryLock)
* [Worker](#Worker)
  * [WorkerConnect](#WorkerConnect)
  * [WorkerJobs](#WorkerJobs)
  * [WorkerStats](#WorkerStats)
## 


### Closing


Perms: read

Inputs: `null`

Response: `{}`

### Discover


Perms: read

Inputs: `null`

Response:
```json
{
  "info": {
    "title": "Lotus RPC API",
    "version": "1.2.1/generated=2020-11-22T08:22:42-06:00"
  },
  "methods": [],
  "openrpc": "1.2.6"
}
```

### Session


Perms: read

Inputs: `null`

Response: `"07070707-0707-0707-0707-070707070707"`

### Shutdown


Perms: admin

Inputs: `null`

Response: `{}`

### Version


Perms: read

Inputs: `null`

Response:
```json
{
  "Version": "string value",
  "APIVersion": 131584,
  "BlockDelay": 42
}
```

## Actor


### ActorAddress


Perms: read

Inputs: `null`

Response: `"f01234"`

### ActorAddressConfig


Perms: read

Inputs: `null`

Response:
```json
{
  "PreCommitControl": [
    "f01234"
  ],
  "CommitControl": [
    "f01234"
  ],
  "TerminateControl": [
    "f01234"
  ],
  "DealPublishControl": [
    "f01234"
  ],
  "DisableOwnerFallback": true,
  "DisableWorkerFallback": true
}
```

### ActorSectorSize


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response: `34359738368`

## Auth


### AuthNew


Perms: admin

Inputs:
```json
[
  [
    "write"
  ]
]
```

Response: `"Ynl0ZSBhcnJheQ=="`

### AuthVerify


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response:
```json
[
  "write"
]
```

## Check


### CheckProvable


Perms: admin

Inputs:
```json
[
  8,
  [
    {
      "ID": {
        "Miner": 1000,
        "Number": 9
      },
      "ProofType": 8
    }
  ],
  true
]
```

Response:
```json
{
  "123": "can't acquire read lock"
}
```

## Compute


### ComputeDataCid


Perms: admin

Inputs:
```json
[
  1024,
  {}
]
```

Response:
```json
{
  "Size": 1032,
  "PieceCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
}
```

### ComputeProof


Perms: read

Inputs:
```json
[
  [
    {
      "SealProof": 8,
      "SectorNumber": 9,
      "SectorKey": null,
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ],
  "Bw==",
  10101,
  16
]
```

Response:
```json
[
  {
    "PoStProof": 8,
    "ProofBytes": "Ynl0ZSBhcnJheQ=="
  }
]
```

### ComputeWindowPoSt


Perms: admin

Inputs:
```json
[
  42,
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ]
]
```

Response:
```json
[
  {
    "Deadline": 42,
    "Partitions": [
      {
        "Index": 42,
        "Skipped": [
          5,
          1
        ]
      }
    ],
    "Proofs": [
      {
        "PoStProof": 8,
        "ProofBytes": "Ynl0ZSBhcnJheQ=="
      }
    ],
    "ChainCommitEpoch": 10101,
    "ChainCommitRand": "Bw=="
  }
]
```

## Create


### CreateBackup
CreateBackup creates node backup onder the specified file name. The
method requires that the lotus-miner is running with the
LOTUS_BACKUP_BASE_PATH environment variable set to some path, and that
the path specified when calling CreateBackup is within the base path


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

## Dagstore


### DagstoreGC
DagstoreGC runs garbage collection on the DAG store.


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Key": "baga6ea4seaqecmtz7iak33dsfshi627abz4i4665dfuzr3qfs4bmad6dx3iigdq",
    "Success": false,
    "Error": "\u003cerror\u003e"
  }
]
```

### DagstoreInitializeAll
DagstoreInitializeAll initializes all uninitialized shards in bulk,
according to the policy passed in the parameters.

It is recommended to set a maximum concurrency to avoid extreme
IO pressure if the storage subsystem has a large amount of deals.

It returns a stream of events to report progress.


Perms: write

Inputs:
```json
[
  {
    "MaxConcurrency": 123,
    "IncludeSealed": true
  }
]
```

Response:
```json
{
  "Key": "string value",
  "Event": "string value",
  "Success": true,
  "Error": "string value",
  "Total": 123,
  "Current": 123
}
```

### DagstoreInitializeShard
DagstoreInitializeShard initializes an uninitialized shard.

Initialization consists of fetching the shard's data (deal payload) from
the storage subsystem, generating an index, and persisting the index
to facilitate later retrievals, and/or to publish to external sources.

This operation is intended to complement the initial migration. The
migration registers a shard for every unique piece CID, with lazy
initialization. Thus, shards are not initialized immediately to avoid
IO activity competing with proving. Instead, shard are initialized
when first accessed. This method forces the initialization of a shard by
accessing it and immediately releasing it. This is useful to warm up the
cache to facilitate subsequent retrievals, and to generate the indexes
to publish them externally.

This operation fails if the shard is not in ShardStateNew state.
It blocks until initialization finishes.


Perms: write

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

### DagstoreListShards
DagstoreListShards returns information about all shards known to the
DAG store. Only available on nodes running the markets subsystem.


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "Key": "baga6ea4seaqecmtz7iak33dsfshi627abz4i4665dfuzr3qfs4bmad6dx3iigdq",
    "State": "ShardStateAvailable",
    "Error": "\u003cerror\u003e"
  }
]
```

### DagstoreLookupPieces
DagstoreLookupPieces returns information about shards that contain the given CID.


Perms: admin

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response:
```json
[
  {
    "Key": "baga6ea4seaqecmtz7iak33dsfshi627abz4i4665dfuzr3qfs4bmad6dx3iigdq",
    "State": "ShardStateAvailable",
    "Error": "\u003cerror\u003e"
  }
]
```

### DagstoreRecoverShard
DagstoreRecoverShard attempts to recover a failed shard.

This operation fails if the shard is not in ShardStateErrored state.
It blocks until recovery finishes. If recovery failed, it returns the
error.


Perms: write

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

### DagstoreRegisterShard
DagstoreRegisterShard registers a shard manually with dagstore with given pieceCID


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

## Deals


### DealsConsiderOfflineRetrievalDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsConsiderOfflineStorageDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsConsiderOnlineRetrievalDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsConsiderOnlineStorageDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsConsiderUnverifiedStorageDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsConsiderVerifiedStorageDeals


Perms: admin

Inputs: `null`

Response: `true`

### DealsImportData


Perms: admin

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "string value"
]
```

Response: `{}`

### DealsList


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Proposal": {
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceSize": 1032,
      "VerifiedDeal": true,
      "Client": "f01234",
      "Provider": "f01234",
      "Label": "",
      "StartEpoch": 10101,
      "EndEpoch": 10101,
      "StoragePricePerEpoch": "0",
      "ProviderCollateral": "0",
      "ClientCollateral": "0"
    },
    "State": {
      "SectorStartEpoch": 10101,
      "LastUpdatedEpoch": 10101,
      "SlashEpoch": 10101
    }
  }
]
```

### DealsPieceCidBlocklist


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

### DealsSetConsiderOfflineRetrievalDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetConsiderOfflineStorageDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetConsiderOnlineRetrievalDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetConsiderOnlineStorageDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetConsiderUnverifiedStorageDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetConsiderVerifiedStorageDeals


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

### DealsSetPieceCidBlocklist


Perms: admin

Inputs:
```json
[
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ]
]
```

Response: `{}`

## I


### ID


Perms: read

Inputs: `null`

Response: `"12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"`

## Indexer


### IndexerAnnounceAllDeals
IndexerAnnounceAllDeals informs the indexer nodes aboutall active deals.


Perms: admin

Inputs: `null`

Response: `{}`

### IndexerAnnounceDeal
IndexerAnnounceDeal informs indexer nodes that a new deal was received,
so they can download its index


Perms: admin

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `{}`

## Log


### LogAlerts


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Type": {
      "System": "string value",
      "Subsystem": "string value"
    },
    "Active": true,
    "LastActive": {
      "Type": "string value",
      "Message": "json raw message",
      "Time": "0001-01-01T00:00:00Z"
    },
    "LastResolved": {
      "Type": "string value",
      "Message": "json raw message",
      "Time": "0001-01-01T00:00:00Z"
    }
  }
]
```

### LogList


Perms: write

Inputs: `null`

Response:
```json
[
  "string value"
]
```

### LogSetLevel


Perms: write

Inputs:
```json
[
  "string value",
  "string value"
]
```

Response: `{}`

## Market


### MarketCancelDataTransfer
MarketCancelDataTransfer cancels a data transfer with the given transfer ID and other peer


Perms: write

Inputs:
```json
[
  3,
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  true
]
```

Response: `{}`

### MarketDataTransferDiagnostics
MarketDataTransferDiagnostics generates debugging information about current data transfers over graphsync


Perms: write

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response:
```json
{
  "ReceivingTransfers": [
    {
      "RequestID": {},
      "RequestState": "string value",
      "IsCurrentChannelRequest": true,
      "ChannelID": {
        "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "ID": 3
      },
      "ChannelState": {
        "TransferID": 3,
        "Status": 1,
        "BaseCID": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
        "IsInitiator": true,
        "IsSender": true,
        "Voucher": "string value",
        "Message": "string value",
        "OtherPeer": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "Transferred": 42,
        "Stages": {
          "Stages": [
            {
              "Name": "string value",
              "Description": "string value",
              "CreatedTime": "0001-01-01T00:00:00Z",
              "UpdatedTime": "0001-01-01T00:00:00Z",
              "Logs": [
                {
                  "Log": "string value",
                  "UpdatedTime": "0001-01-01T00:00:00Z"
                }
              ]
            }
          ]
        }
      },
      "Diagnostics": [
        "string value"
      ]
    }
  ],
  "SendingTransfers": [
    {
      "RequestID": {},
      "RequestState": "string value",
      "IsCurrentChannelRequest": true,
      "ChannelID": {
        "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "ID": 3
      },
      "ChannelState": {
        "TransferID": 3,
        "Status": 1,
        "BaseCID": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
        "IsInitiator": true,
        "IsSender": true,
        "Voucher": "string value",
        "Message": "string value",
        "OtherPeer": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
        "Transferred": 42,
        "Stages": {
          "Stages": [
            {
              "Name": "string value",
              "Description": "string value",
              "CreatedTime": "0001-01-01T00:00:00Z",
              "UpdatedTime": "0001-01-01T00:00:00Z",
              "Logs": [
                {
                  "Log": "string value",
                  "UpdatedTime": "0001-01-01T00:00:00Z"
                }
              ]
            }
          ]
        }
      },
      "Diagnostics": [
        "string value"
      ]
    }
  ]
}
```

### MarketDataTransferUpdates


Perms: write

Inputs: `null`

Response:
```json
{
  "TransferID": 3,
  "Status": 1,
  "BaseCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "IsInitiator": true,
  "IsSender": true,
  "Voucher": "string value",
  "Message": "string value",
  "OtherPeer": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Transferred": 42,
  "Stages": {
    "Stages": [
      {
        "Name": "string value",
        "Description": "string value",
        "CreatedTime": "0001-01-01T00:00:00Z",
        "UpdatedTime": "0001-01-01T00:00:00Z",
        "Logs": [
          {
            "Log": "string value",
            "UpdatedTime": "0001-01-01T00:00:00Z"
          }
        ]
      }
    ]
  }
}
```

### MarketGetAsk


Perms: read

Inputs: `null`

Response:
```json
{
  "Ask": {
    "Price": "0",
    "VerifiedPrice": "0",
    "MinPieceSize": 1032,
    "MaxPieceSize": 1032,
    "Miner": "f01234",
    "Timestamp": 10101,
    "Expiry": 10101,
    "SeqNo": 42
  },
  "Signature": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  }
}
```

### MarketGetDealUpdates


Perms: read

Inputs: `null`

Response:
```json
{
  "Proposal": {
    "PieceCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PieceSize": 1032,
    "VerifiedDeal": true,
    "Client": "f01234",
    "Provider": "f01234",
    "Label": "",
    "StartEpoch": 10101,
    "EndEpoch": 10101,
    "StoragePricePerEpoch": "0",
    "ProviderCollateral": "0",
    "ClientCollateral": "0"
  },
  "ClientSignature": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "ProposalCid": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "AddFundsCid": null,
  "PublishCid": null,
  "Miner": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Client": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "State": 42,
  "PiecePath": ".lotusminer/fstmp123",
  "MetadataPath": ".lotusminer/fstmp123",
  "SlashEpoch": 10101,
  "FastRetrieval": true,
  "Message": "string value",
  "FundsReserved": "0",
  "Ref": {
    "TransferType": "string value",
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PieceCid": null,
    "PieceSize": 1024,
    "RawBlockSize": 42
  },
  "AvailableForRetrieval": true,
  "DealID": 5432,
  "CreationTime": "0001-01-01T00:00:00Z",
  "TransferChannelId": {
    "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "ID": 3
  },
  "SectorNumber": 9,
  "InboundCAR": "string value"
}
```

### MarketGetRetrievalAsk


Perms: read

Inputs: `null`

Response:
```json
{
  "PricePerByte": "0",
  "UnsealPrice": "0",
  "PaymentInterval": 42,
  "PaymentIntervalIncrease": 42
}
```

### MarketImportDealData


Perms: write

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "string value"
]
```

Response: `{}`

### MarketListDataTransfers


Perms: write

Inputs: `null`

Response:
```json
[
  {
    "TransferID": 3,
    "Status": 1,
    "BaseCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "IsInitiator": true,
    "IsSender": true,
    "Voucher": "string value",
    "Message": "string value",
    "OtherPeer": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Transferred": 42,
    "Stages": {
      "Stages": [
        {
          "Name": "string value",
          "Description": "string value",
          "CreatedTime": "0001-01-01T00:00:00Z",
          "UpdatedTime": "0001-01-01T00:00:00Z",
          "Logs": [
            {
              "Log": "string value",
              "UpdatedTime": "0001-01-01T00:00:00Z"
            }
          ]
        }
      ]
    }
  }
]
```

### MarketListDeals


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "Proposal": {
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceSize": 1032,
      "VerifiedDeal": true,
      "Client": "f01234",
      "Provider": "f01234",
      "Label": "",
      "StartEpoch": 10101,
      "EndEpoch": 10101,
      "StoragePricePerEpoch": "0",
      "ProviderCollateral": "0",
      "ClientCollateral": "0"
    },
    "State": {
      "SectorStartEpoch": 10101,
      "LastUpdatedEpoch": 10101,
      "SlashEpoch": 10101
    }
  }
]
```

### MarketListIncompleteDeals


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "Proposal": {
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceSize": 1032,
      "VerifiedDeal": true,
      "Client": "f01234",
      "Provider": "f01234",
      "Label": "",
      "StartEpoch": 10101,
      "EndEpoch": 10101,
      "StoragePricePerEpoch": "0",
      "ProviderCollateral": "0",
      "ClientCollateral": "0"
    },
    "ClientSignature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "ProposalCid": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "AddFundsCid": null,
    "PublishCid": null,
    "Miner": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Client": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "State": 42,
    "PiecePath": ".lotusminer/fstmp123",
    "MetadataPath": ".lotusminer/fstmp123",
    "SlashEpoch": 10101,
    "FastRetrieval": true,
    "Message": "string value",
    "FundsReserved": "0",
    "Ref": {
      "TransferType": "string value",
      "Root": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceCid": null,
      "PieceSize": 1024,
      "RawBlockSize": 42
    },
    "AvailableForRetrieval": true,
    "DealID": 5432,
    "CreationTime": "0001-01-01T00:00:00Z",
    "TransferChannelId": {
      "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "ID": 3
    },
    "SectorNumber": 9,
    "InboundCAR": "string value"
  }
]
```

### MarketListRetrievalDeals


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "PayloadCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "ID": 5,
    "Selector": {
      "Raw": "Ynl0ZSBhcnJheQ=="
    },
    "PieceCID": null,
    "PricePerByte": "0",
    "PaymentInterval": 42,
    "PaymentIntervalIncrease": 42,
    "UnsealPrice": "0",
    "StoreID": 42,
    "ChannelID": {
      "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "ID": 3
    },
    "PieceInfo": {
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Deals": [
        {
          "DealID": 5432,
          "SectorID": 9,
          "Offset": 1032,
          "Length": 1032
        }
      ]
    },
    "Status": 0,
    "Receiver": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "TotalSent": 42,
    "FundsReceived": "0",
    "Message": "string value",
    "CurrentInterval": 42,
    "LegacyProtocol": true
  }
]
```

### MarketPendingDeals


Perms: write

Inputs: `null`

Response:
```json
{
  "Deals": [
    {
      "Proposal": {
        "PieceCID": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
        "PieceSize": 1032,
        "VerifiedDeal": true,
        "Client": "f01234",
        "Provider": "f01234",
        "Label": "",
        "StartEpoch": 10101,
        "EndEpoch": 10101,
        "StoragePricePerEpoch": "0",
        "ProviderCollateral": "0",
        "ClientCollateral": "0"
      },
      "ClientSignature": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      }
    }
  ],
  "PublishPeriodStart": "0001-01-01T00:00:00Z",
  "PublishPeriod": 60000000000
}
```

### MarketPublishPendingDeals


Perms: admin

Inputs: `null`

Response: `{}`

### MarketRestartDataTransfer
MarketRestartDataTransfer attempts to restart a data transfer with the given transfer ID and other peer


Perms: write

Inputs:
```json
[
  3,
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  true
]
```

Response: `{}`

### MarketRetryPublishDeal


Perms: admin

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `{}`

### MarketSetAsk


Perms: admin

Inputs:
```json
[
  "0",
  "0",
  10101,
  1032,
  1032
]
```

Response: `{}`

### MarketSetRetrievalAsk


Perms: admin

Inputs:
```json
[
  {
    "PricePerByte": "0",
    "UnsealPrice": "0",
    "PaymentInterval": 42,
    "PaymentIntervalIncrease": 42
  }
]
```

Response: `{}`

## Mining


### MiningBase


Perms: read

Inputs: `null`

Response:
```json
{
  "Cids": null,
  "Blocks": null,
  "Height": 0
}
```

## Net


### NetAddrsListen


Perms: read

Inputs: `null`

Response:
```json
{
  "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Addrs": [
    "/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior"
  ]
}
```

### NetAgentVersion


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response: `"string value"`

### NetAutoNatStatus


Perms: read

Inputs: `null`

Response:
```json
{
  "Reachability": 1,
  "PublicAddr": "string value"
}
```

### NetBandwidthStats


Perms: read

Inputs: `null`

Response:
```json
{
  "TotalIn": 9,
  "TotalOut": 9,
  "RateIn": 12.3,
  "RateOut": 12.3
}
```

### NetBandwidthStatsByPeer


Perms: read

Inputs: `null`

Response:
```json
{
  "12D3KooWSXmXLJmBR1M7i9RW9GQPNUhZSzXKzxDHWtAgNuJAbyEJ": {
    "TotalIn": 174000,
    "TotalOut": 12500,
    "RateIn": 100,
    "RateOut": 50
  }
}
```

### NetBandwidthStatsByProtocol


Perms: read

Inputs: `null`

Response:
```json
{
  "/fil/hello/1.0.0": {
    "TotalIn": 174000,
    "TotalOut": 12500,
    "RateIn": 100,
    "RateOut": 50
  }
}
```

### NetBlockAdd


Perms: admin

Inputs:
```json
[
  {
    "Peers": [
      "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
    ],
    "IPAddrs": [
      "string value"
    ],
    "IPSubnets": [
      "string value"
    ]
  }
]
```

Response: `{}`

### NetBlockList


Perms: read

Inputs: `null`

Response:
```json
{
  "Peers": [
    "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
  ],
  "IPAddrs": [
    "string value"
  ],
  "IPSubnets": [
    "string value"
  ]
}
```

### NetBlockRemove


Perms: admin

Inputs:
```json
[
  {
    "Peers": [
      "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
    ],
    "IPAddrs": [
      "string value"
    ],
    "IPSubnets": [
      "string value"
    ]
  }
]
```

Response: `{}`

### NetConnect


Perms: write

Inputs:
```json
[
  {
    "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Addrs": [
      "/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior"
    ]
  }
]
```

Response: `{}`

### NetConnectedness


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response: `1`

### NetDisconnect


Perms: write

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response: `{}`

### NetFindPeer


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response:
```json
{
  "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Addrs": [
    "/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior"
  ]
}
```

### NetLimit


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response:
```json
{
  "Memory": 123,
  "Streams": 3,
  "StreamsInbound": 1,
  "StreamsOutbound": 2,
  "Conns": 4,
  "ConnsInbound": 3,
  "ConnsOutbound": 4,
  "FD": 5
}
```

### NetPeerInfo


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response:
```json
{
  "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Agent": "string value",
  "Addrs": [
    "string value"
  ],
  "Protocols": [
    "string value"
  ],
  "ConnMgrMeta": {
    "FirstSeen": "0001-01-01T00:00:00Z",
    "Value": 123,
    "Tags": {
      "name": 42
    },
    "Conns": {
      "name": "2021-03-08T22:52:18Z"
    }
  }
}
```

### NetPeers


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Addrs": [
      "/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior"
    ]
  }
]
```

### NetPing


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

Response: `60000000000`

### NetProtectAdd


Perms: admin

Inputs:
```json
[
  [
    "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
  ]
]
```

Response: `{}`

### NetProtectList


Perms: read

Inputs: `null`

Response:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
]
```

### NetProtectRemove


Perms: admin

Inputs:
```json
[
  [
    "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
  ]
]
```

Response: `{}`

### NetPubsubScores


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Score": {
      "Score": 12.3,
      "Topics": {
        "/blocks": {
          "TimeInMesh": 60000000000,
          "FirstMessageDeliveries": 122,
          "MeshMessageDeliveries": 1234,
          "InvalidMessageDeliveries": 3
        }
      },
      "AppSpecificScore": 12.3,
      "IPColocationFactor": 12.3,
      "BehaviourPenalty": 12.3
    }
  }
]
```

### NetSetLimit


Perms: admin

Inputs:
```json
[
  "string value",
  {
    "Memory": 123,
    "Streams": 3,
    "StreamsInbound": 1,
    "StreamsOutbound": 2,
    "Conns": 4,
    "ConnsInbound": 3,
    "ConnsOutbound": 4,
    "FD": 5
  }
]
```

Response: `{}`

### NetStat


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response:
```json
{
  "System": {
    "NumStreamsInbound": 123,
    "NumStreamsOutbound": 123,
    "NumConnsInbound": 123,
    "NumConnsOutbound": 123,
    "NumFD": 123,
    "Memory": 9
  },
  "Transient": {
    "NumStreamsInbound": 123,
    "NumStreamsOutbound": 123,
    "NumConnsInbound": 123,
    "NumConnsOutbound": 123,
    "NumFD": 123,
    "Memory": 9
  },
  "Services": {
    "abc": {
      "NumStreamsInbound": 1,
      "NumStreamsOutbound": 2,
      "NumConnsInbound": 3,
      "NumConnsOutbound": 4,
      "NumFD": 5,
      "Memory": 123
    }
  },
  "Protocols": {
    "abc": {
      "NumStreamsInbound": 1,
      "NumStreamsOutbound": 2,
      "NumConnsInbound": 3,
      "NumConnsOutbound": 4,
      "NumFD": 5,
      "Memory": 123
    }
  },
  "Peers": {
    "abc": {
      "NumStreamsInbound": 1,
      "NumStreamsOutbound": 2,
      "NumConnsInbound": 3,
      "NumConnsOutbound": 4,
      "NumFD": 5,
      "Memory": 123
    }
  }
}
```

## Pieces


### PiecesGetCIDInfo


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response:
```json
{
  "CID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "PieceBlockLocations": [
    {
      "RelOffset": 42,
      "BlockSize": 42,
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ]
}
```

### PiecesGetPieceInfo


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response:
```json
{
  "PieceCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Deals": [
    {
      "DealID": 5432,
      "SectorID": 9,
      "Offset": 1032,
      "Length": 1032
    }
  ]
}
```

### PiecesListCidInfos


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

### PiecesListPieces


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

## Pledge


### PledgeSector
Temp api for testing


Perms: write

Inputs: `null`

Response:
```json
{
  "Miner": 1000,
  "Number": 9
}
```

## Return


### ReturnAddPiece


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Size": 1032,
    "PieceCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnDataCid
storiface.WorkerReturn


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Size": 1032,
    "PieceCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnFetch


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnFinalizeReplicaUpdate


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnFinalizeSector


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnGenerateSectorKeyFromData


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnMoveStorage


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnProveReplicaUpdate1


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  [
    "Ynl0ZSBhcnJheQ=="
  ],
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnProveReplicaUpdate2


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  "Bw==",
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnReadPiece


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  true,
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnReleaseUnsealed


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnReplicaUpdate


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "NewSealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "NewUnsealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnSealCommit1


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  "Bw==",
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnSealCommit2


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  "Bw==",
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnSealPreCommit1


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  "Bw==",
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnSealPreCommit2


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Unsealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Sealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

### ReturnUnsealPiece


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  },
  {
    "Code": 0,
    "Message": "string value"
  }
]
```

Response: `{}`

## Runtime


### RuntimeSubsystems
RuntimeSubsystems returns the subsystems that are enabled
in this instance.


Perms: read

Inputs: `null`

Response:
```json
[
  "Mining",
  "Sealing",
  "SectorStorage",
  "Markets"
]
```

## Sealing


### SealingAbort


Perms: admin

Inputs:
```json
[
  {
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "ID": "07070707-0707-0707-0707-070707070707"
  }
]
```

Response: `{}`

### SealingSchedDiag
SealingSchedDiag dumps internal sealing scheduler state


Perms: admin

Inputs:
```json
[
  true
]
```

Response: `{}`

## Sector


### SectorAbortUpgrade
SectorAbortUpgrade can be called on sectors that are in the process of being upgraded to abort it


Perms: admin

Inputs:
```json
[
  9
]
```

Response: `{}`

### SectorAddPieceToAny
Add piece to an open sector. If no sectors with enough space are open,
either a new sector will be created, or this call will block until more
sectors can be created.


Perms: admin

Inputs:
```json
[
  1024,
  {},
  {
    "PublishCid": null,
    "DealID": 5432,
    "DealProposal": {
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceSize": 1032,
      "VerifiedDeal": true,
      "Client": "f01234",
      "Provider": "f01234",
      "Label": "",
      "StartEpoch": 10101,
      "EndEpoch": 10101,
      "StoragePricePerEpoch": "0",
      "ProviderCollateral": "0",
      "ClientCollateral": "0"
    },
    "DealSchedule": {
      "StartEpoch": 10101,
      "EndEpoch": 10101
    },
    "KeepUnsealed": true
  }
]
```

Response:
```json
{
  "Sector": 9,
  "Offset": 1032
}
```

### SectorCommitFlush
SectorCommitFlush immediately sends a Commit message with sectors aggregated for Commit.
Returns null if message wasn't sent


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Sectors": [
      123,
      124
    ],
    "FailedSectors": {
      "123": "can't acquire read lock"
    },
    "Msg": null,
    "Error": "string value"
  }
]
```

### SectorCommitPending
SectorCommitPending returns a list of pending Commit sectors to be sent in the next aggregate message


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  }
]
```

### SectorGetExpectedSealDuration
SectorGetExpectedSealDuration gets the expected time for a sector to seal


Perms: read

Inputs: `null`

Response: `60000000000`

### SectorGetSealDelay
SectorGetSealDelay gets the time that a newly-created sector
waits for more deals before it starts sealing


Perms: read

Inputs: `null`

Response: `60000000000`

### SectorMarkForUpgrade


Perms: admin

Inputs:
```json
[
  9,
  true
]
```

Response: `{}`

### SectorMatchPendingPiecesToOpenSectors


Perms: admin

Inputs: `null`

Response: `{}`

### SectorPreCommitFlush
SectorPreCommitFlush immediately sends a PreCommit message with sectors batched for PreCommit.
Returns null if message wasn't sent


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Sectors": [
      123,
      124
    ],
    "Msg": null,
    "Error": "string value"
  }
]
```

### SectorPreCommitPending
SectorPreCommitPending returns a list of pending PreCommit sectors to be sent in the next batch message


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  }
]
```

### SectorRemove
SectorRemove removes the sector from storage. It doesn't terminate it on-chain, which can
be done with SectorTerminate. Removing and not terminating live sectors will cause additional penalties.


Perms: admin

Inputs:
```json
[
  9
]
```

Response: `{}`

### SectorSetExpectedSealDuration
SectorSetExpectedSealDuration sets the expected time for a sector to seal


Perms: write

Inputs:
```json
[
  60000000000
]
```

Response: `{}`

### SectorSetSealDelay
SectorSetSealDelay sets the time that a newly-created sector
waits for more deals before it starts sealing


Perms: write

Inputs:
```json
[
  60000000000
]
```

Response: `{}`

### SectorStartSealing
SectorStartSealing can be called on sectors in Empty or WaitDeals states
to trigger sealing early


Perms: write

Inputs:
```json
[
  9
]
```

Response: `{}`

### SectorTerminate
SectorTerminate terminates the sector on-chain (adding it to a termination batch first), then
automatically removes it from storage


Perms: admin

Inputs:
```json
[
  9
]
```

Response: `{}`

### SectorTerminateFlush
SectorTerminateFlush immediately sends a terminate message with sectors batched for termination.
Returns null if message wasn't sent


Perms: admin

Inputs: `null`

Response: `null`

### SectorTerminatePending
SectorTerminatePending returns a list of pending sector terminations to be sent in the next batch message


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  }
]
```

## Sectors


### SectorsList
List all staged sectors


Perms: read

Inputs: `null`

Response:
```json
[
  123,
  124
]
```

### SectorsListInStates
List sectors in particular states


Perms: read

Inputs:
```json
[
  [
    "Proving"
  ]
]
```

Response:
```json
[
  123,
  124
]
```

### SectorsRefs


Perms: read

Inputs: `null`

Response:
```json
{
  "98000": [
    {
      "SectorID": 100,
      "Offset": 10485760,
      "Size": 1048576
    }
  ]
}
```

### SectorsStatus
Get the status of a given sector by ID


Perms: read

Inputs:
```json
[
  9,
  true
]
```

Response:
```json
{
  "SectorID": 9,
  "State": "Proving",
  "CommD": null,
  "CommR": null,
  "Proof": "Ynl0ZSBhcnJheQ==",
  "Deals": [
    5432
  ],
  "Pieces": [
    {
      "Piece": {
        "Size": 1032,
        "PieceCID": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        }
      },
      "DealInfo": {
        "PublishCid": null,
        "DealID": 5432,
        "DealProposal": {
          "PieceCID": {
            "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
          },
          "PieceSize": 1032,
          "VerifiedDeal": true,
          "Client": "f01234",
          "Provider": "f01234",
          "Label": "",
          "StartEpoch": 10101,
          "EndEpoch": 10101,
          "StoragePricePerEpoch": "0",
          "ProviderCollateral": "0",
          "ClientCollateral": "0"
        },
        "DealSchedule": {
          "StartEpoch": 10101,
          "EndEpoch": 10101
        },
        "KeepUnsealed": true
      }
    }
  ],
  "Ticket": {
    "Value": "Bw==",
    "Epoch": 10101
  },
  "Seed": {
    "Value": "Bw==",
    "Epoch": 10101
  },
  "PreCommitMsg": null,
  "CommitMsg": null,
  "Retries": 42,
  "ToUpgrade": true,
  "ReplicaUpdateMessage": null,
  "LastErr": "string value",
  "Log": [
    {
      "Kind": "string value",
      "Timestamp": 42,
      "Trace": "string value",
      "Message": "string value"
    }
  ],
  "SealProof": 8,
  "Activation": 10101,
  "Expiration": 10101,
  "DealWeight": "0",
  "VerifiedDealWeight": "0",
  "InitialPledge": "0",
  "OnTime": 10101,
  "Early": 10101
}
```

### SectorsSummary
Get summary info of sectors


Perms: read

Inputs: `null`

Response:
```json
{
  "Proving": 120
}
```

### SectorsUnsealPiece


Perms: admin

Inputs:
```json
[
  {
    "ID": {
      "Miner": 1000,
      "Number": 9
    },
    "ProofType": 8
  },
  1040384,
  1024,
  "Bw==",
  null
]
```

Response: `{}`

### SectorsUpdate


Perms: admin

Inputs:
```json
[
  9,
  "Proving"
]
```

Response: `{}`

## Storage


### StorageAddLocal


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

### StorageAttach
SectorIndex


Perms: admin

Inputs:
```json
[
  {
    "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
    "URLs": [
      "string value"
    ],
    "Weight": 42,
    "MaxStorage": 42,
    "CanSeal": true,
    "CanStore": true,
    "Groups": [
      "string value"
    ],
    "AllowTo": [
      "string value"
    ]
  },
  {
    "Capacity": 9,
    "Available": 9,
    "FSAvailable": 9,
    "Reserved": 9,
    "Max": 9,
    "Used": 9
  }
]
```

Response: `{}`

### StorageBestAlloc


Perms: admin

Inputs:
```json
[
  1,
  34359738368,
  "sealing"
]
```

Response:
```json
[
  {
    "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
    "URLs": [
      "string value"
    ],
    "Weight": 42,
    "MaxStorage": 42,
    "CanSeal": true,
    "CanStore": true,
    "Groups": [
      "string value"
    ],
    "AllowTo": [
      "string value"
    ]
  }
]
```

### StorageDeclareSector


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
  {
    "Miner": 1000,
    "Number": 9
  },
  1,
  true
]
```

Response: `{}`

### StorageDropSector


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
  {
    "Miner": 1000,
    "Number": 9
  },
  1
]
```

Response: `{}`

### StorageFindSector


Perms: admin

Inputs:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  },
  1,
  34359738368,
  true
]
```

Response:
```json
[
  {
    "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
    "URLs": [
      "string value"
    ],
    "BaseURLs": [
      "string value"
    ],
    "Weight": 42,
    "CanSeal": true,
    "CanStore": true,
    "Primary": true
  }
]
```

### StorageGetLocks


Perms: admin

Inputs: `null`

Response:
```json
{
  "Locks": [
    {
      "Sector": {
        "Miner": 1000,
        "Number": 123
      },
      "Write": [
        0,
        0,
        1,
        0,
        0
      ],
      "Read": [
        2,
        3,
        0,
        0,
        0
      ]
    }
  ]
}
```

### StorageInfo


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8"
]
```

Response:
```json
{
  "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
  "URLs": [
    "string value"
  ],
  "Weight": 42,
  "MaxStorage": 42,
  "CanSeal": true,
  "CanStore": true,
  "Groups": [
    "string value"
  ],
  "AllowTo": [
    "string value"
  ]
}
```

### StorageList


Perms: admin

Inputs: `null`

Response:
```json
{
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": [
    {
      "Miner": 1000,
      "Number": 100,
      "SectorFileType": 2
    }
  ]
}
```

### StorageLocal


Perms: admin

Inputs: `null`

Response:
```json
{
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": "/data/path"
}
```

### StorageLock


Perms: admin

Inputs:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  },
  1,
  1
]
```

Response: `{}`

### StorageReportHealth


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
  {
    "Stat": {
      "Capacity": 9,
      "Available": 9,
      "FSAvailable": 9,
      "Reserved": 9,
      "Max": 9,
      "Used": 9
    },
    "Err": "string value"
  }
]
```

Response: `{}`

### StorageStat


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8"
]
```

Response:
```json
{
  "Capacity": 9,
  "Available": 9,
  "FSAvailable": 9,
  "Reserved": 9,
  "Max": 9,
  "Used": 9
}
```

### StorageTryLock


Perms: admin

Inputs:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  },
  1,
  1
]
```

Response: `true`

## Worker


### WorkerConnect
WorkerConnect tells the node to connect to workers RPC


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

### WorkerJobs


Perms: admin

Inputs: `null`

Response:
```json
{
  "ef8d99a2-6865-4189-8ffa-9fef0f806eee": [
    {
      "ID": {
        "Sector": {
          "Miner": 1000,
          "Number": 100
        },
        "ID": "76081ba0-61bd-45a5-bc08-af05f1c26e5d"
      },
      "Sector": {
        "Miner": 1000,
        "Number": 100
      },
      "Task": "seal/v0/precommit/2",
      "RunWait": 0,
      "Start": "2020-11-12T09:22:07Z",
      "Hostname": "host"
    }
  ]
}
```

### WorkerStats


Perms: admin

Inputs: `null`

Response:
```json
{
  "ef8d99a2-6865-4189-8ffa-9fef0f806eee": {
    "Info": {
      "Hostname": "host",
      "IgnoreResources": false,
      "Resources": {
        "MemPhysical": 274877906944,
        "MemUsed": 2147483648,
        "MemSwap": 128849018880,
        "MemSwapUsed": 2147483648,
        "CPUs": 64,
        "GPUs": [
          "aGPU 1337"
        ],
        "Resources": {
          "post/v0/windowproof": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 32212254720,
              "MaxMemory": 103079215104,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 64424509440,
              "MaxMemory": 128849018880,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 32212254720,
              "MaxMemory": 103079215104,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 64424509440,
              "MaxMemory": 128849018880,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            }
          },
          "post/v0/winningproof": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/addpiece": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/commit/1": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/commit/2": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/datacid": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/fetch": {
            "0": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/precommit/1": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/precommit/2": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 16106127360,
              "MaxMemory": 16106127360,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 32212254720,
              "MaxMemory": 32212254720,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 16106127360,
              "MaxMemory": 16106127360,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 32212254720,
              "MaxMemory": 32212254720,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/provereplicaupdate/1": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/provereplicaupdate/2": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/regensectorkey": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/replicaupdate": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/unseal": {
            "0": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            }
          }
        }
      }
    },
    "Tasks": null,
    "Enabled": true,
    "MemUsedMin": 0,
    "MemUsedMax": 0,
    "GpuUsed": 0,
    "CpuUse": 0,
    "TaskCounts": null
  }
}
```

