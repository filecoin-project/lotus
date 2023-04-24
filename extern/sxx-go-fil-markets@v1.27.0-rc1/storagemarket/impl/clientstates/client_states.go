package clientstates

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime/datamodel"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

var log = logging.Logger("storagemarket_impl")

// MaxGraceEpochsForDealAcceptance is the maximum number of epochs we will wait for past the start epoch
// for the provider to publish a deal.
const MaxGraceEpochsForDealAcceptance = 10

// ClientDealEnvironment is an abstraction for interacting with
// dependencies from the storage client environment
type ClientDealEnvironment interface {
	// CleanBlockstore cleans up the read-only CARv2 blockstore that provides random access on top of client deal data.
	// It's important to do this when the client deal finishes successfully or errors out.
	CleanBlockstore(rootCid cid.Cid) error
	Node() storagemarket.StorageClientNode
	NewDealStream(ctx context.Context, p peer.ID) (network.StorageDealStream, error)
	StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.TypedVoucher, baseCid cid.Cid, selector datamodel.Node) (datatransfer.ChannelID, error)
	RestartDataTransfer(ctx context.Context, chid datatransfer.ChannelID) error
	GetProviderDealState(ctx context.Context, proposalCid cid.Cid) (*storagemarket.ProviderDealState, error)
	PollingInterval() time.Duration
	network.PeerTagger
}

// ClientStateEntryFunc is the type for all state entry functions on a storage client
type ClientStateEntryFunc func(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error

// ReserveClientFunds attempts to reserve funds for this deal and ensure they are available in the Storage Market Actor
func ReserveClientFunds(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	node := environment.Node()

	mcid, err := node.ReserveFunds(ctx.Context(), deal.Proposal.Client, deal.Proposal.Client, deal.Proposal.ClientBalanceRequirement())
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventReserveFundsFailed, err)
	}

	_ = ctx.Trigger(storagemarket.ClientEventFundsReserved, deal.Proposal.ClientBalanceRequirement())

	// if no message was sent, and there was no error, funds were already available
	if mcid == cid.Undef {
		return ctx.Trigger(storagemarket.ClientEventFundingComplete)
	}
	// Otherwise wait for funds to be added
	return ctx.Trigger(storagemarket.ClientEventFundingInitiated, mcid)
}

// WaitForFunding waits for an AddFunds message to appear on the chain
func WaitForFunding(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	node := environment.Node()

	return node.WaitForMessage(ctx.Context(), *deal.AddFundsCid, func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error {
		if err != nil {
			return ctx.Trigger(storagemarket.ClientEventReserveFundsFailed, xerrors.Errorf("AddFunds err: %w", err))
		}
		if code != exitcode.Ok {
			return ctx.Trigger(storagemarket.ClientEventReserveFundsFailed, xerrors.Errorf("AddFunds exit code: %s", code.String()))
		}
		return ctx.Trigger(storagemarket.ClientEventFundingComplete)

	})
}

// ProposeDeal sends the deal proposal to the provider
func ProposeDeal(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	proposal := network.Proposal{
		DealProposal:  &deal.ClientDealProposal,
		Piece:         deal.DataRef,
		FastRetrieval: deal.FastRetrieval,
	}

	s, err := environment.NewDealStream(ctx.Context(), deal.Miner)
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventWriteProposalFailed, err)
	}

	environment.TagPeer(deal.Miner, deal.ProposalCid.String())

	if err := s.WriteDealProposal(proposal); err != nil {
		return ctx.Trigger(storagemarket.ClientEventWriteProposalFailed, err)
	}

	resp, origBytes, err := s.ReadDealResponse()
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventReadResponseFailed, err)
	}

	err = s.Close()
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventStreamCloseError, err)
	}

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventResponseVerificationFailed)
	}

	verified, err := environment.Node().VerifySignature(ctx.Context(), *resp.Signature, deal.MinerWorker, origBytes, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventResponseVerificationFailed)
	}

	if !verified {
		return ctx.Trigger(storagemarket.ClientEventResponseVerificationFailed)
	}

	if resp.Response.State != storagemarket.StorageDealWaitingForData {
		return ctx.Trigger(storagemarket.ClientEventUnexpectedDealState, resp.Response.State, resp.Response.Message)
	}

	return ctx.Trigger(storagemarket.ClientEventInitiateDataTransfer)
}

