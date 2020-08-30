package stmgr

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
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
