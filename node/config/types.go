package config

import (
	"github.com/filecoin-project/lotus/chain/types"
)

// // NOTE: ONLY PUT STRUCT DEFINITIONS IN THIS FILE
// //
// // After making edits here, run 'make cfgdoc-gen' (or 'make gen')

// Common is common config between full node and miner
type Common struct {
	API     API
	Backup  Backup
	Logging Logging
}

// FullNode is a full node config
type FullNode struct {
	Common
	Libp2p        Libp2p
	Pubsub        Pubsub
	Wallet        Wallet
	Fees          FeeConfig
	Chainstore    Chainstore
	Fevm          FevmConfig
	Events        EventsConfig
	ChainIndexer  ChainIndexerConfig
	FaultReporter FaultReporterConfig
}

// // Common

type Backup struct {
	// When set to true disables metadata log (.lotus/kvlog). This can save disk
	// space by reducing metadata redundancy.
	//
	// Note that in case of metadata corruption it might be much harder to recover
	// your node if metadata log is disabled
	DisableMetadataLog bool
}

// Logging is the logging system config
type Logging struct {
	// SubsystemLevels specify per-subsystem log levels
	SubsystemLevels map[string]string
}

// StorageMiner is a miner config
type StorageMiner struct {
	Common

	Subsystems MinerSubsystemConfig
	Dealmaking DealmakingConfig
	Proving    ProvingConfig
	Sealing    SealingConfig
	Storage    SealerConfig
	Fees       MinerFeeConfig
	Addresses  MinerAddressConfig
	HarmonyDB  HarmonyDB
}

type ApisConfig struct {
	// ChainApiInfo is the API endpoint for the Lotus daemon.
	ChainApiInfo []string

	// RPC Secret for the storage subsystem.
	// If integrating with lotus-miner this must match the value from
	// cat ~/.lotusminer/keystore/MF2XI2BNNJ3XILLQOJUXMYLUMU | jq -r .PrivateKey
	StorageRPCSecret string
}

type JournalConfig struct {
	//Events of the form: "system1:event1,system1:event2[,...]"
	DisabledEvents string
}

type MinerSubsystemConfig struct {
	EnableMining        bool
	EnableSealing       bool
	EnableSectorStorage bool

	// When enabled, the sector index will reside in an external database
	// as opposed to the local KV store in the miner process
	// This is useful to allow workers to bypass the lotus miner to access sector information
	EnableSectorIndexDB bool

	SealerApiInfo      string // if EnableSealing == false
	SectorIndexApiInfo string // if EnableSectorStorage == false

	// When window post is enabled, the miner will automatically submit window post proofs
	// for all sectors that are eligible for window post
	// IF WINDOW POST IS DISABLED, THE MINER WILL NOT SUBMIT WINDOW POST PROOFS
	// THIS WILL RESULT IN FAULTS AND PENALTIES IF NO OTHER MECHANISM IS RUNNING
	// TO SUBMIT WINDOW POST PROOFS.
	// Note: This option is entirely disabling the window post scheduler,
	//   not just the builtin PoSt computation like Proving.DisableBuiltinWindowPoSt.
	//   This option will stop lotus-miner from performing any actions related
	//   to window post, including scheduling, submitting proofs, and recovering
	//   sectors.
	DisableWindowPoSt bool

	// When winning post is disabled, the miner process will NOT attempt to mine
	// blocks. This should only be set when there's an external process mining
	// blocks on behalf of the miner.
	// When disabled and no external block producers are configured, all potential
	// block rewards will be missed!
	DisableWinningPoSt bool
}

type DealmakingConfig struct {
	// Minimum start epoch buffer to give time for sealing of sector with deal.
	StartEpochSealingBuffer uint64
}

