package migrations

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	marketOld "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

//go:generate cbor-gen-for ClientDeal0 MinerDeal0 Balance0 SignedStorageAsk0 StorageAsk0 DataRef0 ProviderDealState0 AskRequest0 AskResponse0 Proposal0 Response0 SignedResponse0 DealStatusRequest0 DealStatusResponse0

// Balance0 is version 0 of Balance
type Balance0 struct {
	Locked    abi.TokenAmount
	Available abi.TokenAmount
}

// StorageAsk0 is version 0 of StorageAsk
type StorageAsk0 struct {
	Price         abi.TokenAmount
	VerifiedPrice abi.TokenAmount

	MinPieceSize abi.PaddedPieceSize
	MaxPieceSize abi.PaddedPieceSize
	Miner        address.Address
	Timestamp    abi.ChainEpoch
	Expiry       abi.ChainEpoch
	SeqNo        uint64
}

// SignedStorageAsk0 is version 0 of SignedStorageAsk
type SignedStorageAsk0 struct {
	Ask       *StorageAsk0
	Signature *crypto.Signature
}

// MinerDeal0 is version 0 of MinerDeal
type MinerDeal0 struct {
	marketOld.ClientDealProposal
	ProposalCid           cid.Cid
	AddFundsCid           *cid.Cid
	PublishCid            *cid.Cid
	Miner                 peer.ID
	Client                peer.ID
	State                 storagemarket.StorageDealStatus
	PiecePath             filestore.Path
	MetadataPath          filestore.Path
	SlashEpoch            abi.ChainEpoch
	FastRetrieval         bool
	Message               string
	StoreID               *uint64
	FundsReserved         abi.TokenAmount
	Ref                   *DataRef0
	AvailableForRetrieval bool

	DealID       abi.DealID
	CreationTime cbg.CborTime
}

// ClientDeal0 is version 0 of ClientDeal
type ClientDeal0 struct {
	market.ClientDealProposal
	ProposalCid    cid.Cid
	AddFundsCid    *cid.Cid
	State          storagemarket.StorageDealStatus
	Miner          peer.ID
	MinerWorker    address.Address
	DealID         abi.DealID
	DataRef        *DataRef0
	Message        string
	PublishMessage *cid.Cid
	SlashEpoch     abi.ChainEpoch
	PollRetryCount uint64
	PollErrorCount uint64
	FastRetrieval  bool
	StoreID        *uint64
	FundsReserved  abi.TokenAmount
	CreationTime   cbg.CborTime
}

// DataRef0 is version 0 of DataRef
type DataRef0 struct {
	TransferType string
	Root         cid.Cid
	PieceCid     *cid.Cid
	PieceSize    abi.UnpaddedPieceSize
}

// ProviderDealState0 is version 0 of ProviderDealState
type ProviderDealState0 struct {
	State         storagemarket.StorageDealStatus
	Message       string
	Proposal      *market.DealProposal
	ProposalCid   *cid.Cid
	AddFundsCid   *cid.Cid
	PublishCid    *cid.Cid
	DealID        abi.DealID
	FastRetrieval bool
}

// Proposal0 is version 0 of Proposal
type Proposal0 struct {
	DealProposal  *market.ClientDealProposal
	Piece         *DataRef0
	FastRetrieval bool
}

// Response0 is version 0 of Response
type Response0 struct {
	State storagemarket.StorageDealStatus

	// DealProposalRejected
	Message  string
	Proposal cid.Cid

	// StorageDealProposalAccepted
	PublishMessage *cid.Cid
}

// SignedResponse0 is version 0 of SignedResponse
type SignedResponse0 struct {
	Response  Response0
	Signature *crypto.Signature
}

// AskRequest0 is version 0 of AskRequest
type AskRequest0 struct {
	Miner address.Address
}

// AskResponse0 is version 0 of AskResponse
type AskResponse0 struct {
	Ask *SignedStorageAsk0
}

// DealStatusRequest0 is version 0 of DealStatusRequest
type DealStatusRequest0 struct {
	Proposal  cid.Cid
	Signature crypto.Signature
}

