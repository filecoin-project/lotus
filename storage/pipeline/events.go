package sealing

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

// `curH`-`ts.Height` = `confidence`
type HeightHandler func(ctx context.Context, tok types.TipSetKey, curH abi.ChainEpoch) error
type RevertHandler func(ctx context.Context, tok types.TipSetKey) error

type Events interface {
	ChainAt(hnd HeightHandler, rev RevertHandler, confidence int, h abi.ChainEpoch) error
}
