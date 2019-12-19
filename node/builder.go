package node

import (
	"context"
	"errors"
	"time"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ci "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/deals"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/metrics"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/paych"
	"github.com/filecoin-project/lotus/peermgr"
	"github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/retrieval/discovery"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

// special is a type used to give keys to modules which
//  can't really be identified by the returned type
type special struct{ id int }

//nolint:golint
var (
	DefaultTransportsKey = special{0} // Libp2p option
	PNetKey              = special{1} // Option + multiret
	DiscoveryHandlerKey  = special{2} // Private type
	AddrsFactoryKey      = special{3} // Libp2p option
	SmuxTransportKey     = special{4} // Libp2p option
	RelayKey             = special{5} // Libp2p option
	SecurityKey          = special{6} // Libp2p option
	BaseRoutingKey       = special{7} // fx groups + multiret
	NatPortMapKey        = special{8} // Libp2p option
	ConnectionManagerKey = special{9} // Libp2p option
)

type invoke int

//nolint:golint
const (
	// libp2p

	PstoreAddSelfKeysKey = invoke(iota)
	StartListeningKey
	BootstrapKey

	// filecoin
	SetGenesisKey

	RunHelloKey
	RunBlockSyncKey
	RunPeerMgrKey

	HandleIncomingBlocksKey
	HandleIncomingMessagesKey

	RunDealClientKey
	RegisterClientValidatorKey

	// storage miner
	GetParamsKey
	HandleDealsKey
	HandleRetrievalKey
	RunSectorServiceKey
	RegisterProviderValidatorKey

	// daemon
	ExtractApiKey
	HeadMetricsKey
	RunPeerTaggerKey

	SetApiEndpointKey

	_nInvokes // keep this last
)

type Settings struct {
	// modules is a map of constructors for DI
	//
	// In most cases the index will be a reflect. Type of element returned by
	// the constructor, but for some 'constructors' it's hard to specify what's
	// the return type should be (or the constructor returns fx group)
	modules map[interface{}]fx.Option

	// invokes are separate from modules as they can't be referenced by return
	// type, and must be applied in correct order
	invokes []fx.Option

	nodeType repo.RepoType

	Online bool // Online option applied
	Config bool // Config option applied
}

func defaults() []Option {
	return []Option{
		Override(new(helpers.MetricsCtx), context.Background),
		Override(new(record.Validator), modules.RecordValidator),

		// Filecoin modules

	}
}

func libp2p() Option {
	return Options(
		Override(new(peerstore.Peerstore), pstoremem.NewPeerstore),

		Override(DefaultTransportsKey, lp2p.DefaultTransports),
		Override(PNetKey, lp2p.PNet),

		Override(new(lp2p.RawHost), lp2p.Host),
		Override(new(host.Host), lp2p.RoutedHost),
		Override(new(lp2p.BaseIpfsRouting), lp2p.DHTRouting(false)),

		Override(DiscoveryHandlerKey, lp2p.DiscoveryHandler),
		Override(AddrsFactoryKey, lp2p.AddrsFactory(nil, nil)),
		Override(SmuxTransportKey, lp2p.SmuxTransport(true)),
		Override(RelayKey, lp2p.Relay(true, false)),
		Override(SecurityKey, lp2p.Security(true, false)),

		Override(BaseRoutingKey, lp2p.BaseRouting),
		Override(new(routing.Routing), lp2p.Routing),

		Override(NatPortMapKey, lp2p.NatPortMap),

		Override(ConnectionManagerKey, lp2p.ConnectionManager(50, 200, 20*time.Second, nil)),

		Override(new(*pubsub.PubSub), lp2p.GossipSub()),

		Override(PstoreAddSelfKeysKey, lp2p.PstoreAddSelfKeys),
		Override(StartListeningKey, lp2p.StartListening(config.DefaultFullNode().Libp2p.ListenAddresses)),
	)
}

func isType(t repo.RepoType) func(s *Settings) bool {
	return func(s *Settings) bool { return s.nodeType == t }
}

// Online sets up basic libp2p node
func Online() Option {
	return Options(
		// make sure that online is applied before Config.
		// This is important because Config overrides some of Online units
		func(s *Settings) error { s.Online = true; return nil },
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the Online option must be set before Config option")),
		),

		libp2p(),

		// common

		// Full node

		ApplyIf(isType(repo.FullNode),
			// TODO: Fix offline mode

			Override(new(dtypes.BootstrapPeers), modules.BuiltinBootstrap),

			Override(HandleIncomingMessagesKey, modules.HandleIncomingMessages),

			Override(new(*store.ChainStore), modules.ChainStore),
			Override(new(*stmgr.StateManager), stmgr.NewStateManager),
			Override(new(*wallet.Wallet), wallet.NewWallet),

			Override(new(dtypes.ChainGCLocker), blockstore.NewGCLocker),
			Override(new(dtypes.ChainGCBlockstore), modules.ChainGCBlockstore),
			Override(new(dtypes.ChainExchange), modules.ChainExchange),
			Override(new(dtypes.ChainBlockService), modules.ChainBlockservice),
			Override(new(dtypes.ClientDAG), testing.MemoryClientDag),

			// Filecoin services
			Override(new(*chain.Syncer), modules.NewSyncer),
			Override(new(*blocksync.BlockSync), blocksync.NewBlockSyncClient),
			Override(new(*messagepool.MessagePool), modules.MessagePool),

			Override(new(modules.Genesis), modules.ErrorGenesis),
			Override(SetGenesisKey, modules.SetGenesis),

			Override(new(*hello.Service), hello.NewHelloService),
			Override(new(*blocksync.BlockSyncService), blocksync.NewBlockSyncService),
			Override(new(*peermgr.PeerMgr), peermgr.NewPeerMgr),

			Override(RunHelloKey, modules.RunHello),
			Override(RunBlockSyncKey, modules.RunBlockSync),
			Override(RunPeerMgrKey, modules.RunPeerMgr),
			Override(HandleIncomingBlocksKey, modules.HandleIncomingBlocks),

			Override(new(*discovery.Local), discovery.NewLocal),
			Override(new(discovery.PeerResolver), modules.RetrievalResolver),

			Override(new(*retrieval.Client), retrieval.NewClient),
			Override(new(dtypes.ClientDealStore), modules.NewClientDealStore),
			Override(new(dtypes.ClientDataTransfer), modules.NewClientDAGServiceDataTransfer),
			Override(new(*deals.ClientRequestValidator), deals.NewClientRequestValidator),
			Override(new(*deals.Client), deals.NewClient),
			Override(RegisterClientValidatorKey, modules.RegisterClientValidator),
			Override(RunDealClientKey, modules.RunDealClient),

			Override(new(*paych.Store), paych.NewStore),
			Override(new(*paych.Manager), paych.NewManager),
			Override(new(*market.FundMgr), market.NewFundMgr),
		),

		// Storage miner
		ApplyIf(func(s *Settings) bool { return s.nodeType == repo.StorageMiner },
			Override(new(*sectorbuilder.SectorBuilder), modules.SectorBuilder),
			Override(new(*sectorblocks.SectorBlocks), sectorblocks.NewSectorBlocks),
			Override(new(storage.TicketFn), modules.SealTicketGen),
			Override(new(*storage.Miner), modules.StorageMiner),

			Override(new(dtypes.StagingDAG), modules.StagingDAG),

			Override(new(*retrieval.Miner), retrieval.NewMiner),
			Override(new(dtypes.ProviderDealStore), modules.NewProviderDealStore),
			Override(new(dtypes.ProviderDataTransfer), modules.NewProviderDAGServiceDataTransfer),
			Override(new(*deals.ProviderRequestValidator), deals.NewProviderRequestValidator),
			Override(new(*deals.Provider), deals.NewProvider),
			Override(RegisterProviderValidatorKey, modules.RegisterProviderValidator),
			Override(HandleRetrievalKey, modules.HandleRetrieval),
			Override(GetParamsKey, modules.GetParams),
			Override(HandleDealsKey, modules.HandleDeals),
			Override(new(gen.ElectionPoStProver), storage.NewElectionPoStProver),
			Override(new(*miner.Miner), modules.SetupBlockProducer),
		),
	)
}

