package splitstore

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

type debugLog struct {
}

func (d *debugLog) LogReadMiss(cid cid.Cid) {
	if d == nil {
		return
	}

	// TODO
}

func (d *debugLog) LogWrite(curTs *types.TipSet, blk blocks.Block, writeEpoch abi.ChainEpoch) {
	if d == nil {
		return
	}

	// TODO
}

func (d *debugLog) LogWriteMany(curTs *types.TipSet, blks []blocks.Block, writeEpoch abi.ChainEpoch) {
	if d == nil {
		return
	}

	// TODO
}

func (d *debugLog) LogMove(curTs *types.TipSet, cid cid.Cid, writeEpoch abi.ChainEpoch) {
	if d == nil {
		return
	}

	// TODO
}

func (d *debugLog) FlushMove() {
	if d == nil {
		return
	}

	// TODO
}

func (d *debugLog) Close() error {
	if d == nil {
		return nil
	}

	// TODO
	return nil
}
