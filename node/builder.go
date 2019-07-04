package node

import (
	"context"
	"errors"
	"github.com/filecoin-project/go-lotus/node/config"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	record "github.com/libp2p/go-libp2p-record"
	"time"

	ci "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/modules/lp2p"
)

// special is a type used to give keys to modules which
//  can't really be identified by the returned type
type special struct{ id int }

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

const (
	PstoreAddSelfKeysKey = invoke(iota)
	StartListeningKey

	_nInvokes // keep this last
)

type settings struct {
	modules map[interface{}]fx.Option

	// invokes are separate from modules as they can't be referenced by return
	// type, and must be applied in correct order
	invokes []fx.Option

	online bool // Online option applied
	config bool // Config option applied
}

var defConf = config.Default()

var defaults = []Option{
	Override(new(helpers.MetricsCtx), context.Background),

	randomIdentity(),

	Override(new(datastore.Batching), datastore.NewMapDatastore),
	Override(new(record.Validator), modules.RecordValidator),
}

func Online() Option {
	return Options(
		func(s *settings) error { s.online = true; return nil },
		applyIf(func(s *settings) bool { return s.config },
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

		Override(NatPortMapKey, lp2p.NatPortMap),
		Override(ConnectionManagerKey, lp2p.ConnectionManager(50, 200, 20*time.Second)),

		Override(PstoreAddSelfKeysKey, lp2p.PstoreAddSelfKeys),
		Override(StartListeningKey, lp2p.StartListening(defConf.Libp2p.ListenAddresses)),
	)
}

func Config(cfg *config.Root) Option {
	return Options(
		func(s *settings) error { s.config = true; return nil },

		applyIf(func(s *settings) bool { return s.online },
			Override(StartListeningKey, lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
		),
	)
}

// New builds and starts new Filecoin node
func New(ctx context.Context, opts ...Option) (api.API, error) {
	var resAPI api.Struct
	settings := settings{
		modules: map[interface{}]fx.Option{},
		invokes: make([]fx.Option, _nInvokes),
	}

	if err := Options(Options(defaults...), Options(opts...))(&settings); err != nil {
		return nil, err
	}

	ctors := make([]fx.Option, 0, len(settings.modules))
	for _, opt := range settings.modules {
		ctors = append(ctors, opt)
	}

	// fill holes in invokes
	for i, opt := range settings.invokes {
		if opt == nil {
			settings.invokes[i] = fx.Options()
		}
	}

	app := fx.New(
		fx.Options(ctors...),
		fx.Options(settings.invokes...),

		fx.Invoke(versionAPI(&resAPI.Internal.Version)),
		fx.Invoke(idAPI(&resAPI.Internal.ID)),
	)

	if err := app.Start(ctx); err != nil {
		return nil, err
	}

	return &resAPI, nil
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

// API IMPL

// TODO: figure out a better way, this isn't usable in long term
func idAPI(set *func(ctx context.Context) (peer.ID, error)) func(id peer.ID) {
	return func(id peer.ID) {
		*set = func(ctx context.Context) (peer.ID, error) {
			return id, nil
		}
	}
}

func versionAPI(set *func(context.Context) (api.Version, error)) func() {
	return func() {
		*set = func(context.Context) (api.Version, error) {
			return api.Version{
				Version: build.Version,
			}, nil
		}
	}
}
