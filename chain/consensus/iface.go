package consensus

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.opencensus.io/stats"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
)

type Consensus interface {
	// ValidateBlockHeader is called by peers when they receive a new block through the network.
	//
	// This is a fast sanity-check validation performed by the PubSub protocol before delivering
	// it to the syncer. It checks that the block has the right format and it performs
	// other consensus-specific light verifications like ensuring that the block is signed by
	// a valid miner, or that it includes all the data required for a full verification.
	ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error)

	// ValidateBlock is called by the syncer to determine if to accept a block or not.
	//
	// It performs all the checks needed by the syncer to accept
	// the block (signature verifications, VRF checks, message validity, etc.)
	ValidateBlock(ctx context.Context, b *types.FullBlock) (err error)

	// IsEpochInConsensusRange returns true if the epoch is "in range" for consensus. That is:
	// - It's not before finality.
	// - It's not too far in the future.
	IsEpochInConsensusRange(epoch abi.ChainEpoch) bool

	// CreateBlock implements all the logic required to propose and assemble a new Filecoin block.
	//
	// This function encapsulate all the consensus-specific actions to propose a new block
	// such as the ordering of transactions, the inclusion of consensus proofs, the signature
	// of the block, etc.
	CreateBlock(ctx context.Context, w api.Wallet, bt *api.BlockTemplate) (*types.FullBlock, error)
}

// RewardFunc parametrizes the logic for rewards when a message is executed.
//
// Each consensus implementation can set their own reward function.
type RewardFunc func(ctx context.Context, vmi vm.Interface, em stmgr.ExecMonitor,
	epoch abi.ChainEpoch, ts *types.TipSet, params *reward.AwardBlockRewardParams) error

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

	blk, what, err := decodeAndCheckBlock(msg)
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		return pubsub.ValidationReject, what
	}

	if !cns.IsEpochInConsensusRange(blk.Header.Height) {
		// We ignore these blocks instead of rejecting to avoid breaking the network if
		// we're recovering from an outage (e.g., where nobody agrees on where "head" is
		// currently).
		log.Warnf("received block outside of consensus range (%d)", blk.Header.Height)
		return pubsub.ValidationIgnore, "invalid_block_height"
	}

	// validate the block meta: the Message CID in the header must match the included messages
	err = validateMsgMeta(ctx, blk)
	if err != nil {
		log.Warnf("error validating message metadata: %s", err)
		return pubsub.ValidationReject, "invalid_block_meta"
	}

	reject, err := cns.ValidateBlockHeader(ctx, blk.Header)
	if err != nil {
		if reject == "" {
			log.Warn("ignoring block msg: ", err)
			return pubsub.ValidationIgnore, reject
		}
		return pubsub.ValidationReject, reject
	}

	// all good, accept the block
	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept, ""
}
