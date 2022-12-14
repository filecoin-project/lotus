package events

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type uncachedAPI interface {
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*store.HeadChange, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*types.MsgLookup, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) // optional / for CalledMsg
}

type cache struct {
	*tipSetCache
	*messageCache
	uncachedAPI
}

func newCache(api EventAPI, gcConfidence abi.ChainEpoch) *cache {
	return &cache{
		newTSCache(api, gcConfidence),
		newMessageCache(api),
		api,
	}
}
