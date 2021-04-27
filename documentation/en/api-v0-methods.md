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
* [Beacon](#Beacon)
  * [BeaconGetEntry](#BeaconGetEntry)
* [Chain](#Chain)
  * [ChainDeleteObj](#ChainDeleteObj)
  * [ChainExport](#ChainExport)
  * [ChainGetBlock](#ChainGetBlock)
  * [ChainGetBlockMessages](#ChainGetBlockMessages)
  * [ChainGetGenesis](#ChainGetGenesis)
  * [ChainGetMessage](#ChainGetMessage)
  * [ChainGetNode](#ChainGetNode)
  * [ChainGetParentMessages](#ChainGetParentMessages)
  * [ChainGetParentReceipts](#ChainGetParentReceipts)
  * [ChainGetPath](#ChainGetPath)
  * [ChainGetRandomnessFromBeacon](#ChainGetRandomnessFromBeacon)
  * [ChainGetRandomnessFromTickets](#ChainGetRandomnessFromTickets)
  * [ChainGetTipSet](#ChainGetTipSet)
  * [ChainGetTipSetByHeight](#ChainGetTipSetByHeight)
  * [ChainHasObj](#ChainHasObj)
  * [ChainHead](#ChainHead)
  * [ChainNotify](#ChainNotify)
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
  * [ClientFindData](#ClientFindData)
  * [ClientGenCar](#ClientGenCar)
  * [ClientGetDealInfo](#ClientGetDealInfo)
  * [ClientGetDealStatus](#ClientGetDealStatus)
  * [ClientGetDealUpdates](#ClientGetDealUpdates)
  * [ClientHasLocal](#ClientHasLocal)
  * [ClientImport](#ClientImport)
  * [ClientListDataTransfers](#ClientListDataTransfers)
  * [ClientListDeals](#ClientListDeals)
  * [ClientListImports](#ClientListImports)
  * [ClientMinerQueryOffer](#ClientMinerQueryOffer)
  * [ClientQueryAsk](#ClientQueryAsk)
  * [ClientRemoveImport](#ClientRemoveImport)
  * [ClientRestartDataTransfer](#ClientRestartDataTransfer)
  * [ClientRetrieve](#ClientRetrieve)
  * [ClientRetrieveTryRestartInsufficientFunds](#ClientRetrieveTryRestartInsufficientFunds)
  * [ClientRetrieveWithEvents](#ClientRetrieveWithEvents)
  * [ClientStartDeal](#ClientStartDeal)
* [Create](#Create)
  * [CreateBackup](#CreateBackup)
* [Gas](#Gas)
  * [GasEstimateFeeCap](#GasEstimateFeeCap)
  * [GasEstimateGasLimit](#GasEstimateGasLimit)
  * [GasEstimateGasPremium](#GasEstimateGasPremium)
  * [GasEstimateMessageGas](#GasEstimateMessageGas)
* [I](#I)
  * [ID](#ID)
* [Log](#Log)
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
  * [NetPeerInfo](#NetPeerInfo)
  * [NetPeers](#NetPeers)
  * [NetPubsubScores](#NetPubsubScores)
* [Paych](#Paych)
  * [PaychAllocateLane](#PaychAllocateLane)
  * [PaychAvailableFunds](#PaychAvailableFunds)
  * [PaychAvailableFundsByFromTo](#PaychAvailableFundsByFromTo)
  * [PaychCollect](#PaychCollect)
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
  * [StateAllMinerFaults](#StateAllMinerFaults)
  * [StateCall](#StateCall)
  * [StateChangedActors](#StateChangedActors)
  * [StateCirculatingSupply](#StateCirculatingSupply)
  * [StateCompute](#StateCompute)
  * [StateDealProviderCollateralBounds](#StateDealProviderCollateralBounds)
  * [StateDecodeParams](#StateDecodeParams)
  * [StateGetActor](#StateGetActor)
  * [StateGetReceipt](#StateGetReceipt)
  * [StateListActors](#StateListActors)
  * [StateListMessages](#StateListMessages)
  * [StateListMiners](#StateListMiners)
  * [StateLookupID](#StateLookupID)
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
  * [StateSearchMsgLimited](#StateSearchMsgLimited)
  * [StateSectorExpiration](#StateSectorExpiration)
  * [StateSectorGetInfo](#StateSectorGetInfo)
  * [StateSectorPartition](#StateSectorPartition)
  * [StateSectorPreCommitInfo](#StateSectorPreCommitInfo)
  * [StateVMCirculatingSupplyInternal](#StateVMCirculatingSupplyInternal)
  * [StateVerifiedClientStatus](#StateVerifiedClientStatus)
  * [StateVerifiedRegistryRootKey](#StateVerifiedRegistryRootKey)
  * [StateVerifierStatus](#StateVerifierStatus)
  * [StateWaitMsg](#StateWaitMsg)
  * [StateWaitMsgLimited](#StateWaitMsgLimited)
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
  "APIVersion": 131328,
  "BlockDelay": 42
}
```

## Auth


### AuthNew


Perms: admin

Inputs:
```json
[
  null
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

Response: `null`

## Beacon
The Beacon method group contains methods for interacting with the random beacon (DRAND)


### BeaconGetEntry
BeaconGetEntry returns the beacon entry for the given filecoin epoch. If
the entry has not yet been produced, the call will block until the entry
becomes available


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

## Chain
The Chain method group contains methods for interacting with the
blockchain, but that do not require any form of state computation.


### ChainDeleteObj
ChainDeleteObj deletes node referenced by the given CID


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
ChainExport returns a stream of bytes with CAR dump of chain data.
The exported chain data includes the header chain from the given tipset
back to genesis, the entire genesis state, and the most recent 'nroots'
state trees.
If oldmsgskip is set, messages from before the requested roots are also not included.


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
ChainGetBlock returns the block specified by the given CID.


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
  "BeaconEntries": null,
  "WinPoStProof": null,
  "Parents": null,
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
ChainGetBlockMessages returns messages stored in the specified block.

Note: If there are multiple blocks in a tipset, it's likely that some
messages will be duplicated. It's also possible for blocks in a tipset to have
different messages from the same sender at the same nonce. When that happens,
only the first message (in a block with lowest ticket) will be considered
for execution

NOTE: THIS METHOD SHOULD ONLY BE USED FOR GETTING MESSAGES IN A SPECIFIC BLOCK

DO NOT USE THIS METHOD TO GET MESSAGES INCLUDED IN A TIPSET
Use ChainGetParentMessages, which will perform correct message deduplication


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
  "BlsMessages": null,
  "SecpkMessages": null,
  "Cids": null
}
```

### ChainGetGenesis
ChainGetGenesis returns the genesis tipset.


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
ChainGetMessage reads a message referenced by the specified CID from the
chain blockstore.


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
ChainGetParentMessages returns messages stored in parent tipset of the
specified block.


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `null`

### ChainGetParentReceipts
ChainGetParentReceipts returns receipts for messages in parent tipset of
the specified block. The receipts in the list returned is one-to-one with the
messages returned by a call to ChainGetParentMessages with the same blockCid.


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  }
]
```

Response: `null`

### ChainGetPath
ChainGetPath returns a set of revert/apply operations needed to get from
one tipset to another, for example:
```
       to
        ^
from   tAA
  ^     ^
tBA    tAB
 ^---*--^
     ^
    tRR
```
Would return `[revert(tBA), apply(tAB), apply(tAA)]`


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

Response: `null`

### ChainGetRandomnessFromBeacon
ChainGetRandomnessFromBeacon is used to sample the beacon for randomness.


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
  2,
  10101,
  "Ynl0ZSBhcnJheQ=="
]
```

Response: `null`

### ChainGetRandomnessFromTickets
ChainGetRandomnessFromTickets is used to sample the chain for randomness.


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
  2,
  10101,
  "Ynl0ZSBhcnJheQ=="
]
```

Response: `null`

### ChainGetTipSet
ChainGetTipSet returns the tipset specified by the given TipSetKey.


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

### ChainGetTipSetByHeight
ChainGetTipSetByHeight looks back for a tipset at the specified epoch.
If there are no blocks at the specified epoch, a tipset at an earlier epoch
will be returned.


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
ChainHasObj checks if a given CID exists in the chain blockstore.


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
ChainHead returns the current head of the chain.


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
ChainNotify returns channel with chain head updates.
First message is guaranteed to be of len == 1, and type == 'current'.


Perms: read

Inputs: `null`

Response: `null`

### ChainReadObj
ChainReadObj reads ipld nodes referenced by the specified CID from chain
blockstore and returns raw bytes.


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
ChainSetHead forcefully sets current chain head. Use with caution.


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
ChainStatObj returns statistics about the graph referenced by 'obj'.
If 'base' is also specified, then the returned stat will be a diff
between the two objects.


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
ChainTipSetWeight computes weight for the specified tipset.


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
The Client methods all have to do with interacting with the storage and
retrieval markets as a client


### ClientCalcCommP
ClientCalcCommP calculates the CommP for a specified file


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
ClientCancelDataTransfer cancels a data transfer with the given transfer ID and other peer


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
ClientCancelRetrievalDeal cancels an ongoing retrieval deal based on DealID


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
    "Stages": null
  }
}
```

### ClientDealPieceCID
ClientCalcCommP calculates the CommP and data size of the specified CID


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
ClientDealSize calculates real deal data size


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

### ClientFindData
ClientFindData identifies peers that have a certain file, and returns QueryOffers (one per peer).


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

Response: `null`

### ClientGenCar
ClientGenCar generates a CAR file for the specified file.


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
ClientGetDealInfo returns the latest information about a given deal.


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
    "Stages": null
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
      "Stages": null
    }
  }
}
```

### ClientGetDealStatus
ClientGetDealStatus returns status given a code


Perms: read

Inputs:
```json
[
  42
]
```

Response: `"string value"`

### ClientGetDealUpdates
ClientGetDealUpdates returns the status of updated deals


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
    "Stages": null
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
      "Stages": null
    }
  }
}
```

### ClientHasLocal
ClientHasLocal indicates whether a certain CID is locally stored.


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
ClientImport imports file under the specified path into filestore.


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
ClientListTransfers returns the status of all ongoing transfers of data


Perms: write

Inputs: `null`

Response: `null`

### ClientListDeals
ClientListDeals returns information about the deals made by the local client.


Perms: write

Inputs: `null`

Response: `null`

### ClientListImports
ClientListImports lists imported files and their root CIDs


Perms: write

Inputs: `null`

Response: `null`

### ClientMinerQueryOffer
ClientMinerQueryOffer returns a QueryOffer for the specific miner and file.


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
ClientQueryAsk returns a signed StorageAsk from the specified miner.


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
  "Price": "0",
  "VerifiedPrice": "0",
  "MinPieceSize": 1032,
  "MaxPieceSize": 1032,
  "Miner": "f01234",
  "Timestamp": 10101,
  "Expiry": 10101,
  "SeqNo": 42
}
```

### ClientRemoveImport
ClientRemoveImport removes file import


Perms: admin

Inputs:
```json
[
  50
]
```

Response: `{}`

### ClientRestartDataTransfer
ClientRestartDataTransfer attempts to restart a data transfer with the given transfer ID and other peer


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
ClientRetrieve initiates the retrieval of a file, as specified in the order.


Perms: admin

Inputs:
```json
[
  {
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Piece": null,
    "Size": 42,
    "LocalStore": 12,
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
  },
  {
    "Path": "string value",
    "IsCAR": true
  }
]
```

Response: `{}`

### ClientRetrieveTryRestartInsufficientFunds
ClientRetrieveTryRestartInsufficientFunds attempts to restart stalled retrievals on a given payment channel
which are stuck due to insufficient funds


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### ClientRetrieveWithEvents
ClientRetrieveWithEvents initiates the retrieval of a file, as specified in the order, and provides a channel
of status updates.


Perms: admin

Inputs:
```json
[
  {
    "Root": {
      "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
    },
    "Piece": null,
    "Size": 42,
    "LocalStore": 12,
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
  },
  {
    "Path": "string value",
    "IsCAR": true
  }
]
```

Response:
```json
{
  "Event": 5,
  "Status": 0,
  "BytesReceived": 42,
  "FundsSpent": "0",
  "Err": "string value"
}
```

### ClientStartDeal
ClientStartDeal proposes a deal with a miner.


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

## Create


### CreateBackup
CreateBackup creates node backup onder the specified file name. The
method requires that the lotus daemon is running with the
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

## Gas


### GasEstimateFeeCap
GasEstimateFeeCap estimates gas fee cap


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
GasEstimateGasLimit estimates gas used by the message and returns it.
It fails if message fails to execute.


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
GasEstimateGasPremium estimates what gas price should be used for a
message to have high likelihood of inclusion in `nblocksincl` epochs.


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
GasEstimateMessageGas estimates gas values for unset message gas fields


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


### LogList


Perms: write

Inputs: `null`

Response: `null`

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
MarketAddBalance adds funds to the market actor


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
MarketGetReserved gets the amount of funds that are currently reserved for the address


Perms: sign

Inputs:
```json
[
  "f01234"
]
```

Response: `"0"`

### MarketReleaseFunds
MarketReleaseFunds releases funds reserved by MarketReserveFunds


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
MarketReserveFunds reserves funds for a deal


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
MarketWithdraw withdraws unlocked funds from the market actor


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
    "BeaconValues": null,
    "Messages": null,
    "Epoch": 10101,
    "Timestamp": 42,
    "WinningPoStProof": null
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
    "BeaconEntries": null,
    "WinPoStProof": null,
    "Parents": null,
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
  "BlsMessages": null,
  "SecpkMessages": null
}
```

### MinerGetBaseInfo
There are not yet any comments for this method.

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
  "Sectors": null,
  "WorkerKey": "f01234",
  "SectorSize": 34359738368,
  "PrevBeaconEntry": {
    "Round": 42,
    "Data": "Ynl0ZSBhcnJheQ=="
  },
  "BeaconEntries": null,
  "EligibleForMining": true
}
```

## Mpool
The Mpool methods are for interacting with the message pool. The message pool
manages all incoming and outgoing 'messages' going over the network.


### MpoolBatchPush
MpoolBatchPush batch pushes a signed message to mempool.


Perms: write

Inputs:
```json
[
  null
]
```

Response: `null`

### MpoolBatchPushMessage
MpoolBatchPushMessage batch pushes a unsigned message to mempool.


Perms: sign

Inputs:
```json
[
  null,
  {
    "MaxFee": "0"
  }
]
```

Response: `null`

### MpoolBatchPushUntrusted
MpoolBatchPushUntrusted batch pushes a signed message to mempool from untrusted sources.


Perms: write

Inputs:
```json
[
  null
]
```

Response: `null`

### MpoolClear
MpoolClear clears pending messages from the mpool


Perms: write

Inputs:
```json
[
  true
]
```

Response: `{}`

### MpoolGetConfig
MpoolGetConfig returns (a copy of) the current mpool config


Perms: read

Inputs: `null`

Response:
```json
{
  "PriorityAddrs": null,
  "SizeLimitHigh": 123,
  "SizeLimitLow": 123,
  "ReplaceByFeeRatio": 12.3,
  "PruneCooldown": 60000000000,
  "GasLimitOverestimation": 12.3
}
```

### MpoolGetNonce
MpoolGetNonce gets next nonce for the specified sender.
Note that this method may not be atomic. Use MpoolPushMessage instead.


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response: `42`

### MpoolPending
MpoolPending returns pending mempool messages.


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

Response: `null`

### MpoolPush
MpoolPush pushes a signed message to mempool.


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
MpoolPushMessage atomically assigns a nonce, signs, and pushes a message
to mempool.
maxFee is only used when GasFeeCap/GasPremium fields aren't specified

When maxFee is set to 0, MpoolPushMessage will guess appropriate fee
based on current chain conditions


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
MpoolPushUntrusted pushes a signed message to mempool from untrusted sources.


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
MpoolSelect returns a list of pending messages for inclusion in the next block


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

Response: `null`

### MpoolSetConfig
MpoolSetConfig sets the mpool config to (a copy of) the supplied config


Perms: admin

Inputs:
```json
[
  {
    "PriorityAddrs": null,
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
The Msig methods are used to interact with multisig wallets on the
filecoin network


### MsigAddApprove
MsigAddApprove approves a previously proposed AddSigner message
It takes the following params: <multisig address>, <sender address of the approve msg>, <proposed message ID>,
<proposer address>, <new signer>, <whether the number of required signers should be increased>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigAddCancel
MsigAddCancel cancels a previously proposed AddSigner message
It takes the following params: <multisig address>, <sender address of the cancel msg>, <proposed message ID>,
<new signer>, <whether the number of required signers should be increased>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigAddPropose
MsigAddPropose proposes adding a signer in the multisig
It takes the following params: <multisig address>, <sender address of the propose msg>,
<new signer>, <whether the number of required signers should be increased>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigApprove
MsigApprove approves a previously-proposed multisig message by transaction ID
It takes the following params: <multisig address>, <proposed transaction ID> <signer address>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigApproveTxnHash
MsigApproveTxnHash approves a previously-proposed multisig message, specified
using both transaction ID and a hash of the parameters used in the
proposal. This method of approval can be used to ensure you only approve
exactly the transaction you think you are.
It takes the following params: <multisig address>, <proposed message ID>, <proposer address>, <recipient address>, <value to transfer>,
<sender address of the approve msg>, <method to call in the proposed message>, <params to include in the proposed message>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigCancel
MsigCancel cancels a previously-proposed multisig message
It takes the following params: <multisig address>, <proposed transaction ID>, <recipient address>, <value to transfer>,
<sender address of the cancel msg>, <method to call in the proposed message>, <params to include in the proposed message>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigCreate
MsigCreate creates a multisig wallet
It takes the following params: <required number of senders>, <approving addresses>, <unlock duration>
<initial balance>, <sender address of the create msg>, <gas price>


Perms: sign

Inputs:
```json
[
  42,
  null,
  10101,
  "0",
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

### MsigGetAvailableBalance
MsigGetAvailableBalance returns the portion of a multisig's balance that can be withdrawn or spent


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
MsigGetPending returns pending transactions for the given multisig
wallet. Once pending transactions are fully approved, they will no longer
appear here.


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

Response: `null`

### MsigGetVested
MsigGetVested returns the amount of FIL that vested in a multisig in a certain period.
It takes the following params: <multisig address>, <start epoch>, <end epoch>


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
MsigGetVestingSchedule returns the vesting details of a given multisig.


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
MsigPropose proposes a multisig message
It takes the following params: <multisig address>, <recipient address>, <value to transfer>,
<sender address of the propose msg>, <method to call in the proposed message>, <params to include in the proposed message>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigRemoveSigner
MsigRemoveSigner proposes the removal of a signer from the multisig.
It accepts the multisig to make the change on, the proposer address to
send the message from, the address to be removed, and a boolean
indicating whether or not the signing threshold should be lowered by one
along with the address removal.


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigSwapApprove
MsigSwapApprove approves a previously proposed SwapSigner
It takes the following params: <multisig address>, <sender address of the approve msg>, <proposed message ID>,
<proposer address>, <old signer>, <new signer>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigSwapCancel
MsigSwapCancel cancels a previously proposed SwapSigner message
It takes the following params: <multisig address>, <sender address of the cancel msg>, <proposed message ID>,
<old signer>, <new signer>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

### MsigSwapPropose
MsigSwapPropose proposes swapping 2 signers in the multisig
It takes the following params: <multisig address>, <sender address of the propose msg>,
<old signer>, <new signer>


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
  "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
}
```

## Net


### NetAddrsListen


Perms: read

Inputs: `null`

Response:
```json
{
  "Addrs": null,
  "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
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
    "Peers": null,
    "IPAddrs": null,
    "IPSubnets": null
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
  "Peers": null,
  "IPAddrs": null,
  "IPSubnets": null
}
```

### NetBlockRemove


Perms: admin

Inputs:
```json
[
  {
    "Peers": null,
    "IPAddrs": null,
    "IPSubnets": null
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
    "Addrs": null,
    "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
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
  "Addrs": null,
  "ID": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf"
}
```

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
  "Addrs": null,
  "Protocols": null,
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

Response: `null`

### NetPubsubScores


Perms: read

Inputs: `null`

Response: `null`

## Paych
The Paych methods are for interacting with and managing payment channels


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

### PaychGet
There are not yet any comments for this method.

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

Response: `null`

### PaychNewPayment


Perms: sign

Inputs:
```json
[
  "f01234",
  "f01234",
  null
]
```

Response:
```json
{
  "Channel": "f01234",
  "WaitSentinel": {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  "Vouchers": null
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
    "SecretPreimage": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": null,
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
    "SecretPreimage": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": null,
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
    "SecretPreimage": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": null,
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
    "SecretPreimage": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": null,
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

Response: `null`

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
    "SecretPreimage": "Ynl0ZSBhcnJheQ==",
    "Extra": {
      "Actor": "f01234",
      "Method": 1,
      "Data": "Ynl0ZSBhcnJheQ=="
    },
    "Lane": 42,
    "Nonce": 42,
    "Amount": "0",
    "MinSettleHeight": 10101,
    "Merges": null,
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
The State methods are used to query, inspect, and interact with chain state.
Most methods take a TipSetKey as a parameter. The state looked up is the parent state of the tipset.
A nil TipSetKey can be provided as a param, this will cause the heaviest tipset in the chain to be used.


### StateAccountKey
StateAccountKey returns the public key address of the given ID address


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

### StateAllMinerFaults
StateAllMinerFaults returns all non-expired Faults that occur within lookback epochs of the given tipset


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

Response: `null`

### StateCall
StateCall runs the given message and returns its result without any persisted changes.

StateCall applies the message to the tipset's parent state. The
message is not applied on-top-of the messages in the passed-in
tipset.


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
    "GasCharges": null,
    "Subcalls": null
  },
  "Error": "string value",
  "Duration": 60000000000
}
```

### StateChangedActors
StateChangedActors returns all the actors whose states change between the two given state CIDs
TODO: Should this take tipset keys instead?


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
StateCirculatingSupply returns the exact circulating supply of Filecoin at the given tipset.
This is not used anywhere in the protocol itself, and is only for external consumption.


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
StateCompute is a flexible command that applies the given messages on the given tipset.
The messages are run as though the VM were at the provided height.

When called, StateCompute will:
- Load the provided tipset, or use the current chain head if not provided
- Compute the tipset state of the provided tipset on top of the parent state
  - (note that this step runs before vmheight is applied to the execution)
  - Execute state upgrade if any were scheduled at the epoch, or in null
    blocks preceding the tipset
  - Call the cron actor on null blocks preceding the tipset
  - For each block in the tipset
    - Apply messages in blocks in the specified
    - Award block reward by calling the reward actor
  - Call the cron actor for the current epoch
- If the specified vmheight is higher than the current epoch, apply any
  needed state upgrades to the state
- Apply the specified messages to the state

The vmheight parameter sets VM execution epoch, and can be used to simulate
message execution in different network versions. If the specified vmheight
epoch is higher than the epoch of the specified tipset, any state upgrades
until the vmheight will be executed on the state before applying messages
specified by the user.

Note that the initial tipset state computation is not affected by the
vmheight parameter - only the messages in the `apply` set are

If the caller wants to simply compute the state, vmheight should be set to
the epoch of the specified tipset.

Messages in the `apply` parameter must have the correct nonces, and gas
values set.


Perms: read

Inputs:
```json
[
  10101,
  null,
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
  "Trace": null
}
```

### StateDealProviderCollateralBounds
StateDealProviderCollateralBounds returns the min and max collateral a storage provider
can issue. It takes the deal size and verified status as parameters.


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
StateDecodeParams attempts to decode the provided params, based on the recipient actor address and method number.


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

### StateGetActor
StateGetActor returns the indicated actor's nonce and balance.


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

### StateGetReceipt
StateGetReceipt returns the message receipt for the given message or for a
matching gas-repriced replacing message

NOTE: If the requested message was replaced, this method will return the receipt
for the replacing message - if the caller needs the receipt for exactly the
requested message, use StateSearchMsg().Receipt, and check that MsgLookup.Message
is matching the requested CID

DEPRECATED: Use StateSearchMsg, this method won't be supported in v1 API


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
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
  "ExitCode": 0,
  "Return": "Ynl0ZSBhcnJheQ==",
  "GasUsed": 9
}
```

### StateListActors
StateListActors returns the addresses of every actor in the state


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

Response: `null`

### StateListMessages
StateListMessages looks back and returns all messages with a matching to or from address, stopping at the given height.


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

Response: `null`

### StateListMiners
StateListMiners returns the addresses of every miner that has claimed power in the Power Actor


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

Response: `null`

### StateLookupID
StateLookupID retrieves the ID address of the given address


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
StateMarketBalance looks up the Escrow and Locked balances of the given address in the Storage Market


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
StateMarketDeals returns information about every deal in the Storage Market


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
      "Label": "string value",
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
StateMarketParticipants returns the Escrow and Locked balances of every participant in the Storage Market


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
StateMarketStorageDeal returns information about the indicated deal


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
    "Label": "string value",
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
StateMinerActiveSectors returns info about sectors that a given miner is actively proving.


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

Response: `null`

### StateMinerAvailableBalance
StateMinerAvailableBalance returns the portion of a miner's balance that can be withdrawn or spent


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
StateMinerDeadlines returns all the proving deadlines for the given miner


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

Response: `null`

### StateMinerFaults
StateMinerFaults returns a bitfield indicating the faulty sectors of the given miner


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
StateMinerInfo returns info about the indicated miner


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
  "ControlAddresses": null,
  "WorkerChangeEpoch": 10101,
  "PeerId": "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf",
  "Multiaddrs": null,
  "WindowPoStProofType": 8,
  "SectorSize": 34359738368,
  "WindowPoStPartitionSectors": 42,
  "ConsensusFaultElapsed": 10101
}
```

### StateMinerInitialPledgeCollateral
StateMinerInitialPledgeCollateral returns the initial pledge collateral for the specified miner's sector


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
    "DealIDs": null,
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
StateMinerPartitions returns all partitions in the specified deadline


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

Response: `null`

### StateMinerPower
StateMinerPower returns the power of the indicated miner


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
StateMinerInitialPledgeCollateral returns the precommit deposit for the specified miner's sector


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
    "DealIDs": null,
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
StateMinerProvingDeadline calculates the deadline at some epoch for a proving period
and returns the deadline-related calculations.


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
StateMinerRecoveries returns a bitfield indicating the recovering sectors of the given miner


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
StateMinerSectorAllocated checks if a sector is allocated


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
StateMinerSectorCount returns the number of sectors in a miner's sector set and proving set


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
StateMinerSectors returns info about the given miner's sectors. If the filter bitfield is nil, all sectors are included.


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

Response: `null`

### StateNetworkName
StateNetworkName returns the name of the network the node is synced to


Perms: read

Inputs: `null`

Response: `"lotus"`

### StateNetworkVersion
StateNetworkVersion returns the network version at the given tipset


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

Response: `11`

### StateReadState
StateReadState returns the indicated actor's state.


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
StateReplay replays a given message, assuming it was included in a block in the specified tipset.

If a tipset key is provided, and a replacing message is found on chain,
the method will return an error saying that the message wasn't found

If no tipset key is provided, the appropriate tipset is looked up, and if
the message was gas-repriced, the on-chain message will be replayed - in
that case the returned InvocResult.MsgCid will not match the Cid param

If the caller wants to ensure that exactly the requested message was executed,
they MUST check that InvocResult.MsgCid is equal to the provided Cid.
Without this check both the requested and original message may appear as
successfully executed on-chain, which may look like a double-spend.

A replacing message is a message with a different CID, any of Gas values, and
different signature, but with all other parameters matching (source/destination,
nonce, params, etc.)


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
    "GasCharges": null,
    "Subcalls": null
  },
  "Error": "string value",
  "Duration": 60000000000
}
```

### StateSearchMsg
StateSearchMsg searches for a message in the chain, and returns its receipt and the tipset where it was executed

NOTE: If a replacing message is found on chain, this method will return
a MsgLookup for the replacing message - the MsgLookup.Message will be a different
CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
result of the execution of the replacing message.

If the caller wants to ensure that exactly the requested message was executed,
they MUST check that MsgLookup.Message is equal to the provided 'cid'.
Without this check both the requested and original message may appear as
successfully executed on-chain, which may look like a double-spend.

A replacing message is a message with a different CID, any of Gas values, and
different signature, but with all other parameters matching (source/destination,
nonce, params, etc.)


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

### StateSearchMsgLimited
StateSearchMsgLimited looks back up to limit epochs in the chain for a message, and returns its receipt and the tipset where it was executed

NOTE: If a replacing message is found on chain, this method will return
a MsgLookup for the replacing message - the MsgLookup.Message will be a different
CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
result of the execution of the replacing message.

If the caller wants to ensure that exactly the requested message was executed,
they MUST check that MsgLookup.Message is equal to the provided 'cid'.
Without this check both the requested and original message may appear as
successfully executed on-chain, which may look like a double-spend.

A replacing message is a message with a different CID, any of Gas values, and
different signature, but with all other parameters matching (source/destination,
nonce, params, etc.)


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  10101
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
StateSectorExpiration returns epoch at which given sector will expire


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
StateSectorGetInfo returns the on-chain info for the specified miner's sector. Returns null in case the sector info isn't found
NOTE: returned info.Expiration may not be accurate in some cases, use StateSectorExpiration to get accurate
expiration epoch


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
  "DealIDs": null,
  "Activation": 10101,
  "Expiration": 10101,
  "DealWeight": "0",
  "VerifiedDealWeight": "0",
  "InitialPledge": "0",
  "ExpectedDayReward": "0",
  "ExpectedStoragePledge": "0"
}
```

### StateSectorPartition
StateSectorPartition finds deadline/partition with the specified sector


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
StateSectorPreCommitInfo returns the PreCommit info for the specified miner's sector


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
    "DealIDs": null,
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
StateVMCirculatingSupplyInternal returns an approximation of the circulating supply of Filecoin at the given tipset.
This is the value reported by the runtime interface to actors code.


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
StateVerifiedClientStatus returns the data cap for the given address.
Returns nil if there is no entry in the data cap table for the
address.


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
StateVerifiedClientStatus returns the address of the Verified Registry's root key


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
StateVerifierStatus returns the data cap for the given address.
Returns nil if there is no entry in the data cap table for the
address.


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
StateWaitMsg looks back in the chain for a message. If not found, it blocks until the
message arrives on chain, and gets to the indicated confidence depth.

NOTE: If a replacing message is found on chain, this method will return
a MsgLookup for the replacing message - the MsgLookup.Message will be a different
CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
result of the execution of the replacing message.

If the caller wants to ensure that exactly the requested message was executed,
they MUST check that MsgLookup.Message is equal to the provided 'cid'.
Without this check both the requested and original message may appear as
successfully executed on-chain, which may look like a double-spend.

A replacing message is a message with a different CID, any of Gas values, and
different signature, but with all other parameters matching (source/destination,
nonce, params, etc.)


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  42
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

### StateWaitMsgLimited
StateWaitMsgLimited looks back up to limit epochs in the chain for a message.
If not found, it blocks until the message arrives on chain, and gets to the
indicated confidence depth.

NOTE: If a replacing message is found on chain, this method will return
a MsgLookup for the replacing message - the MsgLookup.Message will be a different
CID than the one provided in the 'cid' param, MsgLookup.Receipt will contain the
result of the execution of the replacing message.

If the caller wants to ensure that exactly the requested message was executed,
they MUST check that MsgLookup.Message is equal to the provided 'cid'.
Without this check both the requested and original message may appear as
successfully executed on-chain, which may look like a double-spend.

A replacing message is a message with a different CID, any of Gas values, and
different signature, but with all other parameters matching (source/destination,
nonce, params, etc.)


Perms: read

Inputs:
```json
[
  {
    "/": "bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4"
  },
  42,
  10101
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
The Sync method group contains methods for interacting with and
observing the lotus sync service.


### SyncCheckBad
SyncCheckBad checks if a block was marked as bad, and if it was, returns
the reason.


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
SyncCheckpoint marks a blocks as checkpointed, meaning that it won't ever fork away from it.


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
SyncIncomingBlocks returns a channel streaming incoming, potentially not
yet synced block headers.


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
  "BeaconEntries": null,
  "WinPoStProof": null,
  "Parents": null,
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
SyncMarkBad marks a blocks as bad, meaning that it won't ever by synced.
Use with extreme caution.


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
SyncState returns the current status of the lotus sync system.


Perms: read

Inputs: `null`

Response:
```json
{
  "ActiveSyncs": null,
  "VMApplied": 42
}
```

### SyncSubmitBlock
SyncSubmitBlock can be used to submit a newly created block to the.
network through this node


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
      "BeaconEntries": null,
      "WinPoStProof": null,
      "Parents": null,
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
    "BlsMessages": null,
    "SecpkMessages": null
  }
]
```

Response: `{}`

### SyncUnmarkAllBad
SyncUnmarkAllBad purges bad block cache, making it possible to sync to chains previously marked as bad


Perms: admin

Inputs: `null`

Response: `{}`

### SyncUnmarkBad
SyncUnmarkBad unmarks a blocks as bad, making it possible to be validated and synced again.


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
SyncValidateTipset indicates whether the provided tipset is valid or not


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
WalletBalance returns the balance of the given address at the current head of the chain.


Perms: read

Inputs:
```json
[
  "f01234"
]
```

Response: `"0"`

### WalletDefaultAddress
WalletDefaultAddress returns the address marked as default in the wallet.


Perms: write

Inputs: `null`

Response: `"f01234"`

### WalletDelete
WalletDelete deletes an address from the wallet.


Perms: admin

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### WalletExport
WalletExport returns the private key of an address in the wallet.


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
WalletHas indicates whether the given address is in the wallet.


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `true`

### WalletImport
WalletImport receives a KeyInfo, which includes a private key, and imports it into the wallet.


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
WalletList lists all the addresses in the wallet.


Perms: write

Inputs: `null`

Response: `null`

### WalletNew
WalletNew creates a new address in the wallet with the given sigType.
Available key types: bls, secp256k1, secp256k1-ledger
Support for numerical types: 1 - secp256k1, 2 - BLS is deprecated


Perms: write

Inputs:
```json
[
  "bls"
]
```

Response: `"f01234"`

### WalletSetDefault
WalletSetDefault marks the given address as as the default one.


Perms: write

Inputs:
```json
[
  "f01234"
]
```

Response: `{}`

### WalletSign
WalletSign signs the given bytes using the given address.


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
WalletSignMessage signs the given message using the given address.


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
WalletValidateAddress validates whether a given string can be decoded as a well-formed address


Perms: read

Inputs:
```json
[
  "string value"
]
```

Response: `"f01234"`

### WalletVerify
WalletVerify takes an address, a signature, and some bytes, and indicates whether the signature is valid.
The address does not have to be in the wallet.


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

