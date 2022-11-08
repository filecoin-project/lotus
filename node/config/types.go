package config

import (
	"github.com/ipfs/go-cid"

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
	Libp2p  Libp2p
	Pubsub  Pubsub
}

// FullNode is a full node config
type FullNode struct {
	Common
	Client     Client
	Wallet     Wallet
	Fees       FeeConfig
	Chainstore Chainstore
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

	Subsystems    MinerSubsystemConfig
	Dealmaking    DealmakingConfig
	IndexProvider IndexProviderConfig
	Proving       ProvingConfig
	Sealing       SealingConfig
	Storage       SealerConfig
	Fees          MinerFeeConfig
	Addresses     MinerAddressConfig
	DAGStore      DAGStoreConfig
}

type DAGStoreConfig struct {
	// Path to the dagstore root directory. This directory contains three
	// subdirectories, which can be symlinked to alternative locations if
	// need be:
	//  - ./transients: caches unsealed deals that have been fetched from the
	//    storage subsystem for serving retrievals.
	//  - ./indices: stores shard indices.
	//  - ./datastore: holds the KV store tracking the state of every shard
	//    known to the DAG store.
	// Default value: <LOTUS_MARKETS_PATH>/dagstore (split deployment) or
	// <LOTUS_MINER_PATH>/dagstore (monolith deployment)
	RootDir string

	// The maximum amount of indexing jobs that can run simultaneously.
	// 0 means unlimited.
	// Default value: 5.
	MaxConcurrentIndex int

	// The maximum amount of unsealed deals that can be fetched simultaneously
	// from the storage subsystem. 0 means unlimited.
	// Default value: 0 (unlimited).
	MaxConcurrentReadyFetches int

	// The maximum amount of unseals that can be processed simultaneously
	// from the storage subsystem. 0 means unlimited.
	// Default value: 0 (unlimited).
	MaxConcurrentUnseals int

	// The maximum number of simultaneous inflight API calls to the storage
	// subsystem.
	// Default value: 100.
	MaxConcurrencyStorageCalls int

	// The time between calls to periodic dagstore GC, in time.Duration string
	// representation, e.g. 1m, 5m, 1h.
	// Default value: 1 minute.
	GCInterval Duration
}

type MinerSubsystemConfig struct {
	EnableMining        bool
	EnableSealing       bool
	EnableSectorStorage bool
	EnableMarkets       bool

	SealerApiInfo      string // if EnableSealing == false
	SectorIndexApiInfo string // if EnableSectorStorage == false
}

type DealmakingConfig struct {
	// When enabled, the miner can accept online deals
	ConsiderOnlineStorageDeals bool
	// When enabled, the miner can accept offline deals
	ConsiderOfflineStorageDeals bool
	// When enabled, the miner can accept retrieval deals
	ConsiderOnlineRetrievalDeals bool
	// When enabled, the miner can accept offline retrieval deals
	ConsiderOfflineRetrievalDeals bool
	// When enabled, the miner can accept verified deals
	ConsiderVerifiedStorageDeals bool
	// When enabled, the miner can accept unverified deals
	ConsiderUnverifiedStorageDeals bool
	// A list of Data CIDs to reject when making deals
	PieceCidBlocklist []cid.Cid
	// Maximum expected amount of time getting the deal into a sealed sector will take
	// This includes the time the deal will need to get transferred and published
	// before being assigned to a sector
	ExpectedSealDuration Duration
	// Maximum amount of time proposed deal StartEpoch can be in future
	MaxDealStartDelay Duration
	// When a deal is ready to publish, the amount of time to wait for more
	// deals to be ready to publish before publishing them all as a batch
	PublishMsgPeriod Duration
	// The maximum number of deals to include in a single PublishStorageDeals
	// message
	MaxDealsPerPublishMsg uint64
	// The maximum collateral that the provider will put up against a deal,
	// as a multiplier of the minimum collateral bound
	MaxProviderCollateralMultiplier uint64
	// The maximum allowed disk usage size in bytes of staging deals not yet
	// passed to the sealing node by the markets service. 0 is unlimited.
	MaxStagingDealsBytes int64
	// The maximum number of parallel online data transfers for storage deals
	SimultaneousTransfersForStorage uint64
	// The maximum number of simultaneous data transfers from any single client
	// for storage deals.
	// Unset by default (0), and values higher than SimultaneousTransfersForStorage
	// will have no effect; i.e. the total number of simultaneous data transfers
	// across all storage clients is bound by SimultaneousTransfersForStorage
	// regardless of this number.
	SimultaneousTransfersForStoragePerClient uint64
	// The maximum number of parallel online data transfers for retrieval deals
	SimultaneousTransfersForRetrieval uint64
	// Minimum start epoch buffer to give time for sealing of sector with deal.
	StartEpochSealingBuffer uint64

	// A command used for fine-grained evaluation of storage deals
	// see https://lotus.filecoin.io/storage-providers/advanced-configurations/market/#using-filters-for-fine-grained-storage-and-retrieval-deal-acceptance for more details
	Filter string
	// A command used for fine-grained evaluation of retrieval deals
	// see https://lotus.filecoin.io/storage-providers/advanced-configurations/market/#using-filters-for-fine-grained-storage-and-retrieval-deal-acceptance for more details
	RetrievalFilter string

	RetrievalPricing *RetrievalPricing
}

