package api

import (
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

// DealInfo is a tuple of deal identity and its schedule
type PieceDealInfo struct {
	// "Old" builtin-market deal info
	PublishCid   *cid.Cid
	DealID       abi.DealID
	DealProposal *market.DealProposal

	// Common deal info
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
}

// LastStartEpoch returns the last epoch in which the sector containing this deal
// must be sealed in order for the deal to be valid. (todo review: precommitted or committed??)
func (ds *PieceDealInfo) LastStartEpoch() (abi.ChainEpoch, error) {
	switch {
	case ds.isBuiltinMarketDeal():
		return ds.DealSchedule.StartEpoch, nil
	default:
		panic("todo")
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
		return fmt.Sprintf("DirectDataOnboarding{PieceCID: %s, VAllloc: %x}", ds.PieceActivationManifest.CID, ds.PieceActivationManifest.VerifiedAllocationKey)
	}
}
