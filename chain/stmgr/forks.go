package stmgr

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

var ForksAtHeight = map[abi.ChainEpoch]func(context.Context, *StateManager, cid.Cid) (cid.Cid, error){}

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH abi.ChainEpoch) (_ cid.Cid, err error) {
	for i := parentH; i < height; i++ {
		f, ok := ForksAtHeight[i]
		if ok {
			nstate, err := f(ctx, sm, pstate)
			if err != nil {
				return cid.Undef, err
			}
			pstate = nstate
		}
	}

	return pstate, nil
}
