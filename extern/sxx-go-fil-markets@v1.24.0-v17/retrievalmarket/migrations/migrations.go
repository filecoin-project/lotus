package migrations

import (
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-state-types/abi"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecemigrations "github.com/filecoin-project/go-fil-markets/piecestore/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations/maptypes"
)

//go:generate cbor-gen-for Query0 QueryResponse0 DealProposal0 DealResponse0 Params0 QueryParams0 DealPayment0 ClientDealState0 ProviderDealState0 PaymentInfo0 RetrievalPeer0 Ask0

// PaymentInfo0 is version 0 of PaymentInfo
type PaymentInfo0 struct {
	PayCh address.Address
	Lane  uint64
}

// ClientDealState0 is version 0 of ClientDealState
type ClientDealState0 struct {
	DealProposal0
	StoreID              *uint64
	ChannelID            datatransfer.ChannelID
	LastPaymentRequested bool
	AllBlocksReceived    bool
	TotalFunds           abi.TokenAmount
	ClientWallet         address.Address
	MinerWallet          address.Address
	PaymentInfo          *PaymentInfo0
	Status               retrievalmarket.DealStatus
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
}

// ProviderDealState0 is version 0 of ProviderDealState
type ProviderDealState0 struct {
	DealProposal0
	StoreID         uint64
	ChannelID       datatransfer.ChannelID
	PieceInfo       *piecemigrations.PieceInfo0
	Status          retrievalmarket.DealStatus
	Receiver        peer.ID
	TotalSent       uint64
	FundsReceived   abi.TokenAmount
	Message         string
	CurrentInterval uint64
}

// RetrievalPeer0 is version 0 of RetrievalPeer
type RetrievalPeer0 struct {
	Address  address.Address
	ID       peer.ID // optional
	PieceCID *cid.Cid
}

// QueryParams0 is version 0 of QueryParams
type QueryParams0 struct {
	PieceCID *cid.Cid // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//Selector                   ipld.Node // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//MaxPricePerByte            abi.TokenAmount    // optional, tell miner uninterested if more expensive than this
	//MinPaymentInterval         uint64    // optional, tell miner uninterested unless payment interval is greater than this
	//MinPaymentIntervalIncrease uint64    // optional, tell miner uninterested unless payment interval increase is greater than this
}

// Query0 is version 0 of Query
type Query0 struct {
	PayloadCID   cid.Cid // V0
	QueryParams0         // V1
}

// QueryResponse0 is version 0 of QueryResponse
type QueryResponse0 struct {
	Status        retrievalmarket.QueryResponseStatus
	PieceCIDFound retrievalmarket.QueryItemStatus // V1 - if a PieceCID was requested, the result
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

// Params0 is version 0 of Params
type Params0 struct {
	Selector                *cbg.Deferred // V1
	PieceCID                *cid.Cid
	PricePerByte            abi.TokenAmount
	PaymentInterval         uint64 // when to request payment
	PaymentIntervalIncrease uint64
	UnsealPrice             abi.TokenAmount
}

// DealProposal0 is version 0 of DealProposal
type DealProposal0 struct {
	PayloadCID cid.Cid
	ID         retrievalmarket.DealID
	Params0
}

// Type method makes DealProposal0 usable as a voucher
func (dp *DealProposal0) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealProposal"
}

// DealResponse0 is version 0 of DealResponse
type DealResponse0 struct {
	Status retrievalmarket.DealStatus
	ID     retrievalmarket.DealID

	// payment required to proceed
	PaymentOwed abi.TokenAmount

	Message string
}

// Type method makes DealResponse0 usable as a voucher result
func (dr *DealResponse0) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealResponse"
}

// DealPayment0 is version 0 of DealPayment
type DealPayment0 struct {
	ID             retrievalmarket.DealID
	PaymentChannel address.Address
	PaymentVoucher *paychtypes.SignedVoucher
}

// Type method makes DealPayment0 usable as a voucher
func (dr *DealPayment0) Type() datatransfer.TypeIdentifier {
	return "RetrievalDealPayment"
}

// Ask0 is version 0 of Ask
type Ask0 struct {
	PricePerByte            abi.TokenAmount
	UnsealPrice             abi.TokenAmount
	PaymentInterval         uint64
	PaymentIntervalIncrease uint64
}

// MigrateQueryParams0To1 migrates tuple encoded query params to map encoded query params
func MigrateQueryParams0To1(oldParams QueryParams0) retrievalmarket.QueryParams {
	return retrievalmarket.QueryParams{
		PieceCID: oldParams.PieceCID,
	}
}

// MigrateQuery0To1 migrates tuple encoded query to map encoded query
func MigrateQuery0To1(oldQuery Query0) retrievalmarket.Query {
	return retrievalmarket.Query{
		PayloadCID:  oldQuery.PayloadCID,
		QueryParams: MigrateQueryParams0To1(oldQuery.QueryParams0),
	}
}

