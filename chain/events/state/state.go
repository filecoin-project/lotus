package state

import (
	"context"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type StateWatcher struct {
}

type WatcherAPI interface {
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
}

type UserData interface{}

type DiffFunc func(ctx context.Context, oldState, newState *types.TipSet) (changed bool, user UserData, err error)

type Callback func(ctx context.Context, oldState, newState *types.TipSet, events interface{})

type RevertHandler func(ctx context.Context, ts *types.TipSet) error

/*
w := NewWatcher(api, OnActorChange(t04, OnDealStateChange(123)), cb)
*/
func NewWatcher(ctx context.Context, api WatcherAPI, d DiffFunc, apply Callback, revert RevertHandler, confidence, timeout abi.ChainEpoch) {
	go func() {
		notifs, err := api.ChainNotify(ctx)
		if err != nil {
			// bad
			return
		}

		curTs := (<-notifs)[0].Val
		d(ctx, curTs, curTs)

		for {
			select {
			case update := <-notifs:
				for i, change := range update {
					switch change.Type {
					case store.HCApply:
						d(ctx, curTs, change.Val)
					case store.HCRevert:

					}
				}
			}
		}

	}()
}
