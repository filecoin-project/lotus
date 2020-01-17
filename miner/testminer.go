package miner

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/gen"
	lru "github.com/hashicorp/golang-lru"
)

func NewTestMiner(nextCh <-chan struct{}, addr address.Address) func(api.FullNode, gen.ElectionPoStProver) *Miner {
	return func(api api.FullNode, epp gen.ElectionPoStProver) *Miner {
		arc, err := lru.NewARC(10000)
		if err != nil {
			panic(err)
		}

		m := &Miner{
			api:               api,
			waitFunc:          chanWaiter(nextCh),
			epp:               epp,
			minedBlockHeights: arc,
		}

		if err := m.Register(addr); err != nil {
			panic(err)
		}
		return m
	}
}

func chanWaiter(next <-chan struct{}) func(ctx context.Context, _ uint64) error {
	return func(ctx context.Context, _ uint64) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-next:
		}

		return nil
	}
}
