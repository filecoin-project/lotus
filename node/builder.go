package node

import (
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/exchange"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node/hello"

	logging "github.com/ipfs/go-log"
	ci "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/discovery"
	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"

	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/metrics"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	"github.com/filecoin-project/lotus/chain/wallet/remotewallet"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/peermgr"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/markets/dealfilter"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/paychmgr"
	"github.com/filecoin-project/lotus/paychmgr/settler"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

//nolint:deadcode,varcheck
var log = logging.Logger("builder")

// special is a type used to give keys to modules which
//  can't really be identified by the returned type
type special struct{ id int }

//nolint:golint
var (
	DefaultTransportsKey = special{0}  // Libp2p option
	DiscoveryHandlerKey  = special{2}  // Private type
	AddrsFactoryKey      = special{3}  // Libp2p option
	SmuxTransportKey     = special{4}  // Libp2p option
	RelayKey             = special{5}  // Libp2p option
	SecurityKey          = special{6}  // Libp2p option
	BaseRoutingKey       = special{7}  // fx groups + multiret
	NatPortMapKey        = special{8}  // Libp2p option
	ConnectionManagerKey = special{9}  // Libp2p option
	AutoNATSvcKey        = special{10} // Libp2p option
	BandwidthReporterKey = special{11} // Libp2p option
)

type invoke int

// Invokes are called in the order they are defined.
//nolint:golint
const (
	// InitJournal at position 0 initializes the journal global var as soon as
	// the system starts, so that it's available for all other components.
	InitJournalKey = invoke(iota)

	// libp2p

	PstoreAddSelfKeysKey
	StartListeningKey
	BootstrapKey

	// filecoin
	SetGenesisKey

	RunHelloKey
	RunChainExchangeKey
	RunChainGraphsync
	RunPeerMgrKey

	HandleIncomingBlocksKey
	HandleIncomingMessagesKey

	HandlePaymentChannelManagerKey

	// miner
	GetParamsKey
	HandleDealsKey
	HandleRetrievalKey
	RunSectorServiceKey

	// daemon
	ExtractApiKey
	HeadMetricsKey
	SettlePaymentChannelsKey
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
	Lite   bool // Start node in "lite" mode
}

func defaults() []Option {
	return []Option{
		// global system journal.
		Override(new(journal.DisabledEvents), journal.EnvDisabledEvents),
		Override(new(journal.Journal), modules.OpenFilesystemJournal),

		Override(new(helpers.MetricsCtx), context.Background),
		Override(new(record.Validator), modules.RecordValidator),
		Override(new(dtypes.Bootstrapper), dtypes.Bootstrapper(false)),
		Override(new(dtypes.ShutdownChan), make(chan struct{})),

		// Filecoin modules

	}
}

func libp2p() Option {
	return Options(
		Override(new(peerstore.Peerstore), pstoremem.NewPeerstore),

		Override(DefaultTransportsKey, lp2p.DefaultTransports),

		Override(new(lp2p.RawHost), lp2p.Host),
		Override(new(host.Host), lp2p.RoutedHost),
		Override(new(lp2p.BaseIpfsRouting), lp2p.DHTRouting(dht.ModeAuto)),

		Override(DiscoveryHandlerKey, lp2p.DiscoveryHandler),
		Override(AddrsFactoryKey, lp2p.AddrsFactory(nil, nil)),
		Override(SmuxTransportKey, lp2p.SmuxTransport(true)),
		Override(RelayKey, lp2p.NoRelay()),
		Override(SecurityKey, lp2p.Security(true, false)),

		Override(BaseRoutingKey, lp2p.BaseRouting),
		Override(new(routing.Routing), lp2p.Routing),

		Override(NatPortMapKey, lp2p.NatPortMap),
		Override(BandwidthReporterKey, lp2p.BandwidthCounter),

		Override(ConnectionManagerKey, lp2p.ConnectionManager(50, 200, 20*time.Second, nil)),
		Override(AutoNATSvcKey, lp2p.AutoNATService),

		Override(new(*dtypes.ScoreKeeper), lp2p.ScoreKeeper),
		Override(new(*pubsub.PubSub), lp2p.GossipSub),
		Override(new(*config.Pubsub), func(bs dtypes.Bootstrapper) *config.Pubsub {
			return &config.Pubsub{
				Bootstrapper: bool(bs),
			}
		}),

		Override(PstoreAddSelfKeysKey, lp2p.PstoreAddSelfKeys),
		Override(StartListeningKey, lp2p.StartListening(config.DefaultFullNode().Libp2p.ListenAddresses)),
	)
}

func isType(t repo.RepoType) func(s *Settings) bool {
	return func(s *Settings) bool { return s.nodeType == t }
}

// Online sets up basic libp2p node
func Online() Option {
	isFullOrLiteNode := func(s *Settings) bool { return s.nodeType == repo.FullNode }
	isFullNode := func(s *Settings) bool { return s.nodeType == repo.FullNode && !s.Lite }
	isLiteNode := func(s *Settings) bool { return s.nodeType == repo.FullNode && s.Lite }

	return Options(
		// make sure that online is applied before Config.
		// This is important because Config overrides some of Online units
		func(s *Settings) error { s.Online = true; return nil },
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the Online option must be set before Config option")),
		),

		libp2p(),

		// common
		Override(new(*slashfilter.SlashFilter), modules.NewSlashFilter),

		// Full node or lite node
		ApplyIf(isFullOrLiteNode,
			// TODO: Fix offline mode

			Override(new(dtypes.BootstrapPeers), modules.BuiltinBootstrap),
			Override(new(dtypes.DrandBootstrap), modules.DrandBootstrap),
			Override(new(dtypes.DrandSchedule), modules.BuiltinDrandConfig),

			Override(new(ffiwrapper.Verifier), ffiwrapper.ProofVerifier),
			Override(new(vm.SyscallBuilder), vm.Syscalls),
			Override(new(*store.ChainStore), modules.ChainStore),
			Override(new(stmgr.UpgradeSchedule), stmgr.DefaultUpgradeSchedule()),
			Override(new(*stmgr.StateManager), stmgr.NewStateManagerWithUpgradeSchedule),
			Override(new(stmgr.StateManagerAPI), From(new(*stmgr.StateManager))),
			Override(new(*wallet.LocalWallet), wallet.NewWallet),
			Override(new(wallet.Default), From(new(*wallet.LocalWallet))),
			Override(new(api.WalletAPI), From(new(wallet.MultiWallet))),
			Override(new(*messagesigner.MessageSigner), messagesigner.NewMessageSigner),

			Override(new(dtypes.ChainGCLocker), blockstore.NewGCLocker),
			Override(new(dtypes.ChainGCBlockstore), modules.ChainGCBlockstore),
			Override(new(dtypes.ChainBitswap), modules.ChainBitswap),
			Override(new(dtypes.ChainBlockService), modules.ChainBlockService),

			// Filecoin services
			// We don't want the SyncManagerCtor to be used as an fx constructor, but rather as a value.
			// It will be called implicitly by the Syncer constructor.
			Override(new(chain.SyncManagerCtor), func() chain.SyncManagerCtor { return chain.NewSyncManager }),
			Override(new(*chain.Syncer), modules.NewSyncer),
			Override(new(exchange.Client), exchange.NewClient),
			Override(new(*messagepool.MessagePool), modules.MessagePool),

			Override(new(modules.Genesis), modules.ErrorGenesis),
			Override(new(dtypes.AfterGenesisSet), modules.SetGenesis),
			Override(SetGenesisKey, modules.DoSetGenesis),

			Override(new(dtypes.NetworkName), modules.NetworkName),
			Override(new(*hello.Service), hello.NewHelloService),
			Override(new(exchange.Server), exchange.NewServer),
			Override(new(*peermgr.PeerMgr), peermgr.NewPeerMgr),

			Override(new(dtypes.Graphsync), modules.Graphsync),
			Override(new(*dtypes.MpoolLocker), new(dtypes.MpoolLocker)),
			Override(new(*discoveryimpl.Local), modules.NewLocalDiscovery),
			Override(new(discovery.PeerResolver), modules.RetrievalResolver),

			Override(new(retrievalmarket.RetrievalClient), modules.RetrievalClient),
			Override(new(dtypes.ClientDatastore), modules.NewClientDatastore),
			Override(new(dtypes.ClientDataTransfer), modules.NewClientGraphsyncDataTransfer),
			Override(new(modules.ClientDealFunds), modules.NewClientDealFunds),
			Override(new(storagemarket.StorageClient), modules.StorageClient),
			Override(new(storagemarket.StorageClientNode), storageadapter.NewClientNodeAdapter),
			Override(new(beacon.Schedule), modules.RandomSchedule),

			Override(new(*paychmgr.Store), paychmgr.NewStore),
			Override(new(*paychmgr.Manager), paychmgr.NewManager),
			Override(new(*market.FundMgr), market.StartFundManager),
			Override(HandlePaymentChannelManagerKey, paychmgr.HandleManager),
			Override(SettlePaymentChannelsKey, settler.SettlePaymentChannels),
		),

		// Lite node
		ApplyIf(isLiteNode,
			Override(new(messagesigner.MpoolNonceAPI), From(new(modules.MpoolNonceAPI))),
			Override(new(full.ChainModuleAPI), From(new(api.GatewayAPI))),
			Override(new(full.GasModuleAPI), From(new(api.GatewayAPI))),
			Override(new(full.MpoolModuleAPI), From(new(api.GatewayAPI))),
			Override(new(full.StateModuleAPI), From(new(api.GatewayAPI))),
			Override(new(stmgr.StateManagerAPI), modules.NewRPCStateManager),
		),

		// Full node
		ApplyIf(isFullNode,
			Override(new(messagesigner.MpoolNonceAPI), From(new(*messagepool.MessagePool))),
			Override(new(full.ChainModuleAPI), From(new(full.ChainModule))),
			Override(new(full.GasModuleAPI), From(new(full.GasModule))),
			Override(new(full.MpoolModuleAPI), From(new(full.MpoolModule))),
			Override(new(full.StateModuleAPI), From(new(full.StateModule))),
			Override(new(stmgr.StateManagerAPI), From(new(*stmgr.StateManager))),

			Override(RunHelloKey, modules.RunHello),
			Override(RunChainExchangeKey, modules.RunChainExchange),
			Override(RunPeerMgrKey, modules.RunPeerMgr),
			Override(HandleIncomingMessagesKey, modules.HandleIncomingMessages),
			Override(HandleIncomingBlocksKey, modules.HandleIncomingBlocks),
		),

		// miner
		ApplyIf(isType(repo.StorageMiner),
			Override(new(api.Common), From(new(common.CommonAPI))),
			Override(new(sectorstorage.StorageAuth), modules.StorageAuth),

			Override(new(*stores.Index), stores.NewIndex),
			Override(new(stores.SectorIndex), From(new(*stores.Index))),
			Override(new(dtypes.MinerID), modules.MinerID),
			Override(new(dtypes.MinerAddress), modules.MinerAddress),
			Override(new(*ffiwrapper.Config), modules.ProofsConfig),
			Override(new(stores.LocalStorage), From(new(repo.LockedRepo))),
			Override(new(sealing.SectorIDCounter), modules.SectorIDCounter),
			Override(new(*sectorstorage.Manager), modules.SectorStorage),
			Override(new(ffiwrapper.Verifier), ffiwrapper.ProofVerifier),

			Override(new(sectorstorage.SectorManager), From(new(*sectorstorage.Manager))),
			Override(new(storage2.Prover), From(new(sectorstorage.SectorManager))),

			Override(new(*sectorblocks.SectorBlocks), sectorblocks.NewSectorBlocks),
			Override(new(*storage.Miner), modules.StorageMiner(config.DefaultStorageMiner().Fees)),
			Override(new(dtypes.NetworkName), modules.StorageNetworkName),

			Override(new(dtypes.StagingMultiDstore), modules.StagingMultiDatastore),
			Override(new(dtypes.StagingBlockstore), modules.StagingBlockstore),
			Override(new(dtypes.StagingDAG), modules.StagingDAG),
			Override(new(dtypes.StagingGraphsync), modules.StagingGraphsync),
			Override(new(retrievalmarket.RetrievalProvider), modules.RetrievalProvider),
			Override(new(dtypes.ProviderDataTransfer), modules.NewProviderDAGServiceDataTransfer),
			Override(new(dtypes.ProviderPieceStore), modules.NewProviderPieceStore),
			Override(new(*storedask.StoredAsk), modules.NewStorageAsk),
			Override(new(dtypes.DealFilter), modules.BasicDealFilter(nil)),
			Override(new(modules.ProviderDealFunds), modules.NewProviderDealFunds),
			Override(new(storagemarket.StorageProvider), modules.StorageProvider),
			Override(new(storagemarket.StorageProviderNode), storageadapter.NewProviderNodeAdapter(nil)),
			Override(HandleRetrievalKey, modules.HandleRetrieval),
			Override(GetParamsKey, modules.GetParams),
			Override(HandleDealsKey, modules.HandleDeals),
			Override(new(gen.WinningPoStProver), storage.NewWinningPoStProver),
			Override(new(*miner.Miner), modules.SetupBlockProducer),

			Override(new(dtypes.ConsiderOnlineStorageDealsConfigFunc), modules.NewConsiderOnlineStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderOnlineStorageDealsConfigFunc), modules.NewSetConsideringOnlineStorageDealsFunc),
			Override(new(dtypes.ConsiderOnlineRetrievalDealsConfigFunc), modules.NewConsiderOnlineRetrievalDealsConfigFunc),
			Override(new(dtypes.SetConsiderOnlineRetrievalDealsConfigFunc), modules.NewSetConsiderOnlineRetrievalDealsConfigFunc),
			Override(new(dtypes.StorageDealPieceCidBlocklistConfigFunc), modules.NewStorageDealPieceCidBlocklistConfigFunc),
			Override(new(dtypes.SetStorageDealPieceCidBlocklistConfigFunc), modules.NewSetStorageDealPieceCidBlocklistConfigFunc),
			Override(new(dtypes.ConsiderOfflineStorageDealsConfigFunc), modules.NewConsiderOfflineStorageDealsConfigFunc),
			Override(new(dtypes.SetConsiderOfflineStorageDealsConfigFunc), modules.NewSetConsideringOfflineStorageDealsFunc),
			Override(new(dtypes.ConsiderOfflineRetrievalDealsConfigFunc), modules.NewConsiderOfflineRetrievalDealsConfigFunc),
			Override(new(dtypes.SetConsiderOfflineRetrievalDealsConfigFunc), modules.NewSetConsiderOfflineRetrievalDealsConfigFunc),
			Override(new(dtypes.SetSealingConfigFunc), modules.NewSetSealConfigFunc),
			Override(new(dtypes.GetSealingConfigFunc), modules.NewGetSealConfigFunc),
			Override(new(dtypes.SetExpectedSealDurationFunc), modules.NewSetExpectedSealDurationFunc),
			Override(new(dtypes.GetExpectedSealDurationFunc), modules.NewGetExpectedSealDurationFunc),
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
			s.invokes[ExtractApiKey] = fx.Populate(resAPI)
			*out = resAPI
			return nil
		},
	)
}

