package lf3

import (
	"context"
	"errors"
	"path/filepath"

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
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

type F3 struct {
	inner *f3.F3
	ec    *ecWrapper

	signer gpbft.Signer
	leaser *leaser
}

type F3Params struct {
	fx.In

	ManifestProvider manifest.ManifestProvider
	PubSub           *pubsub.PubSub
	Host             host.Host
	ChainStore       *store.ChainStore
	Syncer           *chain.Syncer
	StateManager     *stmgr.StateManager
	Datastore        dtypes.MetadataDS
	Wallet           api.Wallet
	Config           *Config
	LockedRepo       repo.LockedRepo

	Net api.Net
}

var log = logging.Logger("f3")

func New(mctx helpers.MetricsCtx, lc fx.Lifecycle, params F3Params) (*F3, error) {
	ds := namespace.Wrap(params.Datastore, datastore.NewKey("/f3"))
	ec := &ecWrapper{
		ChainStore:   params.ChainStore,
		StateManager: params.StateManager,
		Syncer:       params.Syncer,
	}
	verif := blssig.VerifierWithKeyOnG1()

	f3FsPath := filepath.Join(params.LockedRepo.Path(), "f3")
	module, err := f3.New(mctx, params.ManifestProvider, ds,
		params.Host, params.PubSub, verif, ec, f3FsPath)
	if err != nil {
		return nil, xerrors.Errorf("creating F3: %w", err)
	}
	nodeId, err := params.Net.ID(mctx)
	if err != nil {
		return nil, xerrors.Errorf("getting node ID: %w", err)
	}

	// maxLeasableInstances is the maximum number of leased F3 instances this node
	// would give out.
	const maxLeasableInstances = 5
	status := func() (*manifest.Manifest, gpbft.Instant) {
		return module.Manifest(), module.Progress()
	}
	fff := &F3{
		inner:  module,
		ec:     ec,
		signer: &signer{params.Wallet},
		leaser: newParticipationLeaser(nodeId, status, maxLeasableInstances),
	}

	// Start F3
	lc.Append(fx.Hook{
		OnStart: fff.inner.Start,
		OnStop:  fff.inner.Stop,
	})

	// Start signing F3 messages.
	lCtx, cancel := context.WithCancel(mctx)
	doneCh := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(doneCh)
				fff.runSigningLoop(lCtx)
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			<-doneCh
			return nil
		},
	})

	return fff, nil
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

	msgCh := fff.inner.MessagesToSign()

	var mb *gpbft.MessageBuilder
	alreadyParticipated := make(map[uint64]struct{})
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-fff.leaser.notifyParticipation:
			if mb == nil {
				continue
			}
		case mb = <-msgCh: // never closed
			clear(alreadyParticipated)
		}

		participants := fff.leaser.getParticipantsByInstance(mb.NetworkName, mb.Payload.Instance)
		for _, id := range participants {
			if _, ok := alreadyParticipated[id]; ok {
				continue
			} else if err := participateOnce(ctx, mb, id); err != nil {
				log.Errorf("while participating for miner f0%d: %+v", id, err)
			} else {
				alreadyParticipated[id] = struct{}{}
			}
		}
	}
}

func (fff *F3) GetOrRenewParticipationTicket(_ context.Context, minerID uint64, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {
	return fff.leaser.getOrRenewParticipationTicket(minerID, previous, instances)
}

func (fff *F3) Participate(_ context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	return fff.leaser.participate(ticket)
}

func (fff *F3) GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	return fff.inner.GetCert(ctx, instance)
}

func (fff *F3) GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error) {
	return fff.inner.GetLatestCert(ctx)
}

func (fff *F3) GetManifest() *manifest.Manifest {
	return fff.inner.Manifest()
}

func (fff *F3) GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.ec.getPowerTableLotusTSK(ctx, tsk)
}

func (fff *F3) GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	return fff.inner.GetPowerTable(ctx, tsk.Bytes())
}

func (fff *F3) IsRunning() bool {
	return fff.inner.IsRunning()
}

func (fff *F3) Progress() gpbft.Instant {
	return fff.inner.Progress()
}

func (fff *F3) ListParticipants() []api.F3Participant {
	leases := fff.leaser.getValidLeases()
	participants := make([]api.F3Participant, len(leases))
	for i, lease := range leases {
		participants[i] = api.F3Participant{
			MinerID:      lease.MinerID,
			FromInstance: lease.FromInstance,
			ValidityTerm: lease.ValidityTerm,
		}
	}
	return participants
}
