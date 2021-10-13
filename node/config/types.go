package config

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
)

// // NOTE: ONLY PUT STRUCT DEFINITIONS IN THIS FILE
// //
// // After making edits here, run 'make cfgdoc-gen' (or 'make gen')

// Common is common config between full node and miner
type Common struct {
	API    API
	Backup Backup
	Libp2p Libp2p
	Pubsub Pubsub
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

// StorageMiner is a miner config
type StorageMiner struct {
	Common

	Subsystems MinerSubsystemConfig
	Dealmaking DealmakingConfig
	Sealing    SealingConfig
	Storage    sectorstorage.SealerConfig
	Fees       MinerFeeConfig
	Addresses  MinerAddressConfig
	DAGStore   DAGStoreConfig
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
	// The maximum number of parallel online data transfers for retrieval deals
	SimultaneousTransfersForRetrieval uint64
	// Minimum start epoch buffer to give time for sealing of sector with deal.
	StartEpochSealingBuffer uint64

	// A command used for fine-grained evaluation of storage deals
	// see https://docs.filecoin.io/mine/lotus/miner-configuration/#using-filters-for-fine-grained-storage-and-retrieval-deal-acceptance for more details
	Filter string
	// A command used for fine-grained evaluation of retrieval deals
	// see https://docs.filecoin.io/mine/lotus/miner-configuration/#using-filters-for-fine-grained-storage-and-retrieval-deal-acceptance for more details
	RetrievalFilter string

	RetrievalPricing *RetrievalPricing
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

type SealingConfig struct {
	// Upper bound on how many sectors can be waiting for more deals to be packed in it before it begins sealing at any given time.
	// If the miner is accepting multiple deals in parallel, up to MaxWaitDealsSectors of new sectors will be created.
	// If more than MaxWaitDealsSectors deals are accepted in parallel, only MaxWaitDealsSectors deals will be processed in parallel
	// Note that setting this number too high in relation to deal ingestion rate may result in poor sector packing efficiency
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// Upper bound on how many sectors can be sealing at the same time when creating new CC sectors (0 = unlimited)
	MaxSealingSectors uint64

	// Upper bound on how many sectors can be sealing at the same time when creating new sectors with deals (0 = unlimited)
	MaxSealingSectorsForDeals uint64

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
	// maximum batched commit size - batches will be sent immediately above this size
	MinCommitBatch int
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
	// It can be "universal" (default) or "discard" for discarding cold blocks.
	ColdStoreType string
	// HotStoreType specifies the type of the hotstore.
	// Only currently supported value is "badger".
	HotStoreType string
	// MarkSetType specifies the type of the markset.
	// It can be "map" (default) for in memory marking or "badger" for on-disk marking.
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
}

type Wallet struct {
	RemoteBackend string
	EnableLedger  bool
	DisableLocal  bool
}

type FeeConfig struct {
	DefaultMaxFee types.FIL
}
