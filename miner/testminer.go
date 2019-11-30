package miner

import (
	"context"

	"github.com/filecoin-project/lotus/api"
)

func NewTestMiner(nextCh <-chan struct{}) func(api api.FullNode) *Miner {
	return func(api api.FullNode) *Miner {
		return &Miner{
			api:      api,
			waitFunc: chanWaiter(nextCh),
		}
	}
}

func chanWaiter(next <-chan struct{}) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-next:
		}

		return nil
	}
}