type ProvingConfig struct {
	// Maximum number of sector checks to run in parallel. (0 = unlimited)
	//
	// WARNING: Setting this value too high may make the node crash by running out of stack
	// WARNING: Setting this value too low may make sector challenge reading much slower, resulting in failed PoSt due
	// to late submission.
	//
	// After changing this option, confirm that the new value works in your setup by invoking
	// 'lotus-miner proving compute window-post 0'
	ParallelCheckLimit int

	// Maximum amount of time a proving pre-check can take for a sector. If the check times out the sector will be skipped
	//
	// WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
	// test challenge took longer than this timeout
	// WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this sector are
	// blocked (e.g. in case of disconnected NFS mount)
	SingleCheckTimeout Duration

	// Maximum amount of time a proving pre-check can take for an entire partition. If the check times out, sectors in
	// the partition which didn't get checked on time will be skipped
	//
	// WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
	// test challenge took longer than this timeout
	// WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this partition are
	// blocked or slow
	PartitionCheckTimeout Duration

	// Disable Window PoSt computation on the lotus-miner process even if no window PoSt workers are present.
	//
	// WARNING: If no windowPoSt workers are connected, window PoSt WILL FAIL resulting in faulty sectors which will need
	// to be recovered. Before enabling this option, make sure your PoSt workers work correctly.
	//
	// After changing this option, confirm that the new value works in your setup by invoking
	// 'lotus-miner proving compute window-post 0'
	DisableBuiltinWindowPoSt bool

	// Disable Winning PoSt computation on the lotus-miner process even if no winning PoSt workers are present.
	//
	// WARNING: If no WinningPoSt workers are connected, Winning PoSt WILL FAIL resulting in lost block rewards.
	// Before enabling this option, make sure your PoSt workers work correctly.
	DisableBuiltinWinningPoSt bool

	// Disable WindowPoSt provable sector readability checks.
	//
	// In normal operation, when preparing to compute WindowPoSt, lotus-miner will perform a round of reading challenges
	// from all sectors to confirm that those sectors can be proven. Challenges read in this process are discarded, as
	// we're only interested in checking that sector data can be read.
	//
	// When using builtin proof computation (no PoSt workers, and DisableBuiltinWindowPoSt is set to false), this process
	// can save a lot of time and compute resources in the case that some sectors are not readable - this is caused by
	// the builtin logic not skipping snark computation when some sectors need to be skipped.
	//
	// When using PoSt workers, this process is mostly redundant, with PoSt workers challenges will be read once, and
	// if challenges for some sectors aren't readable, those sectors will just get skipped.
	//
	// Disabling sector pre-checks will slightly reduce IO load when proving sectors, possibly resulting in shorter
	// time to produce window PoSt. In setups with good IO capabilities the effect of this option on proving time should
	// be negligible.
	//
	// NOTE: It likely is a bad idea to disable sector pre-checks in setups with no PoSt workers.
	//
	// NOTE: Even when this option is enabled, recovering sectors will be checked before recovery declaration message is
	// sent to the chain
	//
	// After changing this option, confirm that the new value works in your setup by invoking
	// 'lotus-miner proving compute window-post 0'
	DisableWDPoStPreChecks bool

	// Maximum number of partitions to prove in a single SubmitWindowPoSt messace. 0 = network limit (3 in nv21)
	//
	// A single partition may contain up to 2349 32GiB sectors, or 2300 64GiB sectors.
	//	//
	// Note that setting this value lower may result in less efficient gas use - more messages will be sent,
	// to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
	//
	// Setting this value above the network limit has no effect
	MaxPartitionsPerPoStMessage int

	// Maximum number of partitions to declare in a single DeclareFaultsRecovered message. 0 = no limit.

	// In some cases when submitting DeclareFaultsRecovered messages,
	// there may be too many recoveries to fit in a BlockGasLimit.
	// In those cases it may be necessary to set this value to something low (eg 1);
	// Note that setting this value lower may result in less efficient gas use - more messages will be sent than needed,
	// resulting in more total gas use (but each message will have lower gas limit)
	MaxPartitionsPerRecoveryMessage int

	// Enable single partition per PoSt Message for partitions containing recovery sectors
	//
	// In cases when submitting PoSt messages which contain recovering sectors, the default network limit may still be
	// too high to fit in the block gas limit. In those cases, it becomes useful to only house the single partition
	// with recovering sectors in the post message
	//
	// Note that setting this value lower may result in less efficient gas use - more messages will be sent,
	// to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
	SingleRecoveringPartitionPerPostMessage bool
}

