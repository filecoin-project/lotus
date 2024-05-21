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
	Client        Client
	Wallet        Wallet
	Fees          FeeConfig
	Chainstore    Chainstore
	Fevm          FevmConfig
	Events        EventsConfig
	Index         IndexConfig
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

	Subsystems    MinerSubsystemConfig
	Dealmaking    DealmakingConfig
	IndexProvider IndexProviderConfig
	Proving       ProvingConfig
	Sealing       SealingConfig
	Storage       SealerConfig
	Fees          MinerFeeConfig
	Addresses     MinerAddressConfig
	DAGStore      DAGStoreConfig

	HarmonyDB HarmonyDB
}

type CurioConfig struct {
	Subsystems CurioSubsystemsConfig

	Fees CurioFees

	// Addresses of wallets per MinerAddress (one of the fields).
	Addresses []CurioAddresses
	Proving   CurioProvingConfig
	Ingest    CurioIngestConfig
	Journal   JournalConfig
	Apis      ApisConfig
	Alerting  CurioAlerting
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

type CurioSubsystemsConfig struct {
	// EnableWindowPost enables window post to be executed on this curio instance. Each machine in the cluster
	// with WindowPoSt enabled will also participate in the window post scheduler. It is possible to have multiple
	// machines with WindowPoSt enabled which will provide redundancy, and in case of multiple partitions per deadline,
	// will allow for parallel processing of partitions.
	//
	// It is possible to have instances handling both WindowPoSt and WinningPoSt, which can provide redundancy without
	// the need for additional machines. In setups like this it is generally recommended to run
	// partitionsPerDeadline+1 machines.
	EnableWindowPost   bool
	WindowPostMaxTasks int

	// EnableWinningPost enables winning post to be executed on this curio instance.
	// Each machine in the cluster with WinningPoSt enabled will also participate in the winning post scheduler.
	// It is possible to mix machines with WindowPoSt and WinningPoSt enabled, for details see the EnableWindowPost
	// documentation.
	EnableWinningPost   bool
	WinningPostMaxTasks int

	// EnableParkPiece enables the "piece parking" task to run on this node. This task is responsible for fetching
	// pieces from the network and storing them in the storage subsystem until sectors are sealed. This task is
	// only applicable when integrating with boost, and should be enabled on nodes which will hold deal data
	// from boost until sectors containing the related pieces have the TreeD/TreeR constructed.
	// Note that future Curio implementations will have a separate task type for fetching pieces from the internet.
	EnableParkPiece   bool
	ParkPieceMaxTasks int

	// EnableSealSDR enables SDR tasks to run. SDR is the long sequential computation
	// creating 11 layer files in sector cache directory.
	//
	// SDR is the first task in the sealing pipeline. It's inputs are just the hash of the
	// unsealed data (CommD), sector number, miner id, and the seal proof type.
	// It's outputs are the 11 layer files in the sector cache directory.
	//
	// In lotus-miner this was run as part of PreCommit1.
	EnableSealSDR bool

	// The maximum amount of SDR tasks that can run simultaneously. Note that the maximum number of tasks will
	// also be bounded by resources available on the machine.
	SealSDRMaxTasks int

	// EnableSealSDRTrees enables the SDR pipeline tree-building task to run.
	// This task handles encoding of unsealed data into last sdr layer and building
	// of TreeR, TreeC and TreeD.
	//
	// This task runs after SDR
	// TreeD is first computed with optional input of unsealed data
	// TreeR is computed from replica, which is first computed as field
	//   addition of the last SDR layer and the bottom layer of TreeD (which is the unsealed data)
	// TreeC is computed from the 11 SDR layers
	// The 3 trees will later be used to compute the PoRep proof.
	//
	// In case of SyntheticPoRep challenges for PoRep will be pre-generated at this step, and trees and layers
	// will be dropped. SyntheticPoRep works by pre-generating a very large set of challenges (~30GiB on disk)
	// then using a small subset of them for the actual PoRep computation. This allows for significant scratch space
	// saving between PreCommit and PoRep generation at the expense of more computation (generating challenges in this step)
	//
	// In lotus-miner this was run as part of PreCommit2 (TreeD was run in PreCommit1).
	// Note that nodes with SDRTrees enabled will also answer to Finalize tasks,
	// which just remove unneeded tree data after PoRep is computed.
	EnableSealSDRTrees bool

	// The maximum amount of SealSDRTrees tasks that can run simultaneously. Note that the maximum number of tasks will
	// also be bounded by resources available on the machine.
	SealSDRTreesMaxTasks int

	// FinalizeMaxTasks is the maximum amount of finalize tasks that can run simultaneously.
	// The finalize task is enabled on all machines which also handle SDRTrees tasks. Finalize ALWAYS runs on whichever
	// machine holds sector cache files, as it removes unneeded tree data after PoRep is computed.
	// Finalize will run in parallel with the SubmitCommitMsg task.
	FinalizeMaxTasks int

	// EnableSendPrecommitMsg enables the sending of precommit messages to the chain
	// from this curio instance.
	// This runs after SDRTrees and uses the output CommD / CommR (roots of TreeD / TreeR) for the message
	EnableSendPrecommitMsg bool

	// EnablePoRepProof enables the computation of the porep proof
	//
	// This task runs after interactive-porep seed becomes available, which happens 150 epochs (75min) after the
	// precommit message lands on chain. This task should run on a machine with a GPU. Vanilla PoRep proofs are
	// requested from the machine which holds sector cache files which most likely is the machine which ran the SDRTrees
	// task.
	//
	// In lotus-miner this was Commit1 / Commit2
	EnablePoRepProof bool

	// The maximum amount of PoRepProof tasks that can run simultaneously. Note that the maximum number of tasks will
	// also be bounded by resources available on the machine.
	PoRepProofMaxTasks int

	// EnableSendCommitMsg enables the sending of commit messages to the chain
	// from this curio instance.
	EnableSendCommitMsg bool

	// Whether to abort if any sector activation in a batch fails (newly sealed sectors, only with ProveCommitSectors3).
	RequireActivationSuccess bool
	// Whether to abort if any sector activation in a batch fails (updating sectors, only with ProveReplicaUpdates3).
	RequireNotificationSuccess bool

	// EnableMoveStorage enables the move-into-long-term-storage task to run on this curio instance.
	// This tasks should only be enabled on nodes with long-term storage.
	//
	// The MoveStorage task is the last task in the sealing pipeline. It moves the sealed sector data from the
	// SDRTrees machine into long-term storage. This task runs after the Finalize task.
	EnableMoveStorage bool

	// The maximum amount of MoveStorage tasks that can run simultaneously. Note that the maximum number of tasks will
	// also be bounded by resources available on the machine. It is recommended that this value is set to a number which
	// uses all available network (or disk) bandwidth on the machine without causing bottlenecks.
	MoveStorageMaxTasks int

	// BoostAdapters is a list of tuples of miner address and port/ip to listen for market (e.g. boost) requests.
	// This interface is compatible with the lotus-miner RPC, implementing a subset needed for storage market operations.
	// Strings should be in the format "actor:ip:port". IP cannot be 0.0.0.0. We recommend using a private IP.
	// Example: "f0123:127.0.0.1:32100". Multiple addresses can be specified.
	//
	// When a market node like boost gives Curio's market RPC a deal to placing into a sector, Curio will first store the
	// deal data in a temporary location "Piece Park" before assigning it to a sector. This requires that at least one
	// node in the cluster has the EnableParkPiece option enabled and has sufficient scratch space to store the deal data.
	// This is different from lotus-miner which stored the deal data into an "unsealed" sector as soon as the deal was
	// received. Deal data in PiecePark is accessed when the sector TreeD and TreeR are computed, but isn't needed for
	// the initial SDR layers computation. Pieces in PiecePark are removed after all sectors referencing the piece are
	// sealed.
	//
	// To get API info for boost configuration run 'curio market rpc-info'
	//
	// NOTE: All deal data will flow through this service, so it should be placed on a machine running boost or on
	// a machine which handles ParkPiece tasks.
	BoostAdapters []string

	// EnableWebGui enables the web GUI on this curio instance. The UI has minimal local overhead, but it should
	// only need to be run on a single machine in the cluster.
	EnableWebGui bool

	// The address that should listen for Web GUI requests.
	GuiAddress string
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

	// network BaseFee below which to stop doing precommit batching, instead
	// sending precommit messages to the chain individually. When the basefee is
	// below this threshold, precommit messages will get sent out immediately.
	BatchPreCommitAboveBaseFee types.FIL

	// network BaseFee below which to stop doing commit aggregation, instead
	// submitting proofs to the chain individually
	AggregateAboveBaseFee types.FIL

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

type CurioFees struct {
	DefaultMaxFee      types.FIL
	MaxPreCommitGasFee types.FIL
	MaxCommitGasFee    types.FIL

	// maxBatchFee = maxBase + maxPerSector * nSectors
	MaxPreCommitBatchGasFee BatchFeeConfig
	MaxCommitBatchGasFee    BatchFeeConfig

	MaxTerminateGasFee types.FIL
	// WindowPoSt is a high-value operation, so the default fee should be high.
	MaxWindowPoStGasFee types.FIL
	MaxPublishDealsFee  types.FIL
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

type CurioAddresses struct {
	// Addresses to send PreCommit messages from
	PreCommitControl []string
	// Addresses to send Commit messages from
	CommitControl    []string
	TerminateControl []string

	// DisableOwnerFallback disables usage of the owner address for messages
	// sent automatically
	DisableOwnerFallback bool
	// DisableWorkerFallback disables usage of the worker address for messages
	// sent automatically, if control addresses are configured.
	// A control address that doesn't have enough funds will still be chosen
	// over the worker address if this flag is set.
	DisableWorkerFallback bool

	// MinerAddresses are the addresses of the miner actors to use for sending messages
	MinerAddresses []string
}

type CurioProvingConfig struct {
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

type CurioIngestConfig struct {
	// Maximum number of sectors that can be queued waiting for deals to start processing.
	// 0 = unlimited
	// Note: This mechanism will delay taking deal data from markets, providing backpressure to the market subsystem.
	// The DealSector queue includes deals which are ready to enter the sealing pipeline but are not yet part of it -
	// size of this queue will also impact the maximum number of ParkPiece tasks which can run concurrently.
	// DealSector queue is the first queue in the sealing pipeline, meaning that it should be used as the primary backpressure mechanism.
	MaxQueueDealSector int

	// Maximum number of sectors that can be queued waiting for SDR to start processing.
	// 0 = unlimited
	// Note: This mechanism will delay taking deal data from markets, providing backpressure to the market subsystem.
	// The SDR queue includes deals which are in the process of entering the sealing pipeline. In case of the SDR tasks it is
	// possible that this queue grows more than this limit(CC sectors), the backpressure is only applied to sectors
	// entering the pipeline.
	MaxQueueSDR int

	// Maximum number of sectors that can be queued waiting for SDRTrees to start processing.
	// 0 = unlimited
	// Note: This mechanism will delay taking deal data from markets, providing backpressure to the market subsystem.
	// In case of the trees tasks it is possible that this queue grows more than this limit, the backpressure is only
	// applied to sectors entering the pipeline.
	MaxQueueTrees int

	// Maximum number of sectors that can be queued waiting for PoRep to start processing.
	// 0 = unlimited
	// Note: This mechanism will delay taking deal data from markets, providing backpressure to the market subsystem.
	// Like with the trees tasks, it is possible that this queue grows more than this limit, the backpressure is only
	// applied to sectors entering the pipeline.
	MaxQueuePoRep int

	// Maximum time an open deal sector should wait for more deal before it starts sealing
	MaxDealWaitTime Duration
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

// // Full Node
type Client struct {
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

type FevmConfig struct {
	// EnableEthRPC enables eth_ rpc, and enables storing a mapping of eth transaction hashes to filecoin message Cids.
	// This will also enable the RealTimeFilterAPI and HistoricFilterAPI by default, but they can be disabled by config options above.
	EnableEthRPC bool

	// EthTxHashMappingLifetimeDays the transaction hash lookup database will delete mappings that have been stored for more than x days
	// Set to 0 to keep all mappings
	EthTxHashMappingLifetimeDays int

	Events DeprecatedEvents `toml:"Events,omitempty"`
}

type DeprecatedEvents struct {
	// DisableRealTimeFilterAPI is DEPRECATED and will be removed in a future release. Use Events.DisableRealTimeFilterAPI instead.
	DisableRealTimeFilterAPI bool `moved:"Events.DisableRealTimeFilterAPI" toml:"DisableRealTimeFilterAPI,omitempty"`

	// DisableHistoricFilterAPI is DEPRECATED and will be removed in a future release. Use Events.DisableHistoricFilterAPI instead.
	DisableHistoricFilterAPI bool `moved:"Events.DisableHistoricFilterAPI" toml:"DisableHistoricFilterAPI,omitempty"`

	// FilterTTL is DEPRECATED and will be removed in a future release. Use Events.FilterTTL instead.
	FilterTTL Duration `moved:"Events.FilterTTL" toml:"FilterTTL,omitzero"`

	// MaxFilters is DEPRECATED and will be removed in a future release. Use Events.MaxFilters instead.
	MaxFilters int `moved:"Events.MaxFilters" toml:"MaxFilters,omitzero"`

	// MaxFilterResults is DEPRECATED and will be removed in a future release. Use Events.MaxFilterResults instead.
	MaxFilterResults int `moved:"Events.MaxFilterResults" toml:"MaxFilterResults,omitzero"`

	// MaxFilterHeightRange is DEPRECATED and will be removed in a future release. Use Events.MaxFilterHeightRange instead.
	MaxFilterHeightRange uint64 `moved:"Events.MaxFilterHeightRange" toml:"MaxFilterHeightRange,omitzero"`

	// DatabasePath is DEPRECATED and will be removed in a future release. Use Events.DatabasePath instead.
	DatabasePath string `moved:"Events.DatabasePath" toml:"DatabasePath,omitempty"`
}

type EventsConfig struct {
	// DisableRealTimeFilterAPI will disable the RealTimeFilterAPI that can create and query filters for actor events as they are emitted.
	// The API is enabled when Fevm.EnableEthRPC or EnableActorEventsAPI is true, but can be disabled selectively with this flag.
	DisableRealTimeFilterAPI bool

	// DisableHistoricFilterAPI will disable the HistoricFilterAPI that can create and query filters for actor events
	// that occurred in the past. HistoricFilterAPI maintains a queryable index of events.
	// The API is enabled when Fevm.EnableEthRPC or EnableActorEventsAPI is true, but can be disabled selectively with this flag.
	DisableHistoricFilterAPI bool

	// EnableActorEventsAPI enables the Actor events API that enables clients to consume events
	// emitted by (smart contracts + built-in Actors).
	// This will also enable the RealTimeFilterAPI and HistoricFilterAPI by default, but they can be
	// disabled by setting their respective Disable* options.
	EnableActorEventsAPI bool

	// FilterTTL specifies the time to live for actor event filters. Filters that haven't been accessed longer than
	// this time become eligible for automatic deletion.
	FilterTTL Duration

	// MaxFilters specifies the maximum number of filters that may exist at any one time.
	MaxFilters int

	// MaxFilterResults specifies the maximum number of results that can be accumulated by an actor event filter.
	MaxFilterResults int

	// MaxFilterHeightRange specifies the maximum range of heights that can be used in a filter (to avoid querying
	// the entire chain)
	MaxFilterHeightRange uint64

	// DatabasePath is the full path to a sqlite database that will be used to index actor events to
	// support the historic filter APIs. If the database does not exist it will be created. The directory containing
	// the database must already exist and be writeable. If a relative path is provided here, sqlite treats it as
	// relative to the CWD (current working directory).
	DatabasePath string

	// Others, not implemented yet:
	// Set a limit on the number of active websocket subscriptions (may be zero)
	// Set a timeout for subscription clients
	// Set upper bound on index size
}

type IndexConfig struct {
	// EXPERIMENTAL FEATURE. USE WITH CAUTION
	// EnableMsgIndex enables indexing of messages on chain.
	EnableMsgIndex bool
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

type CurioAlerting struct {
	// PagerDutyEventURL is URL for PagerDuty.com Events API v2 URL. Events sent to this API URL are ultimately
	// routed to a PagerDuty.com service and processed.
	// The default is sufficient for integration with the stock commercial PagerDuty.com company's service.
	PagerDutyEventURL string

	// PageDutyIntegrationKey is the integration key for a PagerDuty.com service. You can find this unique service
	// identifier in the integration page for the service.
	PageDutyIntegrationKey string

	// MinimumWalletBalance is the minimum balance all active wallets. If the balance is below this value, an
	// alerts will be triggered for the wallet
	MinimumWalletBalance types.FIL
}