// RestartDataTransfer restarts a data transfer to the provider that was initiated earlier
func RestartDataTransfer(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	log.Infof("restarting data transfer for deal %s", deal.ProposalCid)

	if deal.TransferChannelID == nil {
		return ctx.Trigger(storagemarket.ClientEventDataTransferRestartFailed, xerrors.New("channelId on client deal is nil"))
	}

	// restart the push data transfer. This will complete asynchronously and the
	// completion of the data transfer will trigger a change in deal state
	err := environment.RestartDataTransfer(ctx.Context(),
		*deal.TransferChannelID,
	)
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventDataTransferRestartFailed, err)
	}

	return nil
}

// InitiateDataTransfer initiates data transfer to the provider
func InitiateDataTransfer(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	if deal.DataRef.TransferType == storagemarket.TTManual {
		log.Infof("manual data transfer for deal %s", deal.ProposalCid)
		return ctx.Trigger(storagemarket.ClientEventDataTransferComplete)
	}

	log.Infof("sending data for a deal %s", deal.ProposalCid)

	voucher := requestvalidation.StorageDataTransferVoucher{Proposal: deal.ProposalCid}
	node := requestvalidation.BindnodeRegistry.TypeToNode(&voucher)

	// initiate a push data transfer. This will complete asynchronously and the
	// completion of the data transfer will trigger a change in deal state
	_, err := environment.StartDataTransfer(ctx.Context(),
		deal.Miner,
		datatransfer.TypedVoucher{Voucher: node, Type: requestvalidation.StorageDataTransferVoucherType},
		deal.DataRef.Root,
		selectorparse.CommonSelector_ExploreAllRecursively,
	)

	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventDataTransferFailed, xerrors.Errorf("failed to open push data channel: %w", err))
	}

	return nil
}

// CheckForDealAcceptance is run until the deal is sealed and published by the provider, or errors
func CheckForDealAcceptance(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	_, currEpoch, err := environment.Node().GetChainHead(ctx.Context())

	if err == nil {
		if currEpoch > deal.Proposal.StartEpoch+MaxGraceEpochsForDealAcceptance {
			return ctx.Trigger(storagemarket.ClientEventDealRejected, deal.State, "start epoch already elapsed")
		}
	}

	dealState, err := environment.GetProviderDealState(ctx.Context(), deal.ProposalCid)
	if err != nil {
		log.Warnf("error when querying provider deal state: %w", err) // TODO: at what point do we fail the deal?
		return waitAgain(ctx, environment, true, storagemarket.StorageDealUnknown)
	}

	if isFailed(dealState.State) {
		return ctx.Trigger(storagemarket.ClientEventDealRejected, dealState.State, dealState.Message)
	}

	if isAccepted(dealState.State) {
		if *dealState.ProposalCid != deal.ProposalCid {
			return ctx.Trigger(storagemarket.ClientEventResponseDealDidNotMatch, *dealState.ProposalCid, deal.ProposalCid)
		}

		return ctx.Trigger(storagemarket.ClientEventDealAccepted, dealState.PublishCid)
	}

	return waitAgain(ctx, environment, false, dealState.State)
}

func waitAgain(ctx fsm.Context, environment ClientDealEnvironment, pollError bool, providerState storagemarket.StorageDealStatus) error {
	t := time.NewTimer(environment.PollingInterval())

	go func() {
		select {
		case <-t.C:
			_ = ctx.Trigger(storagemarket.ClientEventWaitForDealState, pollError, providerState)
		case <-ctx.Context().Done():
			t.Stop()
			return
		}
	}()

	return nil
}

// ValidateDealPublished confirms with the chain that a deal was published
func ValidateDealPublished(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {

	dealID, err := environment.Node().ValidatePublishedDeal(ctx.Context(), deal)
	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventDealPublishFailed, err)
	}

	releaseReservedFunds(ctx, environment, deal)

	// at this point data transfer is complete, so unprotect peer connection
	environment.UntagPeer(deal.Miner, deal.ProposalCid.String())

	return ctx.Trigger(storagemarket.ClientEventDealPublished, dealID)
}

