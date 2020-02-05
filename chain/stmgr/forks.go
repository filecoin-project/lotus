package stmgr

import (
	"context"

	"github.com/ipfs/go-cid"
)

var ForksAtHeight = map[uint64]func(context.Context, *StateManager, cid.Cid) (cid.Cid, error){}

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH uint64) (_ cid.Cid, err error) {
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
