package stmgr

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
)

var ForksAtHeight = map[abi.ChainEpoch]func(context.Context, *StateManager, types.StateTree) error{}

func (sm *StateManager) handleStateForks(ctx context.Context, st types.StateTree, height abi.ChainEpoch) (err error) {
	f, ok := ForksAtHeight[height]
	if ok {
		err := f(ctx, sm, st)
		if err != nil {
			return err
		}
	}

	return nil
}
