# Groups
* [](#)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Allocate](#Allocate)
  * [AllocatePieceToSector](#AllocatePieceToSector)
* [Storage](#Storage)
  * [StorageAddLocal](#StorageAddLocal)
  * [StorageDetachLocal](#StorageDetachLocal)
  * [StorageInfo](#StorageInfo)
  * [StorageList](#StorageList)
  * [StorageLocal](#StorageLocal)
  * [StorageStat](#StorageStat)
## 


### Shutdown


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

