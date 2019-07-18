package node

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/ipfs/go-filestore"

	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	ipld "github.com/ipfs/go-ipld-format"
	ci "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/config"
	"github.com/filecoin-project/go-lotus/node/hello"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/modules/lp2p"
	"github.com/filecoin-project/go-lotus/node/modules/testing"
	"github.com/filecoin-project/go-lotus/node/repo"
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

	// filecoin
	SetGenesisKey

	RunHelloKey
	RunBlockSyncKey

	HandleIncomingBlocksKey
	HandleIncomingMessagesKey

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

	Online bool // Online option applied
	Config bool // Config option applied
}

// Override option changes constructor for a given type
func Override(typ, constructor interface{}) Option {
	return func(s *Settings) error {
		if i, ok := typ.(invoke); ok {
			s.invokes[i] = fx.Invoke(constructor)
			return nil
		}

		if c, ok := typ.(special); ok {
			s.modules[c] = fx.Provide(constructor)
			return nil
		}
		ctor := as(constructor, typ)
		rt := reflect.TypeOf(typ).Elem()

		s.modules[rt] = fx.Provide(ctor)
		return nil
	}
}

var defConf = config.Default()

func defaults() []Option {
	return []Option{
		Override(new(helpers.MetricsCtx), context.Background),

		randomIdentity(),

		Override(new(datastore.Batching), testing.MapDatastore),
		Override(new(blockstore.Blockstore), testing.MapBlockstore), // NOT on top of ds above
		Override(new(record.Validator), modules.RecordValidator),

		// Filecoin modules

		Override(new(*chain.ChainStore), chain.NewChainStore),
	}
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

		//Override(NatPortMapKey, lp2p.NatPortMap), //TODO: reenable when closing logic is actually there
		Override(ConnectionManagerKey, lp2p.ConnectionManager(50, 200, 20*time.Second)),

		Override(new(*pubsub.PubSub), lp2p.GossipSub()),

		Override(PstoreAddSelfKeysKey, lp2p.PstoreAddSelfKeys),
		Override(StartListeningKey, lp2p.StartListening(defConf.Libp2p.ListenAddresses)),

		//

		Override(new(blockstore.GCLocker), blockstore.NewGCLocker),
		Override(new(blockstore.GCBlockstore), blockstore.NewGCBlockstore),
		Override(new(exchange.Interface), modules.Bitswap),
		Override(new(ipld.DAGService), testing.MemoryClientDag),

		// Filecoin services
		Override(new(*chain.Syncer), chain.NewSyncer),
		Override(new(*chain.BlockSync), chain.NewBlockSyncClient),
		Override(new(*chain.Wallet), chain.NewWallet),
		Override(new(*chain.MessagePool), chain.NewMessagePool),

		Override(new(modules.Genesis), testing.MakeGenesis),
		Override(SetGenesisKey, modules.SetGenesis),

		Override(new(*hello.Service), hello.NewHelloService),
		Override(new(*chain.BlockSyncService), chain.NewBlockSyncService),
		Override(RunHelloKey, modules.RunHello),
		Override(RunBlockSyncKey, modules.RunBlockSync),
		Override(HandleIncomingBlocksKey, modules.HandleIncomingBlocks),
		Override(HandleIncomingMessagesKey, modules.HandleIncomingMessages),
	)
}

// Config sets up constructors based on the provided Config
func Config(cfg *config.Root) Option {
	return Options(
		func(s *Settings) error { s.Config = true; return nil },

		ApplyIf(func(s *Settings) bool { return s.Online },
			Override(StartListeningKey, lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
		),
	)
}

func Repo(r repo.Repo) Option {
	lr, err := r.Lock()
	if err != nil {
		return Error(err)
	}
	cfg, err := lr.Config()
	if err != nil {
		return Error(err)
	}
	pk, err := lr.Libp2pIdentity()
	if err != nil {
		return Error(err)
	}
	kstore, err := lr.KeyStore()
	if err != nil {
		return Error(err)
	}

	return Options(
		Config(cfg),
		Override(new(repo.LockedRepo), modules.LockedRepo(lr)), // module handles closing

		Override(new(datastore.Batching), modules.Datastore),
		Override(new(blockstore.Blockstore), modules.Blockstore),

		Override(new(*filestore.Filestore), modules.ClientFstore),
		Override(new(ipld.DAGService), modules.ClientDAG),

		Override(new(ci.PrivKey), pk),
		Override(new(ci.PubKey), ci.PrivKey.GetPublic),
		Override(new(peer.ID), peer.IDFromPublicKey),

		Override(new(types.KeyStore), kstore),
	)
}

// New builds and starts new Filecoin node
func New(ctx context.Context, opts ...Option) (api.API, error) {
	resAPI := &API{}
	settings := Settings{
		modules: map[interface{}]fx.Option{},
		invokes: make([]fx.Option, _nInvokes),
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

		fx.Extract(resAPI),

		fx.NopLogger,
	)

	// TODO: we probably should have a 'firewall' for Closing signal
	//  on this context, and implement closing logic through lifecycles
	//  correctly
	if err := app.Start(ctx); err != nil {
		return nil, err
	}

	return resAPI, nil
}

// In-memory / testing

func randomIdentity() Option {
	sk, pk, err := ci.GenerateKeyPair(ci.RSA, 512)
	if err != nil {
		return Error(err)
	}

	return Options(
		Override(new(ci.PrivKey), sk),
		Override(new(ci.PubKey), pk),
		Override(new(peer.ID), peer.IDFromPublicKey),
	)
}
