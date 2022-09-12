parse error:  open /home/vyzo/src/fvm/lotus/api/.#api_full.go: no such file or directory
# Groups
* [](#)
  * [Closing](#Closing)
  * [Discover](#Discover)
  * [Session](#Session)
  * [Shutdown](#Shutdown)
  * [Version](#Version)
* [Auth](#Auth)
  * [AuthNew](#AuthNew)
  * [AuthVerify](#AuthVerify)
* [Chain](#Chain)
  * [ChainBlockstoreInfo](#ChainBlockstoreInfo)
  * [ChainCheckBlockstore](#ChainCheckBlockstore)
  * [ChainDeleteObj](#ChainDeleteObj)
  * [ChainExport](#ChainExport)
  * [ChainGetBlock](#ChainGetBlock)
  * [ChainGetBlockMessages](#ChainGetBlockMessages)
  * [ChainGetGenesis](#ChainGetGenesis)
  * [ChainGetMessage](#ChainGetMessage)
  * [ChainGetMessagesInTipset](#ChainGetMessagesInTipset)
  * [ChainGetNode](#ChainGetNode)
  * [ChainGetParentMessages](#ChainGetParentMessages)
  * [ChainGetParentReceipts](#ChainGetParentReceipts)
  * [ChainGetPath](#ChainGetPath)
  * [ChainGetTipSet](#ChainGetTipSet)
  * [ChainGetTipSetAfterHeight](#ChainGetTipSetAfterHeight)
  * [ChainGetTipSetByHeight](#ChainGetTipSetByHeight)
  * [ChainHasObj](#ChainHasObj)
  * [ChainHead](#ChainHead)
  * [ChainNotify](#ChainNotify)
  * [ChainPutObj](#ChainPutObj)
  * [ChainReadObj](#ChainReadObj)
  * [ChainSetHead](#ChainSetHead)
  * [ChainStatObj](#ChainStatObj)
  * [ChainTipSetWeight](#ChainTipSetWeight)
* [Client](#Client)
  * [ClientCalcCommP](#ClientCalcCommP)
  * [ClientCancelDataTransfer](#ClientCancelDataTransfer)
  * [ClientCancelRetrievalDeal](#ClientCancelRetrievalDeal)
  * [ClientDataTransferUpdates](#ClientDataTransferUpdates)
  * [ClientDealPieceCID](#ClientDealPieceCID)
  * [ClientDealSize](#ClientDealSize)
  * [ClientExport](#ClientExport)
  * [ClientFindData](#ClientFindData)
  * [ClientGenCar](#ClientGenCar)
  * [ClientGetDealInfo](#ClientGetDealInfo)
  * [ClientGetDealStatus](#ClientGetDealStatus)
  * [ClientGetDealUpdates](#ClientGetDealUpdates)
  * [ClientGetRetrievalUpdates](#ClientGetRetrievalUpdates)
  * [ClientHasLocal](#ClientHasLocal)
  * [ClientImport](#ClientImport)
  * [ClientListDataTransfers](#ClientListDataTransfers)
  * [ClientListDeals](#ClientListDeals)
  * [ClientListImports](#ClientListImports)
  * [ClientListRetrievals](#ClientListRetrievals)
  * [ClientMinerQueryOffer](#ClientMinerQueryOffer)
  * [ClientQueryAsk](#ClientQueryAsk)
  * [ClientRemoveImport](#ClientRemoveImport)
  * [ClientRestartDataTransfer](#ClientRestartDataTransfer)
  * [ClientRetrieve](#ClientRetrieve)
  * [ClientRetrieveTryRestartInsufficientFunds](#ClientRetrieveTryRestartInsufficientFunds)
  * [ClientRetrieveWait](#ClientRetrieveWait)
  * [ClientStartDeal](#ClientStartDeal)
  * [ClientStatelessDeal](#ClientStatelessDeal)
* [Create](#Create)
  * [CreateBackup](#CreateBackup)
* [Eth](#Eth)
  * [EthAccounts](#EthAccounts)
  * [EthBlockNumber](#EthBlockNumber)
  * [EthChainId](#EthChainId)
  * [EthGasPrice](#EthGasPrice)
  * [EthGetBalance](#EthGetBalance)
  * [EthGetBlockByHash](#EthGetBlockByHash)
  * [EthGetBlockByNumber](#EthGetBlockByNumber)
  * [EthGetBlockTransactionCountByHash](#EthGetBlockTransactionCountByHash)
  * [EthGetBlockTransactionCountByNumber](#EthGetBlockTransactionCountByNumber)
  * [EthGetCode](#EthGetCode)
  * [EthGetStorageAt](#EthGetStorageAt)
  * [EthGetTransactionByBlockHashAndIndex](#EthGetTransactionByBlockHashAndIndex)
  * [EthGetTransactionByBlockNumberAndIndex](#EthGetTransactionByBlockNumberAndIndex)
  * [EthGetTransactionByHash](#EthGetTransactionByHash)
  * [EthGetTransactionCount](#EthGetTransactionCount)
  * [EthGetTransactionReceipt](#EthGetTransactionReceipt)
  * [EthMaxPriorityFeePerGas](#EthMaxPriorityFeePerGas)
  * [EthProtocolVersion](#EthProtocolVersion)
* [Gas](#Gas)
  * [GasEstimateFeeCap](#GasEstimateFeeCap)
  * [GasEstimateGasLimit](#GasEstimateGasLimit)
  * [GasEstimateGasPremium](#GasEstimateGasPremium)
  * [GasEstimateMessageGas](#GasEstimateMessageGas)
* [I](#I)
  * [ID](#ID)
* [Log](#Log)
  * [LogAlerts](#LogAlerts)
  * [LogList](#LogList)
  * [LogSetLevel](#LogSetLevel)
* [Market](#Market)
  * [MarketAddBalance](#MarketAddBalance)
  * [MarketGetReserved](#MarketGetReserved)
  * [MarketReleaseFunds](#MarketReleaseFunds)
  * [MarketReserveFunds](#MarketReserveFunds)
  * [MarketWithdraw](#MarketWithdraw)
* [Miner](#Miner)
  * [MinerCreateBlock](#MinerCreateBlock)
  * [MinerGetBaseInfo](#MinerGetBaseInfo)
* [Mpool](#Mpool)
  * [MpoolBatchPush](#MpoolBatchPush)
  * [MpoolBatchPushMessage](#MpoolBatchPushMessage)
  * [MpoolBatchPushUntrusted](#MpoolBatchPushUntrusted)
  * [MpoolCheckMessages](#MpoolCheckMessages)
  * [MpoolCheckPendingMessages](#MpoolCheckPendingMessages)
  * [MpoolCheckReplaceMessages](#MpoolCheckReplaceMessages)
  * [MpoolClear](#MpoolClear)
  * [MpoolGetConfig](#MpoolGetConfig)
  * [MpoolGetNonce](#MpoolGetNonce)
  * [MpoolPending](#MpoolPending)
  * [MpoolPush](#MpoolPush)
  * [MpoolPushMessage](#MpoolPushMessage)
  * [MpoolPushUntrusted](#MpoolPushUntrusted)
  * [MpoolSelect](#MpoolSelect)
  * [MpoolSetConfig](#MpoolSetConfig)
  * [MpoolSub](#MpoolSub)
* [Msig](#Msig)
  * [MsigAddApprove](#MsigAddApprove)
  * [MsigAddCancel](#MsigAddCancel)
  * [MsigAddPropose](#MsigAddPropose)
  * [MsigApprove](#MsigApprove)
  * [MsigApproveTxnHash](#MsigApproveTxnHash)
  * [MsigCancel](#MsigCancel)
  * [MsigCancelTxnHash](#MsigCancelTxnHash)
  * [MsigCreate](#MsigCreate)
  * [MsigGetAvailableBalance](#MsigGetAvailableBalance)
  * [MsigGetPending](#MsigGetPending)
  * [MsigGetVested](#MsigGetVested)
  * [MsigGetVestingSchedule](#MsigGetVestingSchedule)
  * [MsigPropose](#MsigPropose)
  * [MsigRemoveSigner](#MsigRemoveSigner)
  * [MsigSwapApprove](#MsigSwapApprove)
  * [MsigSwapCancel](#MsigSwapCancel)
  * [MsigSwapPropose](#MsigSwapPropose)
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
  * [NetListening](#NetListening)
  * [NetPeerInfo](#NetPeerInfo)
  * [NetPeers](#NetPeers)
  * [NetPing](#NetPing)
  * [NetProtectAdd](#NetProtectAdd)
  * [NetProtectList](#NetProtectList)
  * [NetProtectRemove](#NetProtectRemove)
  * [NetPubsubScores](#NetPubsubScores)
  * [NetSetLimit](#NetSetLimit)
  * [NetStat](#NetStat)
  * [NetVersion](#NetVersion)
* [Node](#Node)
  * [NodeStatus](#NodeStatus)
* [Paych](#Paych)
  * [PaychAllocateLane](#PaychAllocateLane)
  * [PaychAvailableFunds](#PaychAvailableFunds)
  * [PaychAvailableFundsByFromTo](#PaychAvailableFundsByFromTo)
  * [PaychCollect](#PaychCollect)
  * [PaychFund](#PaychFund)
  * [PaychGet](#PaychGet)
  * [PaychGetWaitReady](#PaychGetWaitReady)
  * [PaychList](#PaychList)
  * [PaychNewPayment](#PaychNewPayment)
  * [PaychSettle](#PaychSettle)
  * [PaychStatus](#PaychStatus)
  * [PaychVoucherAdd](#PaychVoucherAdd)
  * [PaychVoucherCheckSpendable](#PaychVoucherCheckSpendable)
  * [PaychVoucherCheckValid](#PaychVoucherCheckValid)
  * [PaychVoucherCreate](#PaychVoucherCreate)
  * [PaychVoucherList](#PaychVoucherList)
  * [PaychVoucherSubmit](#PaychVoucherSubmit)
* [State](#State)
  * [StateAccountKey](#StateAccountKey)
  * [StateActorCodeCIDs](#StateActorCodeCIDs)
  * [StateAllMinerFaults](#StateAllMinerFaults)
  * [StateCall](#StateCall)
  * [StateChangedActors](#StateChangedActors)
  * [StateCirculatingSupply](#StateCirculatingSupply)
  * [StateCompute](#StateCompute)
  * [StateComputeDataCID](#StateComputeDataCID)
  * [StateDealProviderCollateralBounds](#StateDealProviderCollateralBounds)
  * [StateDecodeParams](#StateDecodeParams)
  * [StateEncodeParams](#StateEncodeParams)
  * [StateGetActor](#StateGetActor)
  * [StateGetBeaconEntry](#StateGetBeaconEntry)
  * [StateGetNetworkParams](#StateGetNetworkParams)
  * [StateGetRandomnessFromBeacon](#StateGetRandomnessFromBeacon)
  * [StateGetRandomnessFromTickets](#StateGetRandomnessFromTickets)
  * [StateListActors](#StateListActors)
  * [StateListMessages](#StateListMessages)
  * [StateListMiners](#StateListMiners)
  * [StateLookupID](#StateLookupID)
  * [StateLookupRobustAddress](#StateLookupRobustAddress)
  * [StateMarketBalance](#StateMarketBalance)
  * [StateMarketDeals](#StateMarketDeals)
  * [StateMarketParticipants](#StateMarketParticipants)
  * [StateMarketStorageDeal](#StateMarketStorageDeal)
  * [StateMinerActiveSectors](#StateMinerActiveSectors)
  * [StateMinerAvailableBalance](#StateMinerAvailableBalance)
  * [StateMinerDeadlines](#StateMinerDeadlines)
  * [StateMinerFaults](#StateMinerFaults)
  * [StateMinerInfo](#StateMinerInfo)
  * [StateMinerInitialPledgeCollateral](#StateMinerInitialPledgeCollateral)
  * [StateMinerPartitions](#StateMinerPartitions)
  * [StateMinerPower](#StateMinerPower)
  * [StateMinerPreCommitDepositForPower](#StateMinerPreCommitDepositForPower)
  * [StateMinerProvingDeadline](#StateMinerProvingDeadline)
  * [StateMinerRecoveries](#StateMinerRecoveries)
  * [StateMinerSectorAllocated](#StateMinerSectorAllocated)
  * [StateMinerSectorCount](#StateMinerSectorCount)
  * [StateMinerSectors](#StateMinerSectors)
  * [StateNetworkName](#StateNetworkName)
  * [StateNetworkVersion](#StateNetworkVersion)
  * [StateReadState](#StateReadState)
  * [StateReplay](#StateReplay)
  * [StateSearchMsg](#StateSearchMsg)
  * [StateSectorExpiration](#StateSectorExpiration)
  * [StateSectorGetInfo](#StateSectorGetInfo)
  * [StateSectorPartition](#StateSectorPartition)
  * [StateSectorPreCommitInfo](#StateSectorPreCommitInfo)
  * [StateVMCirculatingSupplyInternal](#StateVMCirculatingSupplyInternal)
  * [StateVerifiedClientStatus](#StateVerifiedClientStatus)
  * [StateVerifiedRegistryRootKey](#StateVerifiedRegistryRootKey)
  * [StateVerifierStatus](#StateVerifierStatus)
  * [StateWaitMsg](#StateWaitMsg)
* [Sync](#Sync)
  * [SyncCheckBad](#SyncCheckBad)
  * [SyncCheckpoint](#SyncCheckpoint)
  * [SyncIncomingBlocks](#SyncIncomingBlocks)
  * [SyncMarkBad](#SyncMarkBad)
  * [SyncState](#SyncState)
  * [SyncSubmitBlock](#SyncSubmitBlock)
  * [SyncUnmarkAllBad](#SyncUnmarkAllBad)
  * [SyncUnmarkBad](#SyncUnmarkBad)
  * [SyncValidateTipset](#SyncValidateTipset)
* [Wallet](#Wallet)
  * [WalletBalance](#WalletBalance)
  * [WalletDefaultAddress](#WalletDefaultAddress)
  * [WalletDelete](#WalletDelete)
  * [WalletExport](#WalletExport)
  * [WalletHas](#WalletHas)
  * [WalletImport](#WalletImport)
  * [WalletList](#WalletList)
  * [WalletNew](#WalletNew)
  * [WalletSetDefault](#WalletSetDefault)
  * [WalletSign](#WalletSign)
  * [WalletSignMessage](#WalletSignMessage)
  * [WalletValidateAddress](#WalletValidateAddress)
  * [WalletVerify](#WalletVerify)
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
  "BlockDelay": 42
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

## Chain


### ChainBlockstoreInfo


Perms: read

Inputs: `null`

Response:
```json
{
  "abc": 123
}
```

### ChainCheckBlockstore


Perms: admin

Inputs: `null`

Response: `{}`

### ChainDeleteObj


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

### ChainExport


Perms: read

Inputs:
```json
[
  10101,
  true,
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

Response: `"Ynl0ZSBhcnJheQ=="`

### ChainGetBlock


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
  "Miner": "f01234",
  "Ticket": {
    "VRFProof": "Ynl0ZSBhcnJheQ=="
  },
  "ElectionProof": {
    "WinCount": 9,
    "VRFProof": "Ynl0ZSBhcnJheQ=="
  },
  "BeaconEntries": [
    {
      "Round": 42,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  ],
  "WinPoStProof": [
    {
      "PoStProof": 8,
      "ProofBytes": "Ynl0ZSBhcnJheQ=="
    }
  ],
  "Parents": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ],
  "ParentWeight": "0",
  "Height": 10101,
  "ParentStateRoot": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "ParentMessageReceipts": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Messages": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "BLSAggregate": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "Timestamp": 42,
  "BlockSig": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "ForkSignaling": 42,
  "ParentBaseFee": "0"
}
```

### ChainGetBlockMessages


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
  "BlsMessages": [
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
  "SecpkMessages": [
    {
      "Message": {
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
      "Signature": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "CID": {
        "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
      }
    }
  ],
  "Cids": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ]
}
```

### ChainGetGenesis


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

### ChainGetMessage


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
```

### ChainGetMessagesInTipset


Perms: read

Inputs:
```json
[
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
    "Cid": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Message": {
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
  }
]
```

### ChainGetNode


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
  "Cid": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Obj": {}
}
```

### ChainGetParentMessages


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
[
  {
    "Cid": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Message": {
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
  }
]
```

### ChainGetParentReceipts


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
[
  {
    "ExitCode": 0,
    "Return": "Ynl0ZSBhcnJheQ==",
    "GasUsed": 9
  }
]
```

### ChainGetPath


Perms: read

Inputs:
```json
[
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
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
    "Type": "string value",
    "Val": {
      "Cids": null,
      "Blocks": null,
      "Height": 0
    }
  }
]
```

### ChainGetTipSet


Perms: read

Inputs:
```json
[
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
{
  "Cids": null,
  "Blocks": null,
  "Height": 0
}
```

### ChainGetTipSetAfterHeight


Perms: read

Inputs:
```json
[
  10101,
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
{
  "Cids": null,
  "Blocks": null,
  "Height": 0
}
```

### ChainGetTipSetByHeight


Perms: read

Inputs:
```json
[
  10101,
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
{
  "Cids": null,
  "Blocks": null,
  "Height": 0
}
```

### ChainHasObj


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `true`

### ChainHead


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

### ChainNotify


Perms: read

Inputs: `null`

Response:
```json
[
  {
    "Type": "string value",
    "Val": {
      "Cids": null,
      "Blocks": null,
      "Height": 0
    }
  }
]
```

### ChainPutObj


Perms: admin

Inputs:
```json
[
  {}
]
```

Response: `{}`

### ChainReadObj


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `"Ynl0ZSBhcnJheQ=="`

### ChainSetHead


Perms: admin

Inputs:
```json
[
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

Response: `{}`

### ChainStatObj


Perms: read

Inputs:
```json
[
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
  "Size": 42,
  "Links": 42
}
```

### ChainTipSetWeight


Perms: read

Inputs:
```json
[
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

Response: `"0"`

## Client


### ClientCalcCommP


Perms: write

Inputs:
```json
[
  "string value"
]
```

Response:
```json
{
  "Root": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Size": 1024
}
```

### ClientCancelDataTransfer


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

### ClientCancelRetrievalDeal


Perms: write

Inputs:
```json
[
  5
]
```

Response: `{}`

### ClientDataTransferUpdates


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

### ClientDealPieceCID


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
  "PayloadSize": 9,
  "PieceSize": 1032,
  "PieceCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
}
```

### ClientDealSize


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
  "PayloadSize": 9,
  "PieceSize": 1032
}
```

### ClientExport


Perms: admin

Inputs:
```json
[
  {
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "DAGs": [
      {
        "DataSelector": "Links/21/Hash/Links/42/Hash",
        "ExportMerkleProof": true
      }
    ],
    "FromLocalCAR": "string value",
    "DealID": 5
  },
  {
    "Path": "string value",
    "IsCAR": true
  }
]
```

Response: `{}`

### ClientFindData


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  null
]
```

Response:
```json
[
  {
    "Err": "string value",
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Piece": null,
    "Size": 42,
    "MinPrice": "0",
    "UnsealPrice": "0",
    "PricePerByte": "0",
    "PaymentInterval": 42,
    "PaymentIntervalIncrease": 42,
    "Miner": "f01234",
    "MinerPeer": {
      "Address": "f01234",
      "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "PieceCID": null
    }
  }
]
```

### ClientGenCar


Perms: write

Inputs:
```json
[
  {
    "Path": "string value",
    "IsCAR": true
  },
  "string value"
]
```

Response: `{}`

### ClientGetDealInfo


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
  "ProposalCid": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "State": 42,
  "Message": "string value",
  "DealStages": {
    "Stages": [
      {
        "Name": "string value",
        "Description": "string value",
        "ExpectedDuration": "string value",
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
  },
  "Provider": "f01234",
  "DataRef": {
    "TransferType": "string value",
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PieceCid": null,
    "PieceSize": 1024,
    "RawBlockSize": 42
  },
  "PieceCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Size": 42,
  "PricePerEpoch": "0",
  "Duration": 42,
  "DealID": 5432,
  "CreationTime": "0001-01-01T00:00:00Z",
  "Verified": true,
  "TransferChannelID": {
    "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "ID": 3
  },
  "DataTransfer": {
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
}
```

### ClientGetDealStatus


Perms: read

Inputs:
```json
[
  42
]
```

Response: `"string value"`

### ClientGetDealUpdates


Perms: write

Inputs: `null`

Response:
```json
{
  "ProposalCid": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "State": 42,
  "Message": "string value",
  "DealStages": {
    "Stages": [
      {
        "Name": "string value",
        "Description": "string value",
        "ExpectedDuration": "string value",
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
  },
  "Provider": "f01234",
  "DataRef": {
    "TransferType": "string value",
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "PieceCid": null,
    "PieceSize": 1024,
    "RawBlockSize": 42
  },
  "PieceCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Size": 42,
  "PricePerEpoch": "0",
  "Duration": 42,
  "DealID": 5432,
  "CreationTime": "0001-01-01T00:00:00Z",
  "Verified": true,
  "TransferChannelID": {
    "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "ID": 3
  },
  "DataTransfer": {
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
}
```

### ClientGetRetrievalUpdates


Perms: write

Inputs: `null`

Response:
```json
{
  "PayloadCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "ID": 5,
  "PieceCID": null,
  "PricePerByte": "0",
  "UnsealPrice": "0",
  "Status": 0,
  "Message": "string value",
  "Provider": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "BytesReceived": 42,
  "BytesPaidFor": 42,
  "TotalPaid": "0",
  "TransferChannelID": {
    "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "ID": 3
  },
  "DataTransfer": {
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
  "Event": 5
}
```

### ClientHasLocal


Perms: write

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `true`

### ClientImport


Perms: admin

Inputs:
```json
[
  {
    "Path": "string value",
    "IsCAR": true
  }
]
```

Response:
```json
{
  "Root": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "ImportID": 50
}
```

### ClientListDataTransfers


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

### ClientListDeals


Perms: write

Inputs: `null`

Response:
```json
[
  {
    "ProposalCid": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "State": 42,
    "Message": "string value",
    "DealStages": {
      "Stages": [
        {
          "Name": "string value",
          "Description": "string value",
          "ExpectedDuration": "string value",
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
    },
    "Provider": "f01234",
    "DataRef": {
      "TransferType": "string value",
      "Root": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceCid": null,
      "PieceSize": 1024,
      "RawBlockSize": 42
    },
    "PieceCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Size": 42,
    "PricePerEpoch": "0",
    "Duration": 42,
    "DealID": 5432,
    "CreationTime": "0001-01-01T00:00:00Z",
    "Verified": true,
    "TransferChannelID": {
      "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "ID": 3
    },
    "DataTransfer": {
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
  }
]
```

### ClientListImports


Perms: write

Inputs: `null`

Response:
```json
[
  {
    "Key": 50,
    "Err": "string value",
    "Root": null,
    "Source": "string value",
    "FilePath": "string value",
    "CARPath": "string value"
  }
]
```

### ClientListRetrievals


Perms: write

Inputs: `null`

Response:
```json
[
  {
    "PayloadCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "ID": 5,
    "PieceCID": null,
    "PricePerByte": "0",
    "UnsealPrice": "0",
    "Status": 0,
    "Message": "string value",
    "Provider": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "BytesReceived": 42,
    "BytesPaidFor": 42,
    "TotalPaid": "0",
    "TransferChannelID": {
      "Initiator": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "Responder": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "ID": 3
    },
    "DataTransfer": {
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
    "Event": 5
  }
]
```

### ClientMinerQueryOffer


Perms: read

Inputs:
```json
[
  "f01234",
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  null
]
```

Response:
```json
{
  "Err": "string value",
  "Root": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Piece": null,
  "Size": 42,
  "MinPrice": "0",
  "UnsealPrice": "0",
  "PricePerByte": "0",
  "PaymentInterval": 42,
  "PaymentIntervalIncrease": 42,
  "Miner": "f01234",
  "MinerPeer": {
    "Address": "f01234",
    "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
    "PieceCID": null
  }
}
```

### ClientQueryAsk


Perms: read

Inputs:
```json
[
  "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "f01234"
]
```

Response:
```json
{
  "Response": {
    "Price": "0",
    "VerifiedPrice": "0",
    "MinPieceSize": 1032,
    "MaxPieceSize": 1032,
    "Miner": "f01234",
    "Timestamp": 10101,
    "Expiry": 10101,
    "SeqNo": 42
  },
  "DealProtocols": [
    "string value"
  ]
}
```

### ClientRemoveImport


Perms: admin

Inputs:
```json
[
  50
]
```

Response: `{}`

### ClientRestartDataTransfer


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

### ClientRetrieve


Perms: admin

Inputs:
```json
[
  {
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Piece": null,
    "DataSelector": "Links/21/Hash/Links/42/Hash",
    "Size": 42,
    "Total": "0",
    "UnsealPrice": "0",
    "PaymentInterval": 42,
    "PaymentIntervalIncrease": 42,
    "Client": "f01234",
    "Miner": "f01234",
    "MinerPeer": {
      "Address": "f01234",
      "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
      "PieceCID": null
    }
  }
]
```

Response:
```json
{
  "DealID": 5
}
```

### ClientRetrieveTryRestartInsufficientFunds


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### ClientRetrieveWait


Perms: admin

Inputs:
```json
[
  5
]
```

Response: `{}`

### ClientStartDeal


Perms: admin

Inputs:
```json
[
  {
    "Data": {
      "TransferType": "string value",
      "Root": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceCid": null,
      "PieceSize": 1024,
      "RawBlockSize": 42
    },
    "Wallet": "f01234",
    "Miner": "f01234",
    "EpochPrice": "0",
    "MinBlocksDuration": 42,
    "ProviderCollateral": "0",
    "DealStartEpoch": 10101,
    "FastRetrieval": true,
    "VerifiedDeal": true
  }
]
```

Response: `null`

### ClientStatelessDeal


Perms: write

Inputs:
```json
[
  {
    "Data": {
      "TransferType": "string value",
      "Root": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "PieceCid": null,
      "PieceSize": 1024,
      "RawBlockSize": 42
    },
    "Wallet": "f01234",
    "Miner": "f01234",
    "EpochPrice": "0",
    "MinBlocksDuration": 42,
    "ProviderCollateral": "0",
    "DealStartEpoch": 10101,
    "FastRetrieval": true,
    "VerifiedDeal": true
  }
]
```

Response: `null`

## Create


### CreateBackup


Perms: admin

Inputs:
```json
[
  "string value"
]
```

Response: `{}`

## Eth


### EthAccounts


Perms: read

Inputs: `null`

Response:
```json
[
  "0x0707070707070707070707070707070707070707"
]
```

### EthBlockNumber


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthChainId


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthGasPrice


Perms: 

Inputs: `null`

Response: `"0x5"`

### EthGetBalance


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707",
  "string value"
]
```

Response: `"0x0"`

### EthGetBlockByHash


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707070707070707070707070707",
  true
]
```

Response:
```json
{
  "parentHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "sha3Uncles": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "miner": "0x0707070707070707070707070707070707070707",
  "stateRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transactionsRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "receiptsRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "difficulty": "0x5",
  "number": "0x5",
  "gasLimit": "0x5",
  "gasUsed": "0x5",
  "timestamp": "0x5",
  "extraData": "Ynl0ZSBhcnJheQ==",
  "mixHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "nonce": "0x0707070707070707",
  "baseFeePerGas": "0x5",
  "transactions": {
    "chainId": "0x5",
    "nonce": 42,
    "hash": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "transacionIndex": "0x5",
    "from": "0x0707070707070707070707070707070707070707",
    "to": "0x0707070707070707070707070707070707070707",
    "value": "0x0",
    "type": "0x5",
    "input": "Ynl0ZSBhcnJheQ==",
    "gas": "0x5",
    "maxFeePerGas": "0x0",
    "maxPriorityFeePerGas": "0x0",
    "v": "0x0",
    "r": "0x0",
    "s": "0x0"
  }
}
```

### EthGetBlockByNumber


Perms: read

Inputs:
```json
[
  "0x5",
  true
]
```

Response:
```json
{
  "parentHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "sha3Uncles": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "miner": "0x0707070707070707070707070707070707070707",
  "stateRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transactionsRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "receiptsRoot": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "difficulty": "0x5",
  "number": "0x5",
  "gasLimit": "0x5",
  "gasUsed": "0x5",
  "timestamp": "0x5",
  "extraData": "Ynl0ZSBhcnJheQ==",
  "mixHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "nonce": "0x0707070707070707",
  "baseFeePerGas": "0x5",
  "transactions": {
    "chainId": "0x5",
    "nonce": 42,
    "hash": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
    "transacionIndex": "0x5",
    "from": "0x0707070707070707070707070707070707070707",
    "to": "0x0707070707070707070707070707070707070707",
    "value": "0x0",
    "type": "0x5",
    "input": "Ynl0ZSBhcnJheQ==",
    "gas": "0x5",
    "maxFeePerGas": "0x0",
    "maxPriorityFeePerGas": "0x0",
    "v": "0x0",
    "r": "0x0",
    "s": "0x0"
  }
}
```

### EthGetBlockTransactionCountByHash


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707070707070707070707070707"
]
```

Response: `"0x5"`

### EthGetBlockTransactionCountByNumber


Perms: read

Inputs:
```json
[
  "0x5"
]
```

Response: `"0x5"`

### EthGetCode


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707"
]
```

Response: `"string value"`

### EthGetStorageAt


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707",
  "0x5",
  "string value"
]
```

Response: `"string value"`

### EthGetTransactionByBlockHashAndIndex


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707070707070707070707070707",
  "0x5"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": 42,
  "hash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transacionIndex": "0x5",
  "from": "0x0707070707070707070707070707070707070707",
  "to": "0x0707070707070707070707070707070707070707",
  "value": "0x0",
  "type": "0x5",
  "input": "Ynl0ZSBhcnJheQ==",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "v": "0x0",
  "r": "0x0",
  "s": "0x0"
}
```

### EthGetTransactionByBlockNumberAndIndex


Perms: read

Inputs:
```json
[
  "0x5",
  "0x5"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": 42,
  "hash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transacionIndex": "0x5",
  "from": "0x0707070707070707070707070707070707070707",
  "to": "0x0707070707070707070707070707070707070707",
  "value": "0x0",
  "type": "0x5",
  "input": "Ynl0ZSBhcnJheQ==",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "v": "0x0",
  "r": "0x0",
  "s": "0x0"
}
```

### EthGetTransactionByHash


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707070707070707070707070707"
]
```

Response:
```json
{
  "chainId": "0x5",
  "nonce": 42,
  "hash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transacionIndex": "0x5",
  "from": "0x0707070707070707070707070707070707070707",
  "to": "0x0707070707070707070707070707070707070707",
  "value": "0x0",
  "type": "0x5",
  "input": "Ynl0ZSBhcnJheQ==",
  "gas": "0x5",
  "maxFeePerGas": "0x0",
  "maxPriorityFeePerGas": "0x0",
  "v": "0x0",
  "r": "0x0",
  "s": "0x0"
}
```

### EthGetTransactionCount


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707",
  "string value"
]
```

Response: `"0x5"`

### EthGetTransactionReceipt


Perms: read

Inputs:
```json
[
  "0x0707070707070707070707070707070707070707070707070707070707070707"
]
```

Response:
```json
{
  "transactionHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "transacionIndex": "0x5",
  "blockHash": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "blockNumber": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "from": "0x0707070707070707070707070707070707070707",
  "to": "0x0707070707070707070707070707070707070707",
  "root": "0x0707070707070707070707070707070707070707070707070707070707070707",
  "status": "0x5",
  "contractAddress": "0x5cbeecf99d3fdb3f25e309cc264f240bb0664031",
  "cumulativeGasUsed": "0x0",
  "gasUsed": "0x0",
  "effectiveGasPrice": "0x0"
}
```

### EthMaxPriorityFeePerGas


Perms: read

Inputs: `null`

Response: `"0x5"`

### EthProtocolVersion


Perms: read

Inputs: `null`

Response: `"0x5"`

## Gas


### GasEstimateFeeCap


Perms: read

Inputs:
```json
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
  },
  9,
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

Response: `"0"`

### GasEstimateGasLimit


Perms: read

Inputs:
```json
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
  },
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

Response: `9`

### GasEstimateGasPremium


Perms: read

Inputs:
```json
[
  42,
  "f01234",
  9,
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

Response: `"0"`

### GasEstimateMessageGas


Perms: read

Inputs:
```json
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
  },
  {
    "MaxFee": "0"
  },
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
```

## I


### ID


Perms: read

Inputs: `null`

Response: `"12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"`

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


### MarketAddBalance


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MarketGetReserved


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response: `"0"`

### MarketReleaseFunds


Perms: sign

Inputs:
```json
[
  "f01234",
  "0"
]
```

Response: `{}`

### MarketReserveFunds


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MarketWithdraw


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

## Miner


### MinerCreateBlock


Perms: write

Inputs:
```json
[
  {
    "Miner": "f01234",
    "Parents": [
      {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      {
        "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
      }
    ],
    "Ticket": {
      "VRFProof": "Ynl0ZSBhcnJheQ=="
    },
    "Eproof": {
      "WinCount": 9,
      "VRFProof": "Ynl0ZSBhcnJheQ=="
    },
    "BeaconValues": [
      {
        "Round": 42,
        "Data": "Ynl0ZSBhcnJheQ=="
      }
    ],
    "Messages": [
      {
        "Message": {
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
        "Signature": {
          "Type": 2,
          "Data": "Ynl0ZSBhcnJheQ=="
        },
        "CID": {
          "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
        }
      }
    ],
    "Epoch": 10101,
    "Timestamp": 42,
    "WinningPoStProof": [
      {
        "PoStProof": 8,
        "ProofBytes": "Ynl0ZSBhcnJheQ=="
      }
    ]
  }
]
```

Response:
```json
{
  "Header": {
    "Miner": "f01234",
    "Ticket": {
      "VRFProof": "Ynl0ZSBhcnJheQ=="
    },
    "ElectionProof": {
      "WinCount": 9,
      "VRFProof": "Ynl0ZSBhcnJheQ=="
    },
    "BeaconEntries": [
      {
        "Round": 42,
        "Data": "Ynl0ZSBhcnJheQ=="
      }
    ],
    "WinPoStProof": [
      {
        "PoStProof": 8,
        "ProofBytes": "Ynl0ZSBhcnJheQ=="
      }
    ],
    "Parents": [
      {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    ],
    "ParentWeight": "0",
    "Height": 10101,
    "ParentStateRoot": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "ParentMessageReceipts": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Messages": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "BLSAggregate": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Timestamp": 42,
    "BlockSig": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "ForkSignaling": 42,
    "ParentBaseFee": "0"
  },
  "BlsMessages": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ],
  "SecpkMessages": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ]
}
```

### MinerGetBaseInfo


Perms: read

Inputs:
```json
[
  "f01234",
  10101,
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
{
  "MinerPower": "0",
  "NetworkPower": "0",
  "Sectors": [
    {
      "SealProof": 8,
      "SectorNumber": 9,
      "SectorKey": null,
      "SealedCID": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    }
  ],
  "WorkerKey": "f01234",
  "SectorSize": 34359738368,
  "PrevBeaconEntry": {
    "Round": 42,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "BeaconEntries": [
    {
      "Round": 42,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  ],
  "EligibleForMining": true
}
```

## Mpool


### MpoolBatchPush


Perms: write

Inputs:
```json
[
  [
    {
      "Message": {
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
      "Signature": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "CID": {
        "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
      }
    }
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

### MpoolBatchPushMessage


Perms: sign

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
    "MaxFee": "0"
  }
]
```

Response:
```json
[
  {
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
]
```

### MpoolBatchPushUntrusted


Perms: write

Inputs:
```json
[
  [
    {
      "Message": {
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
      "Signature": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "CID": {
        "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
      }
    }
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

### MpoolCheckMessages


Perms: read

Inputs:
```json
[
  [
    {
      "Message": {
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
      "ValidNonce": true
    }
  ]
]
```

Response:
```json
[
  [
    {
      "Cid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Code": 0,
      "OK": true,
      "Err": "string value",
      "Hint": {
        "abc": 123
      }
    }
  ]
]
```

### MpoolCheckPendingMessages


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
[
  [
    {
      "Cid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Code": 0,
      "OK": true,
      "Err": "string value",
      "Hint": {
        "abc": 123
      }
    }
  ]
]
```

### MpoolCheckReplaceMessages


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
  ]
]
```

Response:
```json
[
  [
    {
      "Cid": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Code": 0,
      "OK": true,
      "Err": "string value",
      "Hint": {
        "abc": 123
      }
    }
  ]
]
```

### MpoolClear


Perms: write

Inputs:
```json
[
  true
]
```

Response: `{}`

### MpoolGetConfig


Perms: read

Inputs: `null`

Response:
```json
{
  "PriorityAddrs": [
    "f01234"
  ],
  "SizeLimitHigh": 123,
  "SizeLimitLow": 123,
  "ReplaceByFeeRatio": 12.3,
  "PruneCooldown": 60000000000,
  "GasLimitOverestimation": 12.3
}
```

### MpoolGetNonce


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response: `42`

### MpoolPending


Perms: read

Inputs:
```json
[
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
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
]
```

### MpoolPush


Perms: write

Inputs:
```json
[
  {
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MpoolPushMessage


Perms: sign

Inputs:
```json
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
  },
  {
    "MaxFee": "0"
  }
]
```

Response:
```json
{
  "Message": {
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
  "Signature": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "CID": {
    "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
  }
}
```

### MpoolPushUntrusted


Perms: write

Inputs:
```json
[
  {
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MpoolSelect


Perms: read

Inputs:
```json
[
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  12.3
]
```

Response:
```json
[
  {
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
]
```

### MpoolSetConfig


Perms: admin

Inputs:
```json
[
  {
    "PriorityAddrs": [
      "f01234"
    ],
    "SizeLimitHigh": 123,
    "SizeLimitLow": 123,
    "ReplaceByFeeRatio": 12.3,
    "PruneCooldown": 60000000000,
    "GasLimitOverestimation": 12.3
  }
]
```

Response: `{}`

### MpoolSub


Perms: read

Inputs: `null`

Response:
```json
{
  "Type": 0,
  "Message": {
    "Message": {
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
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "CID": {
      "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
    }
  }
}
```

## Msig


### MsigAddApprove


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  42,
  "f01234",
  "f01234",
  true
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigAddCancel


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  42,
  "f01234",
  true
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigAddPropose


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "f01234",
  true
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigApprove


Perms: sign

Inputs:
```json
[
  "f01234",
  42,
  "f01234"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigApproveTxnHash


Perms: sign

Inputs:
```json
[
  "f01234",
  42,
  "f01234",
  "f01234",
  "0",
  "f01234",
  42,
  "Ynl0ZSBhcnJheQ=="
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigCancel


Perms: sign

Inputs:
```json
[
  "f01234",
  42,
  "f01234"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigCancelTxnHash


Perms: sign

Inputs:
```json
[
  "f01234",
  42,
  "f01234",
  "0",
  "f01234",
  42,
  "Ynl0ZSBhcnJheQ=="
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigCreate


Perms: sign

Inputs:
```json
[
  42,
  [
    "f01234"
  ],
  10101,
  "0",
  "f01234",
  "0"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigGetAvailableBalance


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"0"`

### MsigGetPending


Perms: read

Inputs:
```json
[
  "f01234",
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
    "ID": 9,
    "To": "f01234",
    "Value": "0",
    "Method": 1,
    "Params": "Ynl0ZSBhcnJheQ==",
    "Approved": [
      "f01234"
    ]
  }
]
```

### MsigGetVested


Perms: read

Inputs:
```json
[
  "f01234",
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
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

Response: `"0"`

### MsigGetVestingSchedule


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "InitialBalance": "0",
  "StartEpoch": 10101,
  "UnlockDuration": 10101
}
```

### MsigPropose


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0",
  "f01234",
  42,
  "Ynl0ZSBhcnJheQ=="
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigRemoveSigner


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "f01234",
  true
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigSwapApprove


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  42,
  "f01234",
  "f01234",
  "f01234"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigSwapCancel


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  42,
  "f01234",
  "f01234"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
}
```

### MsigSwapPropose


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "f01234",
  "f01234"
]
```

Response:
```json
{
  "Message": {
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
  "ValidNonce": true
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

### NetListening


Perms: read

Inputs: `null`

Response: `true`

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

### NetVersion


Perms: read

Inputs: `null`

Response: `"string value"`

## Node


### NodeStatus


Perms: read

Inputs:
```json
[
  true
]
```

Response:
```json
{
  "SyncStatus": {
    "Epoch": 42,
    "Behind": 42
  },
  "PeerStatus": {
    "PeersToPublishMsgs": 123,
    "PeersToPublishBlocks": 123
  },
  "ChainStatus": {
    "BlocksPerTipsetLast100": 12.3,
    "BlocksPerTipsetLastFinality": 12.3
  }
}
```

## Paych


### PaychAllocateLane


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response: `42`

### PaychAvailableFunds


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
{
  "Channel": "\u003cempty\u003e",
  "From": "f01234",
  "To": "f01234",
  "ConfirmedAmt": "0",
  "PendingAmt": "0",
  "NonReservedAmt": "0",
  "PendingAvailableAmt": "0",
  "PendingWaitSentinel": null,
  "QueuedAmt": "0",
  "VoucherReedeemedAmt": "0"
}
```

### PaychAvailableFundsByFromTo


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234"
]
```

Response:
```json
{
  "Channel": "\u003cempty\u003e",
  "From": "f01234",
  "To": "f01234",
  "ConfirmedAmt": "0",
  "PendingAmt": "0",
  "NonReservedAmt": "0",
  "PendingAvailableAmt": "0",
  "PendingWaitSentinel": null,
  "QueuedAmt": "0",
  "VoucherReedeemedAmt": "0"
}
```

### PaychCollect


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### PaychFund


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0"
]
```

Response:
```json
{
  "Channel": "f01234",
  "WaitSentinel": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
}
```

### PaychGet


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  "0",
  {
    "OffChain": true
  }
]
```

Response:
```json
{
  "Channel": "f01234",
  "WaitSentinel": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
}
```

### PaychGetWaitReady


Perms: sign

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `"f01234"`

### PaychList


Perms: read

Inputs: `null`

Response:
```json
[
  "f01234"
]
```

### PaychNewPayment


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  [
    {
      "Amount": "0",
      "TimeLockMin": 10101,
      "TimeLockMax": 10101,
      "MinSettle": 10101,
      "Extra": {
        "Actor": "f01234",
        "Method": 1,
        "Data": "Ynl0ZSBhcnJheQ=="
      }
    }
  ]
]
```

Response:
```json
{
  "Channel": "f01234",
  "WaitSentinel": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Vouchers": [
    {
      "ChannelAddr": "f01234",
      "TimeLockMin": 10101,
      "TimeLockMax": 10101,
      "SecretHash": "Ynl0ZSBhcnJheQ==",
      "Extra": {
        "Actor": "f01234",
        "Method": 1,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "Lane": 42,
      "Nonce": 42,
      "Amount": "0",
      "MinSettleHeight": 10101,
      "Merges": [
        {
          "Lane": 42,
          "Nonce": 42
        }
      ],
      "Signature": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      }
    }
  ]
}
```

### PaychSettle


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### PaychStatus


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
{
  "ControlAddr": "f01234",
  "Direction": 1
}
```

### PaychVoucherAdd


Perms: write

Inputs:
```json
[
  "f01234",
  {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  },
  "Ynl0ZSBhcnJheQ==",
  "0"
]
```

Response: `"0"`

### PaychVoucherCheckSpendable


Perms: read

Inputs:
```json
[
  "f01234",
  {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  },
  "Ynl0ZSBhcnJheQ==",
  "Ynl0ZSBhcnJheQ=="
]
```

Response: `true`

### PaychVoucherCheckValid


Perms: read

Inputs:
```json
[
  "f01234",
  {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  }
]
```

Response: `{}`

### PaychVoucherCreate


Perms: sign

Inputs:
```json
[
  "f01234",
  "0",
  42
]
```

Response:
```json
{
  "Voucher": {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  },
  "Shortfall": "0"
}
```

### PaychVoucherList


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
[
  {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  }
]
```

### PaychVoucherSubmit


Perms: sign

Inputs:
```json
[
  "f01234",
  {
    "ChannelAddr": "f01234",
    "TimeLockMin": 10101,
    "TimeLockMax": 10101,
    "SecretHash": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": [
      {
        "Lane": 42,
        "Nonce": 42
      }
    ],
    "Signature": {
      "Type": 2,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  },
  "Ynl0ZSBhcnJheQ==",
  "Ynl0ZSBhcnJheQ=="
]
```

Response:
```json
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

## State


### StateAccountKey


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"f01234"`

### StateActorCodeCIDs


Perms: read

Inputs:
```json
[
  16
]
```

Response: `{}`

### StateAllMinerFaults


Perms: read

Inputs:
```json
[
  10101,
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
    "Miner": "f01234",
    "Epoch": 10101
  }
]
```

### StateCall


Perms: read

Inputs:
```json
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
  },
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
    "GasUsed": 9
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
      "GasUsed": 9
    },
    "Error": "string value",
    "Duration": 60000000000,
    "GasCharges": [
      {
        "Name": "string value",
        "loc": [
          {
            "File": "string value",
            "Line": 123,
            "Function": "string value"
          }
        ],
        "tg": 9,
        "cg": 9,
        "sg": 9,
        "vtg": 9,
        "vcg": 9,
        "vsg": 9,
        "tt": 60000000000,
        "ex": {}
      }
    ],
    "Subcalls": [
      {
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
          "GasUsed": 9
        },
        "Error": "string value",
        "Duration": 60000000000,
        "GasCharges": [
          {
            "Name": "string value",
            "loc": [
              {
                "File": "string value",
                "Line": 123,
                "Function": "string value"
              }
            ],
            "tg": 9,
            "cg": 9,
            "sg": 9,
            "vtg": 9,
            "vcg": 9,
            "vsg": 9,
            "tt": 60000000000,
            "ex": {}
          }
        ],
        "Subcalls": null
      }
    ]
  },
  "Error": "string value",
  "Duration": 60000000000
}
```

### StateChangedActors


Perms: read

Inputs:
```json
[
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
  "t01236": {
    "Code": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Head": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Nonce": 42,
    "Balance": "0"
  }
}
```

### StateCirculatingSupply


Perms: read

Inputs:
```json
[
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

Response: `"0"`

### StateCompute


Perms: read

Inputs:
```json
[
  10101,
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
        "GasUsed": 9
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
          "GasUsed": 9
        },
        "Error": "string value",
        "Duration": 60000000000,
        "GasCharges": [
          {
            "Name": "string value",
            "loc": [
              {
                "File": "string value",
                "Line": 123,
                "Function": "string value"
              }
            ],
            "tg": 9,
            "cg": 9,
            "sg": 9,
            "vtg": 9,
            "vcg": 9,
            "vsg": 9,
            "tt": 60000000000,
            "ex": {}
          }
        ],
        "Subcalls": [
          {
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
              "GasUsed": 9
            },
            "Error": "string value",
            "Duration": 60000000000,
            "GasCharges": [
              {
                "Name": "string value",
                "loc": [
                  {
                    "File": "string value",
                    "Line": 123,
                    "Function": "string value"
                  }
                ],
                "tg": 9,
                "cg": 9,
                "sg": 9,
                "vtg": 9,
                "vcg": 9,
                "vsg": 9,
                "tt": 60000000000,
                "ex": {}
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

### StateComputeDataCID


Perms: read

Inputs:
```json
[
  "f01234",
  8,
  [
    5432
  ],
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
{
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### StateDealProviderCollateralBounds


Perms: read

Inputs:
```json
[
  1032,
  true,
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
{
  "Min": "0",
  "Max": "0"
}
```

### StateDecodeParams


Perms: read

Inputs:
```json
[
  "f01234",
  1,
  "Ynl0ZSBhcnJheQ==",
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

Response: `{}`

### StateEncodeParams


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  1,
  "json raw message"
]
```

Response: `"Ynl0ZSBhcnJheQ=="`

### StateGetActor


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "Code": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Head": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Nonce": 42,
  "Balance": "0"
}
```

### StateGetBeaconEntry


Perms: read

Inputs:
```json
[
  10101
]
```

Response:
```json
{
  "Round": 42,
  "Data": "Ynl0ZSBhcnJheQ=="
}
```

### StateGetNetworkParams


Perms: read

Inputs: `null`

Response:
```json
{
  "NetworkName": "lotus",
  "BlockDelaySecs": 42,
  "ConsensusMinerMinPower": "0",
  "SupportedProofTypes": [
    8
  ],
  "PreCommitChallengeDelay": 10101,
  "ForkUpgradeParams": {
    "UpgradeSmokeHeight": 10101,
    "UpgradeBreezeHeight": 10101,
    "UpgradeIgnitionHeight": 10101,
    "UpgradeLiftoffHeight": 10101,
    "UpgradeAssemblyHeight": 10101,
    "UpgradeRefuelHeight": 10101,
    "UpgradeTapeHeight": 10101,
    "UpgradeKumquatHeight": 10101,
    "UpgradePriceListOopsHeight": 10101,
    "BreezeGasTampingDuration": 10101,
    "UpgradeCalicoHeight": 10101,
    "UpgradePersianHeight": 10101,
    "UpgradeOrangeHeight": 10101,
    "UpgradeClausHeight": 10101,
    "UpgradeTrustHeight": 10101,
    "UpgradeNorwegianHeight": 10101,
    "UpgradeTurboHeight": 10101,
    "UpgradeHyperdriveHeight": 10101,
    "UpgradeChocolateHeight": 10101,
    "UpgradeOhSnapHeight": 10101
  }
}
```

### StateGetRandomnessFromBeacon


Perms: read

Inputs:
```json
[
  2,
  10101,
  "Ynl0ZSBhcnJheQ==",
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

Response: `"Bw=="`

### StateGetRandomnessFromTickets


Perms: read

Inputs:
```json
[
  2,
  10101,
  "Ynl0ZSBhcnJheQ==",
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

Response: `"Bw=="`

### StateListActors


Perms: read

Inputs:
```json
[
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
  "f01234"
]
```

### StateListMessages


Perms: read

Inputs:
```json
[
  {
    "To": "f01234",
    "From": "f01234"
  },
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  10101
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

### StateListMiners


Perms: read

Inputs:
```json
[
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
  "f01234"
]
```

### StateLookupID


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"f01234"`

### StateLookupRobustAddress


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"f01234"`

### StateMarketBalance


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "Escrow": "0",
  "Locked": "0"
}
```

### StateMarketDeals


Perms: read

Inputs:
```json
[
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
{
  "t026363": {
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
}
```

### StateMarketParticipants


Perms: read

Inputs:
```json
[
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
{
  "t026363": {
    "Escrow": "0",
    "Locked": "0"
  }
}
```

### StateMarketStorageDeal


Perms: read

Inputs:
```json
[
  5432,
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
```

### StateMinerActiveSectors


Perms: read

Inputs:
```json
[
  "f01234",
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
    "SectorNumber": 9,
    "SealProof": 8,
    "SealedCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "DealIDs": [
      5432
    ],
    "Activation": 10101,
    "Expiration": 10101,
    "DealWeight": "0",
    "VerifiedDealWeight": "0",
    "InitialPledge": "0",
    "ExpectedDayReward": "0",
    "ExpectedStoragePledge": "0",
    "ReplacedSectorAge": 10101,
    "ReplacedDayReward": "0",
    "SectorKeyCID": null
  }
]
```

### StateMinerAvailableBalance


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"0"`

### StateMinerDeadlines


Perms: read

Inputs:
```json
[
  "f01234",
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
    "PostSubmissions": [
      5,
      1
    ],
    "DisputableProofCount": 42
  }
]
```

### StateMinerFaults


Perms: read

Inputs:
```json
[
  "f01234",
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
  5,
  1
]
```

### StateMinerInfo


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "Owner": "f01234",
  "Worker": "f01234",
  "NewWorker": "f01234",
  "ControlAddresses": [
    "f01234"
  ],
  "WorkerChangeEpoch": 10101,
  "PeerId": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Multiaddrs": [
    "Ynl0ZSBhcnJheQ=="
  ],
  "WindowPoStProofType": 8,
  "SectorSize": 34359738368,
  "WindowPoStPartitionSectors": 42,
  "ConsensusFaultElapsed": 10101
}
```

### StateMinerInitialPledgeCollateral


Perms: read

Inputs:
```json
[
  "f01234",
  {
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
    "ReplaceCapacity": true,
    "ReplaceSectorDeadline": 42,
    "ReplaceSectorPartition": 42,
    "ReplaceSectorNumber": 9
  },
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

Response: `"0"`

### StateMinerPartitions


Perms: read

Inputs:
```json
[
  "f01234",
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
    "AllSectors": [
      5,
      1
    ],
    "FaultySectors": [
      5,
      1
    ],
    "RecoveringSectors": [
      5,
      1
    ],
    "LiveSectors": [
      5,
      1
    ],
    "ActiveSectors": [
      5,
      1
    ]
  }
]
```

### StateMinerPower


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "MinerPower": {
    "RawBytePower": "0",
    "QualityAdjPower": "0"
  },
  "TotalPower": {
    "RawBytePower": "0",
    "QualityAdjPower": "0"
  },
  "HasMinPower": true
}
```

### StateMinerPreCommitDepositForPower


Perms: read

Inputs:
```json
[
  "f01234",
  {
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
    "ReplaceCapacity": true,
    "ReplaceSectorDeadline": 42,
    "ReplaceSectorPartition": 42,
    "ReplaceSectorNumber": 9
  },
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

Response: `"0"`

### StateMinerProvingDeadline


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "CurrentEpoch": 10101,
  "PeriodStart": 10101,
  "Index": 42,
  "Open": 10101,
  "Close": 10101,
  "Challenge": 10101,
  "FaultCutoff": 10101,
  "WPoStPeriodDeadlines": 42,
  "WPoStProvingPeriod": 10101,
  "WPoStChallengeWindow": 10101,
  "WPoStChallengeLookback": 10101,
  "FaultDeclarationCutoff": 10101
}
```

### StateMinerRecoveries


Perms: read

Inputs:
```json
[
  "f01234",
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
  5,
  1
]
```

### StateMinerSectorAllocated


Perms: read

Inputs:
```json
[
  "f01234",
  9,
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

Response: `true`

### StateMinerSectorCount


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "Live": 42,
  "Active": 42,
  "Faulty": 42
}
```

### StateMinerSectors


Perms: read

Inputs:
```json
[
  "f01234",
  [
    0
  ],
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
    "SectorNumber": 9,
    "SealProof": 8,
    "SealedCID": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "DealIDs": [
      5432
    ],
    "Activation": 10101,
    "Expiration": 10101,
    "DealWeight": "0",
    "VerifiedDealWeight": "0",
    "InitialPledge": "0",
    "ExpectedDayReward": "0",
    "ExpectedStoragePledge": "0",
    "ReplacedSectorAge": 10101,
    "ReplacedDayReward": "0",
    "SectorKeyCID": null
  }
]
```

### StateNetworkName


Perms: read

Inputs: `null`

Response: `"lotus"`

### StateNetworkVersion


Perms: read

Inputs:
```json
[
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

Response: `16`

### StateReadState


Perms: read

Inputs:
```json
[
  "f01234",
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
{
  "Balance": "0",
  "Code": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "State": {}
}
```

### StateReplay


Perms: read

Inputs:
```json
[
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response:
```json
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
    "GasUsed": 9
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
      "GasUsed": 9
    },
    "Error": "string value",
    "Duration": 60000000000,
    "GasCharges": [
      {
        "Name": "string value",
        "loc": [
          {
            "File": "string value",
            "Line": 123,
            "Function": "string value"
          }
        ],
        "tg": 9,
        "cg": 9,
        "sg": 9,
        "vtg": 9,
        "vcg": 9,
        "vsg": 9,
        "tt": 60000000000,
        "ex": {}
      }
    ],
    "Subcalls": [
      {
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
          "GasUsed": 9
        },
        "Error": "string value",
        "Duration": 60000000000,
        "GasCharges": [
          {
            "Name": "string value",
            "loc": [
              {
                "File": "string value",
                "Line": 123,
                "Function": "string value"
              }
            ],
            "tg": 9,
            "cg": 9,
            "sg": 9,
            "vtg": 9,
            "vcg": 9,
            "vsg": 9,
            "tt": 60000000000,
            "ex": {}
          }
        ],
        "Subcalls": null
      }
    ]
  },
  "Error": "string value",
  "Duration": 60000000000
}
```

### StateSearchMsg


Perms: read

Inputs:
```json
[
  [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  10101,
  true
]
```

Response:
```json
{
  "Message": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Receipt": {
    "ExitCode": 0,
    "Return": "Ynl0ZSBhcnJheQ==",
    "GasUsed": 9
  },
  "ReturnDec": {},
  "TipSet": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  "Height": 10101
}
```

### StateSectorExpiration


Perms: read

Inputs:
```json
[
  "f01234",
  9,
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
{
  "OnTime": 10101,
  "Early": 10101
}
```

### StateSectorGetInfo


Perms: read

Inputs:
```json
[
  "f01234",
  9,
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
{
  "SectorNumber": 9,
  "SealProof": 8,
  "SealedCID": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "DealIDs": [
    5432
  ],
  "Activation": 10101,
  "Expiration": 10101,
  "DealWeight": "0",
  "VerifiedDealWeight": "0",
  "InitialPledge": "0",
  "ExpectedDayReward": "0",
  "ExpectedStoragePledge": "0",
  "ReplacedSectorAge": 10101,
  "ReplacedDayReward": "0",
  "SectorKeyCID": null
}
```

### StateSectorPartition


Perms: read

Inputs:
```json
[
  "f01234",
  9,
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
{
  "Deadline": 42,
  "Partition": 42
}
```

### StateSectorPreCommitInfo


Perms: read

Inputs:
```json
[
  "f01234",
  9,
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
{
  "Info": {
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
    "ReplaceCapacity": true,
    "ReplaceSectorDeadline": 42,
    "ReplaceSectorPartition": 42,
    "ReplaceSectorNumber": 9
  },
  "PreCommitDeposit": "0",
  "PreCommitEpoch": 10101,
  "DealWeight": "0",
  "VerifiedDealWeight": "0"
}
```

### StateVMCirculatingSupplyInternal


Perms: read

Inputs:
```json
[
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
{
  "FilVested": "0",
  "FilMined": "0",
  "FilBurnt": "0",
  "FilLocked": "0",
  "FilCirculating": "0",
  "FilReserveDisbursed": "0"
}
```

### StateVerifiedClientStatus


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"0"`

### StateVerifiedRegistryRootKey


Perms: read

Inputs:
```json
[
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

Response: `"f01234"`

### StateVerifierStatus


Perms: read

Inputs:
```json
[
  "f01234",
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

Response: `"0"`

### StateWaitMsg


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  42,
  10101,
  true
]
```

Response:
```json
{
  "Message": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Receipt": {
    "ExitCode": 0,
    "Return": "Ynl0ZSBhcnJheQ==",
    "GasUsed": 9
  },
  "ReturnDec": {},
  "TipSet": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    {
      "/": "bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve"
    }
  ],
  "Height": 10101
}
```

## Sync


### SyncCheckBad


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `"string value"`

### SyncCheckpoint


Perms: admin

Inputs:
```json
[
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

Response: `{}`

### SyncIncomingBlocks


Perms: read

Inputs: `null`

Response:
```json
{
  "Miner": "f01234",
  "Ticket": {
    "VRFProof": "Ynl0ZSBhcnJheQ=="
  },
  "ElectionProof": {
    "WinCount": 9,
    "VRFProof": "Ynl0ZSBhcnJheQ=="
  },
  "BeaconEntries": [
    {
      "Round": 42,
      "Data": "Ynl0ZSBhcnJheQ=="
    }
  ],
  "WinPoStProof": [
    {
      "PoStProof": 8,
      "ProofBytes": "Ynl0ZSBhcnJheQ=="
    }
  ],
  "Parents": [
    {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    }
  ],
  "ParentWeight": "0",
  "Height": 10101,
  "ParentStateRoot": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "ParentMessageReceipts": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Messages": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "BLSAggregate": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "Timestamp": 42,
  "BlockSig": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "ForkSignaling": 42,
  "ParentBaseFee": "0"
}
```

### SyncMarkBad


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

### SyncState


Perms: read

Inputs: `null`

Response:
```json
{
  "ActiveSyncs": [
    {
      "WorkerID": 42,
      "Base": {
        "Cids": null,
        "Blocks": null,
        "Height": 0
      },
      "Target": {
        "Cids": null,
        "Blocks": null,
        "Height": 0
      },
      "Stage": 1,
      "Height": 10101,
      "Start": "0001-01-01T00:00:00Z",
      "End": "0001-01-01T00:00:00Z",
      "Message": "string value"
    }
  ],
  "VMApplied": 42
}
```

### SyncSubmitBlock


Perms: write

Inputs:
```json
[
  {
    "Header": {
      "Miner": "f01234",
      "Ticket": {
        "VRFProof": "Ynl0ZSBhcnJheQ=="
      },
      "ElectionProof": {
        "WinCount": 9,
        "VRFProof": "Ynl0ZSBhcnJheQ=="
      },
      "BeaconEntries": [
        {
          "Round": 42,
          "Data": "Ynl0ZSBhcnJheQ=="
        }
      ],
      "WinPoStProof": [
        {
          "PoStProof": 8,
          "ProofBytes": "Ynl0ZSBhcnJheQ=="
        }
      ],
      "Parents": [
        {
          "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
        }
      ],
      "ParentWeight": "0",
      "Height": 10101,
      "ParentStateRoot": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "ParentMessageReceipts": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "Messages": {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      },
      "BLSAggregate": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "Timestamp": 42,
      "BlockSig": {
        "Type": 2,
        "Data": "Ynl0ZSBhcnJheQ=="
      },
      "ForkSignaling": 42,
      "ParentBaseFee": "0"
    },
    "BlsMessages": [
      {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    ],
    "SecpkMessages": [
      {
        "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
      }
    ]
  }
]
```

Response: `{}`

### SyncUnmarkAllBad


Perms: admin

Inputs: `null`

Response: `{}`

### SyncUnmarkBad


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

### SyncValidateTipset


Perms: read

Inputs:
```json
[
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

Response: `true`

## Wallet


### WalletBalance


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response: `"0"`

### WalletDefaultAddress


Perms: write

Inputs: `null`

Response: `"f01234"`

### WalletDelete


Perms: admin

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### WalletExport


Perms: admin

Inputs:
```json
[
  "f01234"
]
```

Response:
```json
{
  "Type": "bls",
  "PrivateKey": "Ynl0ZSBhcnJheQ=="
}
```

### WalletHas


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `true`

### WalletImport


Perms: admin

Inputs:
```json
[
  {
    "Type": "bls",
    "PrivateKey": "Ynl0ZSBhcnJheQ=="
  }
]
```

Response: `"f01234"`

### WalletList


Perms: write

Inputs: `null`

Response:
```json
[
  "f01234"
]
```

### WalletNew


Perms: write

Inputs:
```json
[
  "bls"
]
```

Response: `"f01234"`

### WalletSetDefault


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### WalletSign


Perms: sign

Inputs:
```json
[
  "f01234",
  "Ynl0ZSBhcnJheQ=="
]
```

Response:
```json
{
  "Type": 2,
  "Data": "Ynl0ZSBhcnJheQ=="
}
```

### WalletSignMessage


Perms: sign

Inputs:
```json
[
  "f01234",
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
]
```

Response:
```json
{
  "Message": {
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
  "Signature": {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "CID": {
    "/": "bafy2bzacebbpdegvr3i4cosewthysg5xkxpqfn2wfcz6mv2hmoktwbdxkax4s"
  }
}
```

### WalletValidateAddress


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response: `"f01234"`

### WalletVerify


Perms: read

Inputs:
```json
[
  "f01234",
  "Ynl0ZSBhcnJheQ==",
  {
    "Type": 2,
    "Data": "Ynl0ZSBhcnJheQ=="
  }
]
```

Response: `true`

