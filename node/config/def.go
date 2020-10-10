package config

import (
	"encoding"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
)

// Common is common config between full node and miner
type Common struct {
	API    API
	Libp2p Libp2p
	Pubsub Pubsub
}

// FullNode is a full node config
type FullNode struct {
	Common
	Client  Client
	Metrics Metrics
	Wallet  Wallet
}

// // Common

// StorageMiner is a miner config
type StorageMiner struct {
	Common

	Dealmaking DealmakingConfig
	Sealing    SealingConfig
	Storage    sectorstorage.SealerConfig
	Fees       MinerFeeConfig
}

type DealmakingConfig struct {
	ConsiderOnlineStorageDeals    bool
	ConsiderOfflineStorageDeals   bool
	ConsiderOnlineRetrievalDeals  bool
	ConsiderOfflineRetrievalDeals bool
	PieceCidBlocklist             []cid.Cid
	ExpectedSealDuration          Duration

	Filter string
}

type SealingConfig struct {
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectorsForDeals uint64

	WaitDealsDelay Duration
}

type MinerFeeConfig struct {
	MaxPreCommitGasFee     types.FIL
	MaxCommitGasFee        types.FIL
	MaxWindowPoStGasFee    types.FIL
	MaxPublishDealsFee     types.FIL
	MaxMarketBalanceAddFee types.FIL
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
	Bootstrapper bool
	DirectPeers  []string
	RemoteTracer string
}

// // Full Node

type Metrics struct {
	Nickname   string
	HeadNotifs bool
}

type Client struct {
	UseIpfs             bool
	IpfsMAddr           string
	IpfsUseForRetrieval bool
}

type Wallet struct {
	RemoteBackend string
	EnableLedger  bool
	DisableLocal  bool
}

func defCommon() Common {
	return Common{
		API: API{
			ListenAddress: "/ip4/127.0.0.1/tcp/1234/http",
			Timeout:       Duration(30 * time.Second),
		},
		Libp2p: Libp2p{
			ListenAddresses: []string{
				"/ip4/0.0.0.0/tcp/0",
				"/ip6/::/tcp/0",
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
			RemoteTracer: "/dns4/pubsub-tracer.filecoin.io/tcp/4001/p2p/QmTd6UvR47vUidRNZ1ZKXHrAFhqTJAD27rKL9XYghEKgKX",
		},
	}

}

// DefaultFullNode returns the default config
func DefaultFullNode() *FullNode {
	return &FullNode{
		Common: defCommon(),
	}
}

func DefaultStorageMiner() *StorageMiner {
	cfg := &StorageMiner{
		Common: defCommon(),

		Sealing: SealingConfig{
			MaxWaitDealsSectors:       2, // 64G with 32G sectors
			MaxSealingSectors:         0,
			MaxSealingSectorsForDeals: 0,
			WaitDealsDelay:            Duration(time.Hour * 6),
		},

		Storage: sectorstorage.SealerConfig{
			AllowAddPiece:   true,
			AllowPreCommit1: true,
			AllowPreCommit2: true,
			AllowCommit:     true,
			AllowUnseal:     true,

			// Default to 10 - tcp should still be able to figure this out, and
			// it's the ratio between 10gbit / 1gbit
			ParallelFetchLimit: 10,
		},

		Dealmaking: DealmakingConfig{
			ConsiderOnlineStorageDeals:    true,
			ConsiderOfflineStorageDeals:   true,
			ConsiderOnlineRetrievalDeals:  true,
			ConsiderOfflineRetrievalDeals: true,
			PieceCidBlocklist:             []cid.Cid{},
			// TODO: It'd be nice to set this based on sector size
			ExpectedSealDuration: Duration(time.Hour * 24),
		},

		Fees: MinerFeeConfig{
			MaxPreCommitGasFee:     types.FIL(types.BigDiv(types.FromFil(1), types.NewInt(20))), // 0.05
			MaxCommitGasFee:        types.FIL(types.BigDiv(types.FromFil(1), types.NewInt(20))),
			MaxWindowPoStGasFee:    types.FIL(types.FromFil(50)),
			MaxPublishDealsFee:     types.FIL(types.BigDiv(types.FromFil(1), types.NewInt(33))),  // 0.03ish
			MaxMarketBalanceAddFee: types.FIL(types.BigDiv(types.FromFil(1), types.NewInt(100))), // 0.01
		},
	}
	cfg.Common.API.ListenAddress = "/ip4/127.0.0.1/tcp/2345/http"
	cfg.Common.API.RemoteListenAddress = "127.0.0.1:2345"
	return cfg
}

var _ encoding.TextMarshaler = (*Duration)(nil)
var _ encoding.TextUnmarshaler = (*Duration)(nil)

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