// DealStatusResponse0 is version 0 of DealStatusResponse
type DealStatusResponse0 struct {
	DealState ProviderDealState0
	Signature crypto.Signature
}

// MigrateDataRef0To1 migrates a tuple encoded data tref to a map encoded data ref
func MigrateDataRef0To1(oldDr *DataRef0) *storagemarket.DataRef {
	if oldDr == nil {
		return nil
	}
	return &storagemarket.DataRef{
		TransferType: oldDr.TransferType,
		Root:         oldDr.Root,
		PieceCid:     oldDr.PieceCid,
		PieceSize:    oldDr.PieceSize,
	}
}

// MigrateClientDeal0To1 migrates a tuple encoded client deal to a map encoded client deal
func MigrateClientDeal0To1(oldCd *ClientDeal0) (*storagemarket.ClientDeal, error) {
	return &storagemarket.ClientDeal{
		ClientDealProposal: oldCd.ClientDealProposal,
		ProposalCid:        oldCd.ProposalCid,
		AddFundsCid:        oldCd.AddFundsCid,
		State:              oldCd.State,
		Miner:              oldCd.Miner,
		MinerWorker:        oldCd.MinerWorker,
		DealID:             oldCd.DealID,
		DataRef:            MigrateDataRef0To1(oldCd.DataRef),
		Message:            oldCd.Message,
		PublishMessage:     oldCd.PublishMessage,
		SlashEpoch:         oldCd.SlashEpoch,
		PollRetryCount:     oldCd.PollRetryCount,
		PollErrorCount:     oldCd.PollErrorCount,
		FastRetrieval:      oldCd.FastRetrieval,
		FundsReserved:      oldCd.FundsReserved,
		CreationTime:       oldCd.CreationTime,
	}, nil
}

// MigrateMinerDeal0To1 migrates a tuple encoded miner deal to a map encoded miner deal
func MigrateMinerDeal0To1(oldCd *MinerDeal0) (*MinerDeal1, error) {
	return &MinerDeal1{
		ClientDealProposal:    oldCd.ClientDealProposal,
		ProposalCid:           oldCd.ProposalCid,
		AddFundsCid:           oldCd.AddFundsCid,
		PublishCid:            oldCd.PublishCid,
		Miner:                 oldCd.Miner,
		Client:                oldCd.Client,
		State:                 oldCd.State,
		PiecePath:             oldCd.PiecePath,
		MetadataPath:          oldCd.MetadataPath,
		SlashEpoch:            oldCd.SlashEpoch,
		FastRetrieval:         oldCd.FastRetrieval,
		Message:               oldCd.Message,
		FundsReserved:         oldCd.FundsReserved,
		Ref:                   MigrateDataRef0To1(oldCd.Ref),
		AvailableForRetrieval: oldCd.AvailableForRetrieval,
		DealID:                oldCd.DealID,
		CreationTime:          oldCd.CreationTime,
	}, nil
}

// MigrateMinerDeal1To2 migrates a miner deal label to the new format
func MigrateMinerDeal1To2(oldCd *MinerDeal1) (*storagemarket.MinerDeal, error) {
	clientDealProp, err := MigrateClientDealProposal0To1(oldCd.ClientDealProposal)
	if err != nil {
		return nil, fmt.Errorf("migrating deal with proposal cid %s: %w", oldCd.ProposalCid, err)
	}

	return &storagemarket.MinerDeal{
		ClientDealProposal:    *clientDealProp,
		ProposalCid:           oldCd.ProposalCid,
		AddFundsCid:           oldCd.AddFundsCid,
		PublishCid:            oldCd.PublishCid,
		Miner:                 oldCd.Miner,
		Client:                oldCd.Client,
		State:                 oldCd.State,
		PiecePath:             oldCd.PiecePath,
		MetadataPath:          oldCd.MetadataPath,
		SlashEpoch:            oldCd.SlashEpoch,
		FastRetrieval:         oldCd.FastRetrieval,
		Message:               oldCd.Message,
		FundsReserved:         oldCd.FundsReserved,
		Ref:                   oldCd.Ref,
		AvailableForRetrieval: oldCd.AvailableForRetrieval,
		DealID:                oldCd.DealID,
		CreationTime:          oldCd.CreationTime,
	}, nil
}

