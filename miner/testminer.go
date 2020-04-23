package miner

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/gen"
	lru "github.com/hashicorp/golang-lru"
)

func NewTestMiner(nextCh <-chan func(bool), addr address.Address) func(api.FullNode, gen.WinningPoStProver, beacon.RandomBeacon) *Miner {
	return func(api api.FullNode, epp gen.WinningPoStProver, b beacon.RandomBeacon) *Miner {
		arc, err := lru.NewARC(10000)
		if err != nil {
			panic(err)
		}

		m := &Miner{
			beacon:            b,
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

func chanWaiter(next <-chan func(bool)) func(ctx context.Context, _ uint64) (func(bool), error) {
	return func(ctx context.Context, _ uint64) (func(bool), error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case cb := <-next:
			return cb, nil
		}
	}
}
