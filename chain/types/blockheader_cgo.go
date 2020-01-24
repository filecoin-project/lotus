//+build cgo

package types

import (
	"context"

	"github.com/filecoin-project/go-address"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
)

func (blk *BlockHeader) CheckBlockSignature(ctx context.Context, worker address.Address) error {
	_, span := trace.StartSpan(ctx, "checkBlockSignature")
	defer span.End()

	if blk.BlockSig == nil {
		return xerrors.New("block signature not present")
	}

	sigb, err := blk.SigningBytes()
	if err != nil {
		return xerrors.Errorf("failed to get block signing bytes: %w", err)
	}

	return blk.BlockSig.Verify(worker, sigb)
}
