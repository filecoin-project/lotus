package messagepool

import (
	"context"
	"errors"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/lib/result"
)

var (
	HeadChangeCoalesceMinDelay      = 2 * time.Second
	HeadChangeCoalesceMaxDelay      = 6 * time.Second
	HeadChangeCoalesceMergeInterval = time.Second
)

type Provider interface {
	SubscribeHeadChanges(func(rev, app []*types.TipSet) error) *types.TipSet
	PutMessage(ctx context.Context, m types.ChainMsg) (cid.Cid, error)
	PubSubPublish(string, []byte) error
	GetActorBefore(address.Address, *types.TipSet) (*types.Actor, error)
	GetActorAfter(address.Address, *types.TipSet) (*types.Actor, error)
	StateDeterministicAddressAtFinality(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateNetworkVersion(context.Context, abi.ChainEpoch) network.Version
	MessagesForBlock(context.Context, *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
	MessagesForTipset(context.Context, *types.TipSet) ([]types.ChainMsg, error)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainComputeBaseFee(ctx context.Context, ts *types.TipSet) (types.BigInt, error)
	IsLite() bool
}

type actorCacheKey struct {
	types.TipSetKey
	address.Address
}

var nonceCacheSize = 128

type mpoolProvider struct {
	sm *stmgr.StateManager
	ps *pubsub.PubSub

	liteActorCache *lru.Cache[actorCacheKey, result.Result[*types.Actor]]

	lite MpoolNonceAPI
}

var _ Provider = (*mpoolProvider)(nil)

func NewProvider(sm *stmgr.StateManager, ps *pubsub.PubSub) Provider {
	return &mpoolProvider{sm: sm, ps: ps}
}

func NewProviderLite(sm *stmgr.StateManager, ps *pubsub.PubSub, noncer MpoolNonceAPI) Provider {
	return &mpoolProvider{
		sm:             sm,
		ps:             ps,
		lite:           noncer,
		liteActorCache: must.One(lru.New[actorCacheKey, result.Result[*types.Actor]](nonceCacheSize)),
	}
}

func (mpp *mpoolProvider) IsLite() bool {
	return mpp.lite != nil
}

func (mpp *mpoolProvider) getActorLite(addr address.Address, ts *types.TipSet) (act *types.Actor, err error) {
	if !mpp.IsLite() {
		return nil, errors.New("should not use getActorLite on non lite Provider")
	}

	if c, ok := mpp.liteActorCache.Get(actorCacheKey{ts.Key(), addr}); ok {
		return c.Unwrap()
	}

	defer func() {
		mpp.liteActorCache.Add(actorCacheKey{ts.Key(), addr}, result.Wrap(act, err))
	}()

	n, err := mpp.lite.GetNonce(context.TODO(), addr, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting nonce over lite: %w", err)
	}
	a, err := mpp.lite.GetActor(context.TODO(), addr, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting actor over lite: %w", err)
	}
	a.Nonce = n
	return a, nil
}

func (mpp *mpoolProvider) SubscribeHeadChanges(cb func(rev, app []*types.TipSet) error) *types.TipSet {
	mpp.sm.ChainStore().SubscribeHeadChanges(
		store.WrapHeadChangeCoalescer(
			cb,
			HeadChangeCoalesceMinDelay,
			HeadChangeCoalesceMaxDelay,
			HeadChangeCoalesceMergeInterval,
		))
	return mpp.sm.ChainStore().GetHeaviestTipSet()
}

func (mpp *mpoolProvider) PutMessage(ctx context.Context, m types.ChainMsg) (cid.Cid, error) {
	return mpp.sm.ChainStore().PutMessage(ctx, m)
}

func (mpp *mpoolProvider) PubSubPublish(k string, v []byte) error {
	return mpp.ps.Publish(k, v) // nolint
}

func (mpp *mpoolProvider) GetActorBefore(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	if mpp.IsLite() {
		return mpp.getActorLite(addr, ts)
	}

	return mpp.sm.LoadActor(context.TODO(), addr, ts)
}

func (mpp *mpoolProvider) GetActorAfter(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	if mpp.IsLite() {
		return mpp.getActorLite(addr, ts)
	}

	stcid, _, err := mpp.sm.TipSetState(context.TODO(), ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state for GetActor: %w", err)
	}
	st, err := mpp.sm.StateTree(stcid)
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree: %w", err)
	}
	return st.GetActor(addr)
}

func (mpp *mpoolProvider) StateDeterministicAddressAtFinality(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return mpp.sm.ResolveToDeterministicAddressAtFinality(ctx, addr, ts)
}

func (mpp *mpoolProvider) StateNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version {
	return mpp.sm.GetNetworkVersion(ctx, height)
}

func (mpp *mpoolProvider) MessagesForBlock(ctx context.Context, h *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	return mpp.sm.ChainStore().MessagesForBlock(ctx, h)
}

func (mpp *mpoolProvider) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	return mpp.sm.ChainStore().MessagesForTipset(ctx, ts)
}

func (mpp *mpoolProvider) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return mpp.sm.ChainStore().LoadTipSet(ctx, tsk)
}

func (mpp *mpoolProvider) ChainComputeBaseFee(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	baseFee, err := mpp.sm.ChainStore().ComputeBaseFee(ctx, ts)
	if err != nil {
		return types.NewInt(0), xerrors.Errorf("computing base fee at %s: %w", ts, err)
	}
	return baseFee, nil
}
