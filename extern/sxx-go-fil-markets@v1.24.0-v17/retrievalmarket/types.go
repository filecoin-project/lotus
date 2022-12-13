package retrievalmarket

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
)

//go:generate cbor-gen-for --map-encoding Query QueryResponse DealProposal DealResponse Params QueryParams DealPayment ClientDealState ProviderDealState PaymentInfo RetrievalPeer Ask

// QueryProtocolID is the protocol for querying information about retrieval
// deal parameters
const QueryProtocolID = protocol.ID("/fil/retrieval/qry/1.0.0")

// OldQueryProtocolID is the old query protocol for tuple structs
const OldQueryProtocolID = protocol.ID("/fil/retrieval/qry/0.0.1")

// Unsubscribe is a function that unsubscribes a subscriber for either the
// client or the provider
type Unsubscribe func()

// PaymentInfo is the payment channel and lane for a deal, once it is setup
type PaymentInfo struct {
	PayCh address.Address
	Lane  uint64
}

// ClientDealState is the current state of a deal from the point of view
// of a retrieval client
type ClientDealState struct {
	DealProposal
	StoreID *uint64
	// Set when the data transfer is started
	ChannelID            *datatransfer.ChannelID
	LastPaymentRequested bool
	AllBlocksReceived    bool
	TotalFunds           abi.TokenAmount
	ClientWallet         address.Address
	MinerWallet          address.Address
	PaymentInfo          *PaymentInfo
	Status               DealStatus
	Sender               peer.ID
	TotalReceived        uint64
	Message              string
	BytesPaidFor         uint64
	CurrentInterval      uint64
	PaymentRequested     abi.TokenAmount
	FundsSpent           abi.TokenAmount
	UnsealFundsPaid      abi.TokenAmount
	WaitMsgCID           *cid.Cid // the CID of any message the client deal is waiting for
	VoucherShortfall     abi.TokenAmount
	LegacyProtocol       bool
}

func (deal *ClientDealState) NextInterval() uint64 {
	return deal.Params.NextInterval(deal.CurrentInterval)
}

// ProviderDealState is the current state of a deal from the point of view
// of a retrieval provider
type ProviderDealState struct {
	DealProposal
	StoreID uint64

	ChannelID       *datatransfer.ChannelID
	PieceInfo       *piecestore.PieceInfo
	Status          DealStatus
	Receiver        peer.ID
	TotalSent       uint64
	FundsReceived   abi.TokenAmount
	Message         string
	CurrentInterval uint64
	LegacyProtocol  bool
}

func (deal *ProviderDealState) IntervalLowerBound() uint64 {
	return deal.Params.IntervalLowerBound(deal.CurrentInterval)
}

func (deal *ProviderDealState) NextInterval() uint64 {
	return deal.Params.NextInterval(deal.CurrentInterval)
}

// Identifier provides a unique id for this provider deal
func (pds ProviderDealState) Identifier() ProviderDealIdentifier {
	return ProviderDealIdentifier{Receiver: pds.Receiver, DealID: pds.ID}
}

// ProviderDealIdentifier is a value that uniquely identifies a deal
type ProviderDealIdentifier struct {
	Receiver peer.ID
	DealID   DealID
}

func (p ProviderDealIdentifier) String() string {
	return fmt.Sprintf("%v/%v", p.Receiver, p.DealID)
}

// RetrievalPeer is a provider address/peer.ID pair (everything needed to make
// deals for with a miner)
type RetrievalPeer struct {
	Address  address.Address
	ID       peer.ID // optional
	PieceCID *cid.Cid
}

// QueryResponseStatus indicates whether a queried piece is available
type QueryResponseStatus uint64

const (
	// QueryResponseAvailable indicates a provider has a piece and is prepared to
	// return it
	QueryResponseAvailable QueryResponseStatus = iota

	// QueryResponseUnavailable indicates a provider either does not have or cannot
	// serve the queried piece to the client
	QueryResponseUnavailable

	// QueryResponseError indicates something went wrong generating a query response
	QueryResponseError
)

// QueryItemStatus (V1) indicates whether the requested part of a piece (payload or selector)
// is available for retrieval
type QueryItemStatus uint64

const (
	// QueryItemAvailable indicates requested part of the piece is available to be
	// served
	QueryItemAvailable QueryItemStatus = iota

	// QueryItemUnavailable indicates the piece either does not contain the requested
	// item or it cannot be served
	QueryItemUnavailable

	// QueryItemUnknown indicates the provider cannot determine if the given item
	// is part of the requested piece (for example, if the piece is sealed and the
	// miner does not maintain a payload CID index)
	QueryItemUnknown
)