type SealingConfig struct {
	// Upper bound on how many sectors can be waiting for more deals to be packed in it before it begins sealing at any given time.
	// If the miner is accepting multiple deals in parallel, up to MaxWaitDealsSectors of new sectors will be created.
	// If more than MaxWaitDealsSectors deals are accepted in parallel, only MaxWaitDealsSectors deals will be processed in parallel
	// Note that setting this number too high in relation to deal ingestion rate may result in poor sector packing efficiency
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// Upper bound on how many sectors can be sealing+upgrading at the same time when creating new CC sectors (0 = unlimited)
	MaxSealingSectors uint64

	// Upper bound on how many sectors can be sealing+upgrading at the same time when creating new sectors with deals (0 = unlimited)
	MaxSealingSectorsForDeals uint64

	// Prefer creating new sectors even if there are sectors Available for upgrading.
	// This setting combined with MaxUpgradingSectors set to a value higher than MaxSealingSectorsForDeals makes it
	// possible to use fast sector upgrades to handle high volumes of storage deals, while still using the simple sealing
	// flow when the volume of storage deals is lower.
	PreferNewSectorsForDeals bool

	// Upper bound on how many sectors can be sealing+upgrading at the same time when upgrading CC sectors with deals (0 = MaxSealingSectorsForDeals)
	MaxUpgradingSectors uint64

	// When set to a non-zero value, minimum number of epochs until sector expiration required for sectors to be considered
	// for upgrades (0 = DealMinDuration = 180 days = 518400 epochs)
	//
	// Note that if all deals waiting in the input queue have lifetimes longer than this value, upgrade sectors will be
	// required to have expiration of at least the soonest-ending deal
	MinUpgradeSectorExpiration uint64

	// DEPRECATED: Target expiration is no longer used
	MinTargetUpgradeSectorExpiration uint64

	// CommittedCapacitySectorLifetime is the duration a Committed Capacity (CC) sector will
	// live before it must be extended or converted into sector containing deals before it is
	// terminated. Value must be between 180-1278 days (1278 in nv21, 540 before nv21).
	CommittedCapacitySectorLifetime Duration

	// Period of time that a newly created sector will wait for more deals to be packed in to before it starts to seal.
	// Sectors which are fully filled will start sealing immediately
	WaitDealsDelay Duration

	// Whether to keep unsealed copies of deal data regardless of whether the client requested that. This lets the miner
	// avoid the relatively high cost of unsealing the data later, at the cost of more storage space
	AlwaysKeepUnsealedCopy bool

	// Run sector finalization before submitting sector proof to the chain
	FinalizeEarly bool

	// Whether new sectors are created to pack incoming deals
	// When this is set to false no new sectors will be created for sealing incoming deals
	// This is useful for forcing all deals to be assigned as snap deals to sectors marked for upgrade
	MakeNewSectorForDeals bool

	// After sealing CC sectors, make them available for upgrading with deals
	MakeCCSectorsAvailable bool

	// Whether to use available miner balance for sector collateral instead of sending it with each message
	CollateralFromMinerBalance bool
	// Minimum available balance to keep in the miner actor before sending it with messages
	AvailableBalanceBuffer types.FIL
	// Don't send collateral with messages even if there is no available balance in the miner actor
	DisableCollateralFallback bool

	// maximum precommit batch size - batches will be sent immediately above this size
	MaxPreCommitBatch int
	// how long to wait before submitting a batch after crossing the minimum batch size
	PreCommitBatchWait Duration
	// time buffer for forceful batch submission before sectors/deal in batch would start expiring
	PreCommitBatchSlack Duration

	// enable / disable commit aggregation (takes effect after nv13)
	AggregateCommits bool
	// minimum batched commit size - batches above this size will eventually be sent on a timeout
	MinCommitBatch int
	// maximum batched commit size - batches will be sent immediately above this size
	MaxCommitBatch int
	// how long to wait before submitting a batch after crossing the minimum batch size
	CommitBatchWait Duration
	// time buffer for forceful batch submission before sectors/deals in batch would start expiring
	CommitBatchSlack Duration

	// When submitting several sector prove commit messages simultaneously, this option allows you to
	// stagger the number of prove commits submitted per epoch
	// This is done because gas estimates for ProveCommits are non deterministic and increasing as a large
	// number of sectors get committed within the same epoch resulting in occasionally failed msgs.
	// Submitting a smaller number of prove commits per epoch would reduce the possibility of failed msgs
	MaxSectorProveCommitsSubmittedPerEpoch uint64

	TerminateBatchMax  uint64
	TerminateBatchMin  uint64
	TerminateBatchWait Duration

	// Keep this many sectors in sealing pipeline, start CC if needed
	// todo TargetSealingSectors uint64

	// todo TargetSectors - stop auto-pleding new sectors after this many sectors are sealed, default CC upgrade for deals sectors if above

	// UseSyntheticPoRep, when set to true, will reduce the amount of cache data held on disk after the completion of PreCommit 2 to 11GiB.
	UseSyntheticPoRep bool

	// Whether to abort if any sector activation in a batch fails (newly sealed sectors, only with ProveCommitSectors3).
	RequireActivationSuccess bool
	// Whether to abort if any piece activation notification returns a non-zero exit code (newly sealed sectors, only with ProveCommitSectors3).
	RequireActivationSuccessUpdate bool
	// Whether to abort if any sector activation in a batch fails (updating sectors, only with ProveReplicaUpdates3).
	RequireNotificationSuccess bool
	// Whether to abort if any piece activation notification returns a non-zero exit code (updating sectors, only with ProveReplicaUpdates3).
	RequireNotificationSuccessUpdate bool
}

