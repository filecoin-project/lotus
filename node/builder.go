package node

import (
	"context"
	"errors"
	"time"

	logging "github.com/ipfs/go-log/v2"
	metricsi "github.com/ipfs/go-metrics-interface"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	ci "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/peermgr"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/delegated"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/system"
)

//nolint:deadcode,varcheck
var log = logging.Logger("builder")

// special is a type used to give keys to modules which
//
//	can't really be identified by the returned type
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
	ConnGaterKey         = special{12} // Libp2p option
	ResourceManagerKey   = special{14} // Libp2p option
)

type invoke int

// Invokes are called in the order they are defined.
//
//nolint:golint
const (
	// InitJournal at position 0 initializes the journal global var as soon as
	// the system starts, so that it's available for all other components.
	InitJournalKey = invoke(iota)

	// System processes.
	InitMemoryWatchdog

	// health checks
	CheckFDLimit
	CheckFvmConcurrency
	CheckUDPBufferSize

	// libp2p
	PstoreAddSelfKeysKey
	StartListeningKey
	BootstrapKey

	// filecoin
	SetGenesisKey

	RunHelloKey
	RunChainExchangeKey
	RunPeerMgrKey

	HandleIncomingBlocksKey
	HandleIncomingMessagesKey
	HandlePaymentChannelManagerKey

	// Deprecated: RelayIndexerMessagesKey is no longer used, since IPNI has
	// deprecated the use of GossipSub for propagating advertisements. Use IPNI Sync
	// protocol instead.
	//
	// See:
	//  - https://github.com/ipni/specs/blob/main/IPNI_HTTP_PROVIDER.md
	//  - https://github.com/ipni/go-libipni/tree/main/dagsync/ipnisync
	RelayIndexerMessagesKey

	// miner
	PreflightChecksKey
	GetParamsKey
	HandleMigrateProviderFundsKey
	HandleDealsKey
	HandleRetrievalKey
	RunSectorServiceKey
	F3Participation

	// daemon
	ExtractApiKey
	HeadMetricsKey
	SettlePaymentChannelsKey
	RunPeerTaggerKey
	SetupFallbackBlockstoresKey

	ConsensusReporterKey

	SetApiEndpointKey

	StoreEventsKey

	InitChainIndexerKey
	InitApiV2Key

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

	Base   bool // Base option applied
	Config bool // Config option applied
	Lite   bool // Start node in "lite" mode

	enableLibp2pNode bool
}

// Basic lotus-app services
func defaults() []Option {
	return []Option{
		// global system journal.
		Override(new(journal.DisabledEvents), journal.EnvDisabledEvents),
		Override(new(journal.Journal), modules.OpenFilesystemJournal),
		Override(new(*alerting.Alerting), alerting.NewAlertingSystem),
		Override(new(dtypes.NodeStartTime), FromVal(dtypes.NodeStartTime(time.Now()))),

		Override(CheckFDLimit, modules.CheckFdLimit(buildconstants.DefaultFDLimit)),
		Override(CheckFvmConcurrency, modules.CheckFvmConcurrency()),
		Override(CheckUDPBufferSize, modules.CheckUDPBufferSize(2048*1024)),

		Override(new(system.MemoryConstraints), modules.MemoryConstraints),
		Override(InitMemoryWatchdog, modules.MemoryWatchdog),

		Override(new(helpers.MetricsCtx), func() context.Context {
			return metricsi.CtxScope(context.Background(), "lotus")
		}),

		Override(new(dtypes.ShutdownChan), make(chan struct{})),

		// the great context in the sky, otherwise we can't DI build genesis; there has to be a better
		// solution than this hack.
		Override(new(context.Context), func(lc fx.Lifecycle, mctx helpers.MetricsCtx) context.Context {
			return helpers.LifecycleCtx(mctx, lc)
		}),
	}
}

