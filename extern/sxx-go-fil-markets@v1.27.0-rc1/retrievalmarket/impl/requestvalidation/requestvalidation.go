package requestvalidation

import (
	"context"
	"errors"
	"time"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	peer "github.com/libp2p/go-libp2p/core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var allSelector = selectorparse.CommonSelector_ExploreAllRecursively

var askTimeout = 5 * time.Second

// ValidationEnvironment contains the dependencies needed to validate deals
type ValidationEnvironment interface {
	GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid, piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (rm.Ask, error)

	GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error)
	// CheckDealParams verifies the given deal params are acceptable
	CheckDealParams(ask rm.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error
	// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
	RunDealDecisioningLogic(ctx context.Context, state rm.ProviderDealState) (bool, string, error)
	// StateMachines returns the FSM Group to begin tracking with
	BeginTracking(pds rm.ProviderDealState) error
	Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error)
}

// ProviderRequestValidator validates incoming requests for the Retrieval Provider
type ProviderRequestValidator struct {
	env  ValidationEnvironment
	psub *pubsub.PubSub
}

// NewProviderRequestValidator returns a new instance of the ProviderRequestValidator
func NewProviderRequestValidator(env ValidationEnvironment) *ProviderRequestValidator {
	return &ProviderRequestValidator{
		env:  env,
		psub: pubsub.New(queryValidationDispatcher),
	}
}

// ValidatePush validates a push request received from the peer that will send data
func (rv *ProviderRequestValidator) ValidatePush(_ datatransfer.ChannelID, sender peer.ID, voucher datamodel.Node, baseCid cid.Cid, selector datamodel.Node) (datatransfer.ValidationResult, error) {
	return datatransfer.ValidationResult{}, errors.New("No pushes accepted")
}

// ValidatePull validates a pull request received from the peer that will receive data
func (rv *ProviderRequestValidator) ValidatePull(_ datatransfer.ChannelID, receiver peer.ID, voucher datamodel.Node, baseCid cid.Cid, selector datamodel.Node) (datatransfer.ValidationResult, error) {
	proposal, err := rm.DealProposalFromNode(voucher)
	if err != nil {
		return datatransfer.ValidationResult{}, err
	}

	response, err := rv.validatePull(receiver, proposal, baseCid, selector)
	rv.publishValidationEvent(false, receiver, proposal, baseCid, selector, response, err)
	return response, err
}

func (rv *ProviderRequestValidator) publishValidationEvent(restart bool, receiver peer.ID, proposal *retrievalmarket.DealProposal, baseCid cid.Cid, selector datamodel.Node, response datatransfer.ValidationResult, err error) {
	var dealResponse *rm.DealResponse
	if response.VoucherResult != nil {
		dealResponse, _ = rm.DealResponseFromNode(response.VoucherResult.Voucher)
	}
	if err == nil && response.Accepted == false {
		err = datatransfer.ErrRejected
	}
	rv.psub.Publish(retrievalmarket.ProviderValidationEvent{
		IsRestart: false,
		Receiver:  receiver,
		Proposal:  proposal,
		BaseCid:   baseCid,
		Selector:  selector,
		Response:  dealResponse,
		Error:     err,
	})
}

func rejectProposal(proposal *rm.DealProposal, status rm.DealStatus, reason string) (datatransfer.ValidationResult, error) {
	dr := rm.DealResponse{
		ID:      proposal.ID,
		Status:  status,
		Message: reason,
	}
	node := rm.BindnodeRegistry.TypeToNode(&dr)
	return datatransfer.ValidationResult{
		Accepted:      false,
		VoucherResult: &datatransfer.TypedVoucher{Voucher: node, Type: rm.DealResponseType},
	}, nil
}

