package full

import (
	"context"
	"errors"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
)

type ChainIndexerAPI interface {
	ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error)
}

var (
	_ ChainIndexerAPI = *new(api.FullNode)
)

type ChainIndexAPI struct {
	fx.In
	ChainIndexerAPI
}

type ChainIndexHandler struct {
	indexer index.Indexer
}

func (ch *ChainIndexHandler) ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error) {
	if ch.indexer == nil {
		return nil, errors.New("chain indexer is disabled")
	}
	return ch.indexer.ChainValidateIndex(ctx, epoch, backfill)
}

var _ ChainIndexerAPI = (*ChainIndexHandler)(nil)

func NewChainIndexHandler(indexer index.Indexer) *ChainIndexHandler {
	return &ChainIndexHandler{
		indexer: indexer,
	}
}
