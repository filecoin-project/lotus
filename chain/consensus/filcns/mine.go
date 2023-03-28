package filcns

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
)

func (filec *FilecoinEC) CreateBlock(ctx context.Context, w api.Wallet, bt *api.BlockTemplate) (*types.FullBlock, error) {
	pts, err := filec.sm.ChainStore().LoadTipSet(ctx, bt.Parents)
	if err != nil {
		return nil, xerrors.Errorf("failed to load parent tipset: %w", err)
	}

	_, lbst, err := stmgr.GetLookbackTipSetForRound(ctx, filec.sm, pts, bt.Epoch)
	if err != nil {
		return nil, xerrors.Errorf("getting lookback miner actor state: %w", err)
	}

	worker, err := stmgr.GetMinerWorkerRaw(ctx, filec.sm, lbst, bt.Miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner worker: %w", err)
	}

	next, blsMessages, secpkMessages, err := consensus.CreateBlockHeader(ctx, filec.sm, pts, bt)
	if err != nil {
		return nil, xerrors.Errorf("failed to process messages from block template: %w", err)
	}

	if err := signBlock(ctx, w, worker, next); err != nil {
		return nil, xerrors.Errorf("failed to sign new block: %w", err)
	}

	fullBlock := &types.FullBlock{
		Header:        next,
		BlsMessages:   blsMessages,
		SecpkMessages: secpkMessages,
	}

	return fullBlock, nil
}