// MigrateQueryResponse0To1 migrates tuple encoded query response to map encoded query response
func MigrateQueryResponse0To1(oldQr QueryResponse0) retrievalmarket.QueryResponse {
	return retrievalmarket.QueryResponse{
		Status:                     oldQr.Status,
		PieceCIDFound:              oldQr.PieceCIDFound,
		Size:                       oldQr.Size,
		PaymentAddress:             oldQr.PaymentAddress,
		MinPricePerByte:            oldQr.MinPricePerByte,
		MaxPaymentInterval:         oldQr.MaxPaymentInterval,
		MaxPaymentIntervalIncrease: oldQr.MaxPaymentIntervalIncrease,
		Message:                    oldQr.Message,
		UnsealPrice:                oldQr.UnsealPrice,
	}
}

// MigrateParams0To1 migrates tuple encoded deal params to map encoded deal params
func MigrateParams0To1(oldParams Params0) retrievalmarket.Params {
	return retrievalmarket.Params{
		Selector:                oldParams.Selector,
		PieceCID:                oldParams.PieceCID,
		PricePerByte:            oldParams.PricePerByte,
		PaymentInterval:         oldParams.PaymentInterval,
		PaymentIntervalIncrease: oldParams.PaymentIntervalIncrease,
		UnsealPrice:             oldParams.UnsealPrice,
	}
}

// MigrateDealPayment0To1 migrates a tuple encoded DealPayment to a map
// encoded deal payment
func MigrateDealPayment0To1(oldDp DealPayment0) retrievalmarket.DealPayment {
	return retrievalmarket.DealPayment{
		ID:             oldDp.ID,
		PaymentChannel: oldDp.PaymentChannel,
		PaymentVoucher: oldDp.PaymentVoucher,
	}
}

// MigrateDealProposal0To1 migrates a tuple encoded DealProposal to a map
// encoded deal proposal
func MigrateDealProposal0To1(oldDp DealProposal0) retrievalmarket.DealProposal {
	return retrievalmarket.DealProposal{
		PayloadCID: oldDp.PayloadCID,
		ID:         oldDp.ID,
		Params:     MigrateParams0To1(oldDp.Params0),
	}
}

// MigrateDealResponse0To1 migrates a tuple encoded DealResponse to a map
// encoded deal response
func MigrateDealResponse0To1(oldDr DealResponse0) retrievalmarket.DealResponse {
	return retrievalmarket.DealResponse{
		Status:      oldDr.Status,
		ID:          oldDr.ID,
		PaymentOwed: oldDr.PaymentOwed,
		Message:     oldDr.Message,
	}
}

// MigratePaymentInfo0To1 migrates an optional payment info tuple encoded struct
// to a map encoded struct
func MigratePaymentInfo0To1(oldPi *PaymentInfo0) *retrievalmarket.PaymentInfo {
	if oldPi == nil {
		return nil
	}
	return &retrievalmarket.PaymentInfo{
		PayCh: oldPi.PayCh,
		Lane:  oldPi.Lane,
	}
}

// MigrateClientDealState0To1 migrates a tuple encoded deal state to a map encoded deal state
func MigrateClientDealState0To1(oldDs *ClientDealState0) (*maptypes.ClientDealState1, error) {
	return &maptypes.ClientDealState1{
		DealProposal:         MigrateDealProposal0To1(oldDs.DealProposal0),
		StoreID:              oldDs.StoreID,
		ChannelID:            oldDs.ChannelID,
		LastPaymentRequested: oldDs.LastPaymentRequested,
		AllBlocksReceived:    oldDs.AllBlocksReceived,
		TotalFunds:           oldDs.TotalFunds,
		ClientWallet:         oldDs.ClientWallet,
		MinerWallet:          oldDs.MinerWallet,
		PaymentInfo:          MigratePaymentInfo0To1(oldDs.PaymentInfo),
		Status:               oldDs.Status,
		Sender:               oldDs.Sender,
		TotalReceived:        oldDs.TotalReceived,
		Message:              oldDs.Message,
		BytesPaidFor:         oldDs.BytesPaidFor,
		CurrentInterval:      oldDs.CurrentInterval,
		PaymentRequested:     oldDs.PaymentRequested,
		FundsSpent:           oldDs.FundsSpent,
		UnsealFundsPaid:      oldDs.UnsealFundsPaid,
		WaitMsgCID:           oldDs.WaitMsgCID,
		VoucherShortfall:     oldDs.VoucherShortfall,
		LegacyProtocol:       true,
	}, nil
}