// QueryParams - V1 - indicate what specific information about a piece that a retrieval
// client is interested in, as well as specific parameters the client is seeking
// for the retrieval deal
type QueryParams struct {
	PieceCID *cid.Cid // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//Selector                   ipld.Node // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//MaxPricePerByte            abi.TokenAmount    // optional, tell miner uninterested if more expensive than this
	//MinPaymentInterval         uint64    // optional, tell miner uninterested unless payment interval is greater than this
	//MinPaymentIntervalIncrease uint64    // optional, tell miner uninterested unless payment interval increase is greater than this
}

// Query is a query to a given provider to determine information about a piece
// they may have available for retrieval
type Query struct {
	PayloadCID  cid.Cid // V0
	QueryParams         // V1
}

// QueryUndefined is a query with no values
var QueryUndefined = Query{}

// NewQueryV0 creates a V0 query (which only specifies a payload)
func NewQueryV0(payloadCID cid.Cid) Query {
	return Query{PayloadCID: payloadCID}
}

// NewQueryV1 creates a V1 query (which has an optional pieceCID)
func NewQueryV1(payloadCID cid.Cid, pieceCID *cid.Cid) Query {
	return Query{
		PayloadCID: payloadCID,
		QueryParams: QueryParams{
			PieceCID: pieceCID,
		},
	}
}

// QueryResponse is a miners response to a given retrieval query
type QueryResponse struct {
	Status        QueryResponseStatus
	PieceCIDFound QueryItemStatus // V1 - if a PieceCID was requested, the result
	//SelectorFound   QueryItemStatus // V1 - if a Selector was requested, the result

	Size uint64 // Total size of piece in bytes
	//ExpectedPayloadSize uint64 // V1 - optional, if PayloadCID + selector are specified and miner knows, can offer an expected size

	PaymentAddress             address.Address // address to send funds to -- may be different than miner addr
	MinPricePerByte            abi.TokenAmount
	MaxPaymentInterval         uint64
	MaxPaymentIntervalIncrease uint64
	Message                    string
	UnsealPrice                abi.TokenAmount
}

// QueryResponseUndefined is an empty QueryResponse
var QueryResponseUndefined = QueryResponse{}

// PieceRetrievalPrice is the total price to retrieve the piece (size * MinPricePerByte + UnsealedPrice)
func (qr QueryResponse) PieceRetrievalPrice() abi.TokenAmount {
	return big.Add(big.Mul(qr.MinPricePerByte, abi.NewTokenAmount(int64(qr.Size))), qr.UnsealPrice)
}

// PayloadRetrievalPrice is the expected price to retrieve just the given payload
// & selector (V1)
//func (qr QueryResponse) PayloadRetrievalPrice() abi.TokenAmount {
//	return types.BigMul(qr.MinPricePerByte, types.NewInt(qr.ExpectedPayloadSize))
//}

// IsTerminalError returns true if this status indicates processing of this deal
// is complete with an error
func IsTerminalError(status DealStatus) bool {
	return status == DealStatusDealNotFound ||
		status == DealStatusFailing ||
		status == DealStatusRejected
}

// IsTerminalSuccess returns true if this status indicates processing of this deal
// is complete with a success
func IsTerminalSuccess(status DealStatus) bool {
	return status == DealStatusCompleted
}

// IsTerminalStatus returns true if this status indicates processing of a deal is
// complete (either success or error)
func IsTerminalStatus(status DealStatus) bool {
	return IsTerminalError(status) || IsTerminalSuccess(status)
}

// Params are the parameters requested for a retrieval deal proposal
type Params struct {
	Selector                *cbg.Deferred // V1
	PieceCID                *cid.Cid
	PricePerByte            abi.TokenAmount
	PaymentInterval         uint64 // when to request payment
	PaymentIntervalIncrease uint64
	UnsealPrice             abi.TokenAmount
}

func (p Params) SelectorSpecified() bool {
	return p.Selector != nil && !bytes.Equal(p.Selector.Raw, cbg.CborNull)
}

func (p Params) IntervalLowerBound(currentInterval uint64) uint64 {
	intervalSize := p.PaymentInterval
	var lowerBound uint64
	var target uint64
	for target < currentInterval {
		lowerBound = target
		target += intervalSize
		intervalSize += p.PaymentIntervalIncrease
	}
	return lowerBound
}

func (p Params) NextInterval(currentInterval uint64) uint64 {
	intervalSize := p.PaymentInterval
	var nextInterval uint64
	for nextInterval <= currentInterval {
		nextInterval += intervalSize
		intervalSize += p.PaymentIntervalIncrease
	}
	return nextInterval
}

// NewParamsV0 generates parameters for a retrieval deal, which is always a whole piece deal
func NewParamsV0(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) Params {
	return Params{
		PricePerByte:            pricePerByte,
		PaymentInterval:         paymentInterval,
		PaymentIntervalIncrease: paymentIntervalIncrease,
		UnsealPrice:             big.Zero(),
	}
}

