package stmgr

import (
	"context"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
)

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH uint64) (_ cid.Cid, err error) {
	for i := parentH; i < height; i++ {
		switch i {
		case build.ForkNoPowerEPSUpdates:
			pstate, err = sm.forkNoPowerEPS(ctx, pstate)
			if err != nil {
				return cid.Undef, xerrors.Errorf("executing state fork in epoch %d: %w", i, err)
			}

			log.Infof("forkNoPowerEPS state: %s", pstate)
		}
	}

	return pstate, nil
}
