package ffiwrapper

import (
	"context"
	"io"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

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

	UnsealPiece(ctx context.Context, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error
	ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error)
}

type Verifier interface {
	VerifySeal(proof.SealVerifyInfo) (bool, error)
	VerifyWinningPoSt(ctx context.Context, info proof.WinningPoStVerifyInfo) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof.WindowPoStVerifyInfo) (bool, error)

	GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error)
}

type SectorProvider interface {
	// * returns storiface.ErrSectorNotFound if a requested existing sector doesn't exist
	// * returns an error when allocate is set, and existing isn't, and the sector exists
	AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, ptype stores.PathType) (stores.SectorPaths, func(), error)
}

var _ SectorProvider = &basicfs.Provider{}