func MigrateClientDealProposal0To1(prop marketOld.ClientDealProposal) (*storagemarket.ClientDealProposal, error) {
	oldLabel := prop.Proposal.Label

	var err error
	var newLabel market.DealLabel
	if utf8.ValidString(oldLabel) {
		newLabel, err = market.NewLabelFromString(oldLabel)
		if err != nil {
			return nil, fmt.Errorf("migrating deal label to DealLabel (string): %w", err)
		}
	} else {
		newLabel, err = market.NewLabelFromBytes([]byte(oldLabel))
		if err != nil {
			return nil, fmt.Errorf("migrating deal label to DealLabel (byte): %w", err)
		}
	}

	return &storagemarket.ClientDealProposal{
		ClientSignature: prop.ClientSignature,
		Proposal: market.DealProposal{
			PieceCID:             prop.Proposal.PieceCID,
			PieceSize:            prop.Proposal.PieceSize,
			VerifiedDeal:         prop.Proposal.VerifiedDeal,
			Client:               prop.Proposal.Client,
			Provider:             prop.Proposal.Provider,
			Label:                newLabel,
			StartEpoch:           prop.Proposal.StartEpoch,
			EndEpoch:             prop.Proposal.EndEpoch,
			StoragePricePerEpoch: prop.Proposal.StoragePricePerEpoch,
			ProviderCollateral:   prop.Proposal.ProviderCollateral,
			ClientCollateral:     prop.Proposal.ClientCollateral,
		},
	}, nil
}

// MigrateStorageAsk0To1 migrates a tuple encoded storage ask to a map encoded storage ask
func MigrateStorageAsk0To1(oldSa *StorageAsk0) *storagemarket.StorageAsk {
	return &storagemarket.StorageAsk{
		Price:         oldSa.Price,
		VerifiedPrice: oldSa.VerifiedPrice,

		MinPieceSize: oldSa.MinPieceSize,
		MaxPieceSize: oldSa.MaxPieceSize,
		Miner:        oldSa.Miner,
		Timestamp:    oldSa.Timestamp,
		Expiry:       oldSa.Expiry,
		SeqNo:        oldSa.SeqNo,
	}
}

// GetMigrateSignedStorageAsk0To1 returns a function that migrates a tuple encoded signed storage ask to a map encoded signed storage ask
// It needs a signing function to resign the ask -- there's no way around that
func GetMigrateSignedStorageAsk0To1(sign func(ctx context.Context, ask *storagemarket.StorageAsk) (*crypto.Signature, error)) func(*SignedStorageAsk0) (*storagemarket.SignedStorageAsk, error) {
	return func(oldSsa *SignedStorageAsk0) (*storagemarket.SignedStorageAsk, error) {
		newSa := MigrateStorageAsk0To1(oldSsa.Ask)
		sig, err := sign(context.TODO(), newSa)
		if err != nil {
			return nil, err
		}
		return &storagemarket.SignedStorageAsk{
			Ask:       newSa,
			Signature: sig,
		}, nil
	}
}

// ClientMigrations are migrations for the client's store of storage deals
var ClientMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateClientDeal0To1, versioning.VersionKey("1")),
}

// ProviderMigrations are migrations for the providers's store of storage deals
var ProviderMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateMinerDeal0To1, versioning.VersionKey("1")).FilterKeys([]string{
		"/latest-ask", "/storage-ask/latest", "/storage-ask/1/latest", "/storage-ask/versions/current"}),
	versioned.NewVersionedBuilder(MigrateMinerDeal1To2, versioning.VersionKey("2")).FilterKeys([]string{
		"/latest-ask", "/storage-ask/latest", "/storage-ask/1/latest", "/storage-ask/versions/current"}).OldVersion("1"),
}
