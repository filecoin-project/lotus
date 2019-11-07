package smm

import (
    "context"
    "github.com/ipfs/go-cid"
)

type StorageMiningEvents interface {
    // Called when the chain state changes.
    // Epoch may change by >1.
    // In case of a re-org, state will always change back to the fork
    // point before advancing down the new chain.
    OnChainStateChanged(Epoch, StateKey)
}

// add the Node interface
type Node interface {
    // Subscribes to chain state changes.
    // The subscription is scoped to a single miner actor:
    // only changes in that actor’s state, or related to the actor
    // in the storage power or market actors, will cause events.
    SubscribeMiner(ctx context.Context, cb StorageMiningEvents) error

    // Fetches key for the most recent state known by the node.
    MostRecentState(ctx context.Context) (*StateKey, Epoch, error)

    // Cancels a subscription
    UnsubscribeMiner(ctx context.Context, cb StorageMiningEvents) error

    // Gets miner-related on-chain state.
    GetMinerState(ctx context.Context, state *StateKey) (MinerChainState, error)

    // Submits a self-deal to the chain.
    SubmitSelfDeal(ctx context.Context, size uint64) error

    // Retrieves a ticket used in sealing and proving operations.
    GetRandomness(ctx context.Context, state *StateKey, e Epoch, offset uint) ([]byte, error)

    // Submits replicated sector information and requests a seal seed
    // be generated on-chain.
    // This is asynchronous as the request must appear on
    // chain and then await some delay before the seed is provided.
    // The parameters are a subset of OnChainSealVerifyInfo.
    // The miner chooses sector ID.
    SubmitSectorPreCommitment(ctx context.Context, miner Address, id SectorID, commR cid.Cid, deals []cid.Cid)

    // Reads a seal seed previously requested with
    // SubmitSectorPreCommitment.
    // Returns empty if the request and delay have not yet elapsed.
    GetSealSeed(ctx context.Context, miner Address, state StateKey, id SectorID) SealSeed

    // Submits final commitment of a sector, with a proof including the
    // seal seed.
    SubmitSectorCommitment(ctx context.Context, miner Address, id SectorID, proof Proof)

    // Returns the current proving period and, if the miner has
    // been challenged, the challenge seed and period.
    GetProvingPeriod(ctx context.Context, state *StateKey) (ProvingPeriod, error)

    // Submits a PoSt proof to the chain.
    SubmitPoSt(ctx context.Context, miner Address, proof Proof)

    // Submits declaration of IDs of faulty sectors to the chain.
    //SubmitDeclaredFaults(ctx context.Context, miner Address, faults types.BitField)

    // Submits declaration of IDs of recovered sectors to the chain.
    //SubmitDeclaredRecoveries(ctx context.Context, miner Address, recovered types.BitField)
}
