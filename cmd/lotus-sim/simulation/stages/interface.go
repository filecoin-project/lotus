package stages

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
)

// Stage is a stage of the simulation. It's asked to pack messages for every block.
type Stage interface {
	Name() string
	PackMessages(ctx context.Context, bb *blockbuilder.BlockBuilder) error
}

type Funding interface {
	SendAndFund(*blockbuilder.BlockBuilder, *types.Message) (*types.MessageReceipt, error)
	Fund(*blockbuilder.BlockBuilder, address.Address) error
}

type Committer interface {
	EnqueueProveCommit(addr address.Address, preCommitEpoch abi.ChainEpoch, info minertypes.SectorPreCommitInfo) error
}
