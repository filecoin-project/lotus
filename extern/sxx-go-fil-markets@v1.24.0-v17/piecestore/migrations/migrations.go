package migrations

import (
	"github.com/ipfs/go-cid"

	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/piecestore"
)

//go:generate cbor-gen-for PieceInfo0 DealInfo0 BlockLocation0 PieceBlockLocation0 CIDInfo0

// DealInfo0 is version 0 of DealInfo
type DealInfo0 struct {
	DealID   abi.DealID
	SectorID abi.SectorNumber
	Offset   abi.PaddedPieceSize
	Length   abi.PaddedPieceSize
}

// BlockLocation0 is version 0 of BlockLocation
type BlockLocation0 struct {
	RelOffset uint64
	BlockSize uint64
}

// PieceBlockLocation0 is version 0 of PieceBlockLocation
// is inside of
type PieceBlockLocation0 struct {
	BlockLocation0
	PieceCID cid.Cid
}

// CIDInfo0 is version 0 of CIDInfo
type CIDInfo0 struct {
	CID                 cid.Cid
	PieceBlockLocations []PieceBlockLocation0
}

// PieceInfo0 is version 0 of PieceInfo
type PieceInfo0 struct {
	PieceCID cid.Cid
	Deals    []DealInfo0
}

// MigratePieceInfo0To1 migrates a tuple encoded piece info to a map encoded piece info
func MigratePieceInfo0To1(oldPi *PieceInfo0) (*piecestore.PieceInfo, error) {
	deals := make([]piecestore.DealInfo, len(oldPi.Deals))
	for i, oldDi := range oldPi.Deals {
		deals[i] = piecestore.DealInfo{
			DealID:   oldDi.DealID,
			SectorID: oldDi.SectorID,
			Offset:   oldDi.Offset,
			Length:   oldDi.Length,
		}
	}
	return &piecestore.PieceInfo{
		PieceCID: oldPi.PieceCID,
		Deals:    deals,
	}, nil
}

// MigrateCidInfo0To1 migrates a tuple encoded cid info to a map encoded cid info
func MigrateCidInfo0To1(oldCi *CIDInfo0) (*piecestore.CIDInfo, error) {
	pieceBlockLocations := make([]piecestore.PieceBlockLocation, len(oldCi.PieceBlockLocations))
	for i, oldPbl := range oldCi.PieceBlockLocations {
		pieceBlockLocations[i] = piecestore.PieceBlockLocation{
			BlockLocation: piecestore.BlockLocation{
				RelOffset: oldPbl.RelOffset,
				BlockSize: oldPbl.BlockSize,
			},
			PieceCID: oldPbl.PieceCID,
		}
	}
	return &piecestore.CIDInfo{
		CID:                 oldCi.CID,
		PieceBlockLocations: pieceBlockLocations,
	}, nil
}

// PieceInfoMigrations is the list of migrations for migrating PieceInfos
var PieceInfoMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigratePieceInfo0To1, versioning.VersionKey("1")),
}

// CIDInfoMigrations is the list of migrations for migrating CIDInfos
var CIDInfoMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateCidInfo0To1, versioning.VersionKey("1")),
}