type SealerConfig struct {
	ParallelFetchLimit int

	AllowSectorDownload      bool
	AllowAddPiece            bool
	AllowPreCommit1          bool
	AllowPreCommit2          bool
	AllowCommit              bool
	AllowUnseal              bool
	AllowReplicaUpdate       bool
	AllowProveReplicaUpdate2 bool
	AllowRegenSectorKey      bool

	// LocalWorkerName specifies a custom name for the builtin worker.
	// If set to an empty string (default) os hostname will be used
	LocalWorkerName string

	// Assigner specifies the worker assigner to use when scheduling tasks.
	// "utilization" (default) - assign tasks to workers with lowest utilization.
	// "spread" - assign tasks to as many distinct workers as possible.
	Assigner string

	// DisallowRemoteFinalize when set to true will force all Finalize tasks to
	// run on workers with local access to both long-term storage and the sealing
	// path containing the sector.
	// --
	// WARNING: Only set this if all workers have access to long-term storage
	// paths. If this flag is enabled, and there are workers without long-term
	// storage access, sectors will not be moved from them, and Finalize tasks
	// will appear to be stuck.
	// --
	// If you see stuck Finalize tasks after enabling this setting, check
	// 'lotus-miner sealing sched-diag' and 'lotus-miner storage find [sector num]'
	DisallowRemoteFinalize bool

	// ResourceFiltering instructs the system which resource filtering strategy
	// to use when evaluating tasks against this worker. An empty value defaults
	// to "hardware".
	ResourceFiltering ResourceFilteringStrategy
}

type BatchFeeConfig struct {
	Base      types.FIL
	PerSector types.FIL
}

type MinerFeeConfig struct {
	MaxPreCommitGasFee types.FIL
	MaxCommitGasFee    types.FIL

	// maxBatchFee = maxBase + maxPerSector * nSectors
	MaxPreCommitBatchGasFee BatchFeeConfig
	MaxCommitBatchGasFee    BatchFeeConfig

	MaxTerminateGasFee types.FIL
	// WindowPoSt is a high-value operation, so the default fee should be high.
	MaxWindowPoStGasFee    types.FIL
	MaxPublishDealsFee     types.FIL
	MaxMarketBalanceAddFee types.FIL

	MaximizeWindowPoStFeeCap bool
}

