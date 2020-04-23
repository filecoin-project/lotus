package api

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/sector-storage/storiface"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/storage-fsm"
)

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	ActorSectorSize(context.Context, address.Address) (abi.SectorSize, error)

	MiningBase(context.Context) (*types.TipSet, error)

	// Temp api for testing
	PledgeSector(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, abi.SectorNumber) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]abi.SectorNumber, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	SectorsUpdate(context.Context, abi.SectorNumber, sealing.SectorState) error

	StorageList(ctx context.Context) (map[stores.ID][]stores.Decl, error)
	StorageLocal(ctx context.Context) (map[stores.ID]string, error)
	StorageStat(ctx context.Context, id stores.ID) (stores.FsStat, error)

	// WorkerConnect tells the node to connect to workers RPC
	WorkerConnect(context.Context, string) error
	WorkerStats(context.Context) (map[uint64]storiface.WorkerStats, error)

	stores.SectorIndex

	MarketImportDealData(ctx context.Context, propcid cid.Cid, path string) error
	MarketListDeals(ctx context.Context) ([]storagemarket.StorageDeal, error)
	MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error)
	MarketSetPrice(context.Context, types.BigInt) error

	DealsImportData(ctx context.Context, dealPropCid cid.Cid, file string) error
	DealsList(ctx context.Context) ([]storagemarket.StorageDeal, error)

	StorageAddLocal(ctx context.Context, path string) error
}

type SealRes struct {
	Err   string
	GoErr error `json:"-"`

	Proof []byte
}

type SectorLog struct {
	Kind      string
	Timestamp uint64

	Trace string

	Message string
}

type SectorInfo struct {
	SectorID abi.SectorNumber
	State    sealing.SectorState
	CommD    *cid.Cid
	CommR    *cid.Cid
	Proof    []byte
	Deals    []abi.DealID
	Ticket   SealTicket
	Seed     SealSeed
	Retries  uint64

	LastErr string

	Log []SectorLog
}

type SealedRef struct {
	SectorID abi.SectorNumber
	Offset   uint64
	Size     abi.UnpaddedPieceSize
}

type SealedRefs struct {
	Refs []SealedRef
}

type SealTicket struct {
	Value abi.SealRandomness
	Epoch abi.ChainEpoch
}

type SealSeed struct {
	Value abi.InteractiveSealRandomness
	Epoch abi.ChainEpoch
}

func (st *SealTicket) Equals(ost *SealTicket) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

func (st *SealSeed) Equals(ost *SealSeed) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}
