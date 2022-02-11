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
  * [FinalizeReplicaUpdate](#FinalizeReplicaUpdate)
  * [FinalizeSector](#FinalizeSector)
* [Generate](#Generate)
  * [GenerateSectorKeyFromData](#GenerateSectorKeyFromData)
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
    "GPUs": null,
    "Resources": {
      "seal/v0/addpiece": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/commit/1": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/commit/2": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10737418240
        },
        "3": {
          "MinMemory": 32212254720,
          "MaxMemory": 161061273600,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 34359738368
        },
        "4": {
          "MinMemory": 64424509440,
          "MaxMemory": 204010946560,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 68719476736
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10737418240
        },
        "8": {
          "MinMemory": 32212254720,
          "MaxMemory": 161061273600,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 34359738368
        },
        "9": {
          "MinMemory": 64424509440,
          "MaxMemory": 204010946560,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 68719476736
        }
      },
      "seal/v0/fetch": {
        "0": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "1": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "2": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "3": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "4": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "5": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "6": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "7": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "8": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        },
        "9": {
          "MinMemory": 1048576,
          "MaxMemory": 1048576,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 0
        }
      },
      "seal/v0/precommit/1": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 805306368,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1048576
        },
        "3": {
          "MinMemory": 60129542144,
          "MaxMemory": 68719476736,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "4": {
          "MinMemory": 120259084288,
          "MaxMemory": 137438953472,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 805306368,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1048576
        },
        "8": {
          "MinMemory": 60129542144,
          "MaxMemory": 68719476736,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "9": {
          "MinMemory": 120259084288,
          "MaxMemory": 137438953472,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        }
      },
      "seal/v0/precommit/2": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 16106127360,
          "MaxMemory": 16106127360,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 32212254720,
          "MaxMemory": 32212254720,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 0,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 16106127360,
          "MaxMemory": 16106127360,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 32212254720,
          "MaxMemory": 32212254720,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/provereplicaupdate/1": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 0,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/provereplicaupdate/2": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10737418240
        },
        "3": {
          "MinMemory": 32212254720,
          "MaxMemory": 161061273600,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 34359738368
        },
        "4": {
          "MinMemory": 64424509440,
          "MaxMemory": 204010946560,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 68719476736
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1610612736,
          "GPUUtilization": 1,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10737418240
        },
        "8": {
          "MinMemory": 32212254720,
          "MaxMemory": 161061273600,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 34359738368
        },
        "9": {
          "MinMemory": 64424509440,
          "MaxMemory": 204010946560,
          "GPUUtilization": 1,
          "MaxParallelism": -1,
          "MaxParallelismGPU": 6,
          "BaseMinMemory": 68719476736
        }
      },
      "seal/v0/regensectorkey": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/replicaupdate": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "3": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "4": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 1073741824,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "8": {
          "MinMemory": 4294967296,
          "MaxMemory": 4294967296,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        },
        "9": {
          "MinMemory": 8589934592,
          "MaxMemory": 8589934592,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1073741824
        }
      },
      "seal/v0/unseal": {
        "0": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "1": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "2": {
          "MinMemory": 805306368,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1048576
        },
        "3": {
          "MinMemory": 60129542144,
          "MaxMemory": 68719476736,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "4": {
          "MinMemory": 120259084288,
          "MaxMemory": 137438953472,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "5": {
          "MinMemory": 2048,
          "MaxMemory": 2048,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 2048
        },
        "6": {
          "MinMemory": 8388608,
          "MaxMemory": 8388608,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 8388608
        },
        "7": {
          "MinMemory": 805306368,
          "MaxMemory": 1073741824,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 1048576
        },
        "8": {
          "MinMemory": 60129542144,
          "MaxMemory": 68719476736,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        },
        "9": {
          "MinMemory": 120259084288,
          "MaxMemory": 137438953472,
          "GPUUtilization": 0,
          "MaxParallelism": 1,
          "MaxParallelismGPU": 0,
          "BaseMinMemory": 10485760
        }
      }
    }
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

Response: `131584`

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

