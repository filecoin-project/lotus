package node

import (
	"context"
	"errors"
	"reflect"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
)

var errTyp = reflect.TypeOf(new(error)).Elem()

// TODO: type checking, this isn't JS
func provideApi(f interface{}, toProvide interface{}) fx.Option {
	rf := reflect.ValueOf(f)
	tp := reflect.ValueOf(toProvide).Elem()

	ins := make([]reflect.Type, rf.Type().NumIn())
	for i := range ins {
		ins[i] = rf.Type().In(i)
	}

	ctyp := reflect.FuncOf(ins, []reflect.Type{errTyp}, rf.Type().IsVariadic())

	return fx.Invoke(reflect.MakeFunc(ctyp, func(args []reflect.Value) (results []reflect.Value) {
		provided := rf.Call(args)
		tp.Set(provided[0].Elem().Convert(tp.Type()))
		return []reflect.Value{reflect.ValueOf(new(error)).Elem()}
	}).Interface())
}

func apiOption(resAPI *api.Struct) fx.Option {
	in := &resAPI.Internal

	return fx.Options(
		provideApi(versionAPI, &in.Version),
		provideApi(idAPI, &in.ID),

		provideApi(netPeersAPI, &in.NetPeers),
		provideApi(netConnectAPI, &in.NetConnect),
		provideApi(netAddrsListenAPI, &in.NetAddrsListen),
	)
}

func idAPI(id peer.ID) interface{} {
	return func(ctx context.Context) (peer.ID, error) {
		return id, nil
	}
}

func versionAPI() interface{} {
	return func(context.Context) (api.Version, error) {
		return api.Version{
			Version: build.Version,
		}, nil
	}
}

func netPeersAPI(h host.Host) interface{} {
	return func(ctx context.Context) ([]peer.AddrInfo, error) {
		conns := h.Network().Conns()
		out := make([]peer.AddrInfo, len(conns))

		for i, conn := range conns {
			out[i] = peer.AddrInfo{
				ID: conn.RemotePeer(),
				Addrs: []ma.Multiaddr{
					conn.RemoteMultiaddr(),
				},
			}
		}

		return out, nil
	}
}

func netConnectAPI(h host.Host) interface{} {
	return func(ctx context.Context, p peer.AddrInfo) error {
		return errors.New("nope")
	}
}

func netAddrsListenAPI(h host.Host) interface{} {
	return func(context.Context) ([]ma.Multiaddr, error) {
		return h.Addrs(), nil
	}
}
