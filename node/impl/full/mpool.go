package full

import (
	"context"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type MpoolAPI struct {
	fx.In

	PubSub *pubsub.PubSub
	Mpool  *chain.MessagePool
}

func (a *MpoolAPI) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	// TODO: need to make sure we don't return messages that were already included in the referenced chain
	// also need to accept ts == nil just fine, assume nil == chain.Head()
	return a.Mpool.Pending(), nil
}

func (a *MpoolAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	msgb, err := smsg.Serialize()
	if err != nil {
		return err
	}
	if err := a.Mpool.Add(smsg); err != nil {
		return err
	}

	return a.PubSub.Publish("/fil/messages", msgb)
}

func (a *MpoolAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}