// Config sets up constructors based on the provided Config
func ConfigCommon(cfg *config.Common) Option {
	return Options(
		func(s *Settings) error { s.Config = true; return nil },
		Override(new(dtypes.APIEndpoint), func() (dtypes.APIEndpoint, error) {
			return multiaddr.NewMultiaddr(cfg.API.ListenAddress)
		}),
		Override(SetApiEndpointKey, func(lr repo.LockedRepo, e dtypes.APIEndpoint) error {
			return lr.SetAPIEndpoint(e)
		}),
		Override(new(sectorstorage.URLs), func(e dtypes.APIEndpoint) (sectorstorage.URLs, error) {
			ip := cfg.API.RemoteListenAddress

			var urls sectorstorage.URLs
			urls = append(urls, "http://"+ip+"/remote") // TODO: This makes no assumptions, and probably could...
			return urls, nil
		}),
		ApplyIf(func(s *Settings) bool { return s.Online },
			Override(StartListeningKey, lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
			Override(ConnectionManagerKey, lp2p.ConnectionManager(
				cfg.Libp2p.ConnMgrLow,
				cfg.Libp2p.ConnMgrHigh,
				time.Duration(cfg.Libp2p.ConnMgrGrace),
				cfg.Libp2p.ProtectedPeers)),
			Override(new(*pubsub.PubSub), lp2p.GossipSub),
			Override(new(*config.Pubsub), &cfg.Pubsub),

			ApplyIf(func(s *Settings) bool { return len(cfg.Libp2p.BootstrapPeers) > 0 },
				Override(new(dtypes.BootstrapPeers), modules.ConfigBootstrap(cfg.Libp2p.BootstrapPeers)),
			),
		),
		Override(AddrsFactoryKey, lp2p.AddrsFactory(
			cfg.Libp2p.AnnounceAddresses,
			cfg.Libp2p.NoAnnounceAddresses)),
	)
}

func ConfigFullNode(c interface{}) Option {
	cfg, ok := c.(*config.FullNode)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	ipfsMaddr := cfg.Client.IpfsMAddr
	return Options(
		ConfigCommon(&cfg.Common),
		If(cfg.Client.UseIpfs,
			Override(new(dtypes.ClientBlockstore), modules.IpfsClientBlockstore(ipfsMaddr)),
			If(cfg.Client.IpfsUseForRetrieval,
				Override(new(dtypes.ClientRetrievalStoreManager), modules.ClientBlockstoreRetrievalStoreManager),
			),
		),
		If(cfg.Metrics.HeadNotifs,
			Override(HeadMetricsKey, metrics.SendHeadNotifs(cfg.Metrics.Nickname)),
		),

		If(cfg.Wallet.RemoteBackend != "",
			Override(new(*remotewallet.RemoteWallet), remotewallet.SetupRemoteWallet(cfg.Wallet.RemoteBackend)),
		),
		If(cfg.Wallet.EnableLedger,
			Override(new(*ledgerwallet.LedgerWallet), ledgerwallet.NewWallet),
		),
		If(cfg.Wallet.DisableLocal,
			Unset(new(*wallet.LocalWallet)),
			Override(new(wallet.Default), wallet.NilDefault),
		),
	)
}

func ConfigStorageMiner(c interface{}) Option {
	cfg, ok := c.(*config.StorageMiner)
	if !ok {
		return Error(xerrors.Errorf("invalid config from repo, got: %T", c))
	}

	return Options(
		ConfigCommon(&cfg.Common),

		If(cfg.Dealmaking.Filter != "",
			Override(new(dtypes.DealFilter), modules.BasicDealFilter(dealfilter.CliDealFilter(cfg.Dealmaking.Filter))),
		),

		Override(new(storagemarket.StorageProviderNode), storageadapter.NewProviderNodeAdapter(&cfg.Fees)),

		Override(new(sectorstorage.SealerConfig), cfg.Storage),
		Override(new(*storage.Miner), modules.StorageMiner(cfg.Fees)),
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

			Override(new(dtypes.MetadataDS), modules.Datastore),
			Override(new(dtypes.ChainBlockstore), modules.ChainBlockstore),

			Override(new(dtypes.ClientImportMgr), modules.ClientImportMgr),
			Override(new(dtypes.ClientMultiDstore), modules.ClientMultiDatastore),

			Override(new(dtypes.ClientBlockstore), modules.ClientBlockstore),
			Override(new(dtypes.ClientRetrievalStoreManager), modules.ClientRetrievalStoreManager),
			Override(new(ci.PrivKey), lp2p.PrivKey),
			Override(new(ci.PubKey), ci.PrivKey.GetPublic),
			Override(new(peer.ID), peer.IDFromPublicKey),

			Override(new(types.KeyStore), modules.KeyStore),

			Override(new(*dtypes.APIAlg), modules.APISecret),

			ApplyIf(isType(repo.FullNode), ConfigFullNode(c)),
			ApplyIf(isType(repo.StorageMiner), ConfigStorageMiner(c)),
		)(settings)
	}
}

