package fakelm

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// MinimalLMApi is a subset of the LotusMiner API that is exposed by lotus-provider
// for consumption by boost
type MinimalLMApi interface {
	ActorAddress(context.Context) (address.Address, error)

	WorkerJobs(context.Context) (map[uuid.UUID][]storiface.WorkerJob, error)

	SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error)

	SectorsList(context.Context) ([]abi.SectorNumber, error)
	SectorsSummary(ctx context.Context) (map[api.SectorState]int, error)

	SectorsListInStates(context.Context, []api.SectorState) ([]abi.SectorNumber, error)

	StorageRedeclareLocal(context.Context, *storiface.ID, bool) error
	StorageList(context.Context) (map[storiface.ID][]storiface.Decl, error)

	UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error)
	IsUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error)

	ComputeDataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error)
	SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d api.PieceDealInfo) (api.SectorOffset, error)
}
