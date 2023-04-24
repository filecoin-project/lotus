package retrievalimpl

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var _ requestvalidation.ValidationEnvironment = new(providerValidationEnvironment)

type providerValidationEnvironment struct {
	p *Provider
}

func (pve *providerValidationEnvironment) GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid,
	piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (retrievalmarket.Ask, error) {

	pieces, piecesErr := pve.p.getAllPieceInfoForPayload(payloadCid)
	// err may be non-nil, but we may have successfuly found >0 pieces, so defer error handling till
	// we have no other option.
	storageDeals := pve.p.getStorageDealsForPiece(pieceCid != nil, pieces, piece)
	if len(storageDeals) == 0 {
		if piecesErr != nil {
			return retrievalmarket.Ask{}, fmt.Errorf("failed to fetch deals for payload [%s]: %w", payloadCid.String(), piecesErr)
		}
		return retrievalmarket.Ask{}, fmt.Errorf("no storage deals found for payload [%s]", payloadCid.String())
	}

	input := retrievalmarket.PricingInput{
		// piece from which the payload will be retrieved
		PieceCID: piece.PieceCID,

		PayloadCID: payloadCid,
		Unsealed:   isUnsealed,
		Client:     client,
	}

	return pve.p.GetDynamicAsk(ctx, input, storageDeals)
}

func (pve *providerValidationEnvironment) GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error) {
	inPieceCid := cid.Undef
	if pieceCID != nil {
		inPieceCid = *pieceCID
	}

	pieces, piecesErr := pve.p.getAllPieceInfoForPayload(c)
	// err may be non-nil, but we may have successfuly found >0 pieces, so defer error handling till
	// we have no other option.
	pieceInfo, isUnsealed := pve.p.getBestPieceInfoMatch(context.TODO(), pieces, inPieceCid)
	if pieceInfo.Defined() {
		return pieceInfo, isUnsealed, nil
	}
	if piecesErr != nil {
		return piecestore.PieceInfoUndefined, false, piecesErr
	}
	return piecestore.PieceInfoUndefined, false, fmt.Errorf("unknown pieceCID %s", pieceCID.String())
}