type IndexProviderConfig struct {

	// Enable set whether to enable indexing announcement to the network and expose endpoints that
	// allow indexer nodes to process announcements. Enabled by default.
	Enable bool

	// EntriesCacheCapacity sets the maximum capacity to use for caching the indexing advertisement
	// entries. Defaults to 1024 if not specified. The cache is evicted using LRU policy. The
	// maximum storage used by the cache is a factor of EntriesCacheCapacity, EntriesChunkSize and
	// the length of multihashes being advertised. For example, advertising 128-bit long multihashes
	// with the default EntriesCacheCapacity, and EntriesChunkSize means the cache size can grow to
	// 256MiB when full.
	EntriesCacheCapacity int

	// EntriesChunkSize sets the maximum number of multihashes to include in a single entries chunk.
	// Defaults to 16384 if not specified. Note that chunks are chained together for indexing
	// advertisements that include more multihashes than the configured EntriesChunkSize.
	EntriesChunkSize int

	// TopicName sets the topic name on which the changes to the advertised content are announced.
	// If not explicitly specified, the topic name is automatically inferred from the network name
	// in following format: '/indexer/ingest/<network-name>'
	// Defaults to empty, which implies the topic name is inferred from network name.
	TopicName string

	// PurgeCacheOnStart sets whether to clear any cached entries chunks when the provider engine
	// starts. By default, the cache is rehydrated from previously cached entries stored in
	// datastore if any is present.
	PurgeCacheOnStart bool
}

type RetrievalPricing struct {
	Strategy string // possible values: "default", "external"

	Default  *RetrievalPricingDefault
	External *RetrievalPricingExternal
}

type RetrievalPricingExternal struct {
	// Path of the external script that will be run to price a retrieval deal.
	// This parameter is ONLY applicable if the retrieval pricing policy strategy has been configured to "external".
	Path string
}

type RetrievalPricingDefault struct {
	// VerifiedDealsFreeTransfer configures zero fees for data transfer for a retrieval deal
	// of a payloadCid that belongs to a verified storage deal.
	// This parameter is ONLY applicable if the retrieval pricing policy strategy has been configured to "default".
	// default value is true
	VerifiedDealsFreeTransfer bool
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

	// Maximum number of partitions to prove in a single SubmitWindowPoSt messace. 0 = network limit (10 in nv16)
	//
	// A single partition may contain up to 2349 32GiB sectors, or 2300 64GiB sectors.
	//
	// The maximum number of sectors which can be proven in a single PoSt message is 25000 in network version 16, which
	// means that a single message can prove at most 10 partitions
	//
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

	// When set to a non-zero value, minimum number of epochs until sector expiration above which upgrade candidates will
	// be selected based on lowest initial pledge.
	//
	// Target sector expiration is calculated by looking at the input deal queue, sorting it by deal expiration, and
	// selecting N deals from the queue up to sector size. The target expiration will be Nth deal end epoch, or in case
	// where there weren't enough deals to fill a sector, DealMaxDuration (540 days = 1555200 epochs)
	//
	// Setting this to a high value (for example to maximum deal duration - 1555200) will disable selection based on
	// initial pledge - upgrade sectors will always be chosen based on longest expiration
	MinTargetUpgradeSectorExpiration uint64

	// CommittedCapacitySectorLifetime is the duration a Committed Capacity (CC) sector will
	// live before it must be extended or converted into sector containing deals before it is
	// terminated. Value must be between 180-540 days inclusive
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

	// enable / disable precommit batching (takes effect after nv13)
	BatchPreCommits bool
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

	// network BaseFee below which to stop doing precommit batching, instead
	// sending precommit messages to the chain individually
	BatchPreCommitAboveBaseFee types.FIL

	// network BaseFee below which to stop doing commit aggregation, instead
	// submitting proofs to the chain individually
	AggregateAboveBaseFee types.FIL

	TerminateBatchMax  uint64
	TerminateBatchMin  uint64
	TerminateBatchWait Duration

	// Keep this many sectors in sealing pipeline, start CC if needed
	// todo TargetSealingSectors uint64

	// todo TargetSectors - stop auto-pleding new sectors after this many sectors are sealed, default CC upgrade for deals sectors if above
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
}

type Chainstore struct {
	EnableSplitstore bool
	Splitstore       Splitstore
}

type Splitstore struct {
	// ColdStoreType specifies the type of the coldstore.
	// It can be "messages" (default) to store only messages, "universal" to store all chain state or "discard" for discarding cold blocks.
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
}

// // Full Node
type Client struct {
	UseIpfs             bool
	IpfsOnlineMode      bool
	IpfsMAddr           string
	IpfsUseForRetrieval bool
	// The maximum number of simultaneous data transfers between the client
	// and storage providers for storage deals
	SimultaneousTransfersForStorage uint64
	// The maximum number of simultaneous data transfers between the client
	// and storage providers for retrieval deals
	SimultaneousTransfersForRetrieval uint64

	// Require that retrievals perform no on-chain operations. Paid retrievals
	// without existing payment channels with available funds will fail instead
	// of automatically performing on-chain operations.
	OffChainRetrieval bool
}

type Wallet struct {
	RemoteBackend string
	EnableLedger  bool
	DisableLocal  bool
}

type FeeConfig struct {
	DefaultMaxFee types.FIL
}
