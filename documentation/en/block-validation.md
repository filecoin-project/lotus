# Incoming block validations

This document reviews the code flow that takes place inside the full node after receiving a new block from the GossipSub `/fil/blocks` topic and traces all of its protocol-related validation logic. We do not include validation logic *inside* the VM, the analysis stops at `(*VM).Invoke()`. The `V:` tag explicitly signals validations throughout the text.

## `modules.HandleIncomingBlocks()`

We subscribe to the `/fil/blocks` PubSub topic to receive external blocks from peers in the network and register a block validator that operates at the PubSub (`libp2p` stack) level, validating each PubSub message containing a Filecoin block header.

`V:` PubSub message is a valid CBOR `BlockMsg`.

`V:` Total messages in block are under `BlockMessageLimit`.

`V:` Aggregate message CIDs, encapsulated in the `MsgMeta` structure, serialize to the `Messages` CID in the block header (`ValidateMsgMeta()`).

`V:` Miner `Address` in block header is present and corresponds to a public-key address in the current chain state.

`V:` Block signature (`BlockSig`) is present and belongs to the public-key address retrieved for the miner (`CheckBlockSignature()`).

## `sub.HandleIncomingBlocks()`

Assemble a `FullBlock` from the received block header retrieving its Filecoin messages.

`V:` Block messages CIDs can be retrieved from the network and decode into valid CBOR `Message`/`SignedMessage`.

## `(*Syncer).InformNewHead()`

Assemble a `FullTipSet` populated with the single block received earlier.

`V:` `ValidateMsgMeta()` (already done in the topic validator).

`V:` Block's `ParentWeight` is greater than the one from the (first block of the) heaviest tipset.

## `(*Syncer).Sync()`

`(*Syncer).collectHeaders()`: we retrieve all tipsets from the received block down to our chain. Validation now is expanded to *every* block inside these tipsets.

`V`: Beacon entires are ordered by their round number.

`V:` Tipset `Parents` CIDs match the fetched parent tipset through block sync. (This check is not enforced correctly at the moment, see [issue](https://github.com/filecoin-project/lotus/issues/1918).)

## `(*Syncer).ValidateBlock()`

This function contains most of the validation logic grouped in separate closures that run asynchronously, this list does not reflect validation order then.

`V:` Block `Timestamp`:
  * Is not bigger than current time plus `AllowableClockDriftSecs`.
  * Is not smaller than previous block's `Timestamp` plus `BlockDelay` (including null blocks).

### Messages

We check all the messages contained in one block at a time (`(*Syncer).checkBlockMessages()`).

`V:` The block's `BLSAggregate` matches the aggregate of BLS messages digests and public keys (extracted from the messages `From`).

`V:` Each `secp256k1` message `Signature` is signed with the public key extracted from the message `From`.

`V:` Aggregate message CIDs, encapsulated in the `MsgMeta` structure, serialize to the `Messages` CID in block header (similar to `ValidateMsgMeta()` call).

`V:` For each message, in `ValidForBlockInclusion()`:
* Message fields `Version`, `To`, `From`, `Value`, `GasPrice`, and `GasLimit` are correctly defined.
* Message `GasLimit` is under the message minimum gas cost (derived from chain height and message length).

`V:` Actor associated with message `From` exists and is an account actor, its `Nonce` matches the message `Nonce`.

### Miner

`V:` Miner address is registered in the `Claims` HAMT of the Power actor.

### Compute parent tipset state

`V:` Block's `ParentStateRoot` CID matches the state CID computed from the parent tipset.

`V:` Block's `ParentMessageReceipts` CID matches receipts CID computed from the parent tipset.

### Winner

Draw randomness for current epoch with minimum ticket from previous tipset, using `ElectionProofProduction`
domain separation tag.
`V`: `ElectionProof.VRFProof` is computed correctly by checking BLS signature using miner's key.
`V`: Miner is not slashed in `StoragePowerActor`.
`V`: Check if ticket is a winning ticket:
```
h := blake2b(VRFProof)
lhs := AsInt(h) * totalNetworkPower
rhs := minerPower * 2^256
if lhs < rhs { return "Winner" } else { return "Not a winner" }
```

### Block signature

`V:` `CheckBlockSignature()` (same signature validation as the one applied to the incoming block).

### Beacon values check

`V`: Validate that all `BeaconEntries` are valid. Check that every one of them is a signature of a message: `previousSignature || round` signed using drand's public key.
`V`: All entries between `MaxBeaconRoundForEpoch` down to `prevEntry` (from previous tipset) are included.

### Verify VRF Ticket chain

Draw randomness for current epoch with minimum ticket from previous tipset, using `TicketProduction`
domain separation tag.
`V`: `VerifyVRF` using drawn randomness and miner public key.

### Winning PoSt proof

Draw randomness for current epoch with `WinningPoSt` domain separation tag.
Get list of sectors challanged in this epoch for this miner, based on the randomness drawn. 

`V`: Use filecoin proofs system to verify that miner prooved access to sealed versions of these sectors.

## `(*StateManager).TipSetState()`

Called throughout the validation process for the parent of each tipset being validated. The checks here then do not apply to the received new head itself that started the validation process.

### `(*StateManager).computeTipSetState()`

`V:` Every block in the tipset should belong to different a miner.

### `(*StateManager).ApplyBlocks()`

We create a new VM with the tipset's `ParentStateRoot` (this is then the parent state of the parent of the tipset currently being validated) on which to apply all messages from all blocks in the tipset. For each message independently we apply the validations listed next.

### `(*VM).ApplyMessage()`

`V:` Basic gas and value checks in `checkMessage()`:
* Message `GasLimit` is bigger than zero.
* Message `GasPrice` and `Value` are set.

`V:` Message storage gas cost is under the message's `GasLimit`.

`V:` Message's `Nonce` matches nonce in actor retrieved from message's `From`.

`V:` Message's maximum gas cost (derived from its `GasLimit`, `GasPrice`, and `Value`) is under the balance of the actor retrieved from message's `From`.

### `(*VM).send()`

`V:` Message's transfer `Value` is under the balance in actor retrieved from message's `From`.
