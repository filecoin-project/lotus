package lp2p

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type P2PHostIn struct {
	fx.In

	ID        peer.ID
	Peerstore peerstore.Peerstore

	Opts [][]libp2p.Option `group:"libp2p"`
}

// ////////////////////////

type RawHost host.Host

func Peerstore() (peerstore.Peerstore, error) {
	return pstoremem.NewPeerstore()
}

func Host(mctx helpers.MetricsCtx, buildVersion build.BuildVersion, lc fx.Lifecycle, params P2PHostIn) (RawHost, error) {
	pkey := params.Peerstore.PrivKey(params.ID)
	if pkey == nil {
		return nil, fmt.Errorf("missing private key for node ID: %s", params.ID)
	}

	opts := []libp2p.Option{
		libp2p.Identity(pkey),
		libp2p.Peerstore(params.Peerstore),
		libp2p.NoListenAddrs,
		libp2p.Ping(true),
		libp2p.UserAgent(buildconstants.UserAgent + "-" + string(buildVersion)),
	}
	for _, o := range params.Opts {
		opts = append(opts, o...)
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func UserAgentOption(agent string) func() (opts Libp2pOpts, err error) {
	return func() (opts Libp2pOpts, err error) {
		opts.Opts = append(opts.Opts, libp2p.UserAgent(agent))
		return
	}
}

func MockHost(mn mocknet.Mocknet, id peer.ID, ps peerstore.Peerstore) (RawHost, error) {
	return mn.AddPeerWithPeerstore(id, ps)
}

func DHTRouting(mode dht.ModeOpt) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host RawHost, dstore dtypes.MetadataDS, validator record.Validator, nn dtypes.NetworkName, bs dtypes.Bootstrapper) (BaseIpfsRouting, error) {
		ctx := helpers.LifecycleCtx(mctx, lc)

		if bs {
			mode = dht.ModeServer
		}

		opts := []dht.Option{dht.Mode(mode),
			dht.Datastore(dstore),
			dht.Validator(validator),
			dht.ProtocolPrefix(build.DhtProtocolName(nn)),
			dht.QueryFilter(func(_dht interface{}, ai peer.AddrInfo) bool {
				env := strings.ToLower(os.Getenv("LOTUS_P2P_DHT_NO_QUERY_FILTER"))
				if env == "1" || env == "true" {
					log.Warnf("DHT query filter is disabled. (LOTUS_P2P_DHT_NO_QUERY_FILTER=%s)", env)
					return true
				}
				return dht.PublicQueryFilter(_dht, ai)
			}),
			dht.RoutingTableFilter(func(_dht interface{}, p peer.ID) bool {
				env := strings.ToLower(os.Getenv("LOTUS_P2P_DHT_NO_ROUTING_TABLE_FILTER"))
				if env == "1" || env == "true" {
					log.Warnf("DHT query filter is disabled. (LOTUS_P2P_DHT_NO_ROUTING_TABLE_FILTER=%s)", env)
					return true
				}
				return dht.PublicRoutingTableFilter(_dht, p)
			}),
			dht.DisableProviders(),
			dht.DisableValues()}
		d, err := dht.New(
			ctx, host, opts...,
		)

		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return d.Close()
			},
		})

		return d, nil
	}
}

func NilRouting(mctx helpers.MetricsCtx) (BaseIpfsRouting, error) {
	return &routinghelpers.Null{}, nil
}

func RoutedHost(rh RawHost, r BaseIpfsRouting) host.Host {
	return routedhost.Wrap(rh, r)
}
