package miner

import (
	"context"

	"github.com/filecoin-project/go-lotus/lib/vdf"
)

func NewTestMiner(nextCh <-chan struct{}) func(api api) *Miner {
	return func(api api) *Miner {
		return &Miner{
			api:    api,
			runVDF: chanVDF(nextCh),
		}
	}
}

func chanVDF(next <-chan struct{}) func(ctx context.Context, input []byte) ([]byte, []byte, error) {
	return func(ctx context.Context, input []byte) ([]byte, []byte, error) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-next:
		}

		return vdf.Run(input)
	}
}
