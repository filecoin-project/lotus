package kit

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node"
)

// DefaultPresealsPerBootstrapMiner is the number of preseals that every
// bootstrap miner has by default. It can be overridden through the
// PresealSectors option.
const DefaultPresealsPerBootstrapMiner = 2

const TestSpt = abi.RegisteredSealProof_StackedDrg2KiBV1_1

// nodeOpts is an options accumulating struct, where functional options are
// merged into.
type nodeOpts struct {
	balance       abi.TokenAmount
	lite          bool
	sectors       int
	rpc           bool
	ownerKey      *wallet.Key
	extraNodeOpts []node.Option

	subsystems           MinerSubsystem
	mainMiner            *TestMiner
	disableLibp2p        bool
	optBuilders          []OptBuilder
	sectorSize           abi.SectorSize
	maxStagingDealsBytes int64
}

// DefaultNodeOpts are the default options that will be applied to test nodes.
var DefaultNodeOpts = nodeOpts{
	balance:    big.Mul(big.NewInt(100000000), types.NewInt(build.FilecoinPrecision)),
	sectors:    DefaultPresealsPerBootstrapMiner,
	sectorSize: abi.SectorSize(2 << 10), // 2KiB.
}

// OptBuilder is used to create an option after some other node is already
// active. Takes all active nodes as a parameter.
type OptBuilder func(activeNodes []*TestFullNode) node.Option

// NodeOpt is a functional option for test nodes.
type NodeOpt func(opts *nodeOpts) error

func WithAllSubsystems() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.subsystems = opts.subsystems.Add(SMarkets)
		opts.subsystems = opts.subsystems.Add(SMining)
		opts.subsystems = opts.subsystems.Add(SSealing)
		opts.subsystems = opts.subsystems.Add(SSectorStorage)

		return nil
	}
}

func WithSubsystems(systems ...MinerSubsystem) NodeOpt {
	return func(opts *nodeOpts) error {
		for _, s := range systems {
			opts.subsystems = opts.subsystems.Add(s)
		}
		return nil
	}
}

func WithMaxStagingDealsBytes(size int64) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.maxStagingDealsBytes = size
		return nil
	}
}

func DisableLibp2p() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.disableLibp2p = true
		return nil
	}
}

func MainMiner(m *TestMiner) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.mainMiner = m
		return nil
	}
}

// OwnerBalance specifies the balance to be attributed to a miner's owner
// account. Only relevant when creating a miner.
func OwnerBalance(balance abi.TokenAmount) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.balance = balance
		return nil
	}
}

// LiteNode specifies that this node will be a lite node. Only relevant when
// creating a fullnode.
func LiteNode() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.lite = true
		return nil
	}
}

// PresealSectors specifies the amount of preseal sectors to give to a miner
// at genesis. Only relevant when creating a miner.
func PresealSectors(sectors int) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.sectors = sectors
		return nil
	}
}

// ThroughRPC makes interactions with this node throughout the test flow through
// the JSON-RPC API.
func ThroughRPC() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.rpc = true
		return nil
	}
}

// OwnerAddr sets the owner address of a miner. Only relevant when creating
// a miner.
func OwnerAddr(wk *wallet.Key) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.ownerKey = wk
		return nil
	}
}

// ConstructorOpts are Lotus node constructor options that are passed as-is to
// the node.
func ConstructorOpts(extra ...node.Option) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.extraNodeOpts = extra
		return nil
	}
}

// SectorSize sets the sector size for this miner. Start() will populate the
// corresponding proof type depending on the network version (genesis network
// version if the Ensemble is unstarted, or the current network version
// if started).
func SectorSize(sectorSize abi.SectorSize) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.sectorSize = sectorSize
		return nil
	}
}