// NewParamsV1 generates parameters for a retrieval deal, including a selector
func NewParamsV1(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, sel ipld.Node, pieceCid *cid.Cid, unsealPrice abi.TokenAmount) (Params, error) {
	var buffer bytes.Buffer

	if sel == nil {
		return Params{}, xerrors.New("selector required for NewParamsV1")
	}

	err := dagcbor.Encode(sel, &buffer)
	if err != nil {
		return Params{}, xerrors.Errorf("error encoding selector: %w", err)
	}

	return Params{
		Selector:                &cbg.Deferred{Raw: buffer.Bytes()},
		PieceCID:                pieceCid,
		PricePerByte:            pricePerByte,
		PaymentInterval:         paymentInterval,
		PaymentIntervalIncrease: paymentIntervalIncrease,
		UnsealPrice:             unsealPrice,
	}, nil
}

// DealID is an identifier for a retrieval deal (unique to a client)
type DealID uint64

func (d DealID) String() string {
	return fmt.Sprintf("%d", d)
}

// DealProposal is a proposal for a new retrieval deal
type DealProposal struct {
	PayloadCID cid.Cid
	ID         DealID
	Params
}

// Type method makes DealProposal usable as a voucher
func (dp *DealProposal) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealProposal/1"
}

// DealProposalUndefined is an undefined deal proposal
var DealProposalUndefined = DealProposal{}

// DealResponse is a response to a retrieval deal proposal
type DealResponse struct {
	Status DealStatus
	ID     DealID

	// payment required to proceed
	PaymentOwed abi.TokenAmount

	Message string
}

// Type method makes DealResponse usable as a voucher result
func (dr *DealResponse) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealResponse/1"
}

// DealResponseUndefined is an undefined deal response
var DealResponseUndefined = DealResponse{}

// DealPayment is a payment for an in progress retrieval deal
type DealPayment struct {
	ID             DealID
	PaymentChannel address.Address
	PaymentVoucher *paychtypes.SignedVoucher
}

// Type method makes DealPayment usable as a voucher
func (dr *DealPayment) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealPayment/1"
}

// DealPaymentUndefined is an undefined deal payment
var DealPaymentUndefined = DealPayment{}

var (
	// ErrNotFound means a piece was not found during retrieval
	ErrNotFound = errors.New("not found")

	// ErrVerification means a retrieval contained a block response that did not verify
	ErrVerification = errors.New("Error when verify data")
)

type Ask struct {
	PricePerByte            abi.TokenAmount
	UnsealPrice             abi.TokenAmount
	PaymentInterval         uint64
	PaymentIntervalIncrease uint64
}

// ShortfallErorr is an error that indicates a short fall of funds
type ShortfallError struct {
	shortfall abi.TokenAmount
}

// NewShortfallError returns a new error indicating a shortfall of funds
func NewShortfallError(shortfall abi.TokenAmount) error {
	return ShortfallError{shortfall}
}

// Shortfall returns the numerical value of the shortfall
func (se ShortfallError) Shortfall() abi.TokenAmount {
	return se.shortfall
}
func (se ShortfallError) Error() string {
	return fmt.Sprintf("Inssufficient Funds. Shortfall: %s", se.shortfall.String())
}

// ChannelAvailableFunds provides information about funds in a channel
type ChannelAvailableFunds struct {
	// ConfirmedAmt is the amount of funds that have been confirmed on-chain
	// for the channel
	ConfirmedAmt abi.TokenAmount
	// PendingAmt is the amount of funds that are pending confirmation on-chain
	PendingAmt abi.TokenAmount
	// PendingWaitSentinel can be used with PaychGetWaitReady to wait for
	// confirmation of pending funds
	PendingWaitSentinel *cid.Cid
	// QueuedAmt is the amount that is queued up behind a pending request
	QueuedAmt abi.TokenAmount
	// VoucherRedeemedAmt is the amount that is redeemed by vouchers on-chain
	// and in the local datastore
	VoucherReedeemedAmt abi.TokenAmount
}

// PricingInput provides input parameters required to price a retrieval deal.
type PricingInput struct {
	// PayloadCID is the cid of the payload to retrieve.
	PayloadCID cid.Cid
	// PieceCID is the cid of the Piece from which the Payload will be retrieved.
	PieceCID cid.Cid
	// PieceSize is the size of the Piece from which the payload will be retrieved.
	PieceSize abi.UnpaddedPieceSize
	// Client is the peerID of the retrieval client.
	Client peer.ID
	// VerifiedDeal is true if there exists a verified storage deal for the PayloadCID.
	VerifiedDeal bool
	// Unsealed is true if there exists an unsealed sector from which we can retrieve the given payload.
	Unsealed bool
	// CurrentAsk is the current configured ask in the ask-store.
	CurrentAsk Ask
}
