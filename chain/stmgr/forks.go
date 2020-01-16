package stmgr

import (
	"context"

	"github.com/ipfs/go-cid"
)

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH uint64) (_ cid.Cid, err error) {
	for i := parentH; i < height; i++ {
		switch i {
		}
	}

	return pstate, nil
}
