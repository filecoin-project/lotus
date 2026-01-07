# Groups
* [Chain](#Chain)
  * [ChainGetTipSet](#ChainGetTipSet)
* [Eth](#Eth)
  * [EthAccounts](#EthAccounts)
  * [EthAddressToFilecoinAddress](#EthAddressToFilecoinAddress)
  * [EthBlockNumber](#EthBlockNumber)
  * [EthCall](#EthCall)
  * [EthChainId](#EthChainId)
  * [EthEstimateGas](#EthEstimateGas)
  * [EthFeeHistory](#EthFeeHistory)
  * [EthGasPrice](#EthGasPrice)
  * [EthGetBalance](#EthGetBalance)
  * [EthGetBlockByHash](#EthGetBlockByHash)
  * [EthGetBlockByNumber](#EthGetBlockByNumber)
  * [EthGetBlockReceipts](#EthGetBlockReceipts)
  * [EthGetBlockReceiptsLimited](#EthGetBlockReceiptsLimited)
  * [EthGetBlockTransactionCountByHash](#EthGetBlockTransactionCountByHash)
  * [EthGetBlockTransactionCountByNumber](#EthGetBlockTransactionCountByNumber)
  * [EthGetCode](#EthGetCode)
  * [EthGetFilterChanges](#EthGetFilterChanges)
  * [EthGetFilterLogs](#EthGetFilterLogs)
  * [EthGetLogs](#EthGetLogs)
  * [EthGetMessageCidByTransactionHash](#EthGetMessageCidByTransactionHash)
  * [EthGetStorageAt](#EthGetStorageAt)
  * [EthGetTransactionByBlockHashAndIndex](#EthGetTransactionByBlockHashAndIndex)
  * [EthGetTransactionByBlockNumberAndIndex](#EthGetTransactionByBlockNumberAndIndex)
  * [EthGetTransactionByHash](#EthGetTransactionByHash)
  * [EthGetTransactionByHashLimited](#EthGetTransactionByHashLimited)
  * [EthGetTransactionCount](#EthGetTransactionCount)
  * [EthGetTransactionHashByCid](#EthGetTransactionHashByCid)
  * [EthGetTransactionReceipt](#EthGetTransactionReceipt)
  * [EthGetTransactionReceiptLimited](#EthGetTransactionReceiptLimited)
  * [EthMaxPriorityFeePerGas](#EthMaxPriorityFeePerGas)
  * [EthNewBlockFilter](#EthNewBlockFilter)
  * [EthNewFilter](#EthNewFilter)
  * [EthNewPendingTransactionFilter](#EthNewPendingTransactionFilter)
  * [EthProtocolVersion](#EthProtocolVersion)
  * [EthSendRawTransaction](#EthSendRawTransaction)
  * [EthSendRawTransactionUntrusted](#EthSendRawTransactionUntrusted)
  * [EthSubscribe](#EthSubscribe)
  * [EthSyncing](#EthSyncing)
  * [EthTraceBlock](#EthTraceBlock)
  * [EthTraceFilter](#EthTraceFilter)
  * [EthTraceReplayBlockTransactions](#EthTraceReplayBlockTransactions)
  * [EthTraceTransaction](#EthTraceTransaction)
  * [EthUninstallFilter](#EthUninstallFilter)
  * [EthUnsubscribe](#EthUnsubscribe)
* [Filecoin](#Filecoin)
  * [FilecoinAddressToEthAddress](#FilecoinAddressToEthAddress)
* [Net](#Net)
  * [NetListening](#NetListening)
  * [NetVersion](#NetVersion)
* [State](#State)
  * [StateGetActor](#StateGetActor)
  * [StateGetID](#StateGetID)
* [Web3](#Web3)
  * [Web3ClientVersion](#Web3ClientVersion)
## Chain
The Chain method group contains methods for interacting with
the blockchain.

<b>Note: This API is experimental and may change as we explore the
appropriate design for the Filecoin v2 APIs.<b/>

Please see Filecoin V2 API design documentation for more details:
  - https://www.notion.so/filecoindev/Lotus-F3-aware-APIs-1cfdc41950c180ae97fef580e79427d5
  - https://www.notion.so/filecoindev/Filecoin-V2-APIs-1d0dc41950c1808b914de5966d501658


### ChainGetTipSet
ChainGetTipSet retrieves a tipset that corresponds to the specified selector
criteria. The criteria can be provided in the form of a tipset key, a
blockchain height including an optional fallback to previous non-null tipset,
or a designated tag such as "latest", "finalized", or "safe".

The "Finalized" tag returns the tipset that is considered finalized based on
the consensus protocol of the current node, either Filecoin EC Finality or
Filecoin Fast Finality (F3). The finalized tipset selection gracefully falls
back to EC finality in cases where F3 isn't ready or not running.

The "Safe" tag returns the tipset between the "Finalized" tipset and
"Latest - build.SafeHeightDistance". This provides a balance between
finality confidence and recency. If the tipset at the safe height is null,
the first non-nil parent tipset is returned, similar to the behavior of
selecting by height with the 'previous' option set to true.

In a case where no selector is provided, an error is returned. The selector
must be explicitly specified.

For more details, refer to the types.TipSetSelector.

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

## Eth
These methods are used for Ethereum-compatible JSON-RPC calls

### Execution model reconciliation

Ethereum relies on an immediate block-based execution model. The block that includes
a transaction is also the block that executes it. Each block specifies the state root
resulting from executing all transactions within it (output state).

In Filecoin, at every epoch there is an unknown number of round winners all of whom are
entitled to publish a block. Blocks are collected into a tipset. A tipset is committed
only when the subsequent tipset is built on it (i.e. it becomes a parent). Block producers
execute the parent tipset and specify the resulting state root in the block being produced.
In other words, contrary to Ethereum, each block specifies the input state root.

Ethereum clients expect transactions returned via eth_getBlock* to have a receipt
(due to immediate execution). For this reason:

  - eth_blockNumber returns the latest executed epoch (head - 1)
  - The 'latest' block refers to the latest executed epoch (head - 1)
  - The 'pending' block refers to the current speculative tipset (head)
  - eth_getTransactionByHash returns the inclusion tipset of a message, but
    only after it has executed.
  - eth_getTransactionReceipt ditto.

"Latest executed epoch" refers to the tipset that this node currently
accepts as the best parent tipset, based on the blocks it is accumulating
within the HEAD tipset.


### EthAccounts
EthAccounts returns a list of Ethereum accounts managed by the node.
Maps to JSON-RPC method: "eth_accounts".
Lotus does not manage private keys, so this will always return an empty array.


Perms: read

Inputs: `null`

Response:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
]
```

### EthAddressToFilecoinAddress
EthAddressToFilecoinAddress converts an Ethereum address to a Filecoin f410 address.


Perms: read

Inputs:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
]
```

Response: `"f01234"`

### EthBlockNumber
EthBlockNumber returns the number of the latest executed block (head - 1).
Maps to JSON-RPC method: "eth_blockNumber".


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthCall
EthCall executes a read-only call to a contract at a specific block state, identified by
its number, hash, or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_call".


Perms: read

Inputs:
```json
[
  {
    "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "gas": "0x5",
    "gasPrice": "0x0",
    "value": "0x0",
    "data": "0x07"
  },
  "string value"
]
```

Response: `"0x07"`

### EthChainId
EthChainId retrieves the chain ID of the Ethereum-compatible network.
Maps to JSON-RPC method: "eth_chainId".


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthEstimateGas
EthEstimateGas estimates the gas required to execute a transaction.
Maps to JSON-RPC method: "eth_estimateGas".


Perms: read

Inputs:
```json
[
  "Bw=="
]
```

Response: `"0x5"`

### EthFeeHistory
EthFeeHistory retrieves historical gas fee data for a range of blocks.
Maps to JSON-RPC method: "eth_feeHistory".


Perms: read

Inputs:
```json
[
  "Bw=="
]
```

Response:
```json
{
  "oldestBlock": "0x5",
  "baseFeePerGas": [
    "0x0"
  ],
  "gasUsedRatio": [
    12.3
  ],
  "reward": []
}
```

### EthGasPrice
EthGasPrice retrieves the current gas price in the network.
Maps to JSON-RPC method: "eth_gasPrice".


Perms: read

Inputs: `null`

Response: `"0x0"`

### EthGetBalance
EthGetBalance retrieves the balance of an Ethereum address at a specific block state,
identified by its number, hash, or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_getBalance".


Perms: read

Inputs:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "string value"
]
```

Response: `"0x0"`

### EthGetBlockByHash
EthGetBlockByHash retrieves a block by its hash. If fullTxInfo is true, it includes full
transaction objects; otherwise, it includes only transaction hashes.
Maps to JSON-RPC method: "eth_getBlockByHash".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  true
]
```

Response:
```json
{
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "parentHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "sha3Uncles": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "miner": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "stateRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "transactionsRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "receiptsRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "logsBloom": "0x07",
  "difficulty": "0x5",
  "totalDifficulty": "0x5",
  "number": "0x5",
  "gasLimit": "0x5",
  "gasUsed": "0x5",
  "timestamp": "0x5",
  "extraData": "0x07",
  "mixHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "nonce": "0x0707070707070707",
  "baseFeePerGas": "0x0",
  "size": "0x5",
  "transactions": [
    {}
  ],
  "uncles": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ]
}
```

### EthGetBlockByNumber
EthGetBlockByNumber retrieves a block by its number or a special tag like "latest" or
"finalized". If fullTxInfo is true, it includes full transaction objects; otherwise, it
includes only transaction hashes.
Maps to JSON-RPC method: "eth_getBlockByNumber".


Perms: read

Inputs:
```json
[
  "string value",
  true
]
```

Response:
```json
{
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "parentHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "sha3Uncles": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "miner": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "stateRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "transactionsRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "receiptsRoot": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "logsBloom": "0x07",
  "difficulty": "0x5",
  "totalDifficulty": "0x5",
  "number": "0x5",
  "gasLimit": "0x5",
  "gasUsed": "0x5",
  "timestamp": "0x5",
  "extraData": "0x07",
  "mixHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "nonce": "0x0707070707070707",
  "baseFeePerGas": "0x0",
  "size": "0x5",
  "transactions": [
    {}
  ],
  "uncles": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ]
}
```

### EthGetBlockReceipts
EthGetBlockReceipts retrieves all transaction receipts for a block identified by its number,
hash or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_getBlockReceipts".


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
  {
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "transactionIndex": "0x5",
    "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "blockNumber": "0x5",
    "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "root": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "status": "0x5",
    "contractAddress": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "cumulativeGasUsed": "0x5",
    "gasUsed": "0x5",
    "effectiveGasPrice": "0x0",
    "logsBloom": "0x07",
    "logs": [
      {
        "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
        "data": "0x07",
        "topics": [
          "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
        ],
        "removed": true,
        "logIndex": "0x5",
        "transactionIndex": "0x5",
        "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
        "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
        "blockNumber": "0x5"
      }
    ],
    "type": "0x5",
    "authorizationList": [
      {
        "chainId": "0x5",
        "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
        "nonce": "0x5",
        "yParity": 7,
        "r": "0x0",
        "s": "0x0"
      }
    ],
    "delegatedTo": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ]
  }
]
```

### EthGetBlockReceiptsLimited
EthGetBlockReceiptsLimited retrieves all transaction receipts for a block identified by its
number, hash or a special tag like "latest" or "finalized", along with an optional limit on the
chain epoch for state resolution.


Perms: read

Inputs:
```json
[
  "string value",
  10101
]
```

Response:
```json
[
  {
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "transactionIndex": "0x5",
    "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "blockNumber": "0x5",
    "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "root": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "status": "0x5",
    "contractAddress": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
    "cumulativeGasUsed": "0x5",
    "gasUsed": "0x5",
    "effectiveGasPrice": "0x0",
    "logsBloom": "0x07",
    "logs": [
      {
        "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
        "data": "0x07",
        "topics": [
          "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
        ],
        "removed": true,
        "logIndex": "0x5",
        "transactionIndex": "0x5",
        "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
        "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
        "blockNumber": "0x5"
      }
    ],
    "type": "0x5",
    "authorizationList": [
      {
        "chainId": "0x5",
        "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
        "nonce": "0x5",
        "yParity": 7,
        "r": "0x0",
        "s": "0x0"
      }
    ],
    "delegatedTo": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ]
  }
]
```

### EthGetBlockTransactionCountByHash
EthGetBlockTransactionCountByHash returns the number of transactions in a block identified by
its block hash.
Maps to JSON-RPC method: "eth_getBlockTransactionCountByHash".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response: `"0x5"`

### EthGetBlockTransactionCountByNumber
EthGetBlockTransactionCountByNumber returns the number of transactions in a block identified by
its block number or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_getBlockTransactionCountByNumber".


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response: `"0x5"`

### EthGetCode
EthGetCode retrieves the contract code at a specific address and block state, identified by
its number, hash, or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_getCode".


Perms: read

Inputs:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "string value"
]
```

Response: `"0x07"`

### EthGetFilterChanges
EthGetFilterChanges retrieves event logs that occurred since the last poll for a filter.
Maps to JSON-RPC method: "eth_getFilterChanges".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response:
```json
[
  {}
]
```

### EthGetFilterLogs
EthGetFilterLogs retrieves event logs matching filter with given id.
Maps to JSON-RPC method: "eth_getFilterLogs".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response:
```json
[
  {}
]
```

### EthGetLogs
EthGetLogs retrieves event logs matching given filter specification.
Maps to JSON-RPC method: "eth_getLogs".


Perms: read

Inputs:
```json
[
  {
    "fromBlock": "2301220",
    "address": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ],
    "topics": null
  }
]
```

Response:
```json
[
  {}
]
```

### EthGetMessageCidByTransactionHash
EthGetMessageCidByTransactionHash retrieves the Filecoin CID corresponding to an Ethereum
transaction hash.
Maps to JSON-RPC method: "eth_getMessageCidByTransactionHash".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### EthGetStorageAt
EthGetStorageAt retrieves the storage value at a specific position for a contract
at a given block state, identified by its number, hash, or a special tag like "latest" or
"finalized".
Maps to JSON-RPC method: "eth_getStorageAt".


Perms: read

Inputs:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "0x07",
  "string value"
]
```

Response: `"0x07"`

### EthGetTransactionByBlockHashAndIndex
EthGetTransactionByBlockHashAndIndex retrieves a transaction by its block hash and index.
Maps to JSON-RPC method: "eth_getTransactionByBlockHashAndIndex".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "0x5"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": "0x5",
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "transactionIndex": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "value": "0x0",
  "type": "0x5",
  "input": "0x07",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "gasPrice": "0x0",
  "accessList": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ],
  "v": "0x0",
  "r": "0x0",
  "s": "0x0",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ]
}
```

### EthGetTransactionByBlockNumberAndIndex
EthGetTransactionByBlockNumberAndIndex retrieves a transaction by its block number and index.
Maps to JSON-RPC method: "eth_getTransactionByBlockNumberAndIndex".


Perms: read

Inputs:
```json
[
  "string value",
  "0x5"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": "0x5",
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "transactionIndex": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "value": "0x0",
  "type": "0x5",
  "input": "0x07",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "gasPrice": "0x0",
  "accessList": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ],
  "v": "0x0",
  "r": "0x0",
  "s": "0x0",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ]
}
```

### EthGetTransactionByHash
EthGetTransactionByHash retrieves a transaction by its hash.
Maps to JSON-RPC method: "eth_getTransactionByHash".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": "0x5",
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "transactionIndex": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "value": "0x0",
  "type": "0x5",
  "input": "0x07",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "gasPrice": "0x0",
  "accessList": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ],
  "v": "0x0",
  "r": "0x0",
  "s": "0x0",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ]
}
```

### EthGetTransactionByHashLimited
EthGetTransactionByHashLimited retrieves a transaction by its hash, with an optional limit on
the chain epoch for state resolution.


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  10101
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": "0x5",
  "hash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "transactionIndex": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "value": "0x0",
  "type": "0x5",
  "input": "0x07",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "gasPrice": "0x0",
  "accessList": [
    "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
  ],
  "v": "0x0",
  "r": "0x0",
  "s": "0x0",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ]
}
```

### EthGetTransactionCount
EthGetTransactionCount retrieves the number of transactions sent from an address at a specific
block, identified by its number, hash, or a special tag like "latest" or "finalized".
Maps to JSON-RPC method: "eth_getTransactionCount".


Perms: read

Inputs:
```json
[
  "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "string value"
]
```

Response: `"0x5"`

### EthGetTransactionHashByCid
EthGetTransactionHashByCid retrieves the Ethereum transaction hash corresponding to a Filecoin
CID.
Maps to JSON-RPC method: "eth_getTransactionHashByCid".


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthGetTransactionReceipt
EthGetTransactionReceipt retrieves the receipt of a transaction by its hash.
Maps to JSON-RPC method: "eth_getTransactionReceipt".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response:
```json
{
  "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "transactionIndex": "0x5",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "root": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "status": "0x5",
  "contractAddress": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "cumulativeGasUsed": "0x5",
  "gasUsed": "0x5",
  "effectiveGasPrice": "0x0",
  "logsBloom": "0x07",
  "logs": [
    {
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "data": "0x07",
      "topics": [
        "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
      ],
      "removed": true,
      "logIndex": "0x5",
      "transactionIndex": "0x5",
      "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
      "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
      "blockNumber": "0x5"
    }
  ],
  "type": "0x5",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ],
  "delegatedTo": [
    "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
  ]
}
```

### EthGetTransactionReceiptLimited
EthGetTransactionReceiptLimited retrieves the receipt of a transaction by its hash, with an
optional limit on the chain epoch for state resolution.


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  10101
]
```

Response:
```json
{
  "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "transactionIndex": "0x5",
  "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "blockNumber": "0x5",
  "from": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "to": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "root": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
  "status": "0x5",
  "contractAddress": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "cumulativeGasUsed": "0x5",
  "gasUsed": "0x5",
  "effectiveGasPrice": "0x0",
  "logsBloom": "0x07",
  "logs": [
    {
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "data": "0x07",
      "topics": [
        "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
      ],
      "removed": true,
      "logIndex": "0x5",
      "transactionIndex": "0x5",
      "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
      "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
      "blockNumber": "0x5"
    }
  ],
  "type": "0x5",
  "authorizationList": [
    {
      "chainId": "0x5",
      "address": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
      "nonce": "0x5",
      "yParity": 7,
      "r": "0x0",
      "s": "0x0"
    }
  ],
  "delegatedTo": [
    "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
  ]
}
```

### EthMaxPriorityFeePerGas
EthMaxPriorityFeePerGas retrieves the maximum priority fee per gas in the network.
Maps to JSON-RPC method: "eth_maxPriorityFeePerGas".


Perms: read

Inputs: `null`

Response: `"0x0"`

### EthNewBlockFilter
EthNewBlockFilter installs a persistent filter to notify when a new block arrives.
Maps to JSON-RPC method: "eth_newBlockFilter".


Perms: read

Inputs: `null`

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthNewFilter
EthNewFilter installs a persistent filter based on given filter specification.
Maps to JSON-RPC method: "eth_newFilter".


Perms: read

Inputs:
```json
[
  {
    "fromBlock": "2301220",
    "address": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ],
    "topics": null
  }
]
```

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthNewPendingTransactionFilter
EthNewPendingTransactionFilter installs a persistent filter to notify when new messages arrive
in the message pool.
Maps to JSON-RPC method: "eth_newPendingTransactionFilter".


Perms: read

Inputs: `null`

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthProtocolVersion
EthProtocolVersion retrieves the Ethereum protocol version supported by the node.
Maps to JSON-RPC method: "eth_protocolVersion".


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthSendRawTransaction
EthSendRawTransaction submits a raw Ethereum transaction to the network.
Maps to JSON-RPC method: "eth_sendRawTransaction".


Perms: read

Inputs:
```json
[
  "0x07"
]
```

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthSendRawTransactionUntrusted
EthSendRawTransactionUntrusted submits a raw Ethereum transaction from an untrusted source.


Perms: read

Inputs:
```json
[
  "0x07"
]
```

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthSubscribe
EthSubscribe subscribes to different event types using websockets.
Maps to JSON-RPC method: "eth_subscribe".

eventTypes is one or more of:
 - newHeads: notify when new blocks arrive.
 - pendingTransactions: notify when new messages arrive in the message pool.
 - logs: notify new event logs that match a criteria

params contains additional parameters used with the log event type
The client will receive a stream of EthSubscriptionResponse values until EthUnsubscribe is called.


Perms: read

Inputs:
```json
[
  "Bw=="
]
```

Response: `"0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"`

### EthSyncing
EthSyncing checks if the node is currently syncing and returns the sync status.
Maps to JSON-RPC method: "eth_syncing".


Perms: read

Inputs: `null`

Response: `false`

### EthTraceBlock
EthTraceBlock returns an OpenEthereum-compatible trace of the given block.
Maps to JSON-RPC method: "trace_block".

Translates Filecoin semantics into Ethereum semantics and traces both EVM and FVM calls.

Features:

- FVM actor create events, calls, etc. show up as if they were EVM smart contract events.
- Native FVM call inputs are ABI-encoded (Solidity ABI) as if they were calls to a
  `handle_filecoin_method(uint64 method, uint64 codec, bytes params)` function
  (where `codec` is the IPLD codec of `params`).
- Native FVM call outputs (return values) are ABI-encoded as `(uint32 exit_code, uint64
  codec, bytes output)` where `codec` is the IPLD codec of `output`.

Limitations (for now):

1. Block rewards are not included in the trace.
2. SELFDESTRUCT operations are not included in the trace.
3. EVM smart contract "create" events always specify `0xfe` as the "code" for newly created EVM
   smart contracts.


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
  {
    "type": "string value",
    "error": "string value",
    "subtraces": 123,
    "traceAddress": [
      123
    ],
    "action": {},
    "result": {},
    "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "blockNumber": 9,
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "transactionPosition": 123
  }
]
```

### EthTraceFilter
EthTraceFilter returns traces matching the given filter criteria.
Maps to JSON-RPC method: "trace_filter".


Perms: read

Inputs:
```json
[
  {
    "fromBlock": "latest",
    "toBlock": "latest",
    "fromAddress": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ],
    "toAddress": [
      "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"
    ],
    "after": "0x0",
    "count": "0x64"
  }
]
```

Response:
```json
[
  {
    "type": "string value",
    "error": "string value",
    "subtraces": 123,
    "traceAddress": [
      123
    ],
    "action": {},
    "result": {},
    "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "blockNumber": 9,
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "transactionPosition": 123
  }
]
```

### EthTraceReplayBlockTransactions
EthTraceReplayBlockTransactions replays all transactions in a block returning the requested
traces for each transaction.
Maps to JSON-RPC method: "trace_replayBlockTransactions".

The block can be identified by its number, hash or a special tag like "latest" or "finalized".
The `traceTypes` parameter allows filtering the traces based on their types.


Perms: read

Inputs:
```json
[
  "string value",
  [
    "string value"
  ]
]
```

Response:
```json
[
  {
    "output": "0x07",
    "stateDiff": "string value",
    "trace": [
      {
        "type": "string value",
        "error": "string value",
        "subtraces": 123,
        "traceAddress": [
          123
        ],
        "action": {},
        "result": {}
      }
    ],
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "vmTrace": "string value"
  }
]
```

### EthTraceTransaction
EthTraceTransaction returns an OpenEthereum-compatible trace of a specific transaction.
Maps to JSON-RPC method: "trace_transaction".


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
  {
    "type": "string value",
    "error": "string value",
    "subtraces": 123,
    "traceAddress": [
      123
    ],
    "action": {},
    "result": {},
    "blockHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "blockNumber": 9,
    "transactionHash": "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e",
    "transactionPosition": 123
  }
]
```

### EthUninstallFilter
EthUninstallFilter uninstalls a filter with given id.
Maps to JSON-RPC method: "eth_uninstallFilter".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response: `true`

### EthUnsubscribe
EthUnsubscribe unsubscribes from a websocket subscription.
Maps to JSON-RPC method: "eth_unsubscribe".


Perms: read

Inputs:
```json
[
  "0x37690cfec6c1bf4c3b9288c7a5d783e98731e90b0a4c177c2a374c7a9427355e"
]
```

Response: `true`

## Filecoin


### FilecoinAddressToEthAddress
FilecoinAddressToEthAddress converts any Filecoin address to an EthAddress.

This method supports all Filecoin address types:
- "f0" and "f4" addresses: Converted directly.
- "f1", "f2", and "f3" addresses: First converted to their corresponding "f0" ID address, then to an EthAddress.

Requirements:
- For "f1", "f2", and "f3" addresses, they must be instantiated on-chain, as "f0" ID addresses are only assigned to actors when they are created on-chain.
The simplest way to instantiate an address on chain is to send a transaction to the address.

Note on chain reorganizations:
"f0" ID addresses are not permanent and can be affected by chain reorganizations. To account for this,
the API includes a `blkNum` parameter, which specifies the block number that is used to determine the tipset state to use for converting an
"f1"/"f2"/"f3" address to an "f0" address. This parameter functions similarly to the `blkNum` parameter in the existing `EthGetBlockByNumber` API.
See https://docs.alchemy.com/reference/eth-getblockbynumber for more details.

Parameters:
- ctx: The context for the API call.
- filecoinAddress: The Filecoin address to convert.
- blkNum: The block number or state for the conversion. Defaults to "finalized" for maximum safety.
  Possible values: "pending", "latest", "finalized", "safe", or a specific block number represented as hex.

Returns:
- The corresponding EthAddress.
- An error if the conversion fails.


Perms: read

Inputs:
```json
[
  "Bw=="
]
```

Response: `"0x5cbeecf99d3fdb3f25e309cc264f240bb0664031"`

## Net


### NetListening
NetListening checks if the node is actively listening for network connections.
Maps to JSON-RPC method: "net_listening".


Perms: read

Inputs: `null`

Response: `true`

### NetVersion
NetVersion retrieves the current network ID.
Maps to JSON-RPC method: "net_version".


Perms: read

Inputs: `null`

Response: `"string value"`

## State
The State method group contains methods for interacting with the Filecoin
blockchain state, including actor information, addresses, and chain data.
These methods allow querying the blockchain state at any point in its history
using flexible TipSet selection mechanisms.


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

## Web3


### Web3ClientVersion
Web3ClientVersion returns the client version.
Maps to JSON-RPC method: "web3_clientVersion".


Perms: read

Inputs: `null`

Response: `"string value"`