type MinerAddressConfig struct {
	// Addresses to send PreCommit messages from
	PreCommitControl []string
	// Addresses to send Commit messages from
	CommitControl      []string
	TerminateControl   []string
	DealPublishControl []string

	// DisableOwnerFallback disables usage of the owner address for messages
	// sent automatically
	DisableOwnerFallback bool
	// DisableWorkerFallback disables usage of the worker address for messages
	// sent automatically, if control addresses are configured.
	// A control address that doesn't have enough funds will still be chosen
	// over the worker address if this flag is set.
	DisableWorkerFallback bool
}

// API contains configs for API endpoint
type API struct {
	// Binding address for the Lotus API
	ListenAddress       string
	RemoteListenAddress string
	Timeout             Duration
}

// Libp2p contains configs for libp2p
type Libp2p struct {
	// Binding address for the libp2p host - 0 means random port.
	// Format: multiaddress; see https://multiformats.io/multiaddr/
	ListenAddresses []string
	// Addresses to explicitally announce to other peers. If not specified,
	// all interface addresses are announced
	// Format: multiaddress
	AnnounceAddresses []string
	// Addresses to not announce
	// Format: multiaddress
	NoAnnounceAddresses []string
	BootstrapPeers      []string
	ProtectedPeers      []string

	// When not disabled (default), lotus asks NAT devices (e.g., routers), to
	// open up an external port and forward it to the port lotus is running on.
	// When this works (i.e., when your router supports NAT port forwarding),
	// it makes the local lotus node accessible from the public internet
	DisableNatPortMap bool

	// ConnMgrLow is the number of connections that the basic connection manager
	// will trim down to.
	ConnMgrLow uint
	// ConnMgrHigh is the number of connections that, when exceeded, will trigger
	// a connection GC operation. Note: protected/recently formed connections don't
	// count towards this limit.
	ConnMgrHigh uint
	// ConnMgrGrace is a time duration that new connections are immune from being
	// closed by the connection manager.
	ConnMgrGrace Duration
}

type Pubsub struct {
	// Run the node in bootstrap-node mode
	Bootstrapper bool
	// DirectPeers specifies peers with direct peering agreements. These peers are
	// connected outside of the mesh, with all (valid) message unconditionally
	// forwarded to them. The router will maintain open connections to these peers.
	// Note that the peering agreement should be reciprocal with direct peers
	// symmetrically configured at both ends.
	// Type: Array of multiaddress peerinfo strings, must include peerid (/p2p/12D3K...
	DirectPeers           []string
	IPColocationWhitelist []string
	RemoteTracer          string
	// Path to file that will be used to output tracer content in JSON format.
	// If present tracer will save data to defined file.
	// Format: file path
	JsonTracer string
	// Connection string for elasticsearch instance.
	// If present tracer will save data to elasticsearch.
	// Format: https://<username>:<password>@<elasticsearch_url>:<port>/
	ElasticSearchTracer string
	// Name of elasticsearch index that will be used to save tracer data.
	// This property is used only if ElasticSearchTracer propery is set.
	ElasticSearchIndex string
	// Auth token that will be passed with logs to elasticsearch - used for weighted peers score.
	TracerSourceAuth string
}

type Chainstore struct {
	EnableSplitstore bool
	Splitstore       Splitstore
}

