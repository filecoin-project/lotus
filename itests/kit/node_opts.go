package kit

import (
	"math"
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/peer"
	multiaddr "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

// DefaultPresealsPerBootstrapMiner is the number of preseals that every
// bootstrap miner has by default. It can be overridden through the
// PresealSectors option.
const DefaultPresealsPerBootstrapMiner = 2

const TestSpt = abi.RegisteredSealProof_StackedDrg2KiBV1_1
const TestSptNi = abi.RegisteredSealProof_StackedDrg2KiBV1_2_Feat_NiPoRep

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
	minerNoLocalSealing    bool // use worker
	minerAssigner          string
	disallowRemoteFinalize bool
	noStorage              bool

	workerTasks      []sealtasks.TaskType
	workerStorageOpt func(paths.Store) paths.Store
	workerName       string
}

// Libp2p connection gater that only allows outbound connections to loopback addresses.
type loopbackConnGater struct{ connmgr.ConnectionGater }

// InterceptAddrDial implements connmgr.ConnectionGater.
func (l *loopbackConnGater) InterceptAddrDial(p peer.ID, a multiaddr.Multiaddr) (allow bool) {
	if !l.ConnectionGater.InterceptAddrDial(p, a) {
		return false
	}
	ip, err := manet.ToIP(a)
	if err != nil {
		return false
	}
	return ip.IsLoopback()
}

var _ connmgr.ConnectionGater = (*loopbackConnGater)(nil)

// DefaultNodeOpts are the default options that will be applied to test nodes.
var DefaultNodeOpts = nodeOpts{
	balance:    big.Mul(big.NewInt(10_000_000), types.NewInt(buildconstants.FilecoinPrecision)),
	sectors:    DefaultPresealsPerBootstrapMiner,
	sectorSize: abi.SectorSize(2 << 10), // 2KiB.

	cfgOpts: []CfgOption{
		func(cfg *config.FullNode) error {
			// test defaults

			cfg.Fevm.EnableEthRPC = true
			cfg.ChainIndexer.EnableIndexer = true
			cfg.Events.MaxFilterHeightRange = math.MaxInt64
			cfg.Events.EnableActorEventsAPI = true

			// Disable external networking ffs.
			cfg.Libp2p.ListenAddresses = []string{
				"/ip4/127.0.0.1/udp/0/quic-v1",
			}
			cfg.Libp2p.DisableNatPortMap = true

			// Nerf the connection manager.
			cfg.Libp2p.ConnMgrLow = 1024
			cfg.Libp2p.ConnMgrHigh = 2048
			cfg.Libp2p.ConnMgrGrace = config.Duration(time.Hour)
			cfg.ChainIndexer.ReconcileEmptyIndex = true
			cfg.ChainIndexer.MaxReconcileTipsets = 10000
			return nil
		},
	},

	workerTasks:      []sealtasks.TaskType{sealtasks.TTFetch, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed},
	workerStorageOpt: func(store paths.Store) paths.Store { return store },
}

// OptBuilder is used to create an option after some other node is already
// active. Takes all active nodes as a parameter.
type OptBuilder func(activeNodes []*TestFullNode) node.Option

// NodeOpt is a functional option for test nodes.
type NodeOpt func(opts *nodeOpts) error

func WithAllSubsystems() NodeOpt {
	return func(opts *nodeOpts) error {
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
		opts.extraNodeOpts = append(opts.extraNodeOpts, extra...)
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

// F3Config sets the F3 configuration to be used by test node.
func F3Config(cfg *lf3.Config) NodeOpt {
	return ConstructorOpts(
		node.Override(new(*lf3.Config), cfg),
	)
}

// F3Backend overrides the F3 backend implementation used by test node.
func F3Backend(backend lf3.F3Backend) NodeOpt {
	return ConstructorOpts(
		node.Override(new(lf3.F3Backend), backend),
	)
}

// F3Disabled disables the F3 subsystem for this node.
func F3Disabled() NodeOpt {
	return ConstructorOpts(
		node.Unset(new(*lf3.Config)),
		node.Unset(new(lf3.F3Backend)),
		node.Unset(new(full.F3CertificateProvider)),
	)
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

var WithSealWorkerTasks = WithTaskTypes(append([]sealtasks.TaskType{sealtasks.TTAddPiece, sealtasks.TTDataCid, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2, sealtasks.TTUnseal}, DefaultNodeOpts.workerTasks...))

func WithWorkerStorage(transform func(paths.Store) paths.Store) NodeOpt {
	return func(opts *nodeOpts) error {
		opts.workerStorageOpt = transform
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

func SplitstoreDisable() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Chainstore.EnableSplitstore = false
		return nil
	})
}

func WithEthRPC() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Fevm.EnableEthRPC = true
		return nil
	})
}

func DisableETHBlockCache() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Fevm.EthBlkCacheSize = 0
		return nil
	})
}

func DisableEthRPC() NodeOpt {
	return WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.Fevm.EnableEthRPC = false
		return nil
	})
}
