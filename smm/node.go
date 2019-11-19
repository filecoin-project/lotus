package smm

import (
    "context"
    "github.com/ipfs/go-cid"
)

type StateChange struct {
    Epoch    Epoch       // epoch for the state
    StateKey StateKey   // key for the state
}

type StateChangeHandler interface {
    // Called when the chain state changes.
    // Epoch may change by >1.
    // In case of a re-org, state will always change back to the fork
    // point before advancing down the new chain.
    OnChainStateChanged(*StateChange)
}

// Interface for querying the chain state and for submitting messages related to storage mining.
type Node interface {
    // Starts listening for state changes
    Start(context.Context) (*StateChange, error)

    // Fetches key for the most recent state known by the node.
    MostRecentState(ctx context.Context) (StateKey, error)

    // Gets worker-related on-chain state.
    GetMinerState(ctx context.Context, state StateKey) (*MinerChainState, error)

    // Submits a self-deals to the chain.
    SubmitSelfDeals(ctx context.Context, deals []uint64) (cid.Cid, error)

    // Retrieves a ticket used in sealing and proving operations.
    GetRandomness(ctx context.Context, state StateKey, offset uint) ([]byte, error)

    // Submits replicated sector information and requests a seal seed
    // be generated on-chain.
    // This is asynchronous as the request must appear on
    // chain and then await some delay before the seed is provided.
    // The parameters are a subset of OnChainSealVerifyInfo.
    // The worker chooses sector ID.
    SubmitSectorPreCommitment(ctx context.Context, id SectorID, sealEpoch Epoch, commR cid.Cid, dealIDs []uint64) (cid.Cid, error)

    // Reads a seal seed previously requested with
    // SubmitSectorPreCommitment.
    // Returns empty if the request and delay have not yet elapsed.
    GetSealSeed(ctx context.Context, state StateKey, id SectorID) SealSeed

    // Submits final commitment of a sector, with a proof including the
    // seal seed.
    SubmitSectorCommitment(ctx context.Context, id SectorID, proof Proof, dealIDs []uint64) (cid.Cid, error)

    // Returns the current proving period and, if the worker has
    // been challenged, the challenge seed and period.
    GetProvingPeriod(ctx context.Context, state StateKey) (*ProvingPeriod, error)

    // Submits a PoSt proof to the chain.
    SubmitPoSt(ctx context.Context, proof Proof) (cid.Cid, error)

    // Submits declaration of IDs of faulty sectors to the chain.
    SubmitDeclaredFaults(ctx context.Context, faults BitField) (cid.Cid, error)

    // Submits declaration of IDs of recovered sectors to the chain.
    SubmitDeclaredRecoveries(ctx context.Context, recovered BitField) (cid.Cid, error)
}
