package kit

import (
	"context"
	"testing"

	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
)

type MinerBuilder func(context.Context, *testing.T, abi.RegisteredSealProof, address.Address) TestMiner

type TestFullNode struct {
	v1api.FullNode
	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	Stb MinerBuilder
}

type TestMiner struct {
	lapi.StorageMiner
	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	MineOne func(context.Context, miner.MineReq) error
	Stop    func(context.Context) error
}

var PresealGenesis = -1

const GenesisPreseals = 2

const TestSpt = abi.RegisteredSealProof_StackedDrg2KiBV1_1

// Options for setting up a mock storage Miner
type StorageMiner struct {
	Full    int
	Opts    node.Option
	Preseal int
}

type OptionGenerator func([]TestFullNode) node.Option

// Options for setting up a mock full node
type FullNodeOpts struct {
	Lite bool            // run node in "lite" mode
	Opts OptionGenerator // generate dependency injection options
}

// APIBuilder is a function which is invoked in test suite to provide
// test nodes and networks
//
// fullOpts array defines options for each full node
// storage array defines storage nodes, numbers in the array specify full node
// index the storage node 'belongs' to
type APIBuilder func(t *testing.T, full []FullNodeOpts, storage []StorageMiner) ([]TestFullNode, []TestMiner)

func DefaultFullOpts(nFull int) []FullNodeOpts {
	full := make([]FullNodeOpts, nFull)
	for i := range full {
		full[i] = FullNodeOpts{
			Opts: func(nodes []TestFullNode) node.Option {
				return node.Options()
			},
		}
	}
	return full
}

var OneMiner = []StorageMiner{{Full: 0, Preseal: PresealGenesis}}
var OneFull = DefaultFullOpts(1)
var TwoFull = DefaultFullOpts(2)

var FullNodeWithLatestActorsAt = func(upgradeHeight abi.ChainEpoch) FullNodeOpts {
	// Attention: Update this when introducing new actor versions or your tests will be sad
	return FullNodeWithNetworkUpgradeAt(network.Version13, upgradeHeight)
}

var FullNodeWithNetworkUpgradeAt = func(version network.Version, upgradeHeight abi.ChainEpoch) FullNodeOpts {
	fullSchedule := stmgr.UpgradeSchedule{{
		// prepare for upgrade.
		Network:   network.Version9,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}, {
		Network:   network.Version10,
		Height:    2,
		Migration: stmgr.UpgradeActorsV3,
	}, {
		Network:   network.Version12,
		Height:    3,
		Migration: stmgr.UpgradeActorsV4,
	}, {
		Network:   network.Version13,
		Height:    4,
		Migration: stmgr.UpgradeActorsV5,
	}}

	schedule := stmgr.UpgradeSchedule{}
	for _, upgrade := range fullSchedule {
		if upgrade.Network > version {
			break
		}

		schedule = append(schedule, upgrade)
	}

	if upgradeHeight > 0 {
		schedule[len(schedule)-1].Height = upgradeHeight
	}

	return FullNodeOpts{
		Opts: func(nodes []TestFullNode) node.Option {
			return node.Override(new(stmgr.UpgradeSchedule), schedule)
		},
	}
}

var FullNodeWithSDRAt = func(calico, persian abi.ChainEpoch) FullNodeOpts {
	return FullNodeOpts{
		Opts: func(nodes []TestFullNode) node.Option {
			return node.Override(new(stmgr.UpgradeSchedule), stmgr.UpgradeSchedule{{
				Network:   network.Version6,
				Height:    1,
				Migration: stmgr.UpgradeActorsV2,
			}, {
				Network:   network.Version7,
				Height:    calico,
				Migration: stmgr.UpgradeCalico,
			}, {
				Network: network.Version8,
				Height:  persian,
			}})
		},
	}
}

var MineNext = miner.MineReq{
	InjectNulls: 0,
	Done:        func(bool, abi.ChainEpoch, error) {},
}