// VerifyDealPreCommitted verifies that a deal has been pre-committed
func VerifyDealPreCommitted(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	cb := func(sectorNumber abi.SectorNumber, isActive bool, err error) {
		// It's possible that
		// - we miss the pre-commit message and have to wait for prove-commit
		// - the deal is already active (for example if the node is restarted
		//   while waiting for pre-commit)
		// In either of these two cases, isActive will be true.
		switch {
		case err != nil:
			_ = ctx.Trigger(storagemarket.ClientEventDealPrecommitFailed, err)
		case isActive:
			_ = ctx.Trigger(storagemarket.ClientEventDealActivated)
		default:
			_ = ctx.Trigger(storagemarket.ClientEventDealPrecommitted, sectorNumber)
		}
	}

	err := environment.Node().OnDealSectorPreCommitted(ctx.Context(), deal.Proposal.Provider, deal.DealID, deal.Proposal, deal.PublishMessage, cb)

	if err != nil {
		return ctx.Trigger(storagemarket.ClientEventDealPrecommitFailed, err)
	}
	return nil
}

// VerifyDealActivated confirms that a deal was successfully committed to a sector and is active
func VerifyDealActivated(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	cb := func(err error) {
		if err != nil {
			_ = ctx.Trigger(storagemarket.ClientEventDealActivationFailed, err)
		} else {

			_ = ctx.Trigger(storagemarket.ClientEventDealActivated)
		}
	}

	if err := environment.Node().OnDealSectorCommitted(ctx.Context(), deal.Proposal.Provider, deal.DealID, deal.SectorNumber, deal.Proposal, deal.PublishMessage, cb); err != nil {
		return ctx.Trigger(storagemarket.ClientEventDealActivationFailed, err)
	}

	return nil
}

// WaitForDealCompletion waits for the deal to be slashed or to expire
func WaitForDealCompletion(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	node := environment.Node()

	// deal is now active, clean up the blockstore.
	if err := environment.CleanBlockstore(deal.DataRef.Root); err != nil {
		log.Errorf("storage deal active but failed to cleanup rea-only blockstore, proposalCid=%s, err=%s", deal.ProposalCid, err)
	}

	// Called when the deal expires
	expiredCb := func(err error) {
		if err != nil {
			_ = ctx.Trigger(storagemarket.ClientEventDealCompletionFailed, xerrors.Errorf("deal expiration err: %w", err))
		} else {
			_ = ctx.Trigger(storagemarket.ClientEventDealExpired)
		}
	}

	// Called when the deal is slashed
	slashedCb := func(slashEpoch abi.ChainEpoch, err error) {
		if err != nil {
			_ = ctx.Trigger(storagemarket.ClientEventDealCompletionFailed, xerrors.Errorf("deal slashing err: %w", err))
		} else {
			_ = ctx.Trigger(storagemarket.ClientEventDealSlashed, slashEpoch)
		}
	}

	if err := node.OnDealExpiredOrSlashed(ctx.Context(), deal.DealID, expiredCb, slashedCb); err != nil {
		return ctx.Trigger(storagemarket.ClientEventDealCompletionFailed, err)
	}

	return nil
}

// FailDeal cleans up a failing deal
func FailDeal(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) error {
	releaseReservedFunds(ctx, environment, deal)

	// TODO: store in some sort of audit log
	log.Errorf("deal %s failed: %s", deal.ProposalCid, deal.Message)

	environment.UntagPeer(deal.Miner, deal.ProposalCid.String())

	if err := environment.CleanBlockstore(deal.DataRef.Root); err != nil {
		log.Errorf("failed to cleanup read-only blockstore, proposalCid=%s: %s", deal.ProposalCid, err)
	}

	return ctx.Trigger(storagemarket.ClientEventFailed)
}

func releaseReservedFunds(ctx fsm.Context, environment ClientDealEnvironment, deal storagemarket.ClientDeal) {
	if !deal.FundsReserved.Nil() && !deal.FundsReserved.IsZero() {
		err := environment.Node().ReleaseFunds(ctx.Context(), deal.Proposal.Client, deal.FundsReserved)
		if err != nil {
			// nonfatal error
			log.Warnf("failed to release funds: %s", err)
		}
		_ = ctx.Trigger(storagemarket.ClientEventFundsReleased, deal.FundsReserved)
	}
}

func isAccepted(status storagemarket.StorageDealStatus) bool {
	return status == storagemarket.StorageDealStaged ||
		// add by lin
		status == storagemarket.StorageDealStagedOfSxx ||
		// end
		status == storagemarket.StorageDealAwaitingPreCommit ||
		status == storagemarket.StorageDealSealing ||
		status == storagemarket.StorageDealActive ||
		status == storagemarket.StorageDealExpired ||
		status == storagemarket.StorageDealSlashed
}

func isFailed(status storagemarket.StorageDealStatus) bool {
	return status == storagemarket.StorageDealFailing ||
		status == storagemarket.StorageDealError
}
