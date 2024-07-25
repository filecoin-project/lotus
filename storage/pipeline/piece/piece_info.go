package piece

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

// PieceDealInfo is a tuple of deal identity and its schedule
type PieceDealInfo struct {
	// "Old" builtin-market deal info
	PublishCid   *cid.Cid
	DealID       abi.DealID
	DealProposal *market.DealProposal

	// Common deal info, required for all pieces
	// TODO: https://github.com/filecoin-project/lotus/issues/11237
	DealSchedule DealSchedule

	// Direct Data Onboarding
	// When PieceActivationManifest is set, builtin-market deal info must not be set
	PieceActivationManifest *miner.PieceActivationManifest

	// Best-effort deal asks
	KeepUnsealed bool
}

// DealSchedule communicates the time interval of a storage deal. The deal must
// appear in a sealed (proven) sector no later than StartEpoch, otherwise it
// is invalid.
type DealSchedule struct {
	StartEpoch abi.ChainEpoch
	EndEpoch   abi.ChainEpoch
}

func (ds *PieceDealInfo) isBuiltinMarketDeal() bool {
	return ds.PublishCid != nil
}

// Valid validates the deal info after being accepted through RPC, checks that
// the deal metadata is well-formed.
func (ds *PieceDealInfo) Valid(nv network.Version) error {
	hasLegacyDealInfo := ds.PublishCid != nil && ds.DealID != 0 && ds.DealProposal != nil
	hasPieceActivationManifest := ds.PieceActivationManifest != nil

	if hasLegacyDealInfo && hasPieceActivationManifest {
		return xerrors.Errorf("piece deal info has both legacy deal info and piece activation manifest")
	}

	if !hasLegacyDealInfo && !hasPieceActivationManifest {
		return xerrors.Errorf("piece deal info has neither legacy deal info nor piece activation manifest")
	}

	if hasLegacyDealInfo {
		if _, err := ds.DealProposal.Cid(); err != nil {
			return xerrors.Errorf("checking proposal CID: %w", err)
		}
	}

	if ds.DealSchedule.StartEpoch <= 0 {
		return xerrors.Errorf("invalid deal start epoch %d", ds.DealSchedule.StartEpoch)
	}
	if ds.DealSchedule.EndEpoch <= 0 {
		return xerrors.Errorf("invalid deal end epoch %d", ds.DealSchedule.EndEpoch)
	}
	if ds.DealSchedule.EndEpoch <= ds.DealSchedule.StartEpoch {
		return xerrors.Errorf("invalid deal end epoch %d (start %d)", ds.DealSchedule.EndEpoch, ds.DealSchedule.StartEpoch)
	}

	if hasPieceActivationManifest {
		if nv < network.Version22 {
			return xerrors.Errorf("direct-data-onboarding pieces aren't accepted before network version 22")
		}

		// todo any more checks seem reasonable to put here?
	}

	return nil
}

type AllocationAPI interface {
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
}

func (ds *PieceDealInfo) GetAllocation(ctx context.Context, aapi AllocationAPI, tsk types.TipSetKey) (*verifregtypes.Allocation, error) {
	switch {
	case ds.isBuiltinMarketDeal():
		return aapi.StateGetAllocationForPendingDeal(ctx, ds.DealID, tsk)
	default:
		if ds.PieceActivationManifest.VerifiedAllocationKey == nil {
			return nil, nil
		}

		caddr, err := address.NewIDAddress(uint64(ds.PieceActivationManifest.VerifiedAllocationKey.Client))
		if err != nil {
			return nil, err
		}

		all, err := aapi.StateGetAllocation(ctx, caddr, verifregtypes.AllocationId(ds.PieceActivationManifest.VerifiedAllocationKey.ID), tsk)
		if err != nil {
			return nil, err
		}

		if all == nil {
			return nil, nil
		}

		if all.Client != ds.PieceActivationManifest.VerifiedAllocationKey.Client {
			return nil, xerrors.Errorf("allocation client mismatch: %d != %d", all.Client, ds.PieceActivationManifest.VerifiedAllocationKey.Client)
		}

		return all, nil
	}
}

// StartEpoch returns the last epoch in which the sector containing this deal
// must be sealed (committed) in order for the deal to be valid.
func (ds *PieceDealInfo) StartEpoch() (abi.ChainEpoch, error) {
	switch {
	case ds.isBuiltinMarketDeal():
		return ds.DealSchedule.StartEpoch, nil
	default:
		// note - when implementing make sure to cache any dynamically computed values
		// todo do we want a smarter mechanism here
		return ds.DealSchedule.StartEpoch, nil
	}
}

// EndEpoch returns the minimum epoch until which the sector containing this
// deal must be committed until.
func (ds *PieceDealInfo) EndEpoch() (abi.ChainEpoch, error) {
	switch {
	case ds.isBuiltinMarketDeal():
		return ds.DealSchedule.EndEpoch, nil
	default:
		// note - when implementing make sure to cache any dynamically computed values
		// todo do we want a smarter mechanism here
		return ds.DealSchedule.EndEpoch, nil
	}
}

func (ds *PieceDealInfo) PieceCID() cid.Cid {
	switch {
	case ds.isBuiltinMarketDeal():
		return ds.DealProposal.PieceCID
	default:
		return ds.PieceActivationManifest.CID
	}
}

func (ds *PieceDealInfo) String() string {
	switch {
	case ds.isBuiltinMarketDeal():
		return fmt.Sprintf("BuiltinMarket{DealID: %d, PieceCID: %s, PublishCid: %s}", ds.DealID, ds.DealProposal.PieceCID, ds.PublishCid)
	default:
		// todo check that VAlloc doesn't print as a pointer
		return fmt.Sprintf("DirectDataOnboarding{PieceCID: %s, VAllloc: %x}", ds.PieceActivationManifest.CID, ds.PieceActivationManifest.VerifiedAllocationKey)
	}
}

func (ds *PieceDealInfo) Size() abi.PaddedPieceSize {
	switch {
	case ds.isBuiltinMarketDeal():
		return ds.DealProposal.PieceSize
	default:
		return ds.PieceActivationManifest.Size
	}
}

func (ds *PieceDealInfo) KeepUnsealedRequested() bool {
	return ds.KeepUnsealed
}

type PieceKey string

// Key returns a unique identifier for this deal info, for use in maps.
func (ds *PieceDealInfo) Key() PieceKey {
	return PieceKey(ds.String())
}

func (ds *PieceDealInfo) Impl() PieceDealInfo {
	return *ds
}
