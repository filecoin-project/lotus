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
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type F3 struct {
	inner *f3.F3
	ec    *ecWrapper

	signer    gpbft.Signer
	newLeases chan lease
}

type F3Params struct {
	fx.In

	NetworkName      dtypes.NetworkName
	ManifestProvider manifest.ManifestProvider
	PubSub           *pubsub.PubSub
	Host             host.Host
	ChainStore       *store.ChainStore
	StateManager     *stmgr.StateManager
	Datastore        dtypes.MetadataDS
	Wallet           api.Wallet
}

var log = logging.Logger("f3")

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {

	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
	}
	verif := blssig.VerifierWithKeyOnG1()

	senderID, err := peer.Decode(build.ManifestServerID)
	if err != nil {
		return nil, xerrors.Errorf("decoding F3 manifest server identity: %w", err)
	}

	module, err := f3.New(mctx, params.ManifestProvider, ds,
		params.Host, senderID, params.PubSub, verif, ec, log, nil)

	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}
	params.ManifestProvider.SetManifestChangeCallback(f3.ManifestChangeCallback(module))

	fff := &F3{
		inner:     module,
		ec:        ec,
		signer:    &signer{params.Wallet},
		newLeases: make(chan lease, 4), // some buffer to avoid blocking
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
			go fff.runSigningLoop(lCtx)
		}, cancel))

	return fff, nil
}

type lease struct {
	minerID    uint64
	expiration time.Time
}

func (fff *F3) runSigningLoop(ctx context.Context) {
	participateOnce := func(mb *gpbft.MessageBuilder, minerID uint64) error {
		signatureBuilder, err := mb.PrepareSigningInputs(gpbft.ActorID(minerID))
		if errors.Is(err, gpbft.ErrNoPower) {
			// we don't have any power in F3, continue
			log.Debug("no power to participate in F3: %+v", err)
			return nil
		}
		if err != nil {
			return xerrors.Errorf("preparing signing inputs: %+v", err)
		}
		// if worker keys were stored not in the node, the signatureBuilder can be send there
		// the sign can be called where the keys are stored and then
		// {signatureBuilder, payloadSig, vrfSig} can be sent back to lotus for broadcast
		payloadSig, vrfSig, err := signatureBuilder.Sign(fff.signer)
		if err != nil {
			return xerrors.Errorf("signing message: %+v", err)
		}
		log.Infof("miner with id %d is sending message in F3", minerID)
		fff.inner.Broadcast(ctx, signatureBuilder, payloadSig, vrfSig)
		return nil
	}

	leaseMngr := new(leaseManager)
	// create channel for some buffer so we don't get dropped under high load
	msgCh := make(chan *gpbft.MessageBuilder, 4)
	// SubscribeForMessagesToSign will close the channel if it fills up
	// so using the closer is not necessary, we can just drop it on the floor
	_ = fff.inner.SubscribeForMessagesToSign(msgCh)

loop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case l := <-fff.newLeases:
			leaseMngr.Upsert(l.minerID, l.expiration)
		case mb, ok := <-msgCh:
			if !ok {
				// we got dropped, resubscribe
				msgCh = make(chan *gpbft.MessageBuilder, cap(msgCh))
				_ = fff.inner.SubscribeForMessagesToSign(msgCh)
				continue loop
			}

			for _, minerID := range leaseMngr.Active() {
				err := participateOnce(mb, minerID)
				if err != nil {
					log.Errorf("while participating for miner f0%d: %+v", minerID, err)
				}
			}
		}
	}
}

// Participate notifies participation loop about a new lease
func (fff *F3) Participate(ctx context.Context, minerID uint64, leaseExpiration time.Time) {
	select {
	case fff.newLeases <- lease{minerID: minerID, expiration: leaseExpiration}:
	case <-ctx.Done():
	}
}

func (fff *F3) GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	return fff.inner.GetCert(ctx, instance)
}

func (fff *F3) GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error) {
	return fff.inner.GetLatestCert(ctx)
}

func (fff *F3) GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.ec.getPowerTableLotusTSK(ctx, tsk)
}
