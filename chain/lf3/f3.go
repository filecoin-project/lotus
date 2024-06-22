package lf3

import (
	"context"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type F3 struct {
	inner *f3.F3

	signer gpbft.Signer
}

type F3Params struct {
	fx.In

	NetworkName  dtypes.NetworkName
	PubSub       *pubsub.PubSub
	Host         host.Host
	ChainStore   *store.ChainStore
	StateManager *stmgr.StateManager
	Datastore    dtypes.MetadataDS
	Wallet       api.Wallet
}

var log = logging.Logger("f3")

func New(lc fx.Lifecycle, params F3Params) (*F3, error) {
	_ = logging.SetLogLevel("f3", "DEBUG")
	manifest := f3.LocalnetManifest()
	manifest.NetworkName = gpbft.NetworkName(params.NetworkName)
	manifest.ECDelay = time.Duration(build.BlockDelaySecs) * time.Second
	manifest.ECPeriod = manifest.ECDelay
	manifest.BootstrapEpoch = 210
	manifest.ECFinality = 200
	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
		Manifest:     manifest,
	}
	verif := blssig.VerifierWithKeyOnG1()

	module, err := f3.New(context.TODO(), 1000 /*TODO expose signing*/, manifest, ds,
		params.Host, params.PubSub, verif, ec, log, nil)

	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}

	fff := &F3{
		inner:  module,
		signer: &signer{params.Wallet},
	}

	var liftimeContext context.Context
	var cancel func()
	lc.Append(fx.StartStopHook(
		func(ctx context.Context) {
			liftimeContext, cancel = context.WithCancel(ctx)
			go func() {
				err := fff.inner.Run(liftimeContext)
				if err != nil {
					log.Errorf("running f3: %+v", err)
				}
			}()
		}, func() {
			cancel()
		}))

	return fff, nil
}

func (fff *F3) F3Participate(ctx context.Context, miner address.Address) error {
	actorID, err := address.IDFromAddress(miner)
	if err != nil {
		return xerrors.Errorf("miner address in F3Participate not of ID type: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		ch := make(chan *gpbft.MessageBuilder, 4)
		fff.inner.SubscribeForMessagesToSign(ch)
	inner:
		for {
			select {
			case mb, ok := <-ch:
				if !ok {
					// the broadcast bus kicked us out
					log.Infof("lost message bus subscription, retrying")
					break inner
				}
				signatureBuilder, err := mb.PrepareSigningInputs(gpbft.ActorID(actorID))
				if err != nil {
					log.Errorf("preparing signing inputs: %+v", err)
					continue inner
				}
				// signatureBuilder can be sent over RPC
				payloadSig, vrfSig, err := signatureBuilder.Sign(fff.signer)
				if err != nil {
					log.Errorf("signing message: %+v", err)
					continue inner
				}
				// signatureBuilder and signatures can be returned back over RPC
				fff.inner.Broadcast(ctx, signatureBuilder, payloadSig, vrfSig)
			case <-ctx.Done():
				return nil
			}
		}

	}
}
