package ffiwrapper

import (
	"context"
	"errors"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/sector-storage/stores"
)

type UnpaddedByteIndex uint64

type Validator interface {
	CanCommit(sector stores.SectorPaths) (bool, error)
	CanProve(sector stores.SectorPaths) (bool, error)
}

type StorageSealer interface {
	storage.Sealer
	storage.Storage
}

type Storage interface {
	storage.Prover
	StorageSealer

	ReadPieceFromSealedSector(context.Context, abi.SectorID, UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error)
}

type Verifier interface {
	VerifySeal(abi.SealVerifyInfo) (bool, error)
	VerifyWinningPoSt(ctx context.Context, info abi.WinningPoStVerifyInfo) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info abi.WindowPoStVerifyInfo) (bool, error)
}

var ErrSectorNotFound = errors.New("sector not found")

type SectorProvider interface {
	// * returns ErrSectorNotFound if a requested existing sector doesn't exist
	// * returns an error when allocate is set, and existing isn't, and the sector exists
	AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error)
}

var _ SectorProvider = &basicfs.Provider{}
