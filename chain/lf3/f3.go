package lf3

import (
	"context"
	"errors"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
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

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {
	manifest := f3.LocalnetManifest()
	manifest.NetworkName = gpbft.NetworkName(params.NetworkName)
	manifest.ECDelay = 2 * time.Duration(build.BlockDelaySecs) * time.Second
	manifest.ECPeriod = manifest.ECDelay
	manifest.BootstrapEpoch = int64(build.F3BootstrapEpoch)
	manifest.ECFinality = int64(build.Finality)

	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
		Manifest:     manifest,
	}
	verif := blssig.VerifierWithKeyOnG1()

	module, err := f3.New(mctx, manifest, ds,
		params.Host, params.PubSub, verif, ec, log, nil)

	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}

	fff := &F3{
		inner:  module,
		signer: &signer{params.Wallet},
	}

	lCtx, cancel := context.WithCancel(mctx)
	lc.Append(fx.StartStopHook(
		func() {
			go func() {
				err := fff.inner.Run(lCtx)
				if err != nil {
					log.Errorf("running f3: %+v", err)
				}
			}()
		}, cancel))

	return fff, nil
}

// Participate runs the participation loop for givine minerID
// It is blocking
func (fff *F3) Participate(ctx context.Context, minerIDAddress uint64, errCh chan<- string) {
	defer close(errCh)

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// create channel for some buffer so we don't get dropped under high load
		msgCh := make(chan *gpbft.MessageBuilder, 4)
		// SubscribeForMessagesToSign will close the channel if it fills up
		// so using the closer is not necessary, we can just drop it on the floor
		_ = fff.inner.SubscribeForMessagesToSign(msgCh)

		participateOnce := func(mb *gpbft.MessageBuilder) error {
			signatureBuilder, err := mb.PrepareSigningInputs(gpbft.ActorID(minerIDAddress))
			if errors.Is(err, gpbft.ErrNoPower) {
				// we don't have any power in F3, continue
				log.Debug("no power to participate in F3: %+v", err)
				return nil
			}
			if err != nil {
				log.Errorf("preparing signing inputs: %+v", err)
				return err
			}
			// if worker keys were stored not in the node, the signatureBuilder can be send there
			// the sign can be called where the keys are stored and then
			// {signatureBuilder, payloadSig, vrfSig} can be sent back to lotus for broadcast
			payloadSig, vrfSig, err := signatureBuilder.Sign(fff.signer)
			if err != nil {
				log.Errorf("signing message: %+v", err)
				return err
			}
			log.Infof("miner with id %d is sending message in F3", minerIDAddress)
			fff.inner.Broadcast(ctx, signatureBuilder, payloadSig, vrfSig)
			return nil
		}

	inner:
		for {
			select {
			case mb, ok := <-msgCh:
				if !ok {
					// the broadcast bus kicked us out
					log.Infof("lost message bus subscription, retrying")
					break inner
				}

				err := participateOnce(mb)
				if err != nil {
					errCh <- err.Error()
				} else {
					errCh <- ""
				}

			case <-ctx.Done():
				return
			}
		}

	}
}

func (fff *F3) GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	return fff.inner.GetCert(ctx, instance)
}

func (fff *F3) GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error) {
	return fff.inner.GetLatestCert(ctx)
}
