package config

import (
	"encoding"
	"time"

	"github.com/ipfs/go-cid"

	sectorstorage "github.com/filecoin-project/sector-storage"
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
}

// // Common

// StorageMiner is a storage miner config
type StorageMiner struct {
	Common

	Dealmaking DealmakingConfig
	Storage    sectorstorage.SealerConfig
}

type DealmakingConfig struct {
	ConsiderOnlineStorageDeals    bool
	ConsiderOfflineStorageDeals   bool
	ConsiderOnlineRetrievalDeals  bool
	ConsiderOfflineRetrievalDeals bool
	PieceCidBlocklist             []cid.Cid
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
			RemoteTracer: "/ip4/147.75.67.199/tcp/4001/p2p/QmTd6UvR47vUidRNZ1ZKXHrAFhqTJAD27rKL9XYghEKgKX",
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

		Storage: sectorstorage.SealerConfig{
			AllowPreCommit1: true,
			AllowPreCommit2: true,
			AllowCommit:     true,
			AllowUnseal:     true,
		},

		Dealmaking: DealmakingConfig{
			ConsiderOnlineStorageDeals:    true,
			ConsiderOfflineStorageDeals:   true,
			ConsiderOnlineRetrievalDeals:  true,
			ConsiderOfflineRetrievalDeals: true,
			PieceCidBlocklist:             []cid.Cid{},
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
