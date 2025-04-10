# Groups
* [Chain](#Chain)
  * [ChainGetTipSet](#ChainGetTipSet)
* [State](#State)
  * [StateCompute](#StateCompute)
  * [StateGetActor](#StateGetActor)
  * [StateGetID](#StateGetID)
  * [StateSimulate](#StateSimulate)
## Chain
The Chain method group contains methods for interacting with
the blockchain.

<b>Note: This API is experimental and may change in the future.<b/>

Please see Filecoin V2 API design documentation for more details:
  - https://www.notion.so/filecoindev/Lotus-F3-aware-APIs-1cfdc41950c180ae97fef580e79427d5
  - https://www.notion.so/filecoindev/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658


### ChainGetTipSet
ChainGetTipSet retrieves a tipset that corresponds to the specified selector
criteria. The criteria can be provided in the form of a tipset key, a
blockchain height including an optional fallback to previous non-null tipset,
or a designated tag such as "latest" or "finalized".

The "Finalized" tag returns the tipset that is considered finalized based on
the consensus protocol of the current node, either Filecoin EC Finality or
Filecoin Fast Finality (F3). The finalized tipset selection gracefully falls
back to EC finality in cases where F3 isn't ready or not running.

In a case where no selector is provided, an error is returned. The selector
must be explicitly specified.

For more details, refer to the types.TipSetSelector and
types.NewTipSetSelector.

Example usage:

	selector := types.TipSetSelectors.Latest
	tipSet, err := node.ChainGetTipSet(context.Background(), selector)
	if err != nil {
		fmt.Println("Error retrieving tipset:", err)
		return
	}
	fmt.Printf("Latest TipSet: %v\n", tipSet)


Perms: read

Inputs:
```json
[
  {
    "tag": "finalized"
  }
]
```

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

## State
The State method group contains methods for interacting with the Filecoin
blockchain state, including actor information, addresses, and chain data.
These methods allow querying the blockchain state at any point in its history
using flexible TipSet selection mechanisms.


### StateCompute
StateCompute computes the state at the specified tipset and applies the provided messages.

This function:
- Loads the tipset identified by the TipSetSelector
- Calculates the complete state by applying all chain messages within that tipset
- Applies the user-provided messages on top of that state
- Uses the network version of the selected tipset for message execution

The function applies messages with the correct network parameters corresponding
to the tipset's epoch, making it suitable for testing messages against the
current network version.

Messages must have the correct nonces and gas values set.

The TipSetSelector parameter provides flexible options for selecting the base tipset:
  - TipSetSelectors.Latest: the most recent tipset with the heaviest weight
  - TipSetSelectors.Finalized: the most recent finalized tipset
  - TipSetSelectors.Height(epoch, previous, anchor): tipset at the specified height
  - TipSetSelectors.Key(key): tipset with the specified key

See types.TipSetSelector documentation for additional details.

Experimental: This API is experimental and may change without notice.


Perms: read

Inputs:
```json
[
  [
    {
      "Version": 42,
      "To": "f01234",
      "From": "f01234",
      "Nonce": 42,
      "Value": "0",
      "GasLimit": 9,
      "GasFeeCap": "0",
      "GasPremium": "0",
      "Method": 1,
      "Params": "Ynl0ZSBhcnJheQ==",
      "CID": {
        "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
      }
    }
  ],
  {
    "tag": "finalized"
  }
]
```

Response:
```json
{
  "Root": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Trace": [
    {
      "MsgCid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Msg": {
        "Version": 42,
        "To": "f01234",
        "From": "f01234",
        "Nonce": 42,
        "Value": "0",
        "GasLimit": 9,
        "GasFeeCap": "0",
        "GasPremium": "0",
        "Method": 1,
        "Params": "Ynl0ZSBhcnJheQ==",
        "CID": {
          "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
        }
      },
      "MsgRct": {
        "ExitCode": 0,
        "Return": "Ynl0ZSBhcnJheQ==",
        "GasUsed": 9,
        "EventsRoot": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        }
      },
      "GasCost": {
        "Message": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
        "GasUsed": "0",
        "BaseFeeBurn": "0",
        "OverEstimationBurn": "0",
        "MinerPenalty": "0",
        "MinerTip": "0",
        "Refund": "0",
        "TotalCost": "0"
      },
      "ExecutionTrace": {
        "Msg": {
          "From": "f01234",
          "To": "f01234",
          "Value": "0",
          "Method": 1,
          "Params": "Ynl0ZSBhcnJheQ==",
          "ParamsCodec": 42,
          "GasLimit": 42,
          "ReadOnly": true
        },
        "MsgRct": {
          "ExitCode": 0,
          "Return": "Ynl0ZSBhcnJheQ==",
          "ReturnCodec": 42
        },
        "InvokedActor": {
          "Id": 1000,
          "State": {
            "Code": {
              "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
            },
            "Head": {
              "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
            },
            "Nonce": 42,
            "Balance": "0",
            "DelegatedAddress": "f01234"
          }
        },
        "GasCharges": [
          {
            "Name": "string value",
            "tg": 9,
            "cg": 9,
            "sg": 9,
            "tt": 60000000000
          }
        ],
        "Subcalls": [
          {
            "Msg": {
              "From": "f01234",
              "To": "f01234",
              "Value": "0",
              "Method": 1,
              "Params": "Ynl0ZSBhcnJheQ==",
              "ParamsCodec": 42,
              "GasLimit": 42,
              "ReadOnly": true
            },
            "MsgRct": {
              "ExitCode": 0,
              "Return": "Ynl0ZSBhcnJheQ==",
              "ReturnCodec": 42
            },
            "InvokedActor": {
              "Id": 1000,
              "State": {
                "Code": {
                  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
                },
                "Head": {
                  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
                },
                "Nonce": 42,
                "Balance": "0",
                "DelegatedAddress": "f01234"
              }
            },
            "GasCharges": [
              {
                "Name": "string value",
                "tg": 9,
                "cg": 9,
                "sg": 9,
                "tt": 60000000000
              }
            ],
            "Subcalls": null
          }
        ]
      },
      "Error": "string value",
      "Duration": 60000000000
    }
  ]
}
```

### StateGetActor
StateGetActor retrieves the actor information for the specified address at the
selected tipset.

This function returns the on-chain Actor object including:
  - Code CID (determines the actor's type)
  - State root CID
  - Balance in attoFIL
  - Nonce (for account actors)

The TipSetSelector parameter provides flexible options for selecting the tipset:
  - TipSetSelectors.Latest: the most recent tipset with the heaviest weight
  - TipSetSelectors.Finalized: the most recent finalized tipset
  - TipSetSelectors.Height(epoch, previous, anchor): tipset at the specified height
  - TipSetSelectors.Key(key): tipset with the specified key

See types.TipSetSelector documentation for additional details.

If the actor does not exist at the specified tipset, this function returns nil.

Experimental: This API is experimental and may change without notice.


Perms: read

Inputs:
```json
[
  "f01234",
  {
    "tag": "finalized"
  }
]
```

Response:
```json
{
  "Code": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Head": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Nonce": 42,
  "Balance": "0",
  "DelegatedAddress": "f01234"
}
```

### StateGetID
StateGetID retrieves the ID address for the specified address at the selected tipset.

Every actor on the Filecoin network has a unique ID address (format: f0123).
This function resolves any address type (ID, robust, or delegated) to its canonical
ID address representation at the specified tipset.

The function is particularly useful for:
  - Normalizing different address formats to a consistent representation
  - Following address changes across state transitions
  - Verifying that an address corresponds to an existing actor

The TipSetSelector parameter provides flexible options for selecting the tipset.
See StateGetActor documentation for details on selection options.

If the address cannot be resolved at the specified tipset, this function returns nil.

Experimental: This API is experimental and may change without notice.


Perms: read

Inputs:
```json
[
  "f01234",
  {
    "tag": "finalized"
  }
]
```

Response: `"f01234"`

### StateSimulate


Perms: 

Inputs:
```json
[
  [
    {
      "Version": 42,
      "To": "f01234",
      "From": "f01234",
      "Nonce": 42,
      "Value": "0",
      "GasLimit": 9,
      "GasFeeCap": "0",
      "GasPremium": "0",
      "Method": 1,
      "Params": "Ynl0ZSBhcnJheQ==",
      "CID": {
        "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
      }
    }
  ],
  {
    "tag": "finalized"
  },
  {
    "height": 1413
  }
]
```

Response:
```json
{
  "Root": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Trace": [
    {
      "MsgCid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Msg": {
        "Version": 42,
        "To": "f01234",
        "From": "f01234",
        "Nonce": 42,
        "Value": "0",
        "GasLimit": 9,
        "GasFeeCap": "0",
        "GasPremium": "0",
        "Method": 1,
        "Params": "Ynl0ZSBhcnJheQ==",
        "CID": {
          "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
        }
      },
      "MsgRct": {
        "ExitCode": 0,
        "Return": "Ynl0ZSBhcnJheQ==",
        "GasUsed": 9,
        "EventsRoot": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        }
      },
      "GasCost": {
        "Message": {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        },
        "GasUsed": "0",
        "BaseFeeBurn": "0",
        "OverEstimationBurn": "0",
        "MinerPenalty": "0",
        "MinerTip": "0",
        "Refund": "0",
        "TotalCost": "0"
      },
      "ExecutionTrace": {
        "Msg": {
          "From": "f01234",
          "To": "f01234",
          "Value": "0",
          "Method": 1,
          "Params": "Ynl0ZSBhcnJheQ==",
          "ParamsCodec": 42,
          "GasLimit": 42,
          "ReadOnly": true
        },
        "MsgRct": {
          "ExitCode": 0,
          "Return": "Ynl0ZSBhcnJheQ==",
          "ReturnCodec": 42
        },
        "InvokedActor": {
          "Id": 1000,
          "State": {
            "Code": {
              "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
            },
            "Head": {
              "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
            },
            "Nonce": 42,
            "Balance": "0",
            "DelegatedAddress": "f01234"
          }
        },
        "GasCharges": [
          {
            "Name": "string value",
            "tg": 9,
            "cg": 9,
            "sg": 9,
            "tt": 60000000000
          }
        ],
        "Subcalls": [
          {
            "Msg": {
              "From": "f01234",
              "To": "f01234",
              "Value": "0",
              "Method": 1,
              "Params": "Ynl0ZSBhcnJheQ==",
              "ParamsCodec": 42,
              "GasLimit": 42,
              "ReadOnly": true
            },
            "MsgRct": {
              "ExitCode": 0,
              "Return": "Ynl0ZSBhcnJheQ==",
              "ReturnCodec": 42
            },
            "InvokedActor": {
              "Id": 1000,
              "State": {
                "Code": {
                  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
                },
                "Head": {
                  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
                },
                "Nonce": 42,
                "Balance": "0",
                "DelegatedAddress": "f01234"
              }
            },
            "GasCharges": [
              {
                "Name": "string value",
                "tg": 9,
                "cg": 9,
                "sg": 9,
                "tt": 60000000000
              }
            ],
            "Subcalls": null
          }
        ]
      },
      "Error": "string value",
      "Duration": 60000000000
    }
  ]
}
```