// validatePull is called by the data provider when a new graphsync pull
// request is created. This can be the initial pull request or a new request
// created when the data transfer is restarted (eg after a connection failure).
// By default the graphsync request starts immediately sending data, unless
// validatePull returns ErrPause or the data-transfer has not yet started
// (because the provider is still unsealing the data).
func (rv *ProviderRequestValidator) validatePull(receiver peer.ID, proposal *rm.DealProposal, baseCid cid.Cid, selector datamodel.Node) (datatransfer.ValidationResult, error) {
	// Check the proposal CID matches
	if proposal.PayloadCID != baseCid {
		return rejectProposal(proposal, rm.DealStatusRejected, "incorrect CID for this proposal")
	}

	// Check the proposal selector matches
	sel := allSelector
	if proposal.SelectorSpecified() {
		sel = proposal.Selector.Node
	}
	if !ipld.DeepEqual(sel, selector) {
		return rejectProposal(proposal, rm.DealStatusRejected, "incorrect selector specified for this proposal")
	}

	// This is a new graphsync request (not a restart)
	deal := rm.ProviderDealState{
		DealProposal: *proposal,
		Receiver:     receiver,
	}

	pieceInfo, isUnsealed, err := rv.env.GetPiece(deal.PayloadCID, deal.PieceCID)
	if err != nil {
		if err == rm.ErrNotFound {
			return rejectProposal(proposal, rm.DealStatusDealNotFound, err.Error())
		}
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error())
	}

	ctx, cancel := context.WithTimeout(context.TODO(), askTimeout)
	defer cancel()

	ask, err := rv.env.GetAsk(ctx, deal.PayloadCID, deal.PieceCID, pieceInfo, isUnsealed, deal.Receiver)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error())
	}

	// check that the deal parameters match our required parameters or
	// reject outright
	err = rv.env.CheckDealParams(ask, deal.PricePerByte, deal.PaymentInterval, deal.PaymentIntervalIncrease, deal.UnsealPrice)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusRejected, err.Error())
	}

	accepted, reason, err := rv.env.RunDealDecisioningLogic(context.TODO(), deal)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error())
	}
	if !accepted {
		return rejectProposal(proposal, rm.DealStatusRejected, reason)
	}

	deal.PieceInfo = &pieceInfo

	err = rv.env.BeginTracking(deal)
	if err != nil {
		return datatransfer.ValidationResult{}, err
	}

	status := rm.DealStatusAccepted
	if deal.UnsealPrice.GreaterThan(big.Zero()) {
		status = rm.DealStatusFundsNeededUnseal
	}
	// Pause the data transfer while unsealing the data.
	// The state machine will unpause the transfer when unsealing completes.
	dr := rm.DealResponse{
		ID:          proposal.ID,
		Status:      status,
		PaymentOwed: deal.Params.OutstandingBalance(big.Zero(), 0, false),
	}
	node := rm.BindnodeRegistry.TypeToNode(&dr)
	result := datatransfer.ValidationResult{
		Accepted:             true,
		VoucherResult:        &datatransfer.TypedVoucher{Voucher: node, Type: rm.DealResponseType},
		ForcePause:           true,
		DataLimit:            deal.Params.NextInterval(big.Zero()),
		RequiresFinalization: true,
	}
	return result, nil
}

// ValidateRestart validates a request on restart, based on its current state
func (rv *ProviderRequestValidator) ValidateRestart(_ datatransfer.ChannelID, channelState datatransfer.ChannelState) (datatransfer.ValidationResult, error) {
	voucher := channelState.Voucher()
	proposal, err := rm.DealProposalFromNode(voucher.Voucher)
	if err != nil {
		return datatransfer.ValidationResult{}, errors.New("wrong voucher type")
	}
	response, err := rv.validateRestart(proposal, channelState)
	rv.publishValidationEvent(true, channelState.OtherPeer(), proposal, channelState.BaseCID(), channelState.Selector(), response, err)
	return response, err
}

func (rv *ProviderRequestValidator) validateRestart(proposal *rm.DealProposal, channelState datatransfer.ChannelState) (datatransfer.ValidationResult, error) {
	dealID := rm.ProviderDealIdentifier{DealID: proposal.ID, Receiver: channelState.OtherPeer()}

	// read the deal state
	deal, err := rv.env.Get(dealID)
	if err != nil {
		return errorDealResponse(dealID, err)
	}

	// produce validation based on current deal state and channel state
	return datatransfer.ValidationResult{
		Accepted:             true,
		ForcePause:           deal.Status == rm.DealStatusUnsealing || deal.Status == rm.DealStatusFundsNeededUnseal,
		RequiresFinalization: requiresFinalization(deal, channelState),
		DataLimit:            deal.Params.NextInterval(deal.FundsReceived),
	}, nil
}

// requiresFinalization is true unless the deal is in finalization and no further funds are owed
func requiresFinalization(deal rm.ProviderDealState, channelState datatransfer.ChannelState) bool {
	if deal.Status != rm.DealStatusFundsNeededLastPayment && deal.Status != rm.DealStatusFinalizing {
		return true
	}
	owed := deal.Params.OutstandingBalance(deal.FundsReceived, channelState.Queued(), channelState.Status().InFinalization())
	return owed.GreaterThan(big.Zero())
}

func errorDealResponse(dealID rm.ProviderDealIdentifier, err error) (datatransfer.ValidationResult, error) {
	dr := rm.DealResponse{
		ID:      dealID.DealID,
		Message: err.Error(),
		Status:  rm.DealStatusErrored,
	}
	node := rm.BindnodeRegistry.TypeToNode(&dr)
	return datatransfer.ValidationResult{
		Accepted:      false,
		VoucherResult: &datatransfer.TypedVoucher{Voucher: node, Type: rm.DealResponseType},
	}, nil
}

func (rv *ProviderRequestValidator) Subscribe(subscriber retrievalmarket.ProviderValidationSubscriber) retrievalmarket.Unsubscribe {
	return retrievalmarket.Unsubscribe(rv.psub.Subscribe(subscriber))
}

func queryValidationDispatcher(evt pubsub.Event, subscriberFn pubsub.SubscriberFn) error {
	e, ok := evt.(retrievalmarket.ProviderValidationEvent)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb, ok := subscriberFn.(retrievalmarket.ProviderValidationSubscriber)
	if !ok {
		return errors.New("wrong type of callback")
	}
	cb(e)
	return nil
}
