package providerstates

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("retrieval-fsm")

// ProviderDealEnvironment is a bridge to the environment a provider deal is executing in
// It provides access to relevant functionality on the retrieval provider
type ProviderDealEnvironment interface {
	// Node returns the node interface for this deal
	Node() rm.RetrievalProviderNode
	PrepareBlockstore(ctx context.Context, dealID rm.DealID, pieceCid cid.Cid) error
	TrackTransfer(deal rm.ProviderDealState) error
	UntrackTransfer(deal rm.ProviderDealState) error
	DeleteStore(dealID rm.DealID) error
	ResumeDataTransfer(context.Context, datatransfer.ChannelID) error
	CloseDataTransfer(context.Context, datatransfer.ChannelID) error
}

// UnsealData fetches the piece containing data needed for the retrieval,
// unsealing it if necessary
func UnsealData(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	if err := environment.PrepareBlockstore(ctx.Context(), deal.ID, deal.PieceInfo.PieceCID); err != nil {
		return ctx.Trigger(rm.ProviderEventUnsealError, err)
	}
	log.Debugf("blockstore prepared successfully, firing unseal complete for deal %d", deal.ID)
	return ctx.Trigger(rm.ProviderEventUnsealComplete)
}

// TrackTransfer resumes a deal so we can start sending data after its unsealed
func TrackTransfer(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.TrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	return nil
}

// UnpauseDeal resumes a deal so we can start sending data after its unsealed
func UnpauseDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	log.Debugf("unpausing data transfer for deal %d", deal.ID)
	err := environment.TrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	if deal.ChannelID != nil {
		log.Debugf("resuming data transfer for deal %d", deal.ID)
		err = environment.ResumeDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil {
			return ctx.Trigger(rm.ProviderEventDataTransferError, err)
		}
	}
	return nil
}

// CancelDeal clears a deal that went wrong for an unknown reason
func CancelDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	// Read next response (or fail)
	err := environment.UntrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	err = environment.DeleteStore(deal.ID)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventMultiStoreError, err)
	}
	if deal.ChannelID != nil {
		err = environment.CloseDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil && !errors.Is(err, statemachine.ErrTerminated) {
			return ctx.Trigger(rm.ProviderEventDataTransferError, err)
		}
	}
	return ctx.Trigger(rm.ProviderEventCancelComplete)
}

// CleanupDeal runs to do memory cleanup for an in progress deal
func CleanupDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.UntrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	err = environment.DeleteStore(deal.ID)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventMultiStoreError, err)
	}
	return ctx.Trigger(rm.ProviderEventCleanupComplete)
}
