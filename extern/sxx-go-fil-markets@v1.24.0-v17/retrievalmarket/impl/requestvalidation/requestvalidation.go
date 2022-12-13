package requestvalidation

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	peer "github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
)

var allSelectorBytes []byte

var askTimeout = 5 * time.Second

func init() {
	buf := new(bytes.Buffer)
	_ = dagcbor.Encode(selectorparse.CommonSelector_ExploreAllRecursively, buf)
	allSelectorBytes = buf.Bytes()
}

// ValidationEnvironment contains the dependencies needed to validate deals
type ValidationEnvironment interface {
	GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid, piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (retrievalmarket.Ask, error)

	GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error)
	// CheckDealParams verifies the given deal params are acceptable
	CheckDealParams(ask retrievalmarket.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error
	// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
	RunDealDecisioningLogic(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error)
	// StateMachines returns the FSM Group to begin tracking with
	BeginTracking(pds retrievalmarket.ProviderDealState) error
}

// ProviderRequestValidator validates incoming requests for the Retrieval Provider
type ProviderRequestValidator struct {
	env ValidationEnvironment
}

// NewProviderRequestValidator returns a new instance of the ProviderRequestValidator
func NewProviderRequestValidator(env ValidationEnvironment) *ProviderRequestValidator {
	return &ProviderRequestValidator{env}
}

// ValidatePush validates a push request received from the peer that will send data
func (rv *ProviderRequestValidator) ValidatePush(isRestart bool, _ datatransfer.ChannelID, sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, errors.New("No pushes accepted")
}

// ValidatePull validates a pull request received from the peer that will receive data
func (rv *ProviderRequestValidator) ValidatePull(isRestart bool, _ datatransfer.ChannelID, receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	proposal, ok := voucher.(*retrievalmarket.DealProposal)
	var legacyProtocol bool
	if !ok {
		legacyProposal, ok := voucher.(*migrations.DealProposal0)
		if !ok {
			return nil, errors.New("wrong voucher type")
		}
		newProposal := migrations.MigrateDealProposal0To1(*legacyProposal)
		proposal = &newProposal
		legacyProtocol = true
	}
	response, err := rv.validatePull(isRestart, receiver, proposal, legacyProtocol, baseCid, selector)
	if response == nil {
		return nil, err
	}
	if legacyProtocol {
		downgradedResponse := migrations.DealResponse0{
			Status:      response.Status,
			ID:          response.ID,
			Message:     response.Message,
			PaymentOwed: response.PaymentOwed,
		}
		return &downgradedResponse, err
	}
	return response, err
}

// validatePull is called by the data provider when a new graphsync pull
// request is created. This can be the initial pull request or a new request
// created when the data transfer is restarted (eg after a connection failure).
// By default the graphsync request starts immediately sending data, unless
// validatePull returns ErrPause or the data-transfer has not yet started
// (because the provider is still unsealing the data).
func (rv *ProviderRequestValidator) validatePull(isRestart bool, receiver peer.ID, proposal *retrievalmarket.DealProposal, legacyProtocol bool, baseCid cid.Cid, selector ipld.Node) (*retrievalmarket.DealResponse, error) {
	// Check the proposal CID matches
	if proposal.PayloadCID != baseCid {
		return nil, errors.New("incorrect CID for this proposal")
	}

	// Check the proposal selector matches
	buf := new(bytes.Buffer)
	err := dagcbor.Encode(selector, buf)
	if err != nil {
		return nil, err
	}
	bytesCompare := allSelectorBytes
	if proposal.SelectorSpecified() {
		bytesCompare = proposal.Selector.Raw
	}
	if !bytes.Equal(buf.Bytes(), bytesCompare) {
		return nil, errors.New("incorrect selector for this proposal")
	}

	// If the validation is for a restart request, return nil, which means
	// the data-transfer should not be explicitly paused or resumed
	if isRestart {
		return nil, nil
	}

	// This is a new graphsync request (not a restart)
	pds := retrievalmarket.ProviderDealState{
		DealProposal:    *proposal,
		Receiver:        receiver,
		LegacyProtocol:  legacyProtocol,
		CurrentInterval: proposal.PaymentInterval,
	}

	// Decide whether to accept the deal
	status, err := rv.acceptDeal(&pds)

	response := retrievalmarket.DealResponse{
		ID:     proposal.ID,
		Status: status,
	}

	if status == retrievalmarket.DealStatusFundsNeededUnseal {
		response.PaymentOwed = pds.UnsealPrice
	}

	if err != nil {
		response.Message = err.Error()
		return &response, err
	}

	err = rv.env.BeginTracking(pds)
	if err != nil {
		return nil, err
	}

	// Pause the data transfer while unsealing the data.
	// The state machine will unpause the transfer when unsealing completes.
	return &response, datatransfer.ErrPause
}

func (rv *ProviderRequestValidator) acceptDeal(deal *retrievalmarket.ProviderDealState) (retrievalmarket.DealStatus, error) {
	pieceInfo, isUnsealed, err := rv.env.GetPiece(deal.PayloadCID, deal.PieceCID)
	if err != nil {
		if err == retrievalmarket.ErrNotFound {
			return retrievalmarket.DealStatusDealNotFound, err
		}
		return retrievalmarket.DealStatusErrored, err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), askTimeout)
	defer cancel()

	ask, err := rv.env.GetAsk(ctx, deal.PayloadCID, deal.PieceCID, pieceInfo, isUnsealed, deal.Receiver)
	if err != nil {
		return retrievalmarket.DealStatusErrored, err
	}

	// check that the deal parameters match our required parameters or
	// reject outright
	err = rv.env.CheckDealParams(ask, deal.PricePerByte, deal.PaymentInterval, deal.PaymentIntervalIncrease, deal.UnsealPrice)
	if err != nil {
		return retrievalmarket.DealStatusRejected, err
	}

	accepted, reason, err := rv.env.RunDealDecisioningLogic(context.TODO(), *deal)
	if err != nil {
		return retrievalmarket.DealStatusErrored, err
	}
	if !accepted {
		return retrievalmarket.DealStatusRejected, errors.New(reason)
	}

	deal.PieceInfo = &pieceInfo

	if deal.UnsealPrice.GreaterThan(big.Zero()) {
		return retrievalmarket.DealStatusFundsNeededUnseal, nil
	}

	return retrievalmarket.DealStatusAccepted, nil
}
