# Actor Events and Lotus APIs

* [Background](#background)
* [ActorEvent structure](#actorevent-structure)
* [Querying Lotus for ActorEvents](#querying-lotus-for-actorevents)
* [Retrieving events from message receipts](#retrieving-events-from-message-receipts)
* [Current builtin actor event schemas](#current-builtin-actor-event-schemas)
  * [Verified registry actor events](#verified-registry-actor-events)
    * [Verifier balance](#verifier-balance)
    * [Allocation](#allocation)
    * [Allocation removed](#allocation-removed)
    * [Claim](#claim)
    * [Claim updated](#claim-updated)
    * [Claim removed](#claim-removed)
  * [Market actor events](#market-actor-events)
    * [Deal published](#deal-published)
    * [Deal activated](#deal-activated)
    * [Deal terminated](#deal-terminated)
    * [Deal completed](#deal-completed)
  * [Miner actor events](#miner-actor-events)
    * [Sector precommitted](#sector-precommitted)
    * [Sector activated](#sector-activated)
    * [Sector updated](#sector-updated)
    * [Sector terminated](#sector-terminated)

## Background

Actor events are a fire-and-forget mechanism for actors in Filecoin to signal events that occur during execution of their methods to external observers. Actor events are intended to be used by tooling and applications that need to observe and react to events that occur within the chain. The events themselves are not stored in chain state, although a root CID for an array (AMT) of all events emitted for a single message is recorded on message receipts, which are themselves referenced as an array (AMT) in the `ParentMessageReceipts` in each `BlockHeader` of a tipset. A node may optionally retain historical events for querying, but this is not guaranteed and not essential as it does not affect the chain state.

The FVM already has this capability and new events for builtin actors have been added to support a range of new features, starting at network version 22 with a focus on some information gaps for consumers of data onboarding activity insight due to the introduction of [Direct Data Onboarding (DDO)](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md), plus some additional events related to data onboarding, deal lifecycles, sector lifecycles, and DataCap activity. Additional events are expected to be added in the future to support other features and use cases.

Builtin actor events share basic similarities to the existing events emitted by user-programmed actors in FVM, but each have a specific schema that reflects their specific concerns. They also all use CBOR encoding for their values. There are also new APIs in Lotus to support querying for these events that bear some similarities to the existing FEVM `Eth*` APIs for querying events but are unique to builtin actors.

## ActorEvent structure

Introduced in [FIP-0049](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0049.md), events use a structured logging style of composition, containing a list of entries that define properties of the event. The log entries are described below as `EventEntry` and have the same schema for user-programmed and builtin actor events. `ActorEvent` is specifically for representing builtin actor events and includes the list of entries, the actor that emitted the event, and some metadata about the event.

```ipldsch
type ActorEvent struct {
    entries [EventEntry] # Event entries in log form.
    emitter Address      # Filecoin address of the actor that emitted this event.
    # Reverted is set to true if the message that produced this event was reverted because of a
    # network re-org in that case, the event should be considered as reverted as well.
    reverted Bool        
    height ChainEpoch    # Height of the tipset that contained the message that produced this event.
    tipsetCid &Any       # CID of the tipset that contained the message that produced this event.
    msgCid &Any          # CID of message that produced this event.
}

type EventEntry struct {
    flags Int   # A bitmap conveying metadata or hints about this entry.
    key String  # The key of this entry.
    codec Int   # The value's IPLD codec.
    value Bytes # The value of this entry as a byte string, encoded with 'codec'.
}
```

A `flags` field is used to convey metadata or hints about the entry, currently this is used to provide an indication of the suitability of that field for indexing. Suitability for indexing is only a hint, and typically relates to the queriability of the content of that field.

* A `flag` of `0x00` indicates that neither the key nor value are suitable for indexing.
* A `flag` of `0x01` indicates that the key only is suitable for indexing.
* A `flag` of `0x02` indicates that the value is suitable for indexing.
* A `flag` of `0x03` indicates that both the key and value are suitable for indexing.

Typically events contain entries that use either use `0x01` or `0x03` flags.

The structured logging style of composition should be seen in contrast to an alternative representation as a plain map or struct where the keys represent the fields of the event and the values represent the values of those fields. Some entries may duplicate keys, in which case that particular field of the event could be represented as an array. Builtin actor events are sufficiently well defined that translation to such a format is possible, but left up to the user.

## Querying Lotus for ActorEvents

Two Lotus APIs are provided that can be used to obtain direct access to events stored on the node being queried (a node may not have all historical events stored and available for query):

- **[`GetActorEventsRaw`](https://github.com/filecoin-project/lotus/blob/master/documentation/en/api-methods-v1-stable.md#GetActorEventsRaw)** will return all available historical actor events that match a given *filter* argument.
- **[`SubscribeActorEventsRaw`](https://github.com/filecoin-project/lotus/blob/master/documentation/en/api-methods-v1-stable.md#SubscribeActorEventsRaw)** will return a long-lived stream providing all available actor events that match a given *filter* argument as they are generated. Optionally also providing a list of historical events. This API is available via websocket from the Lotus API RPC.

Both APIs take an `EventFilter`  as an argument to determine which events to return. This event filter optionally comprises the following:

- `fromEpoch` determines when to start looking for matching events, either an epoch (in hex form), the string `earliest`  or `latest` . A node is not guaranteed to have historical blocks for a particular epoch however `earliest`  is intended to provide events from the beginning of the available list.
- `toEpoch`  determines when to stop looking for matching events, either an epoch (in hex form), the string `earliest`  or `latest`.
- `addresses` will match a list of addresses that an event comes *from* (currently just a builtin actor address).
- `fields` is a key to value mapping that matches specific event entries. Each field being matched is a property in the `fields`  map and the value of that property is an array of maps, where each entry is a possible matching value for that entry. Each possible match contains a `codec`  integer (currently just CBOR `0x51` for builtin actor events described in this document) and a `value`  bytes blob (Base64 encoded) of the encoded field value (e.g. a Base64 encoded form of a CBOR encoded key string, such as an actor ID or an event ID). Matching first involves finding if an event’s entries contain one of the desired `key`s, then checking that one of the value matchers for that `key` field matches the value. Value matching is performed both on the `codec`  and the `value`  bytes. If an event’s entry is matched, the entire event is considered a match. This may be used to query for particular event types, such as `allocation`.
An example `fields`  with a single matcher would look like: `"fields": { "abc": [{ "codec": 81, "value": "ZGRhdGE=" }]}` where the key being matched is `abc`  with the CBOR codec (`0x51` = `81`) and value is the unicode string `data` encoded as CBOR (then encoded in Base64 to supply to the filter).
- `tipsetCid`  matches a particular TipSet. If this is provided, both `fromBlock`  and `toBlock`  will be ignored.

Described as an [IPLD Schema](https://ipld.io/docs/schemas/), the event filter is:

```ipldsch
type EventFilter struct {
  fromEpoch optional String
  toEpoch optional String
  addresses optional [Address]
  fields optional {String:[ActorEventValue]}
  tipsetCid optional &Any
}

type Address string # Address of an actor

type ActorEventValue struct {
  codec Int   # typically the CBOR codec (0x51)
  value Bytes # typically the CBOR encoded value
}
```

## Retrieving events from message receipts

The Lotus API `ChainGetEvents` can be used to retrieve events given an event root CID. This CID is attached to the message receipt that generated the events. The `StateSearchMsg` API can be used to retrieve the message receipt given a message CID, the receipt contains the `EventsRoot` CID. The events returned from `ChainGetEvents` contain roughly the same information as the `ActorEvent` structure, including the `EventEntry` log array.

## Current builtin actor event schemas

Schemas for currently implemented builtin actor events are provided below. They follow the log structure, where each line in the schema table represents an `EventEntry` in the `ActorEvent` entry list. For simplicity, the `flags` are presented as either `k` for `0x01` (index key) or `kv` for `0x03` (index key and value) and the `codec` is always `0x51` for builtin actors so is omitted.

_Note that the "bigint" CBOR encoding format used below is the same as is used for encoding bigints on the Filecoin chain: a byte array representing a big-endian unsigned integer, compatible with the Golang `big.Int` byte representation, with a `0x00` (positive) or `0x01` (negative) prefix; with a zero-length array representing a value of `0`._

The following events are defined in FIP-0083. Additional events will be added here as they are accepted by FIP.

### Verified registry actor events

#### Verifier balance

The `verifier-balance` event is emitted when the balance of a verifier is updated in the Verified Registry actor.

| Key         | Value                               | Flags |
|-------------|-------------------------------------|-------|
| `"$type"`   | `"verifier-balance"` (string)       | kv    |
| `"verifier"`| <VERIFIER_ACTOR_ID> (int)           | kv    |
| `"balance"` | <VERIFIER_DATACAP_BALANCE> (bigint) | k     |

In structured form, this event would look like:

```ipldsch
type DataCap Bytes # A bigint representing a DataCap

type VerifierBalanceEvent struct {
  verifier Int
  balance  DataCap
}
```

#### Allocation

The `allocation` event is emitted when a verified client allocates DataCap to a specific data piece and storage provider.  

| Key          | Value                   | Flags |
| ------------ | ----------------------- | ----- |
| `"$type"`    | `"allocation"` (string) | kv    |
| `"id"`       | <ALLOCATION_ID> (int)   | kv    |
| `"client"`   | <CLIENT_ACTOR_ID> (int) | kv    |
| `"provider"` | <SP_ACTOR_ID> (int)     | kv    |

In structured form, this event would look like:

```ipldsch
type AllocationEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Allocation removed

The `allocation-removed` event is emitted when a DataCap allocation that is past its expiration epoch is removed.

| Key          | Value                           | Flags |
| ------------ | ------------------------------- | ----- |
| `"$type"`    | `"allocation-removed"` (string) | kv    |
| `"id"`       | <ALLOCATION_ID> (int)           | kv    |
| `"client"`   | <CLIENT_ACTOR_ID> (int)         | kv    |
| `"provider"` | <SP_ACTOR_ID> (int)             | kv    |

In structured form, this event would look like:

```ipldsch
type AllocationRemovedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Claim

The `claim` event is emitted when a client allocation is claimed by a storage provider after the corresponding verified data is provably committed to the chain.

| Key          | Value                   | Flags |
| ------------ | ----------------------- | ----- |
| `"$type"`    | `"claim"` (string)      | kv    |
| `"id"`       | <CLAIM_ID> (int)        | kv    |
| `"client"`   | <CLIENT_ACTOR_ID> (int) | kv    |
| `"provider"` | <SP_ACTOR_ID> (int)     | kv    |

In structured form, this event would look like:

```ipldsch
type ClaimEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Claim updated

The `claim-updated` event is emitted when the term of an existing allocation is extended by the client.

| Key          | Value                      | Flags |
| ------------ | -------------------------- | ----- |
| `"$type"`    | `"claim-updated"` (string) | kv    |
| `"id"`       | <CLAIM_ID> (int)           | kv    |
| `"client"`   | <CLIENT_ACTOR_ID> (int)    | kv    |
| `"provider"` | <SP_ACTOR_ID> (int)        | kv    |

In structured form, this event would look like:

```ipldsch
type ClaimUpdatedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

####  Claim removed

The `claim-removed` event is emitted when an expired claim is removed by the Verified Registry actor.

| Key          | Value                      | Flags |
| ------------ | -------------------------- | ----- |
| `"$type"`    | `"claim-removed"` (string) | kv    |
| `"id"`       | <CLAIM_ID> (int)           | kv    |
| `"client"`   | <CLIENT_ACTOR_ID> (int)    | kv    |
| `"provider"` | <SP_ACTOR_ID> (int)        | kv    |

In structured form, this event would look like:

```ipldsch
type ClaimRemovedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

### Market actor events

The Market actor emits the following deal lifecycle events:

#### Deal published

The `deal-published` event is emitted for each new deal that is successfully published by a storage provider. 

| Key         | Value                             | Flags |
| ----------- | --------------------------------- | ----- |
| `"$type"`   | `"deal-published"` (string)       | kv    |
| `"id"`      | <DEAL_ID> (int)                   | kv    |
| `"client"`  | <STORAGE_CLIENT_ACTOR_ID> (int)   | kv    |
| `"provider"`| <STORAGE_PROVIDER_ACTOR_ID> (int) | kv    |

In structured form, this event would look like:

```ipldsch
type DealPublishedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Deal activated

The `deal-activated` event is emitted for each deal that is successfully activated. 

| Key          | Value                             | Flags |
| ------------ | --------------------------------- | ----- |
| `"$type"`    | `"deal-activated"` (string)       | kv    |
| `"id"`       | <DEAL_ID> (int)                   | kv    |
| `"client"`   | <STORAGE_CLIENT_ACTOR_ID> (int)   | kv    |
| `"provider"` | <STORAGE_PROVIDER_ACTOR_ID> (int) | kv    |

In structured form, this event would look like:

```ipldsch
type DealActivatedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Deal terminated

The `deal-terminated` event is emitted by the market actor cron job when it processes deals that were marked as terminated by the `OnMinerSectorsTerminate` method. 

[FIP-0074](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0074.md) ensures that terminated deals are processed immediately in the `OnMinerSectorsTerminate` method rather than being submitted for deferred processing to the market actor cron job. As of network version 22 this event will be emitted to indicate that a deal has been terminated for deals made after network version 22.

| Key         | Value                             | Flags |
| ----------- | --------------------------------- | ----- |
| `"$type"`   | `"deal-terminated"` (string)      | kv    |
| `"id"`      | <DEAL_ID> (int)                   | kv    |
| `"client"`  | <STORAGE_CLIENT_ACTOR_ID> (int)   | kv    |
| `"provider"`| <STORAGE_PROVIDER_ACTOR_ID> (int) | kv    |

In structured form, this event would look like:

```ipldsch
type DealTerminatedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

#### Deal completed

The `deal-completed` event is emitted when a deal is marked as successfully complete by the Market actor cron job. The cron job will deem a deal to be successfully completed if it is past it’s end epoch without being slashed.

[FIP-0074](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0074.md) ensures that the processing of completed deals is done as part of a method called by the storage provider thus making this event available to clients and also to ensure that storage providers pay the gas costs of processing deal completion and event emission. This applies to new deals made after network version 22. For deals made before network version 22, this event will be emitted by the market actor cron job.

| Key         | Value                             | Flags |
| ----------- | --------------------------------- | ----- |
| `"$type"`   | `"deal-completed"` (string)       | kv    |
| `"id"`      | <DEAL_ID> (int)                   | kv    |
| `"client"`  | <STORAGE_CLIENT_ACTOR_ID> (int)   | kv    |
| `"provider"`| <STORAGE_PROVIDER_ACTOR_ID> (int) | kv    |

In structured form, this event would look like:

```ipldsch
type DealCompletedEvent struct {
  id       Int
  client   Int
  provider Int
}
```

### Miner actor events

The Miner actor emits the following sector lifecycle events:

#### Sector precommitted

The `sector-precommitted` event is emitted for each new sector that is successfully pre-committed by a storage provider.

| Key        | Value                            | Flags |
| ---------- | -------------------------------- | ----- |
| `"$type"`  | `"sector-precommitted"` (string) | kv    |
| `"sector"` | <SECTOR_NUMER> (int)             | kv    |

In structured form, this event would look like:

```ipldsch
type SectorPrecommittedEvent struct {
  sector Int
}
```

#### Sector activated

The `sector-activated` event is emitted for each pre-committed sector that is successfully activated by a storage provider. For now, sector activation corresponds 1:1 with prove-committing a sector but this can change in the future.

| Key              | Value                                                         | Flags |
| ---------------- | ------------------------------------------------------------- | ----- |
| `"$type"`        | `"sector-activated"` (string)                                 | kv    |
| `"sector"`       | <SECTOR_NUMER> (int)                                          | kv    |
| `"unsealed-cid"` | <SECTOR_COMMD> (nullable CID) (null means sector has no data) | kv    |
| `"piece-cid"`    | <PIECE_CID> (CID)                                             | kv    |
| `"piece-size"`   | <PIECE_SIZE> (int)                                            | k     |

_Note that both `"piece-cid"` and `"piece-size"` entries will be included for each piece in the sector, so the keys are repeated._

In structured form, this event would look like:

```ipldsch
type PieceDescription struct {
  cid  &Any
  size Int
}

type SectorActivatedEvent struct {
  sector       Int
  unsealedCid  nullable &Any
  pieces       [PieceDescription]
}
```

#### Sector updated

The `sector-updated` event is emitted for each CC sector that is updated to contained actual sealed data.

| Key             | Value                                                         | Flags |
| --------------- | ------------------------------------------------------------- | ----- |
| `"$type"`       | `"sector-updated"` (string)                                   | kv    |
| `"sector"`      | <SECTOR_NUMER> (int)                                          | kv    |
| `"unsealed-cid"`| <SECTOR_COMMD> (nullable CID) (null means sector has no data) | kv    |
| `"piece-cid"`   | <PIECE_CID> (CID)                                             | kv    |
| `"piece-size"`  | <PIECE_SIZE> (int)                                            | k     |

_Note that both `"piece-cid"` and `"piece-size"` entries will be included for each piece in the sector, so the keys are repeated._

In structured form, this event would look like:

```ipldsch
type PieceDescription struct {
  cid  &Any
  size Int
}

type SectorUpdatedEvent struct {
  sector       Int
  unsealedCid  nullable &Any
  pieces       [PieceDescription]
}
```

#### Sector terminated

The `sector-terminated` event is emitted for each sector that is marked as terminated by a storage provider. 

| Key         | Value                          | Flags |
| ----------- | ------------------------------ | ----- |
| `"$type"`   | `"sector-terminated"` (string) | kv    |
| `"sector"`  | <SECTOR_NUMER> (int)           | kv    |

In structured form, this event would look like:

```ipldsch
type SectorTerminatedEvent struct {
  sector Int
}
```
