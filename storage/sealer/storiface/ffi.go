package storiface

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fr32"
)

var ErrSectorNotFound = errors.New("sector not found")

type UnpaddedByteIndex uint64

func (i UnpaddedByteIndex) Padded() PaddedByteIndex {
	return PaddedByteIndex(abi.UnpaddedPieceSize(i).Padded())
}

func (i UnpaddedByteIndex) Valid() error {
	if i%127 != 0 {
		return xerrors.Errorf("unpadded byte index must be a multiple of 127")
	}

	return nil
}

func UnpaddedFloor(n uint64) UnpaddedByteIndex {
	return UnpaddedByteIndex(n / uint64(fr32.UnpaddedFr32Chunk) * uint64(fr32.UnpaddedFr32Chunk))
}

func UnpaddedCeil(n uint64) UnpaddedByteIndex {
	return UnpaddedByteIndex((n + uint64(fr32.UnpaddedFr32Chunk-1)) / uint64(fr32.UnpaddedFr32Chunk) * uint64(fr32.UnpaddedFr32Chunk))
}

type PaddedByteIndex uint64

type RGetter func(ctx context.Context, id abi.SectorID) (sealed cid.Cid, update bool, err error)