type Splitstore struct {
	// ColdStoreType specifies the type of the coldstore.
	// It can be "discard" (default) for discarding cold blocks, "messages" to store only messages or "universal" to store all chain state..
	ColdStoreType string
	// HotStoreType specifies the type of the hotstore.
	// Only currently supported value is "badger".
	HotStoreType string
	// MarkSetType specifies the type of the markset.
	// It can be "map" for in memory marking or "badger" (default) for on-disk marking.
	MarkSetType string

	// HotStoreMessageRetention specifies the retention policy for messages, in finalities beyond
	// the compaction boundary; default is 0.
	HotStoreMessageRetention uint64
	// HotStoreFullGCFrequency specifies how often to perform a full (moving) GC on the hotstore.
	// A value of 0 disables, while a value 1 will do full GC in every compaction.
	// Default is 20 (about once a week).
	HotStoreFullGCFrequency uint64
	// HotStoreMaxSpaceTarget sets a target max disk size for the hotstore. Splitstore GC
	// will run moving GC if disk utilization gets within a threshold (150 GB) of the target.
	// Splitstore GC will NOT run moving GC if the total size of the move would get
	// within 50 GB of the target, and instead will run a more aggressive online GC.
	// If both HotStoreFullGCFrequency and HotStoreMaxSpaceTarget are set then splitstore
	// GC will trigger moving GC if either configuration condition is met.
	// A reasonable minimum is 2x fully GCed hotstore size + 50 G buffer.
	// At this minimum size moving GC happens every time, any smaller and moving GC won't
	// be able to run. In spring 2023 this minimum is ~550 GB.
	HotStoreMaxSpaceTarget uint64

	// When HotStoreMaxSpaceTarget is set Moving GC will be triggered when total moving size
	// exceeds HotstoreMaxSpaceTarget - HotstoreMaxSpaceThreshold
	HotStoreMaxSpaceThreshold uint64

	// Safety buffer to prevent moving GC from overflowing disk when HotStoreMaxSpaceTarget
	// is set.  Moving GC will not occur when total moving size exceeds
	// HotstoreMaxSpaceTarget - HotstoreMaxSpaceSafetyBuffer
	HotstoreMaxSpaceSafetyBuffer uint64
}

// Full Node

type Wallet struct {
	RemoteBackend string
	EnableLedger  bool
	DisableLocal  bool
}

type FeeConfig struct {
	DefaultMaxFee types.FIL
}

type FevmConfig struct {
	// EnableEthRPC enables eth_ RPC methods.
	// Note: Setting this to true will also require that ChainIndexer is enabled, otherwise it will cause an error at startup.
	// Set EnableIndexer in the ChainIndexer section of the config to true to enable the ChainIndexer.
	EnableEthRPC bool

	// EthTraceFilterMaxResults sets the maximum results returned per request by trace_filter
	EthTraceFilterMaxResults uint64

	// EthBlkCacheSize specifies the size of the cache used for caching Ethereum blocks.
	// This cache enhances the performance of the eth_getBlockByHash RPC call by minimizing the need to access chain state for
	// recently requested blocks that are already cached.
	// The default size of the cache is 500 blocks.
	// Note: Setting this value to 0 disables the cache.
	EthBlkCacheSize int
}

type EventsConfig struct {
	// EnableActorEventsAPI enables the Actor events API that enables clients to consume events
	// emitted by (smart contracts + built-in Actors).
	// Note: Setting this to true will also require that ChainIndexer is enabled, otherwise it will cause an error at startup.
	// Set EnableIndexer in the ChainIndexer section of the config to true to enable the ChainIndexer.
	EnableActorEventsAPI bool

	// FilterTTL specifies the time to live for actor event filters. Filters that haven't been accessed longer than
	// this time become eligible for automatic deletion. Filters consume resources, so if they are unused they
	// should not be retained.
	FilterTTL Duration

	// MaxFilters specifies the maximum number of filters that may exist at any one time.
	// Multi-tenant environments may want to increase this value to serve a larger number of clients. If using
	// lotus-gateway, this global limit can be coupled with --eth-max-filters-per-conn which limits the number
	// of filters per connection.
	MaxFilters int

	// MaxFilterResults specifies the maximum number of results that can be accumulated by an actor event filter.
	MaxFilterResults int

	// MaxFilterHeightRange specifies the maximum range of heights that can be used in a filter (to avoid querying
	// the entire chain)
	MaxFilterHeightRange uint64
}

