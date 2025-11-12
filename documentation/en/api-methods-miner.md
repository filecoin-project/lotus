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
  * [ActorWithdrawBalance](#ActorWithdrawBalance)
* [Auth](#Auth)
  * [AuthNew](#AuthNew)
  * [AuthVerify](#AuthVerify)
* [Beneficiary](#Beneficiary)
  * [BeneficiaryWithdrawBalance](#BeneficiaryWithdrawBalance)
* [Check](#Check)
  * [CheckProvable](#CheckProvable)
* [Compute](#Compute)
  * [ComputeDataCid](#ComputeDataCid)
  * [ComputeProof](#ComputeProof)
  * [ComputeWindowPoSt](#ComputeWindowPoSt)
* [Create](#Create)
  * [CreateBackup](#CreateBackup)
* [Log](#Log)
  * [LogAlerts](#LogAlerts)
  * [LogList](#LogList)
  * [LogSetLevel](#LogSetLevel)
* [Market](#Market)
  * [MarketListDeals](#MarketListDeals)
* [Mining](#Mining)
  * [MiningBase](#MiningBase)
* [Pledge](#Pledge)
  * [PledgeSector](#PledgeSector)
* [Recover](#Recover)
  * [RecoverFault](#RecoverFault)
* [Return](#Return)
  * [ReturnAddPiece](#ReturnAddPiece)
  * [ReturnDataCid](#ReturnDataCid)
  * [ReturnDownloadSector](#ReturnDownloadSector)
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
  * [SealingRemoveRequest](#SealingRemoveRequest)
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
  * [SectorNumAssignerMeta](#SectorNumAssignerMeta)
  * [SectorNumFree](#SectorNumFree)
  * [SectorNumReservations](#SectorNumReservations)
  * [SectorNumReserve](#SectorNumReserve)
  * [SectorNumReserveCount](#SectorNumReserveCount)
  * [SectorPreCommitFlush](#SectorPreCommitFlush)
  * [SectorPreCommitPending](#SectorPreCommitPending)
  * [SectorReceive](#SectorReceive)
  * [SectorRemove](#SectorRemove)
  * [SectorSetExpectedSealDuration](#SectorSetExpectedSealDuration)
  * [SectorSetSealDelay](#SectorSetSealDelay)
  * [SectorStartSealing](#SectorStartSealing)
  * [SectorTerminate](#SectorTerminate)
  * [SectorTerminateFlush](#SectorTerminateFlush)
  * [SectorTerminatePending](#SectorTerminatePending)
  * [SectorUnseal](#SectorUnseal)
* [Sectors](#Sectors)
  * [SectorsList](#SectorsList)
  * [SectorsListInStates](#SectorsListInStates)
  * [SectorsRefs](#SectorsRefs)
  * [SectorsStatus](#SectorsStatus)
  * [SectorsSummary](#SectorsSummary)
  * [SectorsUnsealPiece](#SectorsUnsealPiece)
  * [SectorsUpdate](#SectorsUpdate)
* [Start](#Start)
  * [StartTime](#StartTime)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
  * [StorageAttach](#StorageAttach)
  * [StorageAuthVerify](#StorageAuthVerify)
  * [StorageBestAlloc](#StorageBestAlloc)
  * [StorageDeclareSector](#StorageDeclareSector)
  * [StorageDetach](#StorageDetach)
  * [StorageDetachLocal](#StorageDetachLocal)
  * [StorageDropSector](#StorageDropSector)
  * [StorageFindSector](#StorageFindSector)
  * [StorageGetLocks](#StorageGetLocks)
  * [StorageInfo](#StorageInfo)
  * [StorageList](#StorageList)
  * [StorageLocal](#StorageLocal)
  * [StorageLock](#StorageLock)
  * [StorageRedeclareLocal](#StorageRedeclareLocal)
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
  "APIVersion": 131840,
  "BlockDelay": 42,
  "Agent": "string value"
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

### ActorWithdrawBalance
ActorWithdrawBalance allows to withdraw balance from miner actor to owner address
Specify amount as "0" to withdraw full balance. This method returns a message CID
and does not wait for message execution


Perms: admin

Inputs:
```json
[
  "0"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

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

## Beneficiary


### BeneficiaryWithdrawBalance
BeneficiaryWithdrawBalance allows the beneficiary of a miner to withdraw balance from miner actor
Specify amount as "0" to withdraw full balance. This method returns a message CID
and does not wait for message execution


Perms: admin

Inputs:
```json
[
  "0"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
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
  ]
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
      "SectorKey": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ],
  "Bw==",
  10101,
  28
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
CreateBackup creates node backup under the specified file name. The
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
      "SectorNumber": 9,
      "SectorStartEpoch": 10101,
      "LastUpdatedEpoch": 10101,
      "SlashEpoch": 10101
    }
  }
]
```

## Mining


### MiningBase


Perms: read

Inputs: `null`

Response:
```json
{
  "Cids": [
    {
      "/": "bafy2bzacedo7hjsumaajt6sbor42qycvjyk6goqe4oi4o4ddsjxkdeqrqf42c"
    }
  ],
  "Blocks": [
    {
      "Miner": "f01938223",
      "Ticket": {
        "VRFProof": "rIPyBy+F827Szc5oN/6ylCmpzxfAWr7aI5F4YJrN4pLSyknkcJI3ivsCo2KKjQVZFRnFyEus1maD5LdzQpnFRKMla4138qEuML+Ne/fsgOMrUEAeL34ceVwJd+Mt4Jrz"
      },
      "ElectionProof": {
        "WinCount": 1,
        "VRFProof": "sN51JqjZNf+xWxwoo+wlMH1bpXI9T3wUIrla6FpwTxU4jC1z+ab5NFU/B2ZdDITTE+u8qaiibtLkld5lhNcOEOUqwKNyJ4nwFo5vAhWqvOTNdOiZmxsKpWG0NZUoXb/+"
      },
      "BeaconEntries": [
        {
          "Round": 17133822,
          "Data": "tH4q8euIaP9/QRJt8ALfkBvttSmQ/DOAt8+37wGGV5f8kkhzEFrHhskitNnPS70j"
        },
        {
          "Round": 17133832,
          "Data": "uQD5cEn8U69+sPjpccT8Bm0jVrnXLScf2jBkLJNHvAHLA6tPsZDREzpBIckpVvPy"
        }
      ],
      "WinPoStProof": [
        {
          "PoStProof": 3,
          "ProofBytes": "qOPLMhMui8qm/rE2y/UceyBDv5JvRCH5Fc5Ul+kuN190XDcMme5eKURUCmE2sN1HoQ2dMZX+xNZY351dbG93H/tUr6wuNhkvmemi2Xi62YvqU36/kJh+K2YBiW7h/4LXCUTP/6XAOONOPl+j9GqS7RQxruPLfIyehvzVC0C8dB8+SVWtAnRKRPUUOPJvyHKejlrCyzWXOz/I7JG2/qEGLD0xwazBVwML1vVvuE5NzXeOoQGlnB2PwSRb5Cn8FH8Q"
        }
      ],
      "Parents": [
        {
          "/": "bafy2bzaceba2kdmysmi5ieugzvv5np7f2lobayzpvtk777du74n7jq6xhynda"
        },
        {
          "/": "bafy2bzacecrye24tkqrvvddcf62gfi4z4o33z2tdedbpaalordozaxfrz2jyi"
        },
        {
          "/": "bafy2bzaceab5mrohjvnp3mz7mo33ky7qqlmssrs7veqmjrgouafxyhnd5dy66"
        }
      ],
      "ParentWeight": "116013147118",
      "Height": 4863283,
      "ParentStateRoot": {
        "/": "bafy2bzaceajxzsvzuq3ddzxfrs2jlaxsooqmgdy5uxbqujnjy3y56iumzzy7u"
      },
      "ParentMessageReceipts": {
        "/": "bafy2bzacecfcx2ykqucyv3gkyrcy3upwrvdraz3ktfg7phkqysefdwsggglac"
      },
      "Messages": {
        "/": "bafy2bzacebzofmh6migvc4v6qsme6vuxlhi6pv2ocy4apyic3uihjqm7dum3u"
      },
      "BLSAggregate": {
        "Type": 2,
        "Data": "krFATGA0OBu/kFwtXsThVtKCkppnU7045uTURCeiOeJttxuXfx3wqJrLkCytnJFWFLVC+tiVWI4BxC3wqc9r6eAlNr9dEBx+3KwML/RFG/b5grmknLpGWn7g1EB/2T4y"
      },
      "Timestamp": 1744204890,
      "BlockSig": {
        "Type": 2,
        "Data": "pWiUr+M8xxTxLED7GuU586gSfZCaHyLbLj0uS0HhKYRtHuyG47fIrfIT/04OCmQvEXBD8pFraWbMc3tnFrSsM1mIBJ5M38UPUfXDSspo+QGdouo2kll2X+VNKY3ajb1K"
      },
      "ForkSignaling": 0,
      "ParentBaseFee": "20592036"
    }
  ],
  "Height": 4863283
}
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

## Recover


### RecoverFault
RecoverFault can be used to declare recoveries manually. It sends messages
to the miner actor with details of recovered sectors and returns the CID of messages. It honors the
maxPartitionsPerRecoveryMessage from the config


Perms: admin

Inputs:
```json
[
  [
    123,
    124
  ]
]
```

Response:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
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

### ReturnDownloadSector


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
  "SectorStorage"
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

### SealingRemoveRequest
SealingRemoveRequest removes a request from sealing pipeline


Perms: admin

Inputs:
```json
[
  "07070707-0707-0707-0707-070707070707"
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
    "PublishCid": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
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
    "PieceActivationManifest": {
      "CID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Size": 2032,
      "VerifiedAllocationKey": null,
      "Notify": null
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
    "Msg": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
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

### SectorNumAssignerMeta
SectorNumAssignerMeta returns sector number assigner metadata - reserved/allocated


Perms: read

Inputs: `null`

Response:
```json
{
  "Reserved": [
    5,
    1
  ],
  "Allocated": [
    5,
    1
  ],
  "InUse": [
    5,
    1
  ],
  "Next": 9
}
```

### SectorNumFree
SectorNumFree drops a sector reservation


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

### SectorNumReservations
SectorNumReservations returns a list of sector number reservations


Perms: read

Inputs: `null`

Response:
```json
{
  "": [
    5,
    3,
    2,
    1
  ]
}
```

### SectorNumReserve
SectorNumReserve creates a new sector number reservation. Will fail if any other reservation has colliding
numbers or name. Set force to true to override safety checks.
Valid characters for name: a-z, A-Z, 0-9, _, -


Perms: admin

Inputs:
```json
[
  "string value",
  [
    5,
    1
  ],
  true
]
```

Response: `{}`

### SectorNumReserveCount
SectorNumReserveCount creates a new sector number reservation for `count` sector numbers.
by default lotus will allocate lowest-available sector numbers to the reservation.
For restrictions on `name` see SectorNumReserve


Perms: admin

Inputs:
```json
[
  "string value",
  42
]
```

Response:
```json
[
  5,
  1
]
```

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
    "Msg": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
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

### SectorReceive


Perms: admin

Inputs:
```json
[
  {
    "State": "Proving",
    "Sector": {
      "Miner": 1000,
      "Number": 9
    },
    "Type": 8,
    "Pieces": [
      {
        "Piece": {
          "Size": 1032,
          "PieceCID": {
            "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
          }
        },
        "DealInfo": {
          "PublishCid": {
            "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
          },
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
          "PieceActivationManifest": {
            "CID": {
              "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
            },
            "Size": 2032,
            "VerifiedAllocationKey": null,
            "Notify": null
          },
          "KeepUnsealed": true
        }
      }
    ],
    "TicketValue": "Bw==",
    "TicketEpoch": 10101,
    "PreCommit1Out": "Bw==",
    "CommD": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "CommR": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PreCommitInfo": {
      "SealProof": 8,
      "SectorNumber": 9,
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "SealRandEpoch": 10101,
      "DealIDs": [
        5432
      ],
      "Expiration": 10101,
      "UnsealedCid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    },
    "PreCommitDeposit": "0",
    "PreCommitMessage": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PreCommitTipSet": [
      {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      {
        "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
      }
    ],
    "SeedValue": "Bw==",
    "SeedEpoch": 10101,
    "CommitProof": "Ynl0ZSBhcnJheQ==",
    "CommitMessage": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Log": [
      {
        "Kind": "string value",
        "Timestamp": 42,
        "Trace": "string value",
        "Message": "string value"
      }
    ],
    "DataUnsealed": {
      "Local": true,
      "URL": "string value",
      "Headers": [
        {
          "Key": "string value",
          "Value": "string value"
        }
      ]
    },
    "DataSealed": {
      "Local": true,
      "URL": "string value",
      "Headers": [
        {
          "Key": "string value",
          "Value": "string value"
        }
      ]
    },
    "DataCache": {
      "Local": true,
      "URL": "string value",
      "Headers": [
        {
          "Key": "string value",
          "Value": "string value"
        }
      ]
    },
    "RemoteCommit1Endpoint": "string value",
    "RemoteCommit2Endpoint": "string value",
    "RemoteSealingDoneEndpoint": "string value"
  }
]
```

Response: `{}`

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

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

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

### SectorUnseal
SectorUnseal unseals the provided sector


Perms: admin

Inputs:
```json
[
  9
]
```

Response: `{}`

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
  "CommD": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "CommR": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
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
        "PublishCid": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
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
        "PieceActivationManifest": {
          "CID": {
            "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
          },
          "Size": 2032,
          "VerifiedAllocationKey": null,
          "Notify": null
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
  "PreCommitMsg": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "CommitMsg": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Retries": 42,
  "ToUpgrade": true,
  "ReplicaUpdateMessage": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
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
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
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

## Start


### StartTime


Perms: read

Inputs: `null`

Response: `"0001-01-01T00:00:00Z"`

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
paths.SectorIndex


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
    ],
    "AllowTypes": [
      "string value"
    ],
    "DenyTypes": [
      "string value"
    ],
    "AllowMiners": [
      "string value"
    ],
    "DenyMiners": [
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

### StorageAuthVerify


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

### StorageBestAlloc
StorageBestAlloc returns list of paths where sector files of the specified type can be allocated, ordered by preference.
Paths with more weight and more % of free space are preferred.
Note: This method doesn't filter paths based on AllowTypes/DenyTypes.


Perms: admin

Inputs:
```json
[
  1,
  34359738368,
  "sealing",
  1000
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
    ],
    "AllowTypes": [
      "string value"
    ],
    "DenyTypes": [
      "string value"
    ],
    "AllowMiners": [
      "string value"
    ],
    "DenyMiners": [
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

### StorageDetach


Perms: admin

Inputs:
```json
[
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
  "string value"
]
```

Response: `{}`

### StorageDetachLocal


Perms: admin

Inputs:
```json
[
  "string value"
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
StorageFindSector returns list of paths where the specified sector files exist.

If allowFetch is set, list of paths to which the sector can be fetched will also be returned.
- Paths which have sector files locally (don't require fetching) will be listed first.
- Paths which have sector files locally will not be filtered based on AllowTypes/DenyTypes.
- Paths which require fetching will be filtered based on AllowTypes/DenyTypes. If multiple
  file types are specified, each type will be considered individually, and a union of all paths
  which can accommodate each file type will be returned.


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
    "Primary": true,
    "AllowTypes": [
      "string value"
    ],
    "DenyTypes": [
      "string value"
    ],
    "AllowMiners": [
      "string value"
    ],
    "DenyMiners": [
      "string value"
    ]
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
        0,
        0
      ],
      "Read": [
        2,
        3,
        0,
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
  ],
  "AllowTypes": [
    "string value"
  ],
  "DenyTypes": [
    "string value"
  ],
  "AllowMiners": [
    "string value"
  ],
  "DenyMiners": [
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

### StorageRedeclareLocal


Perms: admin

Inputs:
```json
[
  "1399aa04-2625-44b1-bad4-bd07b59b22c4",
  true
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 32212254720,
              "MaxMemory": 103079215104,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 64424509440,
              "MaxMemory": 128849018880,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
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
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "1": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "10": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
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
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "5": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "6": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "7": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
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
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
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
            "10": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 1048576,
              "MaxMemory": 1048576,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 0,
              "MaxConcurrent": 0
            },
            "14": {
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 0,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 16106127360,
              "MaxMemory": 16106127360,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 32212254720,
              "MaxMemory": 32212254720,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 0,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1610612736,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10737418240,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 32212254720,
              "MaxMemory": 161061273600,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 34359738368,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 64424509440,
              "MaxMemory": 204010946560,
              "GPUUtilization": 1,
              "MaxParallelism": -1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 68719476736,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
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
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            }
          },
          "seal/v0/replicaupdate": {
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "2": {
              "MinMemory": 1073741824,
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "3": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "4": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
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
              "MaxMemory": 1073741824,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "8": {
              "MinMemory": 4294967296,
              "MaxMemory": 4294967296,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
              "BaseMinMemory": 1073741824,
              "MaxConcurrent": 0
            },
            "9": {
              "MinMemory": 8589934592,
              "MaxMemory": 8589934592,
              "GPUUtilization": 1,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 6,
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
            "10": {
              "MinMemory": 2048,
              "MaxMemory": 2048,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 2048,
              "MaxConcurrent": 0
            },
            "11": {
              "MinMemory": 8388608,
              "MaxMemory": 8388608,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 8388608,
              "MaxConcurrent": 0
            },
            "12": {
              "MinMemory": 805306368,
              "MaxMemory": 1073741824,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 1048576,
              "MaxConcurrent": 0
            },
            "13": {
              "MinMemory": 60129542144,
              "MaxMemory": 68719476736,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
              "MaxConcurrent": 0
            },
            "14": {
              "MinMemory": 120259084288,
              "MaxMemory": 137438953472,
              "GPUUtilization": 0,
              "MaxParallelism": 1,
              "MaxParallelismGPU": 0,
              "BaseMinMemory": 10485760,
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

