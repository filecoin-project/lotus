package node

import (
	"context"
	"reflect"
	"time"

	"github.com/ipfs/go-datastore"
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

var defaultListenAddrs = []string{ // TODO: better defaults?
	"/ip4/0.0.0.0/tcp/4001",
	"/ip6/::/tcp/4001",
}

// New builds and starts new Filecoin node
func New(ctx context.Context) (api.API, error) {
	var resAPI api.Struct

	online := true

	app := fx.New(
		fx.Provide(as(ctx, new(helpers.MetricsCtx))),

		//fx.Provide(modules.RandomPeerID),
		randomIdentity(),
		memrepo(),

		fx.Provide(modules.RecordValidator),

		ifOpt(online,
			fx.Provide(
				pstoremem.NewPeerstore,

				lp2p.DefaultTransports,
				lp2p.PNet,
				lp2p.Host,
				lp2p.RoutedHost,
				lp2p.DHTRouting(false),

				lp2p.DiscoveryHandler,
				lp2p.AddrsFactory(nil, nil),
				lp2p.SmuxTransport(true),
				lp2p.Relay(true, false),
				lp2p.Security(true, false),

				lp2p.BaseRouting,
				lp2p.Routing,

				lp2p.NatPortMap,
				lp2p.ConnectionManager(50, 200, 20*time.Second),
			),

			fx.Invoke(
				lp2p.PstoreAddSelfKeys,
				lp2p.StartListening(defaultListenAddrs),
			),
		),

		fx.Invoke(versionAPI(&resAPI.Internal.Version)),
		fx.Invoke(idAPI(&resAPI.Internal.ID)),
	)

	if err := app.Start(ctx); err != nil {
		return nil, err
	}

	return &resAPI, nil
}

// In-memory / testing

func memrepo() fx.Option {
	return fx.Provide(
		func() datastore.Batching {
			return datastore.NewMapDatastore()
		},
	)
}

func randomIdentity() fx.Option {
	sk, pk, err := ci.GenerateKeyPair(ci.RSA, 512)
	if err != nil {
		return fx.Error(err)
	}

	return fx.Options(
		fx.Provide(as(sk, new(ci.PrivKey))),
		fx.Provide(as(pk, new(ci.PubKey))),
		fx.Provide(peer.IDFromPublicKey),
	)
}

// UTILS

func ifOpt(cond bool, options ...fx.Option) fx.Option {
	if cond {
		return fx.Options(options...)
	}
	return fx.Options()
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

// from go-ipfs
// as casts input constructor to a given interface (if a value is given, it
// wraps it into a constructor).
//
// Note: this method may look like a hack, and in fact it is one.
// This is here only because https://github.com/uber-go/fx/issues/673 wasn't
// released yet
//
// Note 2: when making changes here, make sure this method stays at
// 100% coverage. This makes it less likely it will be terribly broken
func as(in interface{}, as interface{}) interface{} {
	outType := reflect.TypeOf(as)

	if outType.Kind() != reflect.Ptr {
		panic("outType is not a pointer")
	}

	if reflect.TypeOf(in).Kind() != reflect.Func {
		ctype := reflect.FuncOf(nil, []reflect.Type{outType.Elem()}, false)

		return reflect.MakeFunc(ctype, func(args []reflect.Value) (results []reflect.Value) {
			out := reflect.New(outType.Elem())
			out.Elem().Set(reflect.ValueOf(in))

			return []reflect.Value{out.Elem()}
		}).Interface()
	}

	inType := reflect.TypeOf(in)

	ins := make([]reflect.Type, inType.NumIn())
	outs := make([]reflect.Type, inType.NumOut())

	for i := range ins {
		ins[i] = inType.In(i)
	}
	outs[0] = outType.Elem()
	for i := range outs[1:] {
		outs[i+1] = inType.Out(i + 1)
	}

	ctype := reflect.FuncOf(ins, outs, false)

	return reflect.MakeFunc(ctype, func(args []reflect.Value) (results []reflect.Value) {
		outs := reflect.ValueOf(in).Call(args)
		out := reflect.New(outType.Elem())
		out.Elem().Set(outs[0])
		outs[0] = out.Elem()

		return outs
	}).Interface()
}
