package simulation

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
)

// Step steps the simulation forward one step. This may move forward by more than one epoch.
func (sim *Simulation) Step(ctx context.Context) (*types.TipSet, error) {
	log.Infow("step", "epoch", sim.head.Height()+1)
	messages, err := sim.popNextMessages(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to select messages for block: %w", err)
	}
	head, err := sim.makeTipSet(ctx, messages)
	if err != nil {
		return nil, xerrors.Errorf("failed to make tipset: %w", err)
	}
	if err := sim.SetHead(head); err != nil {
		return nil, xerrors.Errorf("failed to update head: %w", err)
	}
	return head, nil
}

// popNextMessages generates/picks a set of messages to be included in the next block.
//
//   - This function is destructive and should only be called once per epoch.
//   - This function does not store anything in the repo.
//   - This function handles all gas estimation. The returned messages should all fit in a single
//     block.
func (sim *Simulation) popNextMessages(ctx context.Context) ([]*types.Message, error) {
	parentTs := sim.head

	// First we make sure we don't have an upgrade at this epoch. If we do, we return no
	// messages so we can just create an empty block at that epoch.
	//
	// This isn't what the network does, but it makes things easier. Otherwise, we'd need to run
	// migrations before this epoch and I'd rather not deal with that.
	nextHeight := parentTs.Height() + 1
	prevVer := sim.StateManager.GetNetworkVersion(ctx, nextHeight-1)
	nextVer := sim.StateManager.GetNetworkVersion(ctx, nextHeight)
	if nextVer != prevVer {
		log.Warnw("packing no messages for version upgrade block",
			"old", prevVer,
			"new", nextVer,
			"epoch", nextHeight,
		)
		return nil, nil
	}

	bb, err := blockbuilder.NewBlockBuilder(
		ctx, log.With("simulation", sim.name),
		sim.StateManager, parentTs,
	)
	if err != nil {
		return nil, err
	}

	for _, stage := range sim.stages {
		// We're intentionally ignoring the "full" signal so we can try to pack a few more
		// messages.
		if err := stage.PackMessages(ctx, bb); err != nil && !blockbuilder.IsOutOfGas(err) {
			return nil, xerrors.Errorf("when packing messages with %s: %w", stage.Name(), err)
		}
	}
	return bb.Messages(), nil
}