type ChainIndexerConfig struct {
	// EnableIndexer controls whether the chain indexer is active.
	// The chain indexer is responsible for indexing tipsets, messages, and events from the chain state.
	// It is a crucial component for optimizing Lotus RPC response times.
	//
	// Default: false (indexer is disabled)
	//
	// Setting this to true will enable the indexer, which will significantly improve RPC performance.
	// It is strongly recommended to keep this set to true if you are an RPC provider.
	//
	// If EnableEthRPC or EnableActorEventsAPI are set to true, the ChainIndexer must be enabled using
	// this option to avoid errors at startup.
	EnableIndexer bool

	// GCRetentionEpochs specifies the number of epochs for which data is retained in the Indexer.
	// The garbage collection (GC) process removes data older than this retention period.
	// Setting this to 0 disables GC, preserving all historical data indefinitely.
	//
	// If set, the minimum value must be greater than builtin.EpochsInDay (i.e. "2880" epochs for mainnet).
	// This ensures a reasonable retention period for the indexed data.
	//
	// Default: 0 (GC disabled)
	GCRetentionEpochs int64

	// ReconcileEmptyIndex determines whether to reconcile the index with the chain state
	// during startup when the index is empty.
	//
	// When set to true:
	// - On startup, if the index is empty, the indexer will index the available
	//   chain state on the node albeit within the MaxReconcileTipsets limit.
	//
	// When set to false:
	// - The indexer will not automatically re-index the chain state on startup if the index is empty.
	//
	// Default: false
	//
	// Note: The number of tipsets reconciled (i.e. indexed) during this process can be
	// controlled using the MaxReconcileTipsets option.
	ReconcileEmptyIndex bool

	// MaxReconcileTipsets limits the number of tipsets to reconcile with the chain during startup.
	// It represents the maximum number of tipsets to index from the chain state that are absent in the index.
	//
	// Default: 3 * epochsPerDay (approximately 3 days of chain history)
	//
	// Note: Setting this value too low may result in incomplete indexing, while setting it too high
	// may increase startup time.
	MaxReconcileTipsets uint64

	// AllowIndexReconciliationFailure determines whether node startup should continue
	// if the index reconciliation with the chain state fails.
	//
	// When set to true:
	// - If index reconciliation fails during startup, the node will log a warning but continue to start.
	//
	// When set to false (default):
	// - If index reconciliation fails during startup, the node will fail to start.
	// - This ensures that the index is always in a consistent state with the chain before the node starts.
	//
	// Default: false
	// // WARNING: Only set to true if you are okay with an index that may be out of sync with the chain.
	// This can lead to inaccurate or missing data in RPC responses that depend on the indexer.
	AllowIndexReconciliationFailure bool
}

type HarmonyDB struct {
	// HOSTS is a list of hostnames to nodes running YugabyteDB
	// in a cluster. Only 1 is required
	Hosts []string

	// The Yugabyte server's username with full credentials to operate on Lotus' Database. Blank for default.
	Username string

	// The password for the related username. Blank for default.
	Password string

	// The database (logical partition) within Yugabyte. Blank for default.
	Database string

	// The port to find Yugabyte. Blank for default.
	Port string
}

type FaultReporterConfig struct {
	// EnableConsensusFaultReporter controls whether the node will monitor and
	// report consensus faults. When enabled, the node will watch for malicious
	// behaviors like double-mining and parent grinding, and submit reports to the
	// network. This can earn reporter rewards, but is not guaranteed. Nodes should
	// enable fault reporting with care, as it may increase resource usage, and may
	// generate gas fees without earning rewards.
	EnableConsensusFaultReporter bool

	// ConsensusFaultReporterDataDir is the path where fault reporter state will be
	// persisted. This directory should have adequate space and permissions for the
	// node process.
	ConsensusFaultReporterDataDir string

	// ConsensusFaultReporterAddress is the wallet address used for submitting
	// ReportConsensusFault messages. It will pay for gas fees, and receive any
	// rewards. This address should have adequate funds to cover gas fees.
	ConsensusFaultReporterAddress string
}