// MigrateClientDealState1To2 migrates from v1 to v2 of a ClientDealState.
// The difference is that in v2 the ChannelID is a pointer, because the
// ChannelID is not set until the data transfer has started, so it should
// initially be nil.
func MigrateClientDealState1To2(oldDs *maptypes.ClientDealState1) (*retrievalmarket.ClientDealState, error) {
	var chid *datatransfer.ChannelID
	if oldDs.ChannelID.Initiator != "" && oldDs.ChannelID.Responder != "" {
		chid = &oldDs.ChannelID
	}
	return &retrievalmarket.ClientDealState{
		DealProposal:         oldDs.DealProposal,
		StoreID:              oldDs.StoreID,
		ChannelID:            chid,
		LastPaymentRequested: oldDs.LastPaymentRequested,
		AllBlocksReceived:    oldDs.AllBlocksReceived,
		TotalFunds:           oldDs.TotalFunds,
		ClientWallet:         oldDs.ClientWallet,
		MinerWallet:          oldDs.MinerWallet,
		PaymentInfo:          oldDs.PaymentInfo,
		Status:               oldDs.Status,
		Sender:               oldDs.Sender,
		TotalReceived:        oldDs.TotalReceived,
		Message:              oldDs.Message,
		BytesPaidFor:         oldDs.BytesPaidFor,
		CurrentInterval:      oldDs.CurrentInterval,
		PaymentRequested:     oldDs.PaymentRequested,
		FundsSpent:           oldDs.FundsSpent,
		UnsealFundsPaid:      oldDs.UnsealFundsPaid,
		WaitMsgCID:           oldDs.WaitMsgCID,
		VoucherShortfall:     oldDs.VoucherShortfall,
		LegacyProtocol:       true,
	}, nil
}

// MigrateProviderDealState0To1 migrates a tuple encoded deal state to a map encoded deal state
func MigrateProviderDealState0To1(oldDs *ProviderDealState0) (*maptypes.ProviderDealState1, error) {
	var pieceInfo *piecestore.PieceInfo
	var err error
	if oldDs.PieceInfo != nil {
		pieceInfo, err = piecemigrations.MigratePieceInfo0To1(oldDs.PieceInfo)
		if err != nil {
			return nil, err
		}
	}
	return &maptypes.ProviderDealState1{
		DealProposal:    MigrateDealProposal0To1(oldDs.DealProposal0),
		StoreID:         oldDs.StoreID,
		ChannelID:       oldDs.ChannelID,
		PieceInfo:       pieceInfo,
		Status:          oldDs.Status,
		Receiver:        oldDs.Receiver,
		TotalSent:       oldDs.TotalSent,
		FundsReceived:   oldDs.FundsReceived,
		Message:         oldDs.Message,
		CurrentInterval: oldDs.CurrentInterval,
		LegacyProtocol:  true,
	}, nil
}

// MigrateProviderDealState0To1 migrates from v1 to v2 of a
// MigrateProviderDealState.
// The difference is that in v2 the ChannelID is a pointer, because the
// ChannelID is not set until the data transfer has started, so it should
// initially be nil.
func MigrateProviderDealState1To2(oldDs *maptypes.ProviderDealState1) (*retrievalmarket.ProviderDealState, error) {
	var chid *datatransfer.ChannelID
	if oldDs.ChannelID.Initiator != "" && oldDs.ChannelID.Responder != "" {
		chid = &oldDs.ChannelID
	}
	return &retrievalmarket.ProviderDealState{
		DealProposal:    oldDs.DealProposal,
		StoreID:         oldDs.StoreID,
		ChannelID:       chid,
		PieceInfo:       oldDs.PieceInfo,
		Status:          oldDs.Status,
		Receiver:        oldDs.Receiver,
		TotalSent:       oldDs.TotalSent,
		FundsReceived:   oldDs.FundsReceived,
		Message:         oldDs.Message,
		CurrentInterval: oldDs.CurrentInterval,
		LegacyProtocol:  oldDs.LegacyProtocol,
	}, nil
}

// MigrateAsk0To1 migrates a tuple encoded deal state to a map encoded deal state
func MigrateAsk0To1(oldAsk *Ask0) (*retrievalmarket.Ask, error) {
	return &retrievalmarket.Ask{
		PricePerByte:            oldAsk.PricePerByte,
		UnsealPrice:             oldAsk.UnsealPrice,
		PaymentInterval:         oldAsk.PaymentInterval,
		PaymentIntervalIncrease: oldAsk.PaymentIntervalIncrease,
	}, nil
}

// ClientMigrations are migrations for the client's store of retrieval deals
var ClientMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateClientDealState0To1, "1"),
	versioned.NewVersionedBuilder(MigrateClientDealState1To2, "2").OldVersion("1"),
}

// ProviderMigrations are migrations for the providers's store of retrieval deals
var ProviderMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateProviderDealState0To1, "1").
		FilterKeys([]string{"/retrieval-ask", "/retrieval-ask/latest", "/retrieval-ask/1/latest", "/retrieval-ask/versions/current"}),
	versioned.NewVersionedBuilder(MigrateProviderDealState1To2, "2").OldVersion("1"),
}

// AskMigrations are migrations for the providers's retrieval ask
var AskMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateAsk0To1, versioning.VersionKey("1")),
}
