package config

import (
	"encoding"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
)

func (b *BatchFeeConfig) FeeForSectors(nSectors int) abi.TokenAmount {
	return big.Add(big.Int(b.Base), big.Mul(big.NewInt(int64(nSectors)), big.Int(b.PerSector)))
}

func defCommon() Common {
	return Common{
		API: API{
			ListenAddress: "/ip4/127.0.0.1/tcp/1234/http",
			Timeout:       Duration(30 * time.Second),
		},
		Logging: Logging{
			SubsystemLevels: map[string]string{
				"example-subsystem": "INFO",
			},
		},
		Backup: Backup{
			DisableMetadataLog: true,
		},
	}
}

func DefaultDefaultMaxFee() types.FIL {
	return types.MustParseFIL("0.07")
}

// DefaultFullNode returns the default config
func DefaultFullNode() *FullNode {
	return &FullNode{
		Common: defCommon(),

		Libp2p: Libp2p{
			ListenAddresses: []string{
				"/ip4/0.0.0.0/tcp/0",
				"/ip6/::/tcp/0",
				"/ip4/0.0.0.0/udp/0/quic-v1",
				"/ip6/::/udp/0/quic-v1",
				"/ip4/0.0.0.0/udp/0/quic-v1/webtransport",
				"/ip6/::/udp/0/quic-v1/webtransport",
			},
			AnnounceAddresses:   []string{},
			NoAnnounceAddresses: []string{},

			ConnMgrLow:   150,
			ConnMgrHigh:  180,
			ConnMgrGrace: Duration(20 * time.Second),
		},
		Pubsub: Pubsub{
			Bootstrapper: false,
			DirectPeers:  nil,
		},

		Fees: FeeConfig{
			DefaultMaxFee: DefaultDefaultMaxFee(),
		},

		Chainstore: Chainstore{
			EnableSplitstore: true,
			Splitstore: Splitstore{
				ColdStoreType: "discard",
				HotStoreType:  "badger",
				MarkSetType:   "badger",

				HotStoreFullGCFrequency:      20,
				HotStoreMaxSpaceTarget:       650_000_000_000,
				HotStoreMaxSpaceThreshold:    150_000_000_000,
				HotstoreMaxSpaceSafetyBuffer: 50_000_000_000,
			},
		},
		Fevm: FevmConfig{
			EnableEthRPC:             false,
			EthTraceFilterMaxResults: 500,
			EthBlkCacheSize:          500,
		},
		Events: EventsConfig{
			EnableActorEventsAPI: false,
			FilterTTL:            Duration(time.Hour * 1),
			MaxFilters:           100,
			MaxFilterResults:     10000,
			MaxFilterHeightRange: 2880, // conservative limit of one day
		},
		ChainIndexer: ChainIndexerConfig{
			EnableIndexer:       false,
			GCRetentionEpochs:   0,
			ReconcileEmptyIndex: false,
			MaxReconcileTipsets: 3 * builtin.EpochsInDay,
		},
	}
}

