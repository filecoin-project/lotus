# Groups
* [](#)
  * [Enabled](#Enabled)
  * [Fetch](#Fetch)
  * [Info](#Info)
  * [Paths](#Paths)
  * [Remove](#Remove)
  * [Session](#Session)
  * [Version](#Version)
* [Add](#Add)
  * [AddPiece](#AddPiece)
* [Finalize](#Finalize)
  * [FinalizeSector](#FinalizeSector)
* [Move](#Move)
  * [MoveStorage](#MoveStorage)
* [Process](#Process)
  * [ProcessSession](#ProcessSession)
* [Read](#Read)
  * [ReadPiece](#ReadPiece)
* [Release](#Release)
  * [ReleaseUnsealed](#ReleaseUnsealed)
* [Seal](#Seal)
  * [SealCommit1](#SealCommit1)
  * [SealCommit2](#SealCommit2)
  * [SealPreCommit1](#SealPreCommit1)
  * [SealPreCommit2](#SealPreCommit2)
* [Set](#Set)
  * [SetEnabled](#SetEnabled)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
* [Task](#Task)
  * [TaskDisable](#TaskDisable)
  * [TaskEnable](#TaskEnable)
  * [TaskTypes](#TaskTypes)
* [Unseal](#Unseal)
  * [UnsealPiece](#UnsealPiece)
* [Wait](#Wait)
  * [WaitQuiet](#WaitQuiet)
## 


### Enabled


Perms: admin

Inputs: `null`

Response: `true`

### Fetch


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
  1,
  "sealing",
  "move"
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

### Info


Perms: admin

Inputs: `null`

Response:
```json
{
  "Hostname": "string value",
  "Resources": {
    "MemPhysical": 42,
    "MemSwap": 42,
    "MemReserved": 42,
    "CPUs": 42,
    "GPUs": null
  }
}
```

### Paths


Perms: admin

Inputs: `null`

Response: `null`

### Remove
Storage / Other


Perms: admin

Inputs:
```json
[
  {
    "Miner": 1000,
    "Number": 9
  }
]
```

Response: `{}`

### Session
Like ProcessSession, but returns an error when worker is disabled


Perms: admin

Inputs: `null`

Response: `"07070707-0707-0707-0707-070707070707"`

### Version


Perms: admin

Inputs: `null`

Response: `131328`

## Add


### AddPiece
storiface.WorkerCalls


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
  null,
  1024,
  {}
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Finalize


### FinalizeSector


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
  null
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Move


### MoveStorage


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
  1
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Process


### ProcessSession
returns a random UUID of worker session, generated randomly when worker
process starts


Perms: admin

Inputs: `null`

Response: `"07070707-0707-0707-0707-070707070707"`

## Read


### ReadPiece


Perms: admin

Inputs:
```json
[
  {},
  {
    "ID": {
      "Miner": 1000,
      "Number": 9
    },
    "ProofType": 8
  },
  1040384,
  1024
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Release


### ReleaseUnsealed


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
  null
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Seal


### SealCommit1


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
  null,
  null,
  null,
  {
    "Unsealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Sealed": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  }
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

### SealCommit2


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
  null
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

### SealPreCommit1


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
  null,
  null
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

### SealPreCommit2


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
  null
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Set


### SetEnabled
SetEnabled marks the worker as enabled/disabled. Not that this setting
may take a few seconds to propagate to task scheduler


Perms: admin

Inputs:
```json
[
  true
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

## Task


### TaskDisable


Perms: admin

Inputs:
```json
[
  "seal/v0/commit/2"
]
```

Response: `{}`

### TaskEnable


Perms: admin

Inputs:
```json
[
  "seal/v0/commit/2"
]
```

Response: `{}`

### TaskTypes
TaskType -> Weight


Perms: admin

Inputs: `null`

Response:
```json
{
  "seal/v0/precommit/2": {}
}
```

## Unseal


### UnsealPiece


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
  null,
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response:
```json
{
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Wait


### WaitQuiet
WaitQuiet blocks until there are no tasks running


Perms: admin

Inputs: `null`

Response: `{}`

