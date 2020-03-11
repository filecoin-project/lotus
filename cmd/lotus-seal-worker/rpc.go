package main

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-cid"
)

type worker struct {

}

func (w *worker) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storage.PreCommit1Out, error) {
	panic("implement me")
}

func (w *worker) SealPreCommit2(context.Context, abi.SectorNumber, storage.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	panic("implement me")
}

func (w *worker) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (storage.Commit1Out, error) {
	panic("implement me")
}

func (w *worker) SealCommit2(context.Context, abi.SectorNumber, storage.Commit1Out) (storage.Proof, error) {
	panic("implement me")
}

func (w *worker) FinalizeSector(context.Context, abi.SectorNumber) error {
	panic("implement me")
}

var _ storage.Sealer = &worker{}
