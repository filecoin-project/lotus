package modules

import (
	"github.com/filecoin-project/go-lotus/node/hello"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"go.uber.org/fx"
)

func RunHello(mctx helpers.MetricsCtx, lc fx.Lifecycle, h host.Host, svc *hello.Service) {
	h.SetStreamHandler(hello.ProtocolID, svc.HandleStream)

	bundle := inet.NotifyBundle{
		ConnectedF: func(_ inet.Network, c inet.Conn) {
			go func() {
				if err := svc.SayHello(helpers.LifecycleCtx(mctx, lc), c.RemotePeer()); err != nil {
					log.Warnw("failed to say hello", "error", err)
					return
				}
			}()
		},
	}
	h.Network().Notify(&bundle)
}
