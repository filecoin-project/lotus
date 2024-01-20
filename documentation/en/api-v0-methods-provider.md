# Groups
* [](#)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Allocate](#Allocate)
  * [AllocatePieceToSector](#AllocatePieceToSector)
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

