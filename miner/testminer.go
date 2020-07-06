package miner

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/gen"
	lru "github.com/hashicorp/golang-lru"
)

func NewTestMiner(nextCh <-chan func(bool, error), addr address.Address) func(api.FullNode, gen.WinningPoStProver) *Miner {
	return func(api api.FullNode, epp gen.WinningPoStProver) *Miner {
		arc, err := lru.NewARC(10000)
		if err != nil {
			panic(err)
		}

		m := &Miner{
			api:               api,
			waitFunc:          chanWaiter(nextCh),
			epp:               epp,
			minedBlockHeights: arc,
			address:           addr,
		}

		if err := m.Start(context.TODO()); err != nil {
			panic(err)
		}
		return m
	}
}

func chanWaiter(next <-chan func(bool, error)) func(ctx context.Context, _ uint64) (func(bool, error), error) {
	return func(ctx context.Context, _ uint64) (func(bool, error), error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case cb := <-next:
			return cb, nil
		}
	}
}
