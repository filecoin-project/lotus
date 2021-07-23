package config

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
)

// // NOTE: ONLY PUT STRUCT DEFINITIONS IN THIS FILE

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
	Metrics    Metrics
	Wallet     Wallet
	Fees       FeeConfig
	Chainstore Chainstore
}

// // Common

type Backup struct {
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
	ConsiderOnlineStorageDeals     bool
	ConsiderOfflineStorageDeals    bool
	ConsiderOnlineRetrievalDeals   bool
	ConsiderOfflineRetrievalDeals  bool
	ConsiderVerifiedStorageDeals   bool
	ConsiderUnverifiedStorageDeals bool
	PieceCidBlocklist              []cid.Cid
	ExpectedSealDuration           Duration
	// Maximum amount of time proposed deal StartEpoch can be in future
	MaxDealStartDelay Duration
	// The amount of time to wait for more deals to arrive before
	// publishing
	PublishMsgPeriod Duration
	// The maximum number of deals to include in a single PublishStorageDeals
	// message
	MaxDealsPerPublishMsg uint64
	// The maximum collateral that the provider will put up against a deal,
	// as a multiplier of the minimum collateral bound
	MaxProviderCollateralMultiplier uint64

	// The maximum number of parallel online data transfers (storage+retrieval)
	SimultaneousTransfers uint64

	Filter          string
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
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectorsForDeals uint64

	WaitDealsDelay Duration

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

	MaxTerminateGasFee     types.FIL
	MaxWindowPoStGasFee    types.FIL
	MaxPublishDealsFee     types.FIL
	MaxMarketBalanceAddFee types.FIL
}

type MinerAddressConfig struct {
	PreCommitControl   []string
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
	ListenAddress       string
	RemoteListenAddress string
	Timeout             Duration
}

// Libp2p contains configs for libp2p
type Libp2p struct {
	ListenAddresses     []string
	AnnounceAddresses   []string
	NoAnnounceAddresses []string
	BootstrapPeers      []string
	ProtectedPeers      []string

	ConnMgrLow   uint
	ConnMgrHigh  uint
	ConnMgrGrace Duration
}

type Pubsub struct {
	Bootstrapper          bool
	DirectPeers           []string
	IPColocationWhitelist []string
	RemoteTracer          string
}

type Chainstore struct {
	EnableSplitstore bool
	Splitstore       Splitstore
}

type Splitstore struct {
	ColdStoreType string
	HotStoreType  string
	MarkSetType   string

	HotStoreMessageRetention uint64
}

// // Full Node

type Metrics struct {
	Nickname   string
	HeadNotifs bool
}

type Client struct {
	UseIpfs               bool
	IpfsOnlineMode        bool
	IpfsMAddr             string
	IpfsUseForRetrieval   bool
	SimultaneousTransfers uint64
}

type Wallet struct {
	RemoteBackend string
	EnableLedger  bool
	DisableLocal  bool
}

type FeeConfig struct {
	DefaultMaxFee types.FIL
}
