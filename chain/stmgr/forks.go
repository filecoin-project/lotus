package stmgr

import (
	"context"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/build"
)

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height uint64) (cid.Cid, error) {
	switch height {
	case build.ForkNoPowerEPSUpdates: // TODO: +1?
		return sm.forkNoPowerEPS(ctx, pstate)
	default:
		return pstate, nil
	}
}
