# Groups
* [](#)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Allocate](#Allocate)
  * [AllocatePieceToSector](#AllocatePieceToSector)
* [Log](#Log)
  * [LogList](#LogList)
  * [LogSetLevel](#LogSetLevel)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
  * [StorageDetachLocal](#StorageDetachLocal)
  * [StorageFindSector](#StorageFindSector)
  * [StorageInfo](#StorageInfo)
  * [StorageInit](#StorageInit)
  * [StorageList](#StorageList)
  * [StorageLocal](#StorageLocal)
  * [StorageStat](#StorageStat)
## 


### Shutdown
Trigger shutdown


Perms: admin

Inputs: `null`

Response: `{}`

### Version


Perms: admin

Inputs: `null`

Response: `131840`

## Allocate


### AllocatePieceToSector


Perms: write

Inputs:
```json
[
  "f01234",
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
  },
  9,
  {
    "Scheme": "string value",
    "Opaque": "string value",
    "User": {},
    "Host": "string value",
    "Path": "string value",
    "RawPath": "string value",
    "OmitHost": true,
    "ForceQuery": true,
    "RawQuery": "string value",
    "Fragment": "string value",
    "RawFragment": "string value"
  },
  {
    "Authorization": [
      "Bearer ey.."
    ]
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

## Log


### LogList


Perms: read

Inputs: `null`

Response:
```json
[
  "string value"
]
```

### LogSetLevel


Perms: admin

Inputs:
```json
[
  "string value",
  "string value"
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

### StorageDetachLocal


Perms: admin

Inputs:
```json
[
  "string value"
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

### StorageInit


Perms: admin

Inputs:
```json
[
  "string value",
  {
    "ID": "76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8",
    "Weight": 42,
    "CanSeal": true,
    "CanStore": true,
    "MaxStorage": 42,
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

Response: `{}`

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