var LibP2P = Options(
	// Host config
	Override(new(dtypes.Bootstrapper), dtypes.Bootstrapper(false)),

	// Host dependencies
	Override(new(peerstore.Peerstore), lp2p.Peerstore),
	Override(PstoreAddSelfKeysKey, lp2p.PstoreAddSelfKeys),
	Override(StartListeningKey, lp2p.StartListening(config.DefaultFullNode().Libp2p.ListenAddresses)),

	// Host settings
	Override(DefaultTransportsKey, lp2p.DefaultTransports),
	Override(AddrsFactoryKey, lp2p.AddrsFactory(nil, nil)),
	Override(SmuxTransportKey, lp2p.SmuxTransport()),
	Override(RelayKey, lp2p.NoRelay()),
	Override(SecurityKey, lp2p.Security(true, false)),

	// Host
	Override(new(lp2p.RawHost), lp2p.Host),
	Override(new(host.Host), lp2p.RoutedHost),
	Override(new(lp2p.BaseIpfsRouting), lp2p.DHTRouting(dht.ModeAuto)),

	Override(DiscoveryHandlerKey, lp2p.DiscoveryHandler),

	// Routing
	Override(new(record.Validator), modules.RecordValidator),
	Override(BaseRoutingKey, lp2p.BaseRouting),
	Override(new(routing.Routing), lp2p.Routing),

	// Services
	Override(BandwidthReporterKey, lp2p.BandwidthCounter),
	Override(AutoNATSvcKey, lp2p.AutoNATService),

	// Services (pubsub)
	Override(new(*dtypes.ScoreKeeper), lp2p.ScoreKeeper),
	Override(new(*pubsub.PubSub), lp2p.GossipSub),
	Override(new(*config.Pubsub), func(bs dtypes.Bootstrapper) *config.Pubsub {
		return &config.Pubsub{
			Bootstrapper: bool(bs),
		}
	}),

	// Services (connection management)
	Override(ConnectionManagerKey, lp2p.ConnectionManager(50, 200, 20*time.Second, nil)),
	Override(new(*conngater.BasicConnectionGater), lp2p.ConnGater),
	Override(ConnGaterKey, lp2p.ConnGaterOption),

	// Services (resource management)
	Override(new(network.ResourceManager), lp2p.ResourceManager(200)),
	Override(ResourceManagerKey, lp2p.ResourceManagerOption),
)

func IsType(t repo.RepoType) func(s *Settings) bool {
	return func(s *Settings) bool { return s.nodeType == t }
}

func isFullOrLiteNode(s *Settings) bool { return s.nodeType == repo.FullNode }
func isFullNode(s *Settings) bool {
	return s.nodeType == repo.FullNode && !s.Lite
}
func isLiteNode(s *Settings) bool {
	return s.nodeType == repo.FullNode && s.Lite
}

func Base() Option {
	return Options(
		func(s *Settings) error { s.Base = true; return nil }, // mark Base as applied
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the Base() option must be set before Config option")),
		),
		ApplyIf(func(s *Settings) bool { return s.enableLibp2pNode },
			LibP2P,
		),
		ApplyIf(isFullOrLiteNode, ChainNode),
		ApplyIf(IsType(repo.StorageMiner), MinerNode),
	)
}

// ConfigCommon sets up constructors based on the provided Config
func ConfigCommon(cfg *config.Common, buildVersion build.BuildVersion) Option {
	// setup logging early
	lotuslog.SetLevelsFromConfig(cfg.Logging.SubsystemLevels)

	return Options(
		func(s *Settings) error { s.Config = true; return nil },
		Override(new(build.BuildVersion), buildVersion),
		Override(new(dtypes.APIEndpoint), func() (dtypes.APIEndpoint, error) {
			ma, err := multiaddr.NewMultiaddr(cfg.API.ListenAddress)
			if err != nil {
				return nil, err
			}
			return dtypes.APIEndpoint(ma), nil
		}),
		Override(SetApiEndpointKey, func(lr repo.LockedRepo, e dtypes.APIEndpoint) error {
			return lr.SetAPIEndpoint(multiaddr.Multiaddr(e))
		}),
		Override(new(paths.URLs), func(e dtypes.APIEndpoint) (paths.URLs, error) {
			ip := cfg.API.RemoteListenAddress

			var urls paths.URLs
			urls = append(urls, "http://"+ip+"/remote") // TODO: This makes no assumptions, and probably could...
			return urls, nil
		}),
		ApplyIf(func(s *Settings) bool { return s.Base }), // apply only if Base has already been applied
		Override(new(api.Net), new(api.NetStub)),
		Override(new(api.Common), From(new(common.CommonAPI))),

		Override(new(dtypes.MetadataDS), modules.Datastore(cfg.Backup.DisableMetadataLog)),
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

			Override(new(ci.PrivKey), lp2p.PrivKey),
			Override(new(ci.PubKey), ci.PrivKey.GetPublic),
			Override(new(peer.ID), peer.IDFromPublicKey),

			Override(new(types.KeyStore), modules.KeyStore),

			Override(new(*dtypes.APIAlg), modules.APISecret),

			ApplyIf(IsType(repo.FullNode), ConfigFullNode(c)),
			ApplyIf(IsType(repo.StorageMiner), ConfigStorageMiner(c)),
		)(settings)
	}
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

// For 3rd party dep injection.

func WithRepoType(repoType repo.RepoType) func(s *Settings) error {
	return func(s *Settings) error {
		s.nodeType = repoType
		return nil
	}
}

func WithEnableLibp2pNode(enable bool) func(s *Settings) error {
	return func(s *Settings) error {
		s.enableLibp2pNode = enable
		return nil
	}
}

func WithInvokesKey(i invoke, resApi interface{}) func(s *Settings) error {
	return func(s *Settings) error {
		s.invokes[i] = fx.Populate(resApi)
		return nil
	}
}