type FullOption = Option

func Lite(enable bool) FullOption {
	return func(s *Settings) error {
		s.Lite = enable
		return nil
	}
}

func FullAPI(out *api.FullNode, fopts ...FullOption) Option {
	return Options(
		func(s *Settings) error {
			s.nodeType = repo.FullNode
			return nil
		},
		Options(fopts...),
		func(s *Settings) error {
			resAPI := &impl.FullNodeAPI{}
			s.invokes[ExtractApiKey] = fx.Populate(resAPI)
			*out = resAPI
			return nil
		},
	)
}

type StopFunc func(context.Context) error

// New builds and starts new Filecoin node
func New(ctx context.Context, opts ...Option) (StopFunc, error) {
	settings := Settings{
		modules: map[interface{}]fx.Option{},
		invokes: make([]fx.Option, _nInvokes),
	}

	// apply module options in the right order
	if err := Options(Options(defaults()...), Options(opts...))(&settings); err != nil {
		return nil, xerrors.Errorf("applying node options failed: %w", err)
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
		return nil, xerrors.Errorf("starting node: %w", err)
	}

	return app.Stop, nil
}

// In-memory / testing

func Test() Option {
	return Options(
		Unset(RunPeerMgrKey),
		Unset(new(*peermgr.PeerMgr)),
		Override(new(beacon.Schedule), testing.RandomBeacon),
	)
}
