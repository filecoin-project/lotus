package consensus

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type Consensus interface {
	ValidateBlock(ctx context.Context, b *types.FullBlock) (err error)
	ValidateBlockPubsub(ctx context.Context, self bool, msg *pubsub.Message) (pubsub.ValidationResult, string)
	IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool

	CreateBlock(ctx context.Context, w api.Wallet, bt *api.BlockTemplate) (*types.FullBlock, error)
}