func DefaultStorageMiner() *StorageMiner {
	// TODO: Should we increase this to nv21, which would push it to 3.5 years?
	maxSectorExtentsion, _ := policy.GetMaxSectorExpirationExtension(network.Version20)
	cfg := &StorageMiner{
		Common: defCommon(),

		Sealing: SealingConfig{
			MaxWaitDealsSectors:       2, // 64G with 32G sectors
			MaxSealingSectors:         0,
			MaxSealingSectorsForDeals: 0,
			WaitDealsDelay:            Duration(time.Hour * 6),
			AlwaysKeepUnsealedCopy:    true,
			FinalizeEarly:             false,
			MakeNewSectorForDeals:     true,

			CollateralFromMinerBalance: false,
			AvailableBalanceBuffer:     types.FIL(big.Zero()),
			DisableCollateralFallback:  false,

			MaxPreCommitBatch:  256,
			PreCommitBatchWait: Duration(24 * time.Hour), // this should be less than 31.5 hours, which is the expiration of a precommit ticket
			// XXX snap deals wait deals slack if first
			PreCommitBatchSlack: Duration(3 * time.Hour), // time buffer for forceful batch submission before sectors/deals in batch would start expiring, higher value will lower the chances for message fail due to expiration

			CommittedCapacitySectorLifetime: Duration(builtin.EpochDurationSeconds * uint64(maxSectorExtentsion) * uint64(time.Second)),

			MinCommitBatch:   miner5.MinAggregatedSectors, // per FIP13, we must have at least four proofs to aggregate, where 4 is the cross over point where aggregation wins out on single provecommit gas costs
			MaxCommitBatch:   miner5.MaxAggregatedSectors, // maximum 819 sectors, this is the maximum aggregation per FIP13
			CommitBatchWait:  Duration(24 * time.Hour),    // this can be up to 30 days
			CommitBatchSlack: Duration(1 * time.Hour),     // time buffer for forceful batch submission before sectors/deals in batch would start expiring, higher value will lower the chances for message fail due to expiration

			TerminateBatchMin:                      1,
			TerminateBatchMax:                      100,
			TerminateBatchWait:                     Duration(5 * time.Minute),
			MaxSectorProveCommitsSubmittedPerEpoch: 20,
			UseSyntheticPoRep:                      false,
		},

		Proving: ProvingConfig{
			ParallelCheckLimit:    32,
			PartitionCheckTimeout: Duration(20 * time.Minute),
			SingleCheckTimeout:    Duration(10 * time.Minute),
		},

		Storage: SealerConfig{
			AllowSectorDownload:      true,
			AllowAddPiece:            true,
			AllowPreCommit1:          true,
			AllowPreCommit2:          true,
			AllowCommit:              true,
			AllowUnseal:              true,
			AllowReplicaUpdate:       true,
			AllowProveReplicaUpdate2: true,
			AllowRegenSectorKey:      true,

			// Default to 10 - tcp should still be able to figure this out, and
			// it's the ratio between 10gbit / 1gbit
			ParallelFetchLimit: 10,

			Assigner: "utilization",

			// By default use the hardware resource filtering strategy.
			ResourceFiltering: ResourceFilteringHardware,
		},

		Dealmaking: DealmakingConfig{
			StartEpochSealingBuffer: 480, // 480 epochs buffer == 4 hours from adding deal to sector to sector being sealed
		},

		Subsystems: MinerSubsystemConfig{
			EnableMining:        true,
			EnableSealing:       true,
			EnableSectorStorage: true,
			EnableSectorIndexDB: false,
		},

		Fees: MinerFeeConfig{
			MaxPreCommitGasFee: types.MustParseFIL("0.025"),
			MaxCommitGasFee:    types.MustParseFIL("0.05"),

			MaxPreCommitBatchGasFee: BatchFeeConfig{
				Base:      types.MustParseFIL("0"),
				PerSector: types.MustParseFIL("0.02"),
			},
			MaxCommitBatchGasFee: BatchFeeConfig{
				Base:      types.MustParseFIL("0"),
				PerSector: types.MustParseFIL("0.03"), // enough for 6 agg and 1nFIL base fee
			},

			MaxTerminateGasFee:     types.MustParseFIL("0.5"),
			MaxWindowPoStGasFee:    types.MustParseFIL("5"),
			MaxPublishDealsFee:     types.MustParseFIL("0.05"),
			MaxMarketBalanceAddFee: types.MustParseFIL("0.007"),

			MaximizeWindowPoStFeeCap: true,
		},

		Addresses: MinerAddressConfig{
			PreCommitControl:   []string{},
			CommitControl:      []string{},
			TerminateControl:   []string{},
			DealPublishControl: []string{},
		},

		HarmonyDB: HarmonyDB{
			Hosts:    []string{"127.0.0.1"},
			Username: "yugabyte",
			Password: "yugabyte",
			Database: "yugabyte",
			Port:     "5433",
		},
	}

	cfg.Common.API.ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
	cfg.Common.API.RemoteListenAddress = "127.0.0.1:2345"
	return cfg
}

var (
	_ encoding.TextMarshaler   = (*Duration)(nil)
	_ encoding.TextUnmarshaler = (*Duration)(nil)
)

// Duration is a wrapper type for time.Duration
// for decoding and encoding from/to TOML
type Duration time.Duration

// UnmarshalText implements interface for TOML decoding
func (dur *Duration) UnmarshalText(text []byte) error {
	d, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*dur = Duration(d)
	return err
}

func (dur Duration) MarshalText() ([]byte, error) {
	d := time.Duration(dur)
	return []byte(d.String()), nil
}

// ResourceFilteringStrategy is an enum indicating the kinds of resource
// filtering strategies that can be configured for workers.
type ResourceFilteringStrategy string

const (
	// ResourceFilteringHardware specifies that available hardware resources
	// should be evaluated when scheduling a task against the worker.
	ResourceFilteringHardware = ResourceFilteringStrategy("hardware")

	// ResourceFilteringDisabled disables resource filtering against this
	// worker. The scheduler may assign any task to this worker.
	ResourceFilteringDisabled = ResourceFilteringStrategy("disabled")
)