func StorageMiner(out *api.StorageMiner) Option {
	return Options(
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the StorageMiner option must be set before Config option")),
		),
		ApplyIf(func(s *Settings) bool { return s.Online },
			Error(errors.New("the StorageMiner option must be set before Online option")),
		),

		func(s *Settings) error {
			s.nodeType = repo.StorageMiner
			return nil
		},

		func(s *Settings) error {
			resAPI := &impl.StorageMinerAPI{}
			s.invokes[ExtractApiKey] = fx.Extract(resAPI)
			*out = resAPI
			return nil
		},
	)
}

// Config sets up constructors based on the provided Config
func ConfigCommon(cfg *config.Common) Option {
	return Options(
		func(s *Settings) error { s.Config = true; return nil },

		Override(SetApiEndpointKey, func(lr repo.LockedRepo) error {
			apima, err := multiaddr.NewMultiaddr(cfg.API.ListenAddress)
			if err != nil {
				return err
			}
			return lr.SetAPIEndpoint(apima)
		}),

		ApplyIf(func(s *Settings) bool { return s.Online },
			Override(StartListeningKey, lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
			Override(ConnectionManagerKey, lp2p.ConnectionManager(
				cfg.Libp2p.ConnMgrLow,
				cfg.Libp2p.ConnMgrHigh,
				time.Duration(cfg.Libp2p.ConnMgrGrace),
				cfg.Libp2p.ProtectedPeers)),

			ApplyIf(func(s *Settings) bool { return len(cfg.Libp2p.BootstrapPeers) > 0 },
				Override(new(dtypes.BootstrapPeers), modules.ConfigBootstrap(cfg.Libp2p.BootstrapPeers)),
			),
		),
	)
}

func ConfigFullNode(c interface{}) Option {
	cfg, ok := c.(*config.FullNode)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	return Options(
		ConfigCommon(&cfg.Common),
		If(cfg.Metrics.HeadNotifs,
			Override(HeadMetricsKey, metrics.SendHeadNotifs(cfg.Metrics.Nickname)),
		),
		If(cfg.Metrics.PubsubTracing,
			Override(new(*pubsub.PubSub), lp2p.GossipSub(lp2p.PubsubTracer())),
		),
	)
}

func ConfigStorageMiner(c interface{}, lr repo.LockedRepo) Option {
	cfg, ok := c.(*config.StorageMiner)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	path := cfg.SectorBuilder.Path
	if path == "" {
		path = lr.Path()
	}

	return Options(
		ConfigCommon(&cfg.Common),

		Override(new(*sectorbuilder.Config), modules.SectorBuilderConfig(path,
			cfg.SectorBuilder.WorkerCount,
			cfg.SectorBuilder.DisableLocalPreCommit,
			cfg.SectorBuilder.DisableLocalCommit)),
	)
}

func Repo(r repo.Repo) Option {
	return func(settings *Settings) error {
		lr, err := r.Lock(settings.nodeType)
		if err != nil {
			return err
		}
		c, err := lr.Config()
		if err != nil {
			return err
		}

		return Options(
			Override(new(repo.LockedRepo), modules.LockedRepo(lr)), // module handles closing

			ApplyIf(isType(repo.FullNode), ConfigFullNode(c)),
			ApplyIf(isType(repo.StorageMiner), ConfigStorageMiner(c, lr)),

			Override(new(dtypes.MetadataDS), modules.Datastore),
			Override(new(dtypes.ChainBlockstore), modules.ChainBlockstore),

			Override(new(dtypes.ClientFilestore), modules.ClientFstore),
			Override(new(dtypes.ClientBlockstore), modules.ClientBlockstore),
			Override(new(dtypes.ClientDAG), modules.ClientDAG),

			Override(new(ci.PrivKey), lp2p.PrivKey),
			Override(new(ci.PubKey), ci.PrivKey.GetPublic),
			Override(new(peer.ID), peer.IDFromPublicKey),

			Override(new(types.KeyStore), modules.KeyStore),

			Override(new(*dtypes.APIAlg), modules.APISecret),
		)(settings)
	}
}

func FullAPI(out *api.FullNode) Option {
	return func(s *Settings) error {
		resAPI := &impl.FullNodeAPI{}
		s.invokes[ExtractApiKey] = fx.Extract(resAPI)
		*out = resAPI
		return nil
	}
}

type StopFunc func(context.Context) error

// New builds and starts new Filecoin node
func New(ctx context.Context, opts ...Option) (StopFunc, error) {
	settings := Settings{
		modules:  map[interface{}]fx.Option{},
		invokes:  make([]fx.Option, _nInvokes),
		nodeType: repo.FullNode,
	}

	// apply module options in the right order
	if err := Options(Options(defaults()...), Options(opts...))(&settings); err != nil {
		return nil, err
	}

	// gather constructors for fx.Options
	ctors := make([]fx.Option, 0, len(settings.modules))
	for _, opt := range settings.modules {
		ctors = append(ctors, opt)
	}

	// fill holes in invokes for use in fx.Options
	for i, opt := range settings.invokes {
		if opt == nil {
			settings.invokes[i] = fx.Options()
		}
	}

	app := fx.New(
		fx.Options(ctors...),
		fx.Options(settings.invokes...),

		fx.NopLogger,
	)

	// TODO: we probably should have a 'firewall' for Closing signal
	//  on this context, and implement closing logic through lifecycles
	//  correctly
	if err := app.Start(ctx); err != nil {
		// comment fx.NopLogger few lines above for easier debugging
		return nil, err
	}

	return app.Stop, nil
}

// In-memory / testing

func Test() Option {
	return Options(
		Unset(RunPeerMgrKey),
		Unset(new(*peermgr.PeerMgr)),
	)
}
