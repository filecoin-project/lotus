package consensus

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.opencensus.io/stats"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
)

type Consensus interface {
	ValidateBlock(ctx context.Context, b *types.FullBlock) (err error)
	ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error)
	IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool

	CreateBlock(ctx context.Context, w api.Wallet, bt *api.BlockTemplate) (*types.FullBlock, error)
	SignBlock(ctx context.Context, w api.Wallet, addr address.Address, next *types.BlockHeader) error
	VerifyBlockSignature(ctx context.Context, h *types.BlockHeader, addr address.Address) error
}

// ValidateBlockPubsub implements the common checks performed by all consensus implementations
// when a block is received through the pubsub channel.
func ValidateBlockPubsub(ctx context.Context, cns Consensus, self bool, msg *pubsub.Message) (pubsub.ValidationResult, string) {
	if self {
		return validateLocalBlock(ctx, msg)
	}

	// track validation time
	begin := build.Clock.Now()
	defer func() {
		log.Debugf("block validation time: %s", build.Clock.Since(begin))
	}()

	stats.Record(ctx, metrics.BlockReceived.M(1))

	recordFailureFlagPeer := func(what string) {
		// bv.Validate will flag the peer in that case
		panic(what)
	}

	blk, what, err := decodeAndCheckBlock(msg)
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		recordFailureFlagPeer(what)
		return pubsub.ValidationReject, what
	}

	// validate the block meta: the Message CID in the header must match the included messages
	err = validateMsgMeta(ctx, blk)
	if err != nil {
		log.Warnf("error validating message metadata: %s", err)
		recordFailureFlagPeer("invalid_block_meta")
		return pubsub.ValidationReject, "invalid_block_meta"
	}

	reject, err := cns.ValidateBlockHeader(ctx, blk.Header)
	if err != nil {
		if reject == "" {
			log.Warn("ignoring block msg: ", err)
			return pubsub.ValidationIgnore, reject
		}
		recordFailureFlagPeer(reject)
		return pubsub.ValidationReject, reject
	}

	// all good, accept the block
	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}
