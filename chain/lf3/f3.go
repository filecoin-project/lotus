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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/blssig"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
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
	newLeases chan leaseRequest
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

func init() {
	// Set up otel to prometheus reporting so that F3 metrics are reported via lotus
	// prometheus metrics. This bridge eventually gets picked up by opencensus
	// exporter as HTTP handler. This by default registers an otel collector against
	// the global prometheus registry. In the future, we should clean up metrics in
	// Lotus and move it all to use otel. For now, bridge away.
	if bridge, err := prometheus.New(); err != nil {
		log.Errorf("could not create the otel prometheus exporter: %v", err)
	} else {
		provider := metric.NewMeterProvider(metric.WithReader(bridge))
		otel.SetMeterProvider(provider)
	}
}

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {

	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
	}
	verif := blssig.VerifierWithKeyOnG1()

	module, err := f3.New(mctx, params.ManifestProvider, ds,
		params.Host, params.PubSub, verif, ec)

	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}

	fff := &F3{
		inner:     module,
		ec:        ec,
		signer:    &signer{params.Wallet},
		newLeases: make(chan leaseRequest, 4), // some buffer to avoid blocking
	}

	lCtx, cancel := context.WithCancel(mctx)
	lc.Append(fx.StartStopHook(
		func() {
			err := fff.inner.Start(lCtx)
			if err != nil {
				log.Errorf("running f3: %+v", err)
				return
			}
			go fff.runSigningLoop(lCtx)
		}, cancel))

	return fff, nil
}

type leaseRequest struct {
	minerID       uint64
	newExpiration time.Time
	oldExpiration time.Time
	resultCh      chan<- bool
}

func (fff *F3) runSigningLoop(ctx context.Context) {
	participateOnce := func(ctx context.Context, mb *gpbft.MessageBuilder, minerID uint64) error {
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
		payloadSig, vrfSig, err := signatureBuilder.Sign(ctx, fff.signer)
		if err != nil {
			return xerrors.Errorf("signing message: %+v", err)
		}
		log.Debugf("miner with id %d is sending message in F3", minerID)
		fff.inner.Broadcast(ctx, signatureBuilder, payloadSig, vrfSig)
		return nil
	}

	leaseMngr := new(leaseManager)
	msgCh := fff.inner.MessagesToSign()

loop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case l := <-fff.newLeases:
			// resultCh has only one user and is buffered
			l.resultCh <- leaseMngr.UpsertDefensive(l.minerID, l.newExpiration, l.oldExpiration)
			close(l.resultCh)
		case mb, ok := <-msgCh:
			if !ok {
				continue loop
			}

			for _, minerID := range leaseMngr.Active() {
				err := participateOnce(ctx, mb, minerID)
				if err != nil {
					log.Errorf("while participating for miner f0%d: %+v", minerID, err)
				}
			}
		}
	}
}

// Participate notifies participation loop about a new lease
// Returns true if lease was accepted
func (fff *F3) Participate(ctx context.Context, minerID uint64, newLeaseExpiration, oldLeaseExpiration time.Time) bool {
	resultCh := make(chan bool, 1) //buffer the channel to for sure avoid blocking
	request := leaseRequest{minerID: minerID, newExpiration: newLeaseExpiration, resultCh: resultCh}
	select {
	case fff.newLeases <- request:
		return <-resultCh
	case <-ctx.Done():
		return false
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

func (fff *F3) GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.inner.GetPowerTable(ctx, tsk.Bytes())
}
