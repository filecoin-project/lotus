package retrievalmarket

import (
	"context"
	"io"

	"github.com/filecoin-project/go-state-types/abi"
)

// SectorAccessor provides methods to unseal and get the seal status of a sector
type SectorAccessor interface {
	UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error)
	IsUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error)
}
