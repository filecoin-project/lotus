package piecestore

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared"
)

//go:generate cbor-gen-for --map-encoding PieceInfo DealInfo BlockLocation PieceBlockLocation CIDInfo

// DealInfo is information about a single deal for a given piece
type DealInfo struct {
	DealID   abi.DealID
	SectorID abi.SectorNumber
	Offset   abi.PaddedPieceSize
	Length   abi.PaddedPieceSize
}

// BlockLocation is information about where a given block is relative to the overall piece
type BlockLocation struct {
	RelOffset uint64
	BlockSize uint64
}

// PieceBlockLocation is block information along with the pieceCID of the piece the block
// is inside of
type PieceBlockLocation struct {
	BlockLocation
	PieceCID cid.Cid
}

// CIDInfo is information about where a given CID will live inside a piece
type CIDInfo struct {
	CID                 cid.Cid
	PieceBlockLocations []PieceBlockLocation
}

// CIDInfoUndefined is cid info with no information
var CIDInfoUndefined = CIDInfo{}

// PieceInfo is metadata about a piece a provider may be storing based
// on its PieceCID -- so that, given a pieceCID during retrieval, the miner
// can determine how to unseal it if needed
type PieceInfo struct {
	PieceCID cid.Cid
	Deals    []DealInfo
}

// PieceInfoUndefined is piece info with no information
var PieceInfoUndefined = PieceInfo{}

func (pi PieceInfo) Defined() bool {
	return pi.PieceCID.Defined() || len(pi.Deals) > 0
}

// PieceStore is a saved database of piece info that can be modified and queried
type PieceStore interface {
	Start(ctx context.Context) error
	OnReady(ready shared.ReadyFunc)
	AddDealForPiece(pieceCID cid.Cid, payloadCid cid.Cid, dealInfo DealInfo) error
	AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]BlockLocation) error
	GetPieceInfo(pieceCID cid.Cid) (PieceInfo, error)
	GetCIDInfo(payloadCID cid.Cid) (CIDInfo, error)
	ListCidInfoKeys() ([]cid.Cid, error)
	ListPieceInfoKeys() ([]cid.Cid, error)
}
