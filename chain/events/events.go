package events

import (
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("events")

type (
	// HeightHandler `curH`-`ts.Height` = `confidence`
	HeightHandler func(ctx context.Context, ts *types.TipSet, curH abi.ChainEpoch) error
	RevertHandler func(ctx context.Context, ts *types.TipSet) error
)

// A TipSetObserver receives notifications of tipsets
type TipSetObserver interface {
	Apply(ctx context.Context, from, to *types.TipSet) error
	Revert(ctx context.Context, from, to *types.TipSet) error
}

type EventHelperAPI interface {
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainHead(context.Context) (*types.TipSet, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error)

	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) // optional / for CalledMsg
}

type Events struct {
	*observer
	*heightEvents
	*hcEvents
}

func newEventsWithGCConfidence(ctx context.Context, api EventHelperAPI, gcConfidence abi.ChainEpoch) (*Events, error) {
	cache := newCache(api, gcConfidence)

	ob := newObserver(cache, gcConfidence)
	if err := ob.start(ctx); err != nil {
		return nil, err
	}

	he := newHeightEvents(cache, ob, gcConfidence)
	headChange := newHCEvents(cache, ob)

	return &Events{ob, he, headChange}, nil
}

func NewEvents(ctx context.Context, api EventHelperAPI) (*Events, error) {
	gcConfidence := 2 * policy.ChainFinality
	return newEventsWithGCConfidence(ctx, api, gcConfidence)
}
