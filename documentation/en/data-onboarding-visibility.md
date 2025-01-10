# Data Onboarding Visibility

* [Introduction and background](#introduction-and-background)
* [DDO information flow](#ddo-information-flow)
* [Relevant message contents](#relevant-message-contents)
* [Relevant builtin actor events](#relevant-builtin-actor-events)

## Introduction and background

**Direct Data Onboarding** (DDO) as defined in **[FIP-0076](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md)** provides an optional data onboarding path that is both gas-efficient and paves a path toward eventual smart contract mediated onboarding mechanisms. The existing market actor and market actor mediated onboarding pathway remains largely unchanged but is now optional; it is anticipated that the gas savings alone will see a significant shift away from use of the market actor.

Historically, a large amount of tooling was built around Filecoin that makes use of the market actor (f05) to quantify various data-related metrics. A shift in behaviour toward data onboarding that bypasses the market actor requires adaption in order to have continuity with the some of the data-related metrics being collected. This will continue to be true as Filecoin evolves to enable onboarding mechanisms mediated by smart contracts.

**Deal-lifecycle actor events** as defined in **[FIP-0083](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0083.md)** and detailed in [Actor Events and Lotus APIs](./actor-events-api.md) introduced the first batch of fire-and-forget externally observable events for builtin actors. The FVM already had this capability and these new first events for builtin actors were added to increase the visibility of information around data onboarding, particularly in light of the introduction of DDO which will require metrics gatherers to rely on mechanisms other than the market actor to collect data.

Actor events are an optional method for gaining insight into data onboarding activities and data / storage lifecycles. Messages may also be used as a source of truth for data onboarding metrics, but are more complex to consume and may not be suitable for some workflows. For verified (Filecoin Plus) data, the verified registry actor (f06) should be used as the primary source of truth for data lifecycles; however FIP-0076 introduces the possibility of "sparkling data" which is not verified and not mediated through the builtin market actor or possibly any other actor. This data currently still requires piece commitments to be detailed as part of a miner's sector commitments, and so may be observed through messages and actor events that carry sector commitment piece manifests.

## DDO information flow

The most basic direct onboarding workflow as viewed by the chain is simply:

- At PreCommit, an SP must specify a sector’s data commitment (unsealed CID, CommP) *(but does not need to specify the structure of that data nor any deals or verified allocations)*.
- At ProveCommit or ReplicaUpdate, an SP specifies the pieces of data (CommP and their size) comprising a sector in order to satisfy the data commitment.

This basic form does not touch either the market actor or the verified registry actor. Importantly, it does not result in piece information being stored on chain, even though the ProveCommit message contains this information for the purpose of verifying the sector commitment. This is the most significant change from onboarding mechanics prior to network version 22.

There are two possible additions to this flow:

- Prior to PreCommit, an SP publishes a storage deal to the builtin market actor, in which case deal information exists on chain as it does with non-DDO deals today; then
  - at ProveCommit, or ReplicaUpdate, the SP can notify an actor of the commitment. Currently this can only be the builtin market actor (in the future this may be a list of arbitrary user defined actors), in which case it will be used to activate a deal previously proposed on chain.
- At ProveCommit, or ReplicaUpdate, the SP can claim DataCap that was previously allocated by the client for a particular piece.

💡 **The builtin market actor should not be used as single a source of truth regarding data onboarding activities.** The builtin market actor is only a source of truth for data onboarding mediated by the builtin market actor.

💡 **The builtin market actor should not be used as a source of truth regarding verified claims and metrics related to FIL+ usage (size, clients, providers).** The `VerifiedClaim` property of `DealState` has been removed from the builtin market actor. Instead, the verified registry should be used as the only source of truth regarding both allocations and claims.

💡 **Sector data commitments and their constituent pieces are only stored on chain in the verified registry claims in the case of verified data (pieces) onboarded in any mechanism (DDO and/or builtin market actor).** Piece information for data onboarded that is not verified ("sparkling data") and not mediated through the builtin market actor will only appear in messages and actor events. Messages and actor events may be used as a source of truth for data sector commitments.

## Relevant message contents

Even though chain state is less informative for data onboarding not mediated through the builtin market actor, messages used for chain execution will continue to provide all relevant information and may be used to determine the number and size of pieces within a sector, as well as any DataCap claimed for specific pieces and therefore the client allocating the DataCap.

The most important messages for this purpose are as follows:

At ProveCommit, a [`ProveCommitSectors3`](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md#provecommitsectors3) message will contain a `SectorActivations` property which is a list of `SectorActivationManifest`, one for each sector being activated. Within this per-sector manifest is a list of `Pieces` which details all of the pieces contributing to the sector commitment, each one is a `PieceActivationManifest` of the form:

```ipldsch
type PieceActivationManifest struct {
    CID                   &Any                           # Piece data commitment (CommP)
    Size                  Int                            # Padded piece size
    VerifiedAllocationKey nullable VerifiedAllocationKey # Identifies a verified allocation to be claimed
    Notify                DataActivationNotification     # Notifications to be sent to other actors after activation
}
```

This manifest contains the piece commitment as well as an optional `VerifiedAllocationKey` which lists a client and an allocation to claim from the verified registry actor.

At ReplicaUpdate, the [`ProveReplicaUpdates3`](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0076.md#provereplicaupdates3) message will contain a `SectorUpdates` property which is a list of `SectorUpdateManifest`, one for each sector being updated. This manifest mirrors the `SectorActivationManifest` for ProveCommit, containing a list of `Pieces` which may be similarly inspected for the relevant data.

- **Pieces**: All piece information for each sector's data commitment may be collected from the piece manifests.
- **Verified data**: Claims may be cross-referenced with the verified registry state to access the details of the allocation, including piece information for the claim. The `StateGetClaim`  Lotus API call provides this information.

💡 Making use of the message contents directly is not a trivial activity. Messages need to be filtered by actor and method number, exit code needs to be checked from the receipt, and the parameters would need to be decoded according to the relevant schema for that message. Actor events exist to make this somewhat easier although may not be suitable for some workflows.

## Relevant builtin actor events

Depending on usage and data consumption workflow, consuming builtin actor events using the APIs detailed in [Actor Events and Lotus APIs](./actor-events-api.md), may be simpler and more suitable. The following events are relevant to DDO and may be used to determine the number and size of pieces within a sector, as well as any DataCap claimed for specific pieces and therefore the client allocating the DataCap.

The [`sector-activated`](./actor-events-api.md#sector-activated) and [`sector-updated`](./actor-events-api.md#sector-updated) events are emitted by the miner actor and contain the piece information for each sector. This is submitted to the miner actor by the storage provider in the form of a piece manifest and is summarised as a list of pieces in the events. Both piece CID (CommP) and piece size are available in the event data.

The [`claim`](./actor-events-api.md#claim) event is emitted by the verified registry actor and contains the client and provider for each claim. This event contains the claim ID which can be used to cross-reference with the verified registry state to access the details of the allocation, including piece information for the claim. The `StateGetClaim`  Lotus API call provides this information.
