package metrics

import (
	"context"
	"encoding/json"
	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/node/impl/full"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
)

var log = logging.Logger("metrics")

const baseTopic = "/fil/headnotifs/"

type Update struct {
	Type string
}

func SendHeadNotifs(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, chain full.ChainAPI) error {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			gen, err := chain.Chain.GetGenesis()
			if err != nil {
				return err
			}

			topic := baseTopic + gen.Cid().String()

			go func() {
				if err := sendHeadNotifs(ctx, ps, topic, chain); err != nil {
					log.Error("consensus metrics error", err)
					return
				}
			}()
			go func() {
				sub, err := ps.Subscribe(topic)
				if err != nil {
					return
				}
				defer sub.Cancel()

				for {
					if _, err := sub.Next(ctx); err != nil {
						return
					}
				}

			}()
			return nil
		},
	})

	return nil
}

func sendHeadNotifs(ctx context.Context, ps *pubsub.PubSub, topic string, chain full.ChainAPI) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifs, err := chain.ChainNotify(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case notif := <-notifs:
			n := notif[len(notif)-1]

			b, err := json.Marshal(n.Val)
			if err != nil {
				return err
			}

			if err := ps.Publish(topic, b); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
