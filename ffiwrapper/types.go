package ffiwrapper

import (
	"context"
	"errors"
	"github.com/ipfs/go-cid"
	"io"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/storage/sectorstorage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
)

type UnpaddedByteIndex uint64

type Validator interface {
	CanCommit(sector stores.SectorPaths) (bool, error)
	CanProve(sector stores.SectorPaths) (bool, error)
}

type Sealer interface {
	storage.Sealer
	storage.Storage
}

type Basic interface {
	storage.Prover
	Sealer

	ReadPieceFromSealedSector(context.Context, abi.SectorID, UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error)
}

type Verifier interface {
	VerifySeal(abi.SealVerifyInfo) (bool, error)
	VerifyElectionPost(ctx context.Context, info abi.PoStVerifyInfo) (bool, error)
	VerifyFallbackPost(ctx context.Context, info abi.PoStVerifyInfo) (bool, error)
}

var ErrSectorNotFound = errors.New("sector not found")

type SectorProvider interface {
	// * returns ErrSectorNotFound if a requested existing sector doesn't exist
	// * returns an error when allocate is set, and existing isn't, and the sector exists
	AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error)
}

var _ SectorProvider = &basicfs.Provider{}