// CheckDealParams verifies the given deal params are acceptable
func (pve *providerValidationEnvironment) CheckDealParams(ask retrievalmarket.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error {
	if pricePerByte.LessThan(ask.PricePerByte) {
		return errors.New("Price per byte too low")
	}
	if paymentInterval > ask.PaymentInterval {
		return errors.New("Payment interval too large")
	}
	if paymentIntervalIncrease > ask.PaymentIntervalIncrease {
		return errors.New("Payment interval increase too large")
	}
	if !ask.UnsealPrice.Nil() && unsealPrice.LessThan(ask.UnsealPrice) {
		return errors.New("Unseal price too small")
	}
	return nil
}

// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
func (pve *providerValidationEnvironment) RunDealDecisioningLogic(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
	if pve.p.dealDecider == nil {
		return true, "", nil
	}
	return pve.p.dealDecider(ctx, state)
}

// StateMachines returns the FSM Group to begin tracking with
func (pve *providerValidationEnvironment) BeginTracking(pds retrievalmarket.ProviderDealState) error {
	err := pve.p.stateMachines.Begin(pds.Identifier(), &pds)
	if err != nil {
		return err
	}

	if pds.UnsealPrice.GreaterThan(big.Zero()) {
		return pve.p.stateMachines.Send(pds.Identifier(), retrievalmarket.ProviderEventPaymentRequested)
	}

	return pve.p.stateMachines.Send(pds.Identifier(), retrievalmarket.ProviderEventOpen)
}

func (pve *providerValidationEnvironment) Get(dealID retrievalmarket.ProviderDealIdentifier) (retrievalmarket.ProviderDealState, error) {
	var deal retrievalmarket.ProviderDealState
	err := pve.p.stateMachines.GetSync(context.TODO(), dealID, &deal)
	return deal, err
}

var _ providerstates.ProviderDealEnvironment = new(providerDealEnvironment)

type providerDealEnvironment struct {
	p *Provider
}

// Node returns the node interface for this deal
func (pde *providerDealEnvironment) Node() retrievalmarket.RetrievalProviderNode {
	return pde.p.node
}

// PrepareBlockstore is called when the deal data has been unsealed and we need
// to add all blocks to a blockstore that is used to serve retrieval
func (pde *providerDealEnvironment) PrepareBlockstore(ctx context.Context, dealID retrievalmarket.DealID, pieceCid cid.Cid) error {
	// Load the blockstore that has the deal data
	bs, err := pde.p.dagStore.LoadShard(ctx, pieceCid)
	if err != nil {
		return xerrors.Errorf("failed to load blockstore for piece %s: %w", pieceCid, err)
	}

	log.Debugf("adding blockstore for deal %d to tracker", dealID)
	_, err = pde.p.stores.Track(dealID.String(), bs)
	log.Debugf("added blockstore for deal %d to tracker", dealID)
	return err
}

func (pde *providerDealEnvironment) ChannelState(ctx context.Context, chid datatransfer.ChannelID) (datatransfer.ChannelState, error) {
	return pde.p.dataTransfer.ChannelState(ctx, chid)
}

func (pde *providerDealEnvironment) UpdateValidationStatus(ctx context.Context, chid datatransfer.ChannelID, result datatransfer.ValidationResult) error {
	return pde.p.dataTransfer.UpdateValidationStatus(ctx, chid, result)
}

func (pde *providerDealEnvironment) ResumeDataTransfer(ctx context.Context, chid datatransfer.ChannelID) error {
	return pde.p.dataTransfer.ResumeDataTransferChannel(ctx, chid)
}

func (pde *providerDealEnvironment) CloseDataTransfer(ctx context.Context, chid datatransfer.ChannelID) error {
	// When we close the data transfer, we also send a cancel message to the peer.
	// Make sure we don't wait too long to send the message.
	ctx, cancel := context.WithTimeout(ctx, shared.CloseDataTransferTimeout)
	defer cancel()

	err := pde.p.dataTransfer.CloseDataTransferChannel(ctx, chid)
	if shared.IsCtxDone(err) {
		log.Warnf("failed to send cancel data transfer channel %s to client within timeout %s",
			chid, shared.CloseDataTransferTimeout)
		return nil
	}
	return err
}

func (pde *providerDealEnvironment) DeleteStore(dealID retrievalmarket.DealID) error {
	// close the read-only blockstore and stop tracking it for the deal
	if err := pde.p.stores.Untrack(dealID.String()); err != nil {
		return xerrors.Errorf("failed to clean read-only blockstore for deal %d: %w", dealID, err)
	}

	return nil
}

var _ dtutils.StoreGetter = &providerStoreGetter{}

type providerStoreGetter struct {
	p *Provider
}

func (psg *providerStoreGetter) Get(otherPeer peer.ID, dealID retrievalmarket.DealID) (bstore.Blockstore, error) {
	var deal retrievalmarket.ProviderDealState
	provDealID := retrievalmarket.ProviderDealIdentifier{Receiver: otherPeer, DealID: dealID}
	err := psg.p.stateMachines.Get(provDealID).Get(&deal)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deal state: %w", err)
	}

	//
	// When a request for data is received
	// 1. The data transfer layer calls Get to get the blockstore
	// 2. The data for the deal is unsealed
	// 3. The unsealed data is put into the blockstore (in this case a CAR file)
	// 4. The data is served from the blockstore (using blockstore.Get)
	//
	// So we use a "lazy" blockstore that can be returned in step 1
	// but is only accessed in step 4 after the data has been unsealed.
	//
	return newLazyBlockstore(func() (dagstore.ReadBlockstore, error) {
		return psg.p.stores.Get(dealID.String())
	}), nil
}
