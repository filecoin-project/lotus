package full

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/types"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type SyncAPI struct {
	fx.In

	Syncer *chain.Syncer
	PubSub *pubsub.PubSub
}

func (a *SyncAPI) SyncState(ctx context.Context) (*api.SyncState, error) {
	states := a.Syncer.State()

	out := &api.SyncState{}

	for _, ss := range states {
		out.ActiveSyncs = append(out.ActiveSyncs, api.ActiveSync{
			Base:   ss.Base,
			Target: ss.Target,
			Stage:  ss.Stage,
			Height: ss.Height,
		})
	}
	return out, nil
}

func (a *SyncAPI) SyncSubmitBlock(ctx context.Context, blk *types.BlockMsg) error {
	// TODO: should we have some sort of fast path to adding a local block?
	bmsgs, err := a.Syncer.ChainStore().LoadMessagesFromCids(blk.BlsMessages)
	if err != nil {
		return xerrors.Errorf("failed to load bls messages: %w", err)
	}

	smsgs, err := a.Syncer.ChainStore().LoadSignedMessagesFromCids(blk.SecpkMessages)
	if err != nil {
		return xerrors.Errorf("failed to load secpk message: %w", err)
	}

	fb := &types.FullBlock{
		Header:        blk.Header,
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
	}

	if err := a.Syncer.ValidateMsgMeta(fb); err != nil {
		xerrors.Errorf("provided messages did not match block: %w", err)
	}

	ts, err := types.NewTipSet([]*types.BlockHeader{blk.Header})
	if err != nil {
		return xerrors.Errorf("somehow failed to make a tipset out of a single block: %w", err)
	}
	if err := a.Syncer.Sync(ctx, ts); err != nil {
		return xerrors.Errorf("sync to submitted block failed: %w", err)
	}

	b, err := blk.Serialize()
	if err != nil {
		return xerrors.Errorf("serializing block for pubsub publishing failed: %w", err)
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}
