# Groups
* [](#)
  * [Enabled](#Enabled)
  * [Fetch](#Fetch)
  * [Info](#Info)
  * [Paths](#Paths)
  * [Remove](#Remove)
  * [Session](#Session)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Add](#Add)
  * [AddPiece](#AddPiece)
* [Data](#Data)
  * [DataCid](#DataCid)
* [Download](#Download)
  * [DownloadSectorData](#DownloadSectorData)
* [Finalize](#Finalize)
  * [FinalizeReplicaUpdate](#FinalizeReplicaUpdate)
  * [FinalizeSector](#FinalizeSector)
* [Generate](#Generate)
  * [GenerateSectorKeyFromData](#GenerateSectorKeyFromData)
  * [GenerateWindowPoSt](#GenerateWindowPoSt)
  * [GenerateWinningPoSt](#GenerateWinningPoSt)
* [Move](#Move)
  * [MoveStorage](#MoveStorage)
* [Process](#Process)
  * [ProcessSession](#ProcessSession)
* [Prove](#Prove)
  * [ProveReplicaUpdate1](#ProveReplicaUpdate1)
  * [ProveReplicaUpdate2](#ProveReplicaUpdate2)
* [Release](#Release)
  * [ReleaseUnsealed](#ReleaseUnsealed)
* [Replica](#Replica)
  * [ReplicaUpdate](#ReplicaUpdate)
* [Seal](#Seal)
  * [SealCommit1](#SealCommit1)
  * [SealCommit2](#SealCommit2)
  * [SealPreCommit1](#SealPreCommit1)
  * [SealPreCommit2](#SealPreCommit2)
* [Set](#Set)
  * [SetEnabled](#SetEnabled)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
  * [StorageDetachAll](#StorageDetachAll)
  * [StorageDetachLocal](#StorageDetachLocal)
  * [StorageLocal](#StorageLocal)
  * [StorageRedeclareLocal](#StorageRedeclareLocal)
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
  "IgnoreResources": true,
  "Resources": {
    "MemPhysical": 42,
    "MemUsed": 42,
    "MemSwap": 42,
    "MemSwapUsed": 42,
    "CPUs": 42,
    "GPUs": [
      "string value"
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
}
```

### Paths


Perms: admin

Inputs: `null`

Response:
```json
[
  {
    "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
    "Weight": 42,
    "LocalPath": "string value",
    "CanSeal": true,
    "CanStore": true
  }
]
```

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

### Shutdown
Trigger shutdown


Perms: admin

Inputs: `null`

Response: `{}`

### Version


Perms: admin

Inputs: `null`

Response: `131840`

## Add


### AddPiece


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
  [
    1024
  ],
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

## Data


### DataCid
storiface.WorkerCalls


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
  "Sector": {
    "Miner": 1000,
    "Number": 9
  },
  "ID": "07070707-0707-0707-0707-070707070707"
}
```

## Download


### DownloadSectorData


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
  true,
  {
    "2": {
      "Local": false,
      "URL": "https://example.com/sealingservice/sectors/s-f0123-12345",
      "Headers": null
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

## Finalize


### FinalizeReplicaUpdate


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

## Generate


### GenerateSectorKeyFromData


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

### GenerateWindowPoSt


Perms: admin

Inputs:
```json
[
  8,
  1000,
  [
    {
      "SealProof": 8,
      "SectorNumber": 9,
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Challenge": [
        42
      ],
      "Update": true
    }
  ],
  123,
  "Bw=="
]
```

Response:
```json
{
  "PoStProofs": {
    "PoStProof": 8,
    "ProofBytes": "Ynl0ZSBhcnJheQ=="
  },
  "Skipped": [
    {
      "Miner": 1000,
      "Number": 9
    }
  ]
}
```

### GenerateWinningPoSt


Perms: admin

Inputs:
```json
[
  8,
  1000,
  [
    {
      "SealProof": 8,
      "SectorNumber": 9,
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Challenge": [
        42
      ],
      "Update": true
    }
  ],
  "Bw=="
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

## Prove


### ProveReplicaUpdate1


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
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
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

### ProveReplicaUpdate2


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
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  [
    "Ynl0ZSBhcnJheQ=="
  ]
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
  [
    {
      "Offset": 1024,
      "Size": 1024
    }
  ]
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

## Replica


### ReplicaUpdate


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
  [
    {
      "Size": 1032,
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ]
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
  "Bw==",
  "Bw==",
  [
    {
      "Size": 1032,
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ],
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
  "Bw=="
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
  "Bw==",
  [
    {
      "Size": 1032,
      "PieceCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ]
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
  "Bw=="
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

### StorageDetachAll


Perms: admin

Inputs: `null`

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

### StorageLocal


Perms: admin

Inputs: `null`

Response:
```json
{
  "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": "/data/path"
}
```

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
  "Bw==",
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

