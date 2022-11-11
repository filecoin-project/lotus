package kit

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
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
	ownerKey      *key.Key
	extraNodeOpts []node.Option
	cfgOpts       []CfgOption
	fsrepo        bool

	subsystems             MinerSubsystem
	mainMiner              *TestMiner
	disableLibp2p          bool
	optBuilders            []OptBuilder
	sectorSize             abi.SectorSize
	maxStagingDealsBytes   int64
	minerNoLocalSealing    bool // use worker
	minerAssigner          string
	disallowRemoteFinalize bool
	noStorage              bool

	workerTasks      []sealtasks.TaskType
	workerStorageOpt func(paths.Store) paths.Store
	workerName       string
	workerExecutor   sealer.ExecutorFunc
}

// DefaultNodeOpts are the default options that will be applied to test nodes.
var DefaultNodeOpts = nodeOpts{
	balance:    big.Mul(big.NewInt(100000000), types.NewInt(build.FilecoinPrecision)),
	sectors:    DefaultPresealsPerBootstrapMiner,
	sectorSize: abi.SectorSize(2 << 10), // 2KiB.

	workerTasks:      []sealtasks.TaskType{sealtasks.TTFetch, sealtasks.TTCommit1, sealtasks.TTFinalize},
	workerStorageOpt: func(store paths.Store) paths.Store { return store },
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

func WithNoLocalSealing(nope bool) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.minerNoLocalSealing = nope
		return nil
	}
}

func WithAssigner(a string) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.minerAssigner = a
		return nil
	}
}

func WithDisallowRemoteFinalize(d bool) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.disallowRemoteFinalize = d
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

// NoStorage initializes miners with no writable storage paths (just read-only preseal paths)
func NoStorage() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.noStorage = true
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
func OwnerAddr(wk *key.Key) NodeOpt {
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

func MutateSealingConfig(mut func(sc *config.SealingConfig)) NodeOpt {
	return ConstructorOpts(
		node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
			return func() (sealiface.Config, error) {
				cf := config.DefaultStorageMiner()
				mut(&cf.Sealing)
				return modules.ToSealingConfig(cf.Dealmaking, cf.Sealing), nil
			}, nil
		})))
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

func WithTaskTypes(tt []sealtasks.TaskType) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.workerTasks = tt
		return nil
	}
}

func WithWorkerName(n string) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.workerName = n
		return nil
	}
}

var WithSealWorkerTasks = WithTaskTypes([]sealtasks.TaskType{sealtasks.TTFetch, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2, sealtasks.TTUnseal})

func WithWorkerStorage(transform func(paths.Store) paths.Store) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.workerStorageOpt = transform
		return nil
	}
}

func WithWorkerExecutor(exec sealer.ExecutorFunc) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.workerExecutor = exec
		return nil
	}
}

func FsRepo() NodeOpt {
	return func(opts *nodeOpts) error {
		opts.fsrepo = true
		return nil
	}
}

func WithCfgOpt(opt CfgOption) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.cfgOpts = append(opts.cfgOpts, opt)
		return nil
	}
}

type CfgOption func(cfg *config.FullNode) error

func SplitstoreDiscard() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Chainstore.EnableSplitstore = true
		cfg.Chainstore.Splitstore.HotStoreFullGCFrequency = 0 // turn off full gc
		cfg.Chainstore.Splitstore.ColdStoreType = "discard"   // no cold store
		return nil
	})
}

func SplitstoreUniversal() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Chainstore.EnableSplitstore = true
		cfg.Chainstore.Splitstore.HotStoreFullGCFrequency = 0 // turn off full gc
		cfg.Chainstore.Splitstore.ColdStoreType = "universal" // universal bs is coldstore
		return nil
	})
}

func SplitstoreMessges() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Chainstore.EnableSplitstore = true
		cfg.Chainstore.Splitstore.HotStoreFullGCFrequency = 0 // turn off full gc
		cfg.Chainstore.Splitstore.ColdStoreType = "messages"  // universal bs is coldstore, and it accepts messages
		return nil
	})
}
